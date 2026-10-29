package server

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// DefaultPorts per protocol.
var DefaultPorts = map[string]int{"ssh": 22, "rdp": 3389, "vnc": 5900, "telnet": 23}

// Prober tests TCP reachability of connection targets in the background.
type Prober struct {
	st       *store.Store
	interval time.Duration
	log      *slog.Logger
	mu       sync.RWMutex
	status   map[int64]string
}

// NewProber creates a prober; interval 0 disables it.
func NewProber(st *store.Store, interval time.Duration, log *slog.Logger) *Prober {
	return &Prober{st: st, interval: interval, log: log, status: map[int64]string{}}
}

// Status returns online|offline|unknown.
func (p *Prober) Status(id int64) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if s, ok := p.status[id]; ok {
		return s
	}
	return "unknown"
}

// Run probes until ctx is cancelled.
func (p *Prober) Run(ctx context.Context) {
	if p.interval <= 0 {
		return
	}
	p.round(ctx)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.round(ctx)
		}
	}
}

func targetAddr(c *store.Connection) string {
	host := c.Parameters["hostname"]
	if host == "" {
		return ""
	}
	port, _ := strconv.Atoi(c.Parameters["port"])
	if port == 0 {
		port = DefaultPorts[c.Protocol]
	}
	if port == 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func (p *Prober) round(ctx context.Context) {
	conns, err := p.st.ListConnections(ctx)
	if err != nil {
		p.log.Warn("probe: list connections", "err", err)
		return
	}
	next := map[int64]string{}
	var mu sync.Mutex
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for _, c := range conns {
		addr := targetAddr(c)
		if addr == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id int64, addr string) {
			defer wg.Done()
			defer func() { <-sem }()
			d := net.Dialer{Timeout: 2 * time.Second}
			st := "offline"
			if conn, err := d.DialContext(ctx, "tcp", addr); err == nil {
				conn.Close()
				st = "online"
			}
			mu.Lock()
			next[id] = st
			mu.Unlock()
		}(c.ID, addr)
	}
	wg.Wait()
	p.mu.Lock()
	p.status = next
	p.mu.Unlock()
}
