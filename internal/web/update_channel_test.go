package web

import "testing"

// TestAnInstallationFollowsTheChannelItCameFrom.
//
// With no channel chosen, a beta installation followed the stable channel —
// where nothing is published while there are only betas — and was told it was
// up to date with the next beta already out. A choice still wins either way.
func TestAnInstallationFollowsTheChannelItCameFrom(t *testing.T) {
	cases := []struct {
		channel, current string
		want             bool
	}{
		{"", "v1.0.0-Beta.03", true},
		{"", "v1.0.0", false},
		{"", "dev", false},
		{"stable", "v1.0.0-Beta.03", false},
		{"beta", "v1.0.0", true},
	}
	for _, c := range cases {
		if got := followsBeta(c.channel, c.current); got != c.want {
			t.Errorf("followsBeta(%q, %q) = %v, want %v", c.channel, c.current, got, c.want)
		}
	}
}
