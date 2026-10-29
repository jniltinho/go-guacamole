package guac

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// SupportedVersion is the highest protocol version this implementation speaks.
const SupportedVersion = "VERSION_1_5_0"

// HandshakeTimeout bounds the whole handshake.
const HandshakeTimeout = 15 * time.Second

// ClientInfo carries the browser-declared display capabilities.
type ClientInfo struct {
	Width, Height, DPI int
	Audio, Video       []string
	Image              []string
	Timezone           string
}

// Config describes the connection to establish.
type Config struct {
	Protocol   string
	Parameters map[string]string
	Client     ClientInfo
}

// Conn is an established guacd connection after a successful handshake.
type Conn struct {
	net.Conn
	Reader       *Reader
	ConnectionID string
	Version      string
}

// DialOptions configures how guacd is reached.
type DialOptions struct {
	Addr      string
	TLSConfig *tls.Config // nil → plaintext
}

// Dial connects to guacd. When TLSConfig is set the TLS handshake must
// succeed; there is never a plaintext fallback.
func Dial(ctx context.Context, o DialOptions) (net.Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	if o.TLSConfig != nil {
		td := tls.Dialer{NetDialer: &d, Config: o.TLSConfig}
		c, err := td.DialContext(ctx, "tcp", o.Addr)
		if err != nil {
			var nerr net.Error
			var opErr *net.OpError
			if errors.As(err, &opErr) && opErr.Op == "dial" || errors.As(err, &nerr) && nerr.Timeout() {
				return nil, Errorf(StatusUpstreamUnavailable, "guacd unavailable: %v", err)
			}
			return nil, Errorf(StatusUpstreamError, "guacd TLS handshake failed: %v", err)
		}
		return c, nil
	}
	c, err := d.DialContext(ctx, "tcp", o.Addr)
	if err != nil {
		return nil, Errorf(StatusUpstreamUnavailable, "guacd unavailable: %v", err)
	}
	return c, nil
}

// Handshake performs the Guacamole handshake over c.
func Handshake(ctx context.Context, c net.Conn, cfg Config) (*Conn, error) {
	deadline := time.Now().Add(HandshakeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = c.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	rd := NewReader(c, 0)
	write := func(ins Instruction) error {
		_, err := c.Write(ins.Bytes())
		return err
	}

	if err := write(New("select", cfg.Protocol)); err != nil {
		return nil, wrapIO(ctx, err)
	}
	args, err := expect(rd, "args")
	if err != nil {
		return nil, wrapIO(ctx, err)
	}

	version := ""
	values := make([]string, len(args.Args))
	for i, name := range args.Args {
		if strings.HasPrefix(name, "VERSION_") {
			version = NegotiateVersion(name)
			values[i] = version
			continue
		}
		values[i] = cfg.Parameters[name]
	}

	ci := cfg.Client
	if ci.Width <= 0 {
		ci.Width = 1024
	}
	if ci.Height <= 0 {
		ci.Height = 768
	}
	if ci.DPI <= 0 {
		ci.DPI = 96
	}
	steps := []Instruction{
		New("size", strconv.Itoa(ci.Width), strconv.Itoa(ci.Height), strconv.Itoa(ci.DPI)),
		New("audio", ci.Audio...),
		New("video", ci.Video...),
		New("image", ci.Image...),
	}
	if version != "" && ci.Timezone != "" {
		steps = append(steps, New("timezone", ci.Timezone))
	}
	steps = append(steps, New("connect", values...))
	for _, s := range steps {
		if err := write(s); err != nil {
			return nil, wrapIO(ctx, err)
		}
	}

	ready, err := expect(rd, "ready")
	if err != nil {
		return nil, wrapIO(ctx, err)
	}
	if len(ready.Args) == 0 {
		return nil, Errorf(StatusServerError, "guacd sent ready without connection id")
	}
	_ = c.SetDeadline(time.Time{})
	return &Conn{Conn: c, Reader: rd, ConnectionID: ready.Args[0], Version: version}, nil
}

// NegotiateVersion returns the lower of the offered version and SupportedVersion.
func NegotiateVersion(offered string) string {
	if compareVersion(offered, SupportedVersion) < 0 {
		return offered
	}
	return SupportedVersion
}

func compareVersion(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(a, "VERSION_"), "_")
	pb := strings.Split(strings.TrimPrefix(b, "VERSION_"), "_")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func expect(rd *Reader, opcode string) (Instruction, error) {
	ins, err := rd.Read()
	if err != nil {
		return ins, err
	}
	if ins.Opcode == "error" {
		msg, code := "guacd error", StatusServerError
		if len(ins.Args) > 0 {
			msg = ins.Args[0]
		}
		if len(ins.Args) > 1 {
			if n, err := strconv.Atoi(ins.Args[1]); err == nil {
				code = Status(n)
			}
		}
		return ins, &Error{Status: code, Message: msg}
	}
	if ins.Opcode == "disconnect" {
		return ins, Errorf(StatusUpstreamError, "guacd disconnected during handshake")
	}
	if ins.Opcode != opcode {
		return ins, Errorf(StatusServerError, "expected %q from guacd, got %q", opcode, ins.Opcode)
	}
	return ins, nil
}

func wrapIO(ctx context.Context, err error) error {
	var ge *Error
	if errors.As(err, &ge) {
		return ge
	}
	if ctx.Err() != nil {
		return Errorf(StatusResourceClosed, "handshake cancelled")
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return Errorf(StatusUpstreamTimeout, "guacd handshake timed out")
	}
	if errors.Is(err, ErrMalformed) || errors.Is(err, ErrTooLarge) {
		return Errorf(StatusServerError, "invalid data from guacd: %v", err)
	}
	return Errorf(StatusUpstreamError, "guacd handshake failed: %v", err)
}
