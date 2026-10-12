package web

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"
	"testing"
)

func aPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		img.Set(x, x, color.RGBA{R: 232, G: 82, B: 14, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestATeamPictureShowsBesideTheirReplyAndNowhereElse.
//
// A reply on a public page used to carry only a name, so the page drew initials for
// everybody. The picture a teammate chose is shown beside their replies, as an
// address — and the address answers for the people whose name is already on a page
// and nobody else, so it cannot be used to count accounts or to read a picture that
// is not a plain raster.
func TestATeamPictureShowsBesideTheirReplyAndNowhereElse(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	published, _ := team.anonymous().submit("codigosh", "lastboard", "idea", "dark mode")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/moderation", map[string]string{"state": "published"}))

	// Nothing chosen yet: a reply carries no picture, and the page draws the default.
	_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/reply", map[string]string{"body": "no picture yet"}))
	thread := func() []map[string]any {
		raw, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/items/"+published, nil))["comments"].([]any)
		out := make([]map[string]any, 0, len(raw))
		for _, c := range raw {
			out = append(out, c.(map[string]any))
		}
		return out
	}
	if _, has := thread()[0]["avatar"]; has {
		t.Errorf("a reply from an account with no picture carries one: %v", thread()[0])
	}

	// Chosen: the reply carries the address, and the address serves the picture.
	if got := status(t, team.do(http.MethodPost, "/api/me/avatar", map[string]string{"avatar": aPNG(t)})); got != http.StatusOK {
		t.Fatalf("choosing a picture: %d", got)
	}
	address, _ := thread()[0]["avatar"].(string)
	if len(address) < len("/avatars/1") || address[:9] != "/avatars/" {
		t.Fatalf("a reply from an account with a picture carries %q, want /avatars/<id>", address)
	}
	resp := team.anonymous().do(http.MethodGet, address, nil)
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("the picture answered %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if _, err := png.Decode(bytes.NewReader(body.Bytes())); err != nil {
		t.Errorf("what was served is not the picture: %v", err)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" ||
		resp.Header.Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Errorf("a picture opened on its own is not locked down: %v", resp.Header)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag, so nothing can ask whether it changed")
	}
	req, _ := http.NewRequest(http.MethodGet, team.server.URL+address, nil)
	req.Header.Set("If-None-Match", etag)
	again, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = again.Body.Close()
	if again.StatusCode != http.StatusNotModified {
		t.Errorf("asking again with the ETag answered %d, want 304", again.StatusCode)
	}

	// A visitor's message never carries one.
	for _, c := range thread() {
		if c["author"] != "team" {
			if _, has := c["avatar"]; has {
				t.Errorf("a message from a stranger carries a picture: %v", c)
			}
		}
	}

	// An account that has written no reply is not served, whatever its picture: the
	// address must not tell who has an account.
	if got := status(t, team.do(http.MethodPost, "/api/people", map[string]string{
		"username": "quiet", "password": "a good long passphrase",
	})); got != http.StatusCreated {
		t.Fatalf("creating an account: %d", got)
	}
	if _, err := team.db().Exec(`UPDATE app_user SET avatar = ? WHERE username = 'quiet'`, aPNG(t)); err != nil {
		t.Fatal(err)
	}
	var quiet int64
	if err := team.db().QueryRow(`SELECT id FROM app_user WHERE username = 'quiet'`).Scan(&quiet); err != nil {
		t.Fatal(err)
	}
	if got := status(t, team.anonymous().do(http.MethodGet, "/avatars/"+strconv.FormatInt(quiet, 10), nil)); got != http.StatusNotFound {
		t.Errorf("the picture of an account with no reply answered %d, want 404", got)
	}
	for _, path := range []string{"/avatars/0", "/avatars/-1", "/avatars/abc", "/avatars/999999"} {
		if got := status(t, team.anonymous().do(http.MethodGet, path, nil)); got != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", path, got)
		}
	}

	// Not a plain raster: a vector picture, and bytes that are not a picture at all
	// whatever the header claims, are never served on their own address.
	for name, stored := range map[string]string{
		"a vector picture":   "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)),
		"html with a header": "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<html><script>alert(1)</script></html>")),
		"not base64":         "data:image/png;base64,@@@@",
		"no payload":         "data:image/png;base64,",
	} {
		if _, err := team.db().Exec(`UPDATE app_user SET avatar = ? WHERE username = ?`, stored, "owner"); err != nil {
			t.Fatal(err)
		}
		if got := status(t, team.anonymous().do(http.MethodGet, address, nil)); got != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", name, got)
		}
	}
}
