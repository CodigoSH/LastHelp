package web

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"
)

func fetchFeed(t *testing.T, i *instance, path string) (*http.Response, string) {
	t.Helper()
	resp, err := i.client.Get(i.server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

// TestTheFeedIsTheChannelThatNeedsNoConfiguration.
//
// No address, no account, nothing stored about whoever subscribes. It is the
// reason the 1.0 does not need SMTP to be useful.
func TestTheFeedIsValidAtomAndPublic(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	stranger := team.anonymous()
	resp, body := fetchFeed(t, stranger, "/feed/codigosh/lastboard/atom.xml")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the feed answered %d to somebody with no session", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/atom+xml") {
		t.Errorf("content type is %q; a reader decides what this is by that header", got)
	}

	// Parsed, not string-matched: a feed that does not parse is a feed no
	// reader will accept, and every field being present says nothing about that.
	var parsed struct {
		Title   string `xml:"title"`
		ID      string `xml:"id"`
		Updated string `xml:"updated"`
		Entries []struct {
			Title   string `xml:"title"`
			ID      string `xml:"id"`
			Updated string `xml:"updated"`
			Link    struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("the feed is not valid XML: %v\n%s", err, body)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("the feed has %d entries, want 1", len(parsed.Entries))
	}
	if parsed.Entries[0].Title != "dark mode" {
		t.Errorf("entry title is %q", parsed.Entries[0].Title)
	}
	if parsed.Entries[0].Updated == "" || parsed.Updated == "" {
		t.Error("a timestamp is missing; some readers refuse a feed without one")
	}
	if !strings.Contains(parsed.Entries[0].ID, id) {
		t.Errorf("the entry id %q does not identify the item", parsed.Entries[0].ID)
	}
}

// TestTheFeedNeverCarriesWhatNobodyHasRead.
//
// It goes through the same listing the public page uses, so it cannot drift
// into publishing something the page would not — but that is a claim, and this
// is the claim checked.
func TestTheFeedNeverCarriesWhatNobodyHasRead(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	public.submit("codigosh", "lastboard", "idea", "still in the queue")

	_, body := fetchFeed(t, public, "/feed/codigosh/lastboard/atom.xml")
	if strings.Contains(body, "still in the queue") {
		t.Error("something waiting for a human is in the public feed")
	}
}

// TestAnEmptyBoardStillProducesAValidFeed.
//
// A feed with a malformed or missing `updated` is one some readers refuse
// outright, and an empty board is the state every new instance is in.
func TestAnEmptyBoardStillProducesAValidFeed(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	resp, body := fetchFeed(t, team.anonymous(), "/feed/codigosh/lastboard/atom.xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("an empty board's feed answered %d", resp.StatusCode)
	}

	var parsed struct {
		Updated string `xml:"updated"`
	}
	if err := xml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("an empty feed does not parse: %v", err)
	}
	if parsed.Updated == "" {
		t.Error("an empty feed has no updated timestamp")
	}
}

// TestWhatSomebodyWroteIsNotHTMLInTheFeed.
//
// A feed reader renders what it is given. The body was written by a stranger,
// so it goes out as text with the type saying so.
func TestWhatSomebodyWroteIsNotHTMLInTheFeed(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	base := "/api/orgs/codigosh/repos/lastboard"
	form := decode(t, public.do(http.MethodGet, base+"/form", nil))
	public.tick.advance(5_000_000_000) // five seconds

	resp := public.do(http.MethodPost, base+"/items", map[string]any{
		"kind": "idea", "title": "a title",
		"body":  `<script>alert("hello")</script>`,
		"token": form["token"],
	})
	created := decode(t, resp)
	id, _ := created["public_id"].(string)

	resp = team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	_, body := fetchFeed(t, public, "/feed/codigosh/lastboard/atom.xml")

	if strings.Contains(body, "<script>") {
		t.Error("a script tag went into the feed unescaped")
	}
	if !strings.Contains(body, `type="text"`) {
		t.Error("the summary is not declared as text, so a reader may render it as markup")
	}
}

func TestAFeedForNothingIs404(t *testing.T) {
	team := newInstance(t)
	team.claim()

	resp, _ := fetchFeed(t, team.anonymous(), "/feed/nobody/nothing/atom.xml")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a feed for a board that does not exist answered %d", resp.StatusCode)
	}
}
