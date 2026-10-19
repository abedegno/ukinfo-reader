package publichttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func get(h http.Handler, path, peer, claimed string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = peer
	if claimed != "" {
		r.Header.Set("CF-Connecting-IP", claimed)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }

func TestProxySpoofingAndIPIsolation(t *testing.T) {
	l, _ := New("192.0.2.10/32")
	l.now = func() time.Time { return time.Unix(1000, 0) }
	h := l.Wrap(http.HandlerFunc(ok))
	for i := range 10 {
		claim := "198.51.100." + string(rune('1'+i))
		if get(h, "/api/search", "203.0.113.1:1234", claim).Code != 200 {
			t.Fatal("untrusted bucket rejected early")
		}
	}
	if get(h, "/api/search", "203.0.113.1:1234", "198.51.100.90").Code != 429 {
		t.Fatal("untrusted header reset the quota")
	}
	for _, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		for range 10 {
			if get(h, "/api/search", "192.0.2.10:1234", ip).Code != 200 {
				t.Fatal("trusted clients share a quota")
			}
		}
	}
	for _, ip := range []string{"", "198.51.100.1, 198.51.100.2", "not-an-ip", "fe80::1%eth0"} {
		if get(h, "/api/search", "192.0.2.10:1234", ip).Code != 400 {
			t.Fatal("malformed trusted identity accepted")
		}
	}
	r := httptest.NewRequest("GET", "/api/search", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	r.Header.Add("CF-Connecting-IP", "198.51.100.1")
	r.Header.Add("CF-Connecting-IP", "198.51.100.2")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("duplicate identity accepted")
	}
}

func TestRefillExportsIPv6AndBoundedState(t *testing.T) {
	l, _ := New("")
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	l.maxClients = 2
	h := l.Wrap(http.HandlerFunc(ok))
	for range 2 {
		if get(h, "/api/export", "[2001:db8::1]:1", "").Code != 200 {
			t.Fatal("export rejected early")
		}
	}
	w := get(h, "/api/export", "[2001:db8::abcd]:1", "")
	if w.Code != 429 || w.Header().Get("Retry-After") != "10" {
		t.Fatal("IPv6 rotation bypassed export quota")
	}
	now = now.Add(10 * time.Second)
	if get(h, "/api/export", "[2001:db8::2]:1", "").Code != 200 {
		t.Fatal("export did not refill")
	}
	if get(h, "/api/search", "203.0.113.1:1", "").Code != 200 {
		t.Fatal("second bucket rejected")
	}
	if get(h, "/api/search", "203.0.113.2:1", "").Code != 429 || len(l.clients) != 2 {
		t.Fatal("state cap not enforced")
	}
	now = now.Add(11 * time.Minute)
	if get(h, "/api/search", "203.0.113.2:1", "").Code != 200 || len(l.clients) != 1 {
		t.Fatal("expired state not reclaimed")
	}
}

func TestConcurrencyAndLocalHealth(t *testing.T) {
	l, _ := New("")
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	h := l.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			entered <- struct{}{}
			<-release
		}
		w.WriteHeader(200)
	}))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); get(h, "/api/search", "203.0.113."+string(rune('1'+i))+":1", "") }(i)
	}
	for range 8 {
		<-entered
	}
	if get(h, "/api/datasets", "198.51.100.1:1", "").Code != 429 {
		t.Fatal("global admission cap bypassed")
	}
	if get(h, "/api/health", "127.0.0.1:1", "").Code != 200 {
		t.Fatal("health shares saturated search capacity")
	}
	if get(h, "/app.js", "198.51.100.1:1", "").Code != 200 {
		t.Fatal("static files share API capacity")
	}
	close(release)
	wg.Wait()
	if len(l.slots) != 0 {
		t.Fatal("admission slot leaked")
	}
}

func TestPerClientConcurrencyAndRequestBounds(t *testing.T) {
	l, _ := New("")
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	h := l.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			entered <- struct{}{}
			<-release
		}
		w.WriteHeader(200)
	}))
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); get(h, "/api/search", "203.0.113.1:1", "") }()
	}
	for range 2 {
		<-entered
	}
	if get(h, "/api/search", "203.0.113.1:1", "").Code != 429 {
		t.Fatal("client monopolized global capacity")
	}
	if get(h, "/api/datasets", "203.0.113.1:1", "").Code != 200 {
		t.Fatal("metadata shares the per-client work cap")
	}
	close(release)
	wg.Wait()
	for _, method := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
		r := httptest.NewRequest(method, "/api/search", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 405 {
			t.Fatal("non-read method accepted")
		}
	}
	if get(h, "/api/search?x="+strings.Repeat("a", 8193), "203.0.113.1:1", "").Code != 414 {
		t.Fatal("oversized query accepted")
	}
	if _, e := New("0.0.0.0/0"); e == nil {
		t.Fatal("universal proxy trust accepted")
	}
}
