// Package cmd defines the goguacamole Cobra command-line interface
// (serve, migrate, user, import, config, healthcheck, version).
// Configuration comes from config.toml and APP_*/TLS_*/DB_*/GUACD_* env vars.
package cmd

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/secrets"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// Build metadata, injected with -ldflags "-X github.com/jniltinho/go-guacamole/cmd.Version=...".
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var (
	configFile string
	webFS      fs.FS
)

// NewRoot builds the root command.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "goguacamole",
		Short:         "GoGuacamole: a web gateway for Apache guacd (RDP, SSH, VNC)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&configFile, "config", "c", "", "TOML config file (default: $APP_CONFIG, ./config.toml, /etc/goguacamole/config.toml)")
	root.PersistentFlags().String("db-driver", "", "database driver: sqlite, mysql or postgres (env DB_DRIVER)")
	root.PersistentFlags().String("db-dsn", "", "database DSN (env DB_DSN)")
	root.PersistentFlags().String("log-level", "", "log level: debug, info, warn, error (env APP_LOG_LEVEL)")
	root.AddCommand(newServeCmd(), newMigrateCmd(), newUserCmd(), newImportCmd(), newConfigCmd(), newHealthcheckCmd(), newVersionCmd())
	return root
}

// Execute runs the CLI with the embedded frontend and returns the exit code.
func Execute(assets embed.FS) int {
	dist, err := fs.Sub(assets, "web/dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	webFS = dist
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root := NewRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(root.ErrOrStderr(), "Error:", err)
		return 1
	}
	return 0
}

func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, _, err := config.Load(configFile, cmd.Flags())
	return cfg, err
}

func newLogger(cfg *config.Config, w io.Writer) *slog.Logger {
	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(cfg.App.LogLevel))
	opts := &slog.HandlerOptions{Level: lvl}
	if cfg.App.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

// openStore opens the configured database, optionally migrating it.
func openStore(ctx context.Context, cfg *config.Config, migrate bool) (*store.Store, error) {
	db, err := store.Open(ctx, cfg.DB.Driver, cfg.DB.DSN, cfg.DB.MaxOpenConns)
	if err != nil {
		return nil, err
	}
	if migrate {
		if _, err := db.MigrateUp(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	key, err := resolveSecretKey(cfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	box, err := secrets.NewBox(key)
	if err != nil {
		db.Close()
		return nil, err
	}
	st := store.New(db, box)
	if migrate {
		if sealed, err := st.HasSealedSecrets(ctx); err == nil && sealed != "" {
			if _, err := box.Open(sealed); err != nil {
				db.Close()
				return nil, fmt.Errorf("encrypted connection parameters exist but APP_SECRET_KEY is missing or wrong")
			}
		}
	}
	return st, nil
}

// resolveSecretKey returns APP_SECRET_KEY, or the content of
// APP_SECRET_KEY_FILE, generating that file (0600) on first use.
func resolveSecretKey(cfg *config.Config) (string, error) {
	if cfg.App.SecretKey != "" {
		return cfg.App.SecretKey, nil
	}
	path := cfg.App.SecretKeyFile
	if path == "" {
		return "", nil
	}
	if b, err := os.ReadFile(path); err == nil {
		key := strings.TrimSpace(string(b))
		if len(key) < 32 {
			return "", fmt.Errorf("APP_SECRET_KEY_FILE %s: key must have at least 32 characters", path)
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read APP_SECRET_KEY_FILE: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	key := store.RandomHex(32)
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("create APP_SECRET_KEY_FILE: %w", err)
	}
	slog.Warn("generated a new secret key file; back it up together with the database", "path", path)
	return key, nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "goguacamole %s (commit %s, built %s)\n", Version, GitCommit, BuildDate)
		},
	}
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect configuration"}
	cmd.AddCommand(&cobra.Command{
		Use:   "print",
		Short: "Print the effective configuration as TOML (secrets redacted)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if cfg == nil {
				return err
			}
			out, rerr := cfg.Redacted()
			if rerr != nil {
				return rerr
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			if err != nil {
				return fmt.Errorf("configuration is invalid: %w", err)
			}
			return nil
		},
	})
	return cmd
}
