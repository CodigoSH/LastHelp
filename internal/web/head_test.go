package web

import (
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/landing"
)

// TestASharedLinkShowsThePictureAtAnAddressAnyoneCanFetch: an og:image is
// fetched by somebody else's server, so it is absolute — and without a public
// address configured there is nothing trustworthy to make it absolute with.
func TestASharedLinkShowsThePictureAtAnAddressAnyoneCanFetch(t *testing.T) {
	image := landing.ImagePrefix + strings.Repeat("a", 32) + ".webp"
	set := landing.Settings{ShareImage: image}

	got := headTags(set, "Acme", "https://help.example.com/")
	want := `<meta property="og:image" content="https://help.example.com` + image + `" />`
	if !strings.Contains(got, want) || !strings.Contains(got, "summary_large_image") {
		t.Errorf("with a public address, the head is:\n%s", got)
	}
	if got := headTags(set, "Acme", ""); strings.Contains(got, "og:image") {
		t.Errorf("without a public address a picture was announced anyway:\n%s", got)
	}
	if got := headTags(landing.Settings{}, "Acme", "https://help.example.com"); strings.Contains(got, "og:image") {
		t.Errorf("a picture was announced with none chosen:\n%s", got)
	}
}
