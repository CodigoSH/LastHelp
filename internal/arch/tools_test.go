package arch

import (
	"strings"
	"testing"
)

// TestVerificationToolsStayOutOfWhatWeShip.
//
// internal/forge/fake is a forge that answers "yes" to everything and signs its
// own deliveries. That is exactly what a test needs and exactly what must never
// be reachable from a release: a binary that could be pointed at it is a binary
// with a forge that approves anything.
//
// It finds nothing today. That is the point of writing it now — the import that
// would break this is one somebody adds while debugging, at four in the
// afternoon, meaning to take it out again.
func TestVerificationToolsStayOutOfWhatWeShip(t *testing.T) {
	const tool = "internal/forge/fake"

	for _, f := range loadSources(t) {
		if strings.HasSuffix(f.path, "_test.go") || under(f.pkg, []string{tool}) {
			continue
		}
		for _, imp := range f.imports() {
			if under(imp, []string{tool}) {
				t.Errorf("\n%s imports %s\n\n"+
					"That package is a forge which answers yes to everything and signs its own\n"+
					"deliveries. It belongs to tests. Reachable from a release, it is a way to\n"+
					"point this server at a forge that approves anything.", f.path, imp)
			}
		}
	}
}

// TestEveryAdapterUsesTheGate.
//
// The outbound rule already forbids constructing an http.Client outside the
// gate. This is the other direction: an adapter that talks to somebody else's
// server must actually hold the gate, rather than being handed a plain client
// by a caller who meant well.
func TestEveryAdapterUsesTheGate(t *testing.T) {
	seen := false
	for _, f := range loadSources(t) {
		if !under(f.pkg, []string{"internal/forge"}) || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		if under(f.pkg, []string{"internal/forge/fake"}) {
			continue
		}
		seen = true

		src := f.render(t)
		if !strings.Contains(src, "outbound.Client") && strings.Contains(src, "http.NewRequest") {
			t.Errorf("\n%s builds requests without holding the outbound gate\n\n"+
				"Every connection to somebody else's server goes through\n"+
				"internal/platform/outbound: it is what the offline switch stops, what\n"+
				"bounds the response, and what refuses a redirect to another host.", f.path)
		}
	}
	if !seen {
		t.Error("no adapter files were read; this rule is checking nothing")
	}
}
