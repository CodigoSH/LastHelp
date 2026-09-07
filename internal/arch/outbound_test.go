package arch

import (
	"regexp"
	"strings"
	"testing"
)

// TestEveryOutboundConnectionGoesThroughAGate.
//
// SECURITY.md promises there is no telemetry, no CDN, no captcha service and no
// AI provider — that what people write in a portal reaches its database and
// nowhere else. The only outbound connections this binary makes are the ones
// the operator configured: their forge, and their notification channel.
//
// Both promises are worth the paper unless there is exactly one way out. This
// is the rule that keeps them true a year from now, when somebody adds a
// feature that needs to fetch one small thing and reaches for net/http in a
// package where that looks perfectly reasonable.
//
// It finds nothing today: neither gate package exists yet. That is deliberate.
// A rule added after the first violation is a rule written with the violation
// exempted.
func TestEveryOutboundConnectionGoesThroughAGate(t *testing.T) {
	// The packages that are allowed to open a socket.
	allowed := []string{
		// Every forge call and every operator-configured URL goes through here.
		"internal/platform/outbound",
		// The mail relay is an address the operator chose, so it cannot use the
		// guard that refuses private addresses.
		"internal/platform/mail",
	}

	// A separate binary whose only address is loopback: it asks the server next
	// to it whether it is alive, which is what a container healthcheck is. It is
	// named here rather than left to the rule because it is genuinely an
	// exception and not an oversight — and because the next thing anybody adds
	// to cmd/ should have to argue for itself. The rule below holds it to that.
	allowedFiles := []string{
		"cmd/healthcheck/main.go",
	}

	// What dialling looks like. Constructing a client is enough to fail: a
	// package holding its own http.Client is a package that will use it.
	//
	// Matched on word boundaries, not as substrings. The first version of this
	// looked for "http.Head" inside the source and fired on every file that
	// mentioned http.Header — which is most of them. A rule that cries wolf is
	// a rule people silence with exemptions instead of fixing.
	dialers := []string{
		"net.Dial", "net.DialTimeout", "net.Dialer",
		"http.Get", "http.Post", "http.PostForm", "http.Head",
		"http.DefaultClient", "http.DefaultTransport",
		"http.Client",
		"smtp.Dial", "smtp.SendMail",
		"tls.Dial",
	}

	for _, f := range loadSources(t) {
		if strings.HasSuffix(f.path, "_test.go") || under(f.pkg, allowed) {
			continue
		}
		if contains(allowedFiles, f.path) {
			continue
		}
		src := f.render(t)
		for _, d := range dialers {
			if !mentions(src, d) {
				continue
			}
			t.Errorf("\n%s uses %s\n\n"+
				"Every outbound connection goes through internal/platform/outbound, or\n"+
				"through internal/platform/mail for the relay the operator configured.\n"+
				"SECURITY.md lists what they can reach and promises there is nothing\n"+
				"else. A connection opened anywhere else is one that document does not\n"+
				"mention.", f.path, d)
		}
	}
}

// TestTheHealthcheckOnlyTalksToItself.
//
// The exemption above is granted on the strength of one claim: that this binary
// only ever addresses loopback. This is that claim, asserted.
func TestTheHealthcheckOnlyTalksToItself(t *testing.T) {
	var found bool
	for _, f := range loadSources(t) {
		if f.path != "cmd/healthcheck/main.go" {
			continue
		}
		found = true
		src := f.render(t)
		if !strings.Contains(src, "127.0.0.1") {
			t.Errorf("\n%s no longer mentions 127.0.0.1\n\n"+
				"It is exempt from the outbound rule because the only address it uses\n"+
				"is loopback. If that stopped being true, the exemption has to go too.",
				f.path)
		}
		if strings.Contains(src, "http://") && strings.Count(src, "http://") > 1 {
			t.Errorf("\n%s builds more than one URL\n\n"+
				"It asks one question of one address. More than one URL here means it\n"+
				"has grown into something the exemption was not granted for.", f.path)
		}
	}
	if !found {
		t.Error("cmd/healthcheck/main.go was not read; the exemption above is being granted to a file nobody checked")
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// mentions reports whether the source uses this identifier, rather than merely
// containing its letters. "http.Head" is a prefix of "http.Header", and
// "net.Dial" of "net.Dialer" — without the boundary, half the repository looks
// like it opens sockets.
func mentions(source, identifier string) bool {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(identifier) + `\b`)
	for _, match := range pattern.FindAllStringIndex(source, -1) {
		end := match[1]
		// Reject a match that is only a prefix of a longer identifier:
		// FindAllStringIndex with \b already stops "http.Header", but an
		// identifier ending in a letter followed by a digit or underscore
		// would slip through.
		if end < len(source) && (source[end] == '_' || isWordByte(source[end])) {
			continue
		}
		return true
	}
	return false
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
