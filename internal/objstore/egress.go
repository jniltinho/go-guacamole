//go:build !noobjstore

package objstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Endpoint safety (openspec add-async-jobs-object-storage, design D10). rclone dials with its own net.Dialer and
// honours environment proxies, so object storage traffic goes through an in-process egress proxy on 127.0.0.1 that
// rclone is pointed at (per context, never globally): the proxy resolves the destination and checks every address at
// dial time (no DNS rebinding), refuses private and metadata networks unless the operator allows them, and never
// chains to another proxy.

var blocked = mustCIDRs(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "fd00:ec2::254/128",
)

func mustCIDRs(cs ...string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// allowedHost reports whether the operator listed the host (exact name) or one of its addresses (IP or CIDR).
func (p Policy) allowedHost(host string, ip net.IP) bool {
	for _, a := range p.Allowed {
		a = strings.TrimSpace(strings.ToLower(a))
		if a == "" {
			continue
		}
		if a == strings.ToLower(host) {
			return true
		}
		if strings.Contains(a, "/") {
			if _, n, err := net.ParseCIDR(a); err == nil && n.Contains(ip) {
				return true
			}
		} else if aip := net.ParseIP(a); aip != nil && aip.Equal(ip) {
			return true
		}
	}
	return false
}

// checkIP refuses private, loopback, link-local (metadata) and other special addresses not allowed by the operator.
func (p Policy) checkIP(host string, ip net.IP) error {
	if p.allowedHost(host, ip) {
		return nil
	}
	for _, n := range blocked {
		if n.Contains(ip) {
			return fmt.Errorf("%w: %s resolves to %s", ErrEndpointNotAllowed, host, ip)
		}
	}
	return nil
}

// dial resolves addr and connects to the first address that passes the policy; every address is checked.
func (p Policy) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if err := p.checkIP(host, ip); err != nil {
			return nil, err // one forbidden answer refuses the host (no mixed answers)
		}
	}
	d := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	var last error
	for _, ip := range ips {
		c, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return c, nil
		}
		last = err
	}
	return nil, last
}

// precheck gives a clear error before any request when the endpoint is refused (the dial-time check still applies,
// so DNS rebinding after this point cannot bypass the policy).
func (p Policy) precheck(ctx context.Context, endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid endpoint %q", endpoint)
	}
	if u.Scheme == "http" && !p.AllowHTTP {
		return fmt.Errorf("%w: plain HTTP endpoint", ErrEndpointNotAllowed)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", u.Hostname())
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if err := p.checkIP(u.Hostname(), ip); err != nil {
			return err
		}
	}
	return nil
}

// egress is the in-process proxy: CONNECT tunnels for HTTPS, plain forwarding only when the policy allows HTTP.
type egress struct {
	policy Policy
	ln     net.Listener
	srv    *http.Server
	fwd    *httputil.ReverseProxy
}

func startEgress(p Policy) (*egress, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	e := &egress{policy: p, ln: ln}
	tr := &http.Transport{DialContext: p.dial, Proxy: nil, ForceAttemptHTTP2: false, MaxIdleConns: 16, IdleConnTimeout: 60 * time.Second}
	e.fwd = &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.Out.URL = r.In.URL }, Transport: tr,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}}
	e.srv = &http.Server{Handler: http.HandlerFunc(e.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = e.srv.Serve(ln) }()
	return e, nil
}

func (e *egress) url() string { return "http://" + e.ln.Addr().String() }

func (e *egress) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		e.tunnel(w, r)
		return
	}
	if !e.policy.AllowHTTP || r.URL.Scheme != "http" || r.URL.Host == "" {
		http.Error(w, "plain HTTP object storage endpoints are not allowed", http.StatusForbidden)
		return
	}
	e.fwd.ServeHTTP(w, r)
}

func (e *egress) tunnel(w http.ResponseWriter, r *http.Request) {
	dst, err := e.policy.dial(r.Context(), "tcp", r.Host)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, ErrEndpointNotAllowed) {
			code = http.StatusForbidden
		}
		http.Error(w, err.Error(), code)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		dst.Close()
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	src, buf, err := hj.Hijack()
	if err != nil {
		dst.Close()
		return
	}
	_, _ = src.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(dst, buf); closeWrite(dst) }()
	go func() { defer wg.Done(); _, _ = io.Copy(src, dst); closeWrite(src) }()
	wg.Wait()
	src.Close()
	dst.Close()
}

func closeWrite(c net.Conn) {
	if tc, ok := c.(interface{ CloseWrite() error }); ok {
		_ = tc.CloseWrite()
	}
}

func (e *egress) close() { _ = e.srv.Close() }
