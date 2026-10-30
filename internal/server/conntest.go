package server

import (
	"context"
	"crypto/x509"
	"errors"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/ssh"

	"github.com/jniltinho/go-guacamole/internal/guac"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

// TestTimeout bounds a connection test: guacd, the remote host and its login.
const TestTimeout = 20 * time.Second

// testSkip are the parameters that run something on the remote host or touch guacd's disk: a test only connects
// and logs in, so it never runs a command that a recorded session would.
var testSkip = []string{"command", "initial-program", "remote-app", "remote-app-dir", "remote-app-args",
	"enable-drive", "drive-path", "create-drive-path", "enable-sftp", "wol-send-packet"}

type testResult struct {
	OK bool `json:"ok"`
	// UNVERIFIED: guacd reached the host but the login cannot be checked without credentials (SSH asks in the terminal)
	Code    string `json:"code,omitempty"`
	Status  int    `json:"status,omitempty"`  // Guacamole status code of the failure
	Message string `json:"message,omitempty"` // guacd's own explanation, when it gives one
	Ms      int64  `json:"ms"`
}

// testConnection opens a real guacd connection with the form's values (a new connection, or an edited one whose
// stored secrets fill what was not typed again) and reports whether the remote host answered and logged in.
// Nothing is recorded and the connection is closed right away.
func (a *App) testConnection(c *echo.Context) error {
	var in struct {
		connectionInput
		ID string `json:"id"`
	}
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	eff, u := effOf(c), userOf(c)
	base := &store.Connection{Protocol: in.Protocol}
	var existing map[string]string
	if in.ID != "" {
		id, err := parseID(in.ID)
		if err != nil {
			return err
		}
		cur, err := a.Store.GetConnection(ctx, id)
		if err != nil || !eff.CanRead(cur) && !eff.CanEditConnection(cur) {
			return errNotFound
		}
		if !eff.CanEditConnection(cur) {
			return errForbidden
		}
		base, existing = cur, cur.Parameters
	}
	conn := *base
	if err := in.apply(&conn, existing); err != nil {
		return err
	}
	if in.ID == "" && !eff.CanCreateIn(conn.ParentID, a.folderVisible(ctx, eff)) {
		return errForbidden
	}
	if guacdChanged(base, &conn) && !eff.IsAdmin() {
		return apiErr(http.StatusForbidden, "FORBIDDEN", "only administrators can change the protocol or the guacd endpoint")
	}
	if !a.launches.Hit(idStr(u.ID)) {
		return apiErr(http.StatusTooManyRequests, "TOO_MANY_LAUNCHES", "too many connection attempts, try again later")
	}
	res := a.tryConnection(ctx, &conn)
	outcome := "success"
	if !res.OK {
		outcome = "failure"
	}
	a.audit(c, &u.ID, u.Username, "connection.test", "connection", in.ID, conn.Name, outcome,
		map[string]any{"protocol": conn.Protocol, "status": res.Status, "code": res.Code})
	return c.JSON(http.StatusOK, res)
}

// errKeyPassphrase: the passphrase does not open the private key (guacd would ask for another in the terminal).
var errKeyPassphrase = errors.New("passphrase does not open the private key")

// sshLogin reports whether an SSH test can check the login. guacd asks in the terminal, instead of failing, when
// the username, the password or key, or the key's passphrase is missing, and when it cannot read the key.
func sshLogin(p map[string]string) (bool, error) {
	if p["username"] == "" {
		return false, nil
	}
	key := p["private-key"]
	if key == "" {
		return p["password"] != "", nil
	}
	_, err := ssh.ParseRawPrivateKey([]byte(key))
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		return true, nil
	case !errors.As(err, &missing):
		return false, nil // a format guacd may still read: not verified rather than failed
	case p["passphrase"] == "":
		return false, nil
	}
	_, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(key), []byte(p["passphrase"]))
	if errors.Is(err, x509.IncorrectPasswordError) {
		return false, errKeyPassphrase
	}
	return err == nil, nil // a cipher this parser lacks: not verified
}

// tryConnection runs the guacd handshake and reads until the remote side answers: the first frame (sync) after
// something was drawn means connected and logged in (RDP sends an empty sync before it connects); an error, or
// guacd asking for credentials, is a failure.
func (a *App) tryConnection(ctx context.Context, conn *store.Connection) testResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, TestTimeout)
	defer cancel()
	// only guacd's own explanation reaches the browser (never our dial errors, which name the guacd address)
	fail := func(e *guac.Error) testResult {
		return testResult{Status: int(e.Status), Ms: time.Since(start).Milliseconds()}
	}
	verified := true
	if conn.Protocol == "ssh" {
		ok, err := sshLogin(conn.Parameters)
		if err != nil {
			r := fail(guac.Errorf(guac.StatusClientUnauthorized, ""))
			r.Code = "KEY_PASSPHRASE"
			return r
		}
		verified = ok
	}
	opts, err := a.guacdDial(conn)
	if err != nil {
		return fail(guac.Errorf(guac.StatusServerError, "guacd TLS configuration error"))
	}
	raw, err := guac.Dial(ctx, opts)
	if err != nil {
		return fail(tunnel.AsGuacError(err))
	}
	defer raw.Close()
	// the stored parameters without testSkip: no recording, no color scheme, no command
	params := maps.Clone(conn.Parameters)
	for _, k := range testSkip {
		delete(params, k)
	}
	g, err := guac.Handshake(ctx, raw, guac.Config{Protocol: conn.Protocol, Parameters: params})
	if err != nil {
		return fail(tunnel.AsGuacError(err))
	}
	stop := context.AfterFunc(ctx, func() { _ = g.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	drawn := false
	for {
		ins, err := g.Reader.Read()
		if err != nil {
			if ctx.Err() != nil {
				return fail(guac.Errorf(guac.StatusUpstreamTimeout, ""))
			}
			return fail(guac.Errorf(guac.StatusUpstreamError, ""))
		}
		switch ins.Opcode {
		case "size", "img", "png", "jpeg", "webp", "rect":
			drawn = true
		case "sync":
			if !drawn {
				continue
			}
			_, _ = g.Write(guac.New("disconnect").Bytes())
			r := testResult{OK: true, Ms: time.Since(start).Milliseconds()}
			if !verified {
				r.Code = "UNVERIFIED"
			}
			return r
		case "error":
			st := guac.StatusUpstreamError
			if len(ins.Args) > 1 {
				if n, err := strconv.Atoi(ins.Args[1]); err == nil {
					st = guac.Status(n)
				}
			}
			r := fail(guac.Errorf(st, ""))
			if len(ins.Args) > 0 && !strings.HasPrefix(ins.Args[0], "Aborted") { // "Aborted. See logs." says nothing
				r.Message = ins.Args[0]
			}
			return r
		case "required":
			r := fail(guac.Errorf(guac.StatusClientUnauthorized, ""))
			r.Code, r.Message = "CREDENTIALS_REQUIRED", strings.Join(ins.Args, ", ")
			return r
		case "disconnect":
			return fail(guac.Errorf(guac.StatusUpstreamError, ""))
		}
	}
}
