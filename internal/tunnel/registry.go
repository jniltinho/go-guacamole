// Package tunnel manages launch tickets, the registry of active tunnels and
// the WebSocket ⇄ guacd relay.
package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/jniltinho/go-guacamole/internal/guac"
)

// TicketTTL is how long a launch ticket stays valid.
const TicketTTL = 30 * time.Second

// Errors returned by the registry.
var (
	ErrLimit         = errors.New("CONNECTION_LIMIT")
	ErrTicketInvalid = errors.New("TICKET_INVALID")
)

// State of a tunnel.
type State string

const (
	StatePending   State = "pending"   // ticket issued, not redeemed
	StateHandshake State = "handshake" // redeemed, guacd handshake running
	StateActive    State = "active"
)

// Tunnel is a reservation that becomes an active tunnel.
type Tunnel struct {
	ID             string
	Ticket         string
	SessionID      string
	UserID         int64
	Username       string
	ConnectionID   int64
	ConnectionName string
	Protocol       string
	ClientIP       string
	StartAt        time.Time
	ExpiresAt      time.Time
	MaxConn        int
	MaxPerUser     int
	// recording snapshot taken at launch (see server recording policy) and its history entry
	RecVideo, RecText, RecKeys, RecException bool
	HistoryID                                int64

	mu     sync.Mutex
	state  State
	ctx    context.Context
	cancel context.CancelCauseFunc
}

// State returns the tunnel state.
func (t *Tunnel) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Context is cancelled when the tunnel is revoked or killed; the cause is a *guac.Error.
func (t *Tunnel) Context() context.Context { return t.ctx }

// Info is the public view of a tunnel.
type Info struct {
	ID             string `json:"id"`
	UserID         int64  `json:"userId,string"`
	Username       string `json:"username"`
	ConnectionID   int64  `json:"connectionId,string"`
	ConnectionName string `json:"connectionName"`
	Protocol       string `json:"protocol"`
	ClientIP       string `json:"clientIp"`
	StartAt        int64  `json:"startAt"`
	State          State  `json:"state"`
	RecVideo       bool   `json:"recVideo"`
	RecText        bool   `json:"recText"`
	RecKeys        bool   `json:"recKeys"`
}

// Registry holds every reservation and tunnel of this process.
type Registry struct {
	mu      sync.Mutex
	byID    map[string]*Tunnel
	tickets map[string]*Tunnel
	now     func() time.Time
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]*Tunnel{}, tickets: map[string]*Tunnel{}, now: time.Now}
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewUUID returns a random RFC 4122 v4 UUID string (36 chars).
func NewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func (r *Registry) expireLocked(now time.Time) {
	for tk, t := range r.tickets {
		if t.state == StatePending && now.After(t.ExpiresAt) {
			delete(r.tickets, tk)
			delete(r.byID, t.ID)
			t.cancel(guac.Errorf(guac.StatusClientTimeout, "ticket expired"))
		}
	}
}

// ExpireTickets drops tickets past their TTL (also done lazily on Issue/Redeem).
func (r *Registry) ExpireTickets() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(r.now())
}

// Issue atomically reserves capacity and returns a ticket bound to the tunnel.
// MaxConn / MaxPerUser of 0 mean unlimited.
func (r *Registry) Issue(t *Tunnel) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.expireLocked(now)
	if t.MaxConn > 0 || t.MaxPerUser > 0 {
		total, mine := 0, 0
		for _, o := range r.byID {
			if o.ConnectionID != t.ConnectionID {
				continue
			}
			total++
			if o.UserID == t.UserID {
				mine++
			}
		}
		if (t.MaxConn > 0 && total >= t.MaxConn) || (t.MaxPerUser > 0 && mine >= t.MaxPerUser) {
			return "", ErrLimit
		}
	}
	t.ID = NewUUID()
	t.Ticket = randomID(16)
	t.ExpiresAt = now.Add(TicketTTL)
	t.state = StatePending
	t.ctx, t.cancel = context.WithCancelCause(context.Background())
	r.byID[t.ID] = t
	r.tickets[t.Ticket] = t
	return t.Ticket, nil
}

// Redeem consumes a ticket for the given session. It is single use.
func (r *Registry) Redeem(ticket, sessionID string) (*Tunnel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.expireLocked(now)
	t, ok := r.tickets[ticket]
	if !ok {
		return nil, ErrTicketInvalid
	}
	delete(r.tickets, ticket)
	if t.SessionID != sessionID {
		delete(r.byID, t.ID)
		t.cancel(guac.Errorf(guac.StatusClientForbidden, "ticket bound to another session"))
		return nil, ErrTicketInvalid
	}
	t.mu.Lock()
	t.state = StateHandshake
	t.mu.Unlock()
	return t, nil
}

// Activate marks a tunnel active once the guacd handshake is done.
func (r *Registry) Activate(t *Tunnel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := context.Cause(t.ctx); err != nil {
		return err
	}
	if _, ok := r.byID[t.ID]; !ok {
		return guac.Errorf(guac.StatusResourceClosed, "tunnel removed")
	}
	t.mu.Lock()
	t.state = StateActive
	t.StartAt = r.now()
	t.mu.Unlock()
	return nil
}

// Remove drops a tunnel from the registry.
func (r *Registry) Remove(t *Tunnel) {
	r.mu.Lock()
	delete(r.byID, t.ID)
	if t.Ticket != "" {
		delete(r.tickets, t.Ticket)
	}
	r.mu.Unlock()
	t.cancel(guac.Errorf(guac.StatusSuccess, "closed"))
}

// Revoke cancels every reservation/tunnel matching fn with the given status.
// It returns the number of affected entries.
func (r *Registry) Revoke(fn func(*Tunnel) bool, status guac.Status, msg string) int {
	r.mu.Lock()
	var hit []*Tunnel
	for id, t := range r.byID {
		if fn(t) {
			hit = append(hit, t)
			delete(r.byID, id)
			delete(r.tickets, t.Ticket)
		}
	}
	r.mu.Unlock()
	for _, t := range hit {
		t.cancel(guac.Errorf(status, "%s", msg))
	}
	return len(hit)
}

// Get returns a tunnel by id.
func (r *Registry) Get(id string) *Tunnel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byID[id]
}

// Snapshot returns all tunnels matching fn (nil → all).
func (r *Registry) Snapshot(fn func(*Tunnel) bool) []*Tunnel {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Tunnel
	for _, t := range r.byID {
		if fn == nil || fn(t) {
			out = append(out, t)
		}
	}
	return out
}

// List returns active tunnels, newest first.
func (r *Registry) List(fn func(*Tunnel) bool) []Info {
	var out []Info
	for _, t := range r.Snapshot(fn) {
		st := t.State()
		if st != StateActive {
			continue
		}
		out = append(out, Info{ID: t.ID, UserID: t.UserID, Username: t.Username, ConnectionID: t.ConnectionID,
			ConnectionName: t.ConnectionName, Protocol: t.Protocol, ClientIP: t.ClientIP, StartAt: t.StartAt.UnixMilli(), State: st,
			RecVideo: t.RecVideo && !t.RecException, RecText: t.RecText && !t.RecException, RecKeys: t.RecKeys && !t.RecException})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartAt > out[j].StartAt })
	return out
}

// CountActive returns the number of active tunnels matching fn.
func (r *Registry) CountActive(fn func(*Tunnel) bool) int { return len(r.List(fn)) }

// CloseAll cancels everything (shutdown).
func (r *Registry) CloseAll(msg string) {
	r.Revoke(func(*Tunnel) bool { return true }, guac.StatusServerError, msg)
}
