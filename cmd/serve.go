package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/server"
	"github.com/jniltinho/go-guacamole/internal/store"
)

func newServeCmd() *cobra.Command {
	var noMigrate bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP(S) server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			return serve(cmd.Context(), cfg, !noMigrate)
		},
	}
	cmd.Flags().String("addr", "", "listen address (env APP_ADDR)")
	cmd.Flags().String("guacd", "", "guacd address host:port (env GUACD_ADDR)")
	cmd.Flags().BoolVar(&noMigrate, "no-migrate", false, "do not apply pending migrations on start")
	return cmd
}

func serve(parent context.Context, cfg *config.Config, migrate bool) error {
	log := newLogger(cfg, os.Stderr)
	slog.SetDefault(log)
	if parent == nil {
		parent = context.Background()
	}
	ctx := parent

	st, err := openStore(ctx, cfg, migrate)
	if err != nil {
		return err
	}
	defer st.DB().Close()
	if err := bootstrapAdmin(ctx, cfg, st, log); err != nil {
		return err
	}

	app := server.New(cfg, st, log, webFS)
	app.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.App.Addr,
		Handler:           app.Echo,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	var certs *certReloader
	if cfg.TLS.Enabled {
		minV, _ := config.ParseTLSVersion(cfg.TLS.MinVersion)
		certs, err = newCertReloader(cfg.TLS, log)
		if err != nil {
			return err
		}
		srv.TLSConfig = &tls.Config{MinVersion: minV, GetCertificate: certs.get, NextProtos: []string{"h2", "http/1.1"}}
		go watchSIGHUP(ctx, certs, log)
	}

	errc := make(chan error, 2)
	ln, err := net.Listen("tcp", cfg.App.Addr)
	if err != nil {
		return err
	}
	go func() {
		scheme := "http"
		if cfg.TLS.Enabled {
			scheme = "https"
			ln = tls.NewListener(ln, srv.TLSConfig)
		}
		log.Info("goguacamole listening", "addr", cfg.App.Addr, "scheme", scheme, "version", Version, "db", cfg.DB.Driver, "guacd", cfg.Guacd.Addr)
		errc <- srv.Serve(ln)
	}()
	var redirect *http.Server
	if cfg.TLS.Enabled && cfg.TLS.RedirectAddr != "" {
		_, httpsPort, _ := net.SplitHostPort(cfg.App.Addr)
		redirect = &http.Server{Addr: cfg.TLS.RedirectAddr, ReadHeaderTimeout: 5 * time.Second,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host, _, err := net.SplitHostPort(r.Host)
				if err != nil {
					host = r.Host
				}
				if httpsPort != "443" {
					host = net.JoinHostPort(host, httpsPort)
				}
				http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
			})}
		go func() { errc <- redirect.ListenAndServe() }()
	}

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	log.Info("shutting down")
	app.Shutdown()
	sctx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()
	if redirect != nil {
		_ = redirect.Shutdown(sctx)
	}
	_ = srv.Shutdown(sctx)
	// let tunnel goroutines finalize history
	time.Sleep(300 * time.Millisecond)
	return nil
}

func bootstrapAdmin(ctx context.Context, cfg *config.Config, st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	name, pass, generated := cfg.App.AdminUser, cfg.App.AdminPassword, false
	if name == "" {
		name = "admin"
	}
	if pass == "" {
		pass, generated = store.RandomHex(10), true
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return fmt.Errorf("APP_ADMIN_PASSWORD: %w", err)
	}
	u := &store.User{Username: name, PasswordHash: hash, PasswordScheme: store.SchemeBcrypt, MustChangePassword: true}
	if err := st.CreateUser(ctx, u); err != nil {
		return err
	}
	if err := st.GrantSystem(ctx, store.SubjectUser, u.ID, store.PermAdminister); err != nil {
		return err
	}
	if generated {
		log.Warn("bootstrap administrator created; change this password at first login", "username", name, "password", pass)
	} else {
		log.Info("bootstrap administrator created", "username", name)
	}
	return nil
}

type certReloader struct {
	cfg  config.TLSConfig
	log  *slog.Logger
	cert atomic.Pointer[tls.Certificate]
}

func newCertReloader(cfg config.TLSConfig, log *slog.Logger) (*certReloader, error) {
	r := &certReloader{cfg: cfg, log: log}
	if cfg.SelfSigned && (cfg.CertFile == "" || cfg.KeyFile == "") {
		c, err := selfSigned()
		if err != nil {
			return nil, err
		}
		r.cert.Store(c)
		log.Warn("using an in-memory self-signed TLS certificate (lab use only)")
		return r, nil
	}
	return r, r.reload()
}

func (r *certReloader) reload() error {
	if r.cfg.CertFile == "" {
		return nil
	}
	c, err := tls.LoadX509KeyPair(r.cfg.CertFile, r.cfg.KeyFile)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}
	r.cert.Store(&c)
	return nil
}

func (r *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) { return r.cert.Load(), nil }

func watchSIGHUP(ctx context.Context, r *certReloader, log *slog.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			if err := r.reload(); err != nil {
				log.Error("TLS certificate reload failed; keeping previous certificate", "err", err)
			} else {
				log.Info("TLS certificate reloaded")
			}
		}
	}
}

func selfSigned() (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "goguacamole-selfsigned"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost", strings.ToLower(host)},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
