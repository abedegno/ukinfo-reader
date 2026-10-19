// Package publichttp bounds anonymous API work without another proxy service.
package publichttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type client struct {
	tokens, exports float64
	last            time.Time
	active          int
}

// Limits are per process. Replicas multiply the global capacity limit.
type Limits struct {
	mu         sync.Mutex
	clients    map[netip.Prefix]*client
	trusted    []netip.Prefix
	slots      chan struct{}
	now        func() time.Time
	lastSweep  time.Time
	maxClients int
	denied     atomic.Uint64
}

// New allows eight concurrent API requests, two requests/sec/client (burst ten),
// and six exports/minute (burst two). Expensive routes allow two concurrent
// requests/client; startup metadata requests do not share that smaller cap.
// At most 8192 client buckets are retained.
// Only explicitly trusted socket peers can supply CF-Connecting-IP. XFF is ignored.
func New(trusted string) (*Limits, error) {
	l := &Limits{clients: make(map[netip.Prefix]*client), slots: make(chan struct{}, 8), now: time.Now, maxClients: 8192}
	if trusted != "" {
		for _, value := range strings.Split(trusted, ",") {
			p, err := netip.ParsePrefix(strings.TrimSpace(value))
			if err != nil || p.Bits() == 0 {
				return nil, fmt.Errorf("trusted-proxy requires explicit, non-universal IP CIDRs")
			}
			l.trusted = append(l.trusted, p.Masked())
		}
	}
	return l, nil
}

func peer(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	return ip.Unmap(), err == nil && ip.Zone() == ""
}

func (l *Limits) identity(r *http.Request, socket netip.Addr) (netip.Addr, bool) {
	for _, p := range l.trusted {
		if p.Contains(socket) {
			values := r.Header.Values("CF-Connecting-IP")
			if len(values) != 1 {
				return netip.Addr{}, false
			}
			ip, err := netip.ParseAddr(values[0])
			return ip.Unmap(), err == nil && ip.Zone() == ""
		}
	}
	return socket, true
}

func (l *Limits) reject(w http.ResponseWriter, status int, message string, retry string) {
	l.denied.Add(1)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if retry != "" {
		w.Header().Set("Retry-After", retry)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// WriteMetrics appends aggregate counters only, on the private metrics listener.
func (l *Limits) WriteMetrics(w io.Writer) {
	l.mu.Lock()
	clients := len(l.clients)
	l.mu.Unlock()
	fmt.Fprintf(w, "# TYPE ukinfo_public_denied_total counter\nukinfo_public_denied_total %d\n", l.denied.Load())
	fmt.Fprintf(w, "# TYPE ukinfo_public_active gauge\nukinfo_public_active %d\n", len(l.slots))
	fmt.Fprintf(w, "# TYPE ukinfo_public_client_buckets gauge\nukinfo_public_client_buckets %d\n", clients)
}

func (l *Limits) allow(ip netip.Addr, export bool) bool {
	bits := 32
	if ip.Is6() {
		bits = 64
	} // Rotating addresses within one IPv6 subnet share a quota.
	key := netip.PrefixFrom(ip, bits).Masked()
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	c := l.clients[key]
	if c == nil {
		if len(l.clients) >= l.maxClients && now.Sub(l.lastSweep) >= time.Minute {
			for key, candidate := range l.clients {
				if now.Sub(candidate.last) >= 10*time.Minute {
					delete(l.clients, key)
				}
			}
			l.lastSweep = now
		}
		// Do not evict live buckets: rotation must not reset an existing quota.
		if len(l.clients) >= l.maxClients {
			return false
		}
		c = &client{tokens: 10, exports: 2, last: now}
		l.clients[key] = c
	}
	elapsed := max(0, now.Sub(c.last).Seconds())
	c.tokens = min(10, c.tokens+elapsed*2)
	c.exports = min(2, c.exports+elapsed*.1)
	c.last = now
	if c.tokens < 1 || (export && c.exports < 1) {
		return false
	}
	c.tokens--
	if export {
		c.exports--
	}
	return true
}

func (l *Limits) concurrent(ip netip.Addr, change int) bool {
	bits := 32
	if ip.Is6() {
		bits = 64
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.clients[netip.PrefixFrom(ip, bits).Masked()]
	if c == nil || (change > 0 && c.active >= 2) {
		return false
	}
	c.active += change
	return true
}

func (l *Limits) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			l.reject(w, 405, "Only GET and HEAD are supported", "")
			return
		}
		if len(r.URL.RawQuery) > 8192 {
			l.reject(w, 414, "Query is too long", "")
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		socket, ok := peer(r)
		if !ok {
			l.reject(w, 400, "Invalid client address", "")
			return
		}
		// Container health checks must remain independent of public traffic/quotas.
		if r.URL.Path == "/api/health" && socket.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}
		ip, ok := l.identity(r, socket)
		if !ok {
			l.reject(w, 400, "Invalid proxy client address", "")
			return
		}
		export := r.URL.Path == "/api/export"
		if !l.allow(ip, export) {
			retry := "1"
			if export {
				retry = "10"
			}
			l.reject(w, 429, "Request limit reached; retry shortly", retry)
			return
		}
		select {
		case l.slots <- struct{}{}:
			defer func() { <-l.slots }()
		default:
			l.reject(w, 429, "Search capacity reached; retry shortly", "1")
			return
		}
		switch r.URL.Path {
		case "/api/search", "/api/export", "/api/record", "/api/address", "/api/group", "/api/map":
			if !l.concurrent(ip, 1) {
				l.reject(w, 429, "Too many concurrent requests; retry shortly", "1")
				return
			}
			defer l.concurrent(ip, -1)
		}
		next.ServeHTTP(w, r)
	})
}
