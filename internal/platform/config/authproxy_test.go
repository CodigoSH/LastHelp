package config

import (
	"net/netip"
	"testing"
)

func lookupFrom(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

// Off unless the header and the proxy's addresses are both there and readable;
// never half on.
func TestReadAuthProxy(t *testing.T) {
	for _, c := range []struct {
		name     string
		env      map[string]string
		enabled  bool
		problems int
		logout   string
	}{
		{name: "nothing set", env: map[string]string{}},
		{name: "addresses without a header", env: map[string]string{"LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1"}, problems: 1},
		{name: "a header without addresses", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote-User"}, problems: 1},
		{name: "a header that is not one", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote User", "LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1"}, problems: 1},
		{name: "only unreadable addresses", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote-User", "LASTHELP_AUTH_PROXY_TRUSTED": "proxy.local"}, problems: 2},
		{name: "on", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote-User", "LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1, 172.18.0.0/16"}, enabled: true},
		{name: "a bad address among good ones", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote-User", "LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1,nope"}, enabled: true, problems: 1},
		{name: "with its sign-out", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote-User", "LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1", "LASTHELP_AUTH_PROXY_LOGOUT_URL": "https://auth.example.com/logout"}, enabled: true, logout: "https://auth.example.com/logout"},
		{name: "a sign-out that is not a web address", env: map[string]string{"LASTHELP_AUTH_PROXY_HEADER": "Remote-User", "LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1", "LASTHELP_AUTH_PROXY_LOGOUT_URL": "javascript:alert(1)"}, enabled: true, problems: 1},
	} {
		p, problems := ReadAuthProxy(lookupFrom(c.env))
		if p.Enabled() != c.enabled || len(problems) != c.problems || p.LogoutURL != c.logout {
			t.Errorf("%s: enabled %v, %d problems %q, logout %q", c.name, p.Enabled(), len(problems), problems, p.LogoutURL)
		}
	}
}

// An address is trusted when it is one of the proxy's, IPv4 written as IPv6 included.
func TestTheProxysAddresses(t *testing.T) {
	p, _ := ReadAuthProxy(lookupFrom(map[string]string{
		"LASTHELP_AUTH_PROXY_HEADER": "Remote-User", "LASTHELP_AUTH_PROXY_TRUSTED": "10.0.0.1, 172.18.0.0/16",
	}))
	for addr, want := range map[string]bool{
		"10.0.0.1": true, "10.0.0.2": false, "172.18.4.5": true, "::ffff:10.0.0.1": true, "192.168.1.1": false,
	} {
		if got := p.Trusts(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s: trusted %v, want %v", addr, got, want)
		}
	}
}
