package objstore

import (
	"context"
	"encoding/base64"
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

var (
	secretsMu sync.Mutex
	known     = NewSanitizer()
)

// Secrets hygiene (design D11): configured secret values, their base64 forms, PEM/JSON key material, Authorization
// headers and signature query parameters never reach logs, stored errors, test results or the queue.

var redactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization:\s*)[^\r\n]+`),
	regexp.MustCompile(`(?i)((?:x-amz-signature|x-goog-signature|signature|x-amz-security-token|x-amz-credential)=)[^&\s"']+`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`("private_key(?:_id)?"\s*:\s*")[^"]*`),
	// headers and provider error bodies that echo signing material or session tokens
	regexp.MustCompile(`(?i)((?:x-amz-security-token|x-amz-content-sha256|x-goog-signature)\s*:\s*)[^\r\n]+`),
	regexp.MustCompile(`(?is)(<(SignatureProvided|StringToSign|StringToSignBytes|CanonicalRequest|CanonicalRequestBytes|SecurityToken|Token)>).*?(</\w+>)`),
	regexp.MustCompile(`(?i)("(?:access_token|refresh_token|id_token|sessionToken)"\s*:\s*")[^"]*`),
}

// Sanitizer removes secrets from text.
type Sanitizer struct{ secrets []string }

// NewSanitizer knows the configured secret values (empty ones are ignored).
func NewSanitizer(secrets ...string) *Sanitizer {
	s := &Sanitizer{}
	for _, v := range secrets {
		if len(v) < 4 {
			continue
		}
		s.secrets = append(s.secrets, v, base64.StdEncoding.EncodeToString([]byte(v)), base64.RawURLEncoding.EncodeToString([]byte(v)))
	}
	return s
}

// Clean returns text without secrets.
func (s *Sanitizer) Clean(text string) string {
	for _, v := range s.secrets {
		text = strings.ReplaceAll(text, v, "********")
	}
	for _, re := range redactions {
		text = re.ReplaceAllStringFunc(text, func(m string) string {
			if sub := re.FindStringSubmatch(m); len(sub) > 1 {
				return sub[1] + "********"
			}
			return "********"
		})
	}
	return text
}

// Error returns err's text without secrets ("" for nil).
func (s *Sanitizer) Error(err error) string {
	if err == nil {
		return ""
	}
	return s.Clean(err.Error())
}

// handler is the slog handler installed for rclone's own logs: every message and attribute is cleaned.
type handler struct {
	next  slog.Handler
	clean func(string) string
}

func (h handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }
func (h handler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, h.clean(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(slog.String(a.Key, h.clean(a.Value.String())))
		return true
	})
	return h.next.Handle(ctx, out)
}
func (h handler) WithAttrs(as []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(as))
	for _, a := range as {
		clean = append(clean, slog.String(a.Key, h.clean(a.Value.String())))
	}
	return handler{h.next.WithAttrs(clean), h.clean}
}
func (h handler) WithGroup(n string) slog.Handler { return handler{h.next.WithGroup(n), h.clean} }

// remember adds secret values to the global sanitizer (rclone's logs, stored errors).
func remember(secrets ...string) {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	known = NewSanitizer(append(known.values(), secrets...)...)
}

func (s *Sanitizer) values() []string {
	var out []string
	for i := 0; i < len(s.secrets); i += 3 { // value, std base64, url base64
		out = append(out, s.secrets[i])
	}
	return out
}

// Clean removes every known secret from text.
func Clean(text string) string {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	return known.Clean(text)
}
