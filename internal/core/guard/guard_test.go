package guard

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var epoch = time.Unix(1_700_000_000, 0)

func guard(t *testing.T) Guard {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`INSERT INTO organization (id, slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES (1, 'o', 'O', 'none', '', '', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at)
		VALUES (1, 1, 'r', 'R', '', 0)`); err != nil {
		t.Fatal(err)
	}
	return Guard{DB: db, Key: []byte("a key for the tests only")}
}

// TestAPostStraightAtTheEndpointIsRefused.
//
// The bulk of automated submission never loads the page: it finds the endpoint
// and posts at it. This is the layer that costs that nothing to write and
// everything to work around.
func TestAPostStraightAtTheEndpointIsRefused(t *testing.T) {
	g := guard(t)
	for _, token := range []string{"", "nonsense", "1.2.3.4", "1.1700000000.abcd.invalidsig"} {
		if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch); err == nil {
			t.Errorf("token %q was accepted", token)
		}
	}
}

func TestAGenuineFormWorks(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)
	if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch.Add(10*time.Second)); err != nil {
		t.Fatalf("a form fetched and filled in normally was refused: %v", err)
	}
}

// TestATokenIsSpentOnce.
//
// Without this, one fetch of the form is a licence to submit for ever. With it,
// a thousand submissions cost a thousand page loads.
func TestATokenIsSpentOnce(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)

	if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch.Add(20*time.Second)); !errors.Is(err, ErrUsed) {
		t.Errorf("the same token submitted twice gave %v", err)
	}
}

func TestATokenForOneBoardCannotSubmitToAnother(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)
	if err := g.CheckToken(token, 2, "192.0.2.0/24", epoch.Add(10*time.Second)); err == nil {
		t.Error("a token minted for one board submitted to another")
	}
}

func TestFasterThanAPersonTypesIsRefused(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)
	if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch.Add(200*time.Millisecond)); !errors.Is(err, ErrTooFast) {
		t.Errorf("a form filled in in 200ms gave %v", err)
	}
}

func TestAnOldTokenIsRefused(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)
	if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch.Add(3*time.Hour)); !errors.Is(err, ErrStale) {
		t.Errorf("a three-hour-old token gave %v", err)
	}
}

func TestATokenFromElsewhereIsRefused(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)
	if err := g.CheckToken(token, 1, "203.0.113.0/24", epoch.Add(10*time.Second)); !errors.Is(err, ErrStale) {
		t.Errorf("a token minted for one part of the internet was accepted from another: %v", err)
	}
}

// TestPurgeKeepsTheTableFromGrowingForEver.
func TestPurgeKeepsTheTableFromGrowingForEver(t *testing.T) {
	g := guard(t)
	token := g.IssueToken(1, "192.0.2.0/24", epoch)
	if err := g.CheckToken(token, 1, "192.0.2.0/24", epoch.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := g.PurgeTokens(epoch.Add(3 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := g.DB.QueryRow(`SELECT count(*) FROM form_token`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d spent tokens survived their expiry", left)
	}
}

// TestTheSameParagraphAgainIsCaught is the copy-paste flood.
func TestTheSameParagraphAgainIsCaught(t *testing.T) {
	g := guard(t)
	seed(t, g, "Buy cheap watches", "visit our site", epoch)

	verdict, err := g.Inspect(1, "buy   CHEAP watches", "visit\nour site", nil, epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if verdict.OK {
		t.Error("the same text with different case and wrapping was not recognised")
	}
	if verdict.Layer != "duplicate" {
		t.Errorf("refused by %q", verdict.Layer)
	}
}

func TestSomethingGenuineGetsThrough(t *testing.T) {
	g := guard(t)
	verdict, err := g.Inspect(1, "crash on import",
		"it closes when I import photos, log at https://paste.example.com/abc", nil, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.OK {
		t.Errorf("a real report with one link was refused by %q", verdict.Layer)
	}
}

func TestAnAdvertIsCaught(t *testing.T) {
	g := guard(t)
	body := strings.Repeat("visit https://example.com now ", 6)
	verdict, err := g.Inspect(1, "great deals", body, nil, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.OK || verdict.Layer != "links" {
		t.Errorf("six links got through, refused by %q", verdict.Layer)
	}
}

func TestTheOperatorsOwnWordList(t *testing.T) {
	g := guard(t)
	verdict, err := g.Inspect(1, "cheap CRYPTO offers", "", []string{"crypto", "casino"}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.OK || verdict.Layer != "stoplist" {
		t.Errorf("a listed word got through, refused by %q", verdict.Layer)
	}

	// And a board where that word is ordinary traffic keeps working.
	verdict, err = g.Inspect(1, "cheap CRYPTO offers", "", nil, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.OK {
		t.Error("a word nobody listed was refused anyway")
	}
}

func TestTheHoneypot(t *testing.T) {
	if err := Honeypot(""); err != nil {
		t.Errorf("an empty honeypot was treated as filled: %v", err)
	}
	if err := Honeypot("  "); err != nil {
		t.Errorf("whitespace counted as filled: %v", err)
	}
	if err := Honeypot("http://spam"); !errors.Is(err, ErrHoneypot) {
		t.Error("a filled honeypot got through")
	}
}

// TestTheBreakerTripsBeforeTheQueueIsUnreadable.
//
// This is the layer that protects attention rather than data: past a point,
// refusing everything for a while is better than handing a person a queue they
// will not open.
func TestTheBreakerTripsBeforeTheQueueIsUnreadable(t *testing.T) {
	g := guard(t)

	ok, err := g.Accepting(1, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("a quiet board is refusing submissions")
	}

	for i := 0; i < HourlyLimit; i++ {
		seed(t, g, "flood", strings.Repeat("x", i+1), epoch)
	}

	ok, err = g.Accepting(1, epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("still accepting after %d submissions in an hour", HourlyLimit)
	}

	// And it recovers on its own: an hour later the count has moved on.
	ok, err = g.Accepting(1, epoch.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("the breaker never resets; a single flood would close a board for ever")
	}
}

func seed(t *testing.T, g Guard, title, body string, at time.Time) {
	t.Helper()
	hash := item.BodyHash(title, body)
	if _, err := g.DB.Exec(`INSERT INTO item
		(public_id, repo_id, kind, title, body, status, moderation_state, body_hash, follow_token_hash, created_at)
		VALUES (?, 1, 'bug', ?, ?, 'new', 'pending', ?, 'x', ?)`,
		title+body, title, body, hash, at.Unix()); err != nil {
		t.Fatal(err)
	}
}
