package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
)

// noticed sends one request through the watcher and says whether it noticed a
// proxy.
func noticed(trust bool, peer string, header, value string) bool {
	s := &Server{cfg: config.Config{TrustProxy: trust}}
	h := s.noticeForwarded(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = peer
	if header != "" {
		r.Header.Set(header, value)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	return s.forwardedUnread.Load()
}

// A proxy in front that the configuration does not know about is noticed by the
// headers it adds, from wherever such a proxy lives: this machine, a container
// network, the LAN.
func TestAnUndeclaredProxyIsNoticed(t *testing.T) {
	for _, c := range []struct{ peer, header string }{
		{"127.0.0.1:50000", "X-Forwarded-For"},
		{"[::1]:50000", "X-Forwarded-For"},
		{"172.18.0.4:50000", "X-Real-Ip"},
		{"192.168.1.20:50000", "Forwarded"},
		{"10.0.0.7:50000", "X-Forwarded-For"},
	} {
		if !noticed(false, c.peer, c.header, "203.0.113.9") {
			t.Errorf("%s from %s was not noticed", c.header, c.peer)
		}
	}
}

// Nothing is said when the proxy is declared, when nothing says there is one,
// or when the header comes from the internet, where anybody can type it.
func TestNoNoticeWithoutGrounds(t *testing.T) {
	for name, got := range map[string]bool{
		"the proxy is declared":         noticed(true, "127.0.0.1:50000", "X-Forwarded-For", "203.0.113.9"),
		"no proxy header":               noticed(false, "127.0.0.1:50000", "", ""),
		"a header from the internet":    noticed(false, "198.51.100.4:50000", "X-Forwarded-For", "203.0.113.9"),
		"a peer that is not an address": noticed(false, "nonsense", "X-Forwarded-For", "203.0.113.9"),
	} {
		if got {
			t.Errorf("%s: noticed a proxy", name)
		}
	}
}

// What was noticed reaches the administrator on About, and not before.
func TestAboutSaysAProxyIsUndeclared(t *testing.T) {
	i := newInstance(t)
	i.claim()

	if _, said := decode(t, i.do(http.MethodGet, "/api/server", nil))["proxy_unread"]; said {
		t.Fatal("About says a proxy is undeclared before any proxy header arrived")
	}

	req, err := http.NewRequest(http.MethodGet, i.server.URL+"/api/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	resp, err := i.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got := decode(t, i.do(http.MethodGet, "/api/server", nil))["proxy_unread"]; got != true {
		t.Errorf("after a proxy header from this machine, About says proxy_unread=%v, want true", got)
	}
}
