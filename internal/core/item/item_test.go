package item

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var epoch = time.Unix(1_700_000_000, 0)

func store(t *testing.T) Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`INSERT INTO organization (id, slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES (1, 'codigosh', 'CodigoSH', 'forgejo', '', '', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at)
		VALUES (1, 1, 'lastboard', 'Lastboard', '', 0)`); err != nil {
		t.Fatal(err)
	}
	return Store{DB: db}
}

func report(t *testing.T, s Store, kind Kind, title, body string) (Item, string) {
	t.Helper()
	it, token, err := s.Create(New{RepoID: 1, Kind: kind, Title: title, Body: body}, Pending, epoch)
	if err != nil {
		t.Fatal(err)
	}
	return it, token
}

func TestReportingWithoutAnAccount(t *testing.T) {
	s := store(t)

	it, token, err := s.Create(New{
		RepoID: 1, Kind: Bug, Title: "crash on import", Body: "it closes",
		Fields: []Field{{Key: "version", Label: "Version", Value: "1.7.4"}},
	}, Pending, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if it.PublicID == "" || token == "" {
		t.Fatal("no public identifier or follow link was produced")
	}
	if it.Moderation != Pending || it.Status != StatusNew {
		t.Errorf("a new report is %s/%s", it.Moderation, it.Status)
	}

	loaded, err := s.ByPublicID(it.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Fields) != 1 || loaded.Fields[0].Label != "Version" {
		t.Errorf("the answers did not come back: %+v", loaded.Fields)
	}
}

// TestTheFollowLinkIsStoredHashed.
//
// It is the one credential an anonymous reporter has. A leaked database backup
// must not be a list of links that let somebody reply as any of them.
func TestTheFollowLinkIsStoredHashed(t *testing.T) {
	s := store(t)
	it, token := report(t, s, Idea, "dark mode", "")

	var stored string
	if err := s.DB.QueryRow(`SELECT follow_token_hash FROM item WHERE id = ?`, it.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token {
		t.Fatal("the follow token itself is in the database")
	}

	if _, err := s.CheckFollowToken(it.PublicID, token); err != nil {
		t.Fatalf("the real follow link was refused: %v", err)
	}
	if _, err := s.CheckFollowToken(it.PublicID, "not-the-token"); !errors.Is(err, ErrNotFound) {
		t.Error("a wrong follow link was accepted")
	}
}

// TestOnlyIdeasAreVoted is one of the two rules that separate the journeys.
//
// Two hundred people agreeing that something crashes does not make it crash
// harder. What a bug needs is somebody able to reproduce it, and a vote count
// on one invites treating popularity as severity.
func TestOnlyIdeasAreVoted(t *testing.T) {
	s := store(t)
	bug, _ := report(t, s, Bug, "crash", "")
	idea, _ := report(t, s, Idea, "dark mode", "")

	if err := s.Vote(bug.ID, "voter-1", epoch); !errors.Is(err, ErrNotVotable) {
		t.Errorf("voting a bug gave %v, want ErrNotVotable", err)
	}
	if err := s.Vote(idea.ID, "voter-1", epoch); err != nil {
		t.Fatalf("voting an idea failed: %v", err)
	}
}

func TestVotingTwiceCountsOnce(t *testing.T) {
	s := store(t)
	idea, _ := report(t, s, Idea, "dark mode", "")

	if err := s.Vote(idea.ID, "voter-1", epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.Vote(idea.ID, "voter-1", epoch); !errors.Is(err, ErrAlreadyVoted) {
		t.Errorf("the second vote gave %v", err)
	}

	loaded, err := s.ByID(idea.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VoteCount != 1 {
		t.Errorf("count is %d after voting twice from one voter", loaded.VoteCount)
	}
}

func TestTakingAVoteBackNeverGoesBelowZero(t *testing.T) {
	s := store(t)
	idea, _ := report(t, s, Idea, "dark mode", "")

	if err := s.Vote(idea.ID, "voter-1", epoch); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Unvote(idea.ID, "voter-1"); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := s.ByID(idea.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VoteCount != 0 {
		t.Errorf("count is %d; a count that can go negative is a count that will", loaded.VoteCount)
	}
}

// TestNothingWaitingIsPublic is the moderation queue, asserted from the side
// the public sees.
func TestNothingWaitingIsPublic(t *testing.T) {
	s := store(t)
	waiting, _ := report(t, s, Idea, "waiting", "")
	shown, _ := report(t, s, Idea, "shown", "")
	if err := s.Approve(shown.ID, epoch); err != nil {
		t.Fatal(err)
	}

	public, err := s.List(Filter{RepoID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].ID != shown.ID {
		t.Fatalf("the public list has %d items; %v is waiting for a human", len(public), waiting.PublicID)
	}

	queue, err := s.List(Filter{RepoID: 1, Moderation: Pending})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != waiting.ID {
		t.Errorf("the queue has %d items", len(queue))
	}
}

// TestAStatusOutsideItsKindIsRefused.
//
// A bug is never "planned". A status nobody wrote a screen for renders as
// blank, which reads as broken.
func TestAStatusOutsideItsKindIsRefused(t *testing.T) {
	s := store(t)
	bug, _ := report(t, s, Bug, "crash", "")
	idea, _ := report(t, s, Idea, "dark mode", "")

	if err := s.SetStatus(bug.ID, StatusPlanned, epoch); err == nil {
		t.Error("a bug was marked planned")
	}
	if err := s.SetStatus(idea.ID, StatusNotReproducible, epoch); err == nil {
		t.Error("an idea was marked not reproducible")
	}
	if err := s.SetStatus(bug.ID, StatusConfirmed, epoch); err != nil {
		t.Errorf("a bug could not be confirmed: %v", err)
	}
}

// TestMergingMovesVotesWithoutDoubleCounting.
//
// Somebody who voted both must count once afterwards. Adding the two counts
// would reward splitting an idea in two.
func TestMergingMovesVotesWithoutDoubleCounting(t *testing.T) {
	s := store(t)
	survivor, _ := report(t, s, Idea, "dark mode", "")
	duplicate, _ := report(t, s, Idea, "night theme", "")

	for _, voter := range []string{"a", "b"} {
		if err := s.Vote(survivor.ID, voter, epoch); err != nil {
			t.Fatal(err)
		}
	}
	for _, voter := range []string{"b", "c"} { // b voted both
		if err := s.Vote(duplicate.ID, voter, epoch); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Merge(duplicate.ID, survivor.ID, epoch); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.ByID(survivor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VoteCount != 3 {
		t.Errorf("the survivor has %d votes, want 3 (a, b, c — b voted both and counts once)", loaded.VoteCount)
	}

	// And the duplicate is out of every listing, including the rejected pile:
	// it is not a rejection to review, it is an address that now points
	// somewhere else.
	for _, state := range []Moderation{"", Pending, Rejected, Spam} {
		list, err := s.List(Filter{RepoID: 1, Moderation: state})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range list {
			if it.ID == duplicate.ID {
				t.Errorf("the merged duplicate still appears in the %q listing", state)
			}
		}
	}

	// But it is still there, and it knows where it went — which is what lets
	// its old link redirect instead of 404.
	gone, err := s.ByID(duplicate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gone.MergedInto != survivor.ID {
		t.Errorf("the duplicate points at %d, want the survivor %d", gone.MergedInto, survivor.ID)
	}
}

// TestSimilarFindsTheThingSomebodyIsAboutToDuplicate.
func TestSimilarFindsTheThingSomebodyIsAboutToDuplicate(t *testing.T) {
	s := store(t)
	existing, _ := report(t, s, Idea, "dark mode for the dashboard", "a theme that is not white")
	if err := s.Approve(existing.ID, epoch); err != nil {
		t.Fatal(err)
	}
	other, _ := report(t, s, Idea, "export to CSV", "")
	if err := s.Approve(other.ID, epoch); err != nil {
		t.Fatal(err)
	}

	found, err := s.Similar(1, "dark mode", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 || found[0].ID != existing.ID {
		t.Fatalf("looking for 'dark mode' found %d items, first %v", len(found), found)
	}
}

// TestSearchSurvivesWhatPeopleActuallyType.
//
// FTS5 has a query language: NEAR, OR, quotes, asterisks. A title containing
// one of those would be a syntax error rather than a search, and the parameter
// placeholder does not help because the parameter *is* the query.
func TestSearchSurvivesWhatPeopleActuallyType(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash when I press OR and NEAR", "")
	if err := s.Approve(it.ID, epoch); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{
		`OR`, `NEAR`, `"unbalanced`, `*`, `AND OR NOT`, `a"b"c`, `(`, `^`, `-`,
	} {
		if _, err := s.List(Filter{RepoID: 1, Search: query}); err != nil {
			t.Errorf("searching for %q failed: %v", query, err)
		}
	}
}

func TestListingsAreBounded(t *testing.T) {
	s := store(t)
	for i := 0; i < 60; i++ {
		it, _ := report(t, s, Idea, "idea", "")
		if err := s.Approve(it.ID, epoch); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(Filter{RepoID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 50 {
		t.Errorf("an unasked-for listing returned %d rows; every listing is bounded", len(list))
	}
	if _, err := s.List(Filter{RepoID: 1, Limit: 100_000}); err != nil {
		t.Fatal(err)
	}
}

// TestTheSameTextTwiceHashesTheSame is what makes a copy-paste flood a lookup.
func TestTheSameTextTwiceHashesTheSame(t *testing.T) {
	a := BodyHash("Buy   cheap WATCHES", "visit\nour site")
	b := BodyHash("buy cheap watches", "visit our site")
	if a != b {
		t.Error("the same text with different case and wrapping hashed differently")
	}
	if a == BodyHash("something else", "") {
		t.Error("different text hashed the same")
	}
}

func TestTheThreadWaitsForAHumanExceptForTheTeam(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Idea, "dark mode", "")

	fromTeam, err := s.Comment(it.ID, AuthorTeam, "alex", "looking at it", epoch)
	if err != nil {
		t.Fatal(err)
	}
	if fromTeam.Moderation != Published {
		t.Error("the team's own reply is waiting in a queue for the team")
	}

	// The reporter proved who they are with the secret link. That proves
	// identity, not that what they wrote belongs on a public page.
	fromReporter, err := s.Comment(it.ID, AuthorReporter, "", "any news?", epoch)
	if err != nil {
		t.Fatal(err)
	}
	if fromReporter.Moderation != Pending {
		t.Error("a reporter's follow-up was published without review")
	}

	published, err := s.Comments(it.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 {
		t.Errorf("%d comments are public, want only the team's", len(published))
	}
}

func TestOverlongInputIsRefused(t *testing.T) {
	s := store(t)
	if _, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: strings.Repeat("x", maxTitle+1)}, Pending, epoch); err == nil {
		t.Error("an overlong title was accepted")
	}
	if _, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: "ok", Body: strings.Repeat("x", maxBody+1)}, Pending, epoch); err == nil {
		t.Error("an overlong body was accepted")
	}
	if _, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: "   "}, Pending, epoch); err == nil {
		t.Error("a blank title was accepted")
	}
}

func TestAReportForNoRepositoryIsRefused(t *testing.T) {
	s := store(t)
	if _, _, err := s.Create(New{RepoID: 999, Kind: Bug, Title: "ghost"}, Pending, epoch); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// TestListingSeveralBoardsStillHidesWhatIsWaiting.
//
// The panel's own listing crosses boards, which meant a second way into this
// query. The dangerous half is not the join: it is that "every state" and "the
// public list" are one boolean apart, and the zero value has to be the safe
// one. A caller that asks for four boards and forgets to think about moderation
// must get what the public gets.
func TestListingSeveralBoardsStillHidesWhatIsWaiting(t *testing.T) {
	s := store(t)
	if _, err := s.DB.Exec(`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at)
		VALUES (2, 1, 'lastdav', 'LastDAV', '', 0)`); err != nil {
		t.Fatal(err)
	}

	shown, _ := report(t, s, Idea, "published here", "")
	if err := s.Approve(shown.ID, epoch); err != nil {
		t.Fatal(err)
	}
	waiting, _, err := s.Create(New{RepoID: 2, Kind: Bug, Title: "waiting there", Body: ""}, Pending, epoch)
	if err != nil {
		t.Fatal(err)
	}

	both := []int64{1, 2}

	public, err := s.List(Filter{RepoIDs: both})
	if err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].ID != shown.ID {
		t.Fatalf("listing two boards returned %d items; %s is still waiting for a human",
			len(public), waiting.PublicID)
	}

	everything, err := s.List(Filter{RepoIDs: both, AnyModeration: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(everything) != 2 {
		t.Errorf("asking for every state across both boards returned %d items, want 2", len(everything))
	}

	// And no board means no rows rather than every row: a caller that forgets
	// to say which boards must not be handed the whole installation.
	none, err := s.List(Filter{AnyModeration: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("a listing with no board returned %d items", len(none))
	}
}

// TestCountsGroupWhatTheListingWouldShow.
//
// The numbers on the tabs come from here rather than from counting the rows on
// screen, which is a page that says "3 waiting" because three fit on the first
// page.
func TestCountsGroupWhatTheListingWouldShow(t *testing.T) {
	s := store(t)
	first, _ := report(t, s, Idea, "one", "")
	report(t, s, Bug, "two", "")
	if err := s.Approve(first.ID, epoch); err != nil {
		t.Fatal(err)
	}

	byState, err := s.Counts([]int64{1}, "moderation_state")
	if err != nil {
		t.Fatal(err)
	}
	if byState["published"] != 1 || byState["pending"] != 1 {
		t.Errorf("counted %v", byState)
	}

	byKind, err := s.Counts([]int64{1}, "kind")
	if err != nil {
		t.Fatal(err)
	}
	if byKind["idea"] != 1 || byKind["bug"] != 1 {
		t.Errorf("counted %v", byKind)
	}

	// A column nobody wrote a screen for is refused rather than concatenated
	// into the query: the column name is the one thing a parameter cannot carry.
	// Refused by the whitelist, not by SQLite's parser further down: a syntax
	// error is what a badly built query happens to produce today, and asserting
	// on "some error" would go on passing with the whitelist deleted.
	_, err = s.Counts([]int64{1}, "title FROM item WHERE 1=1 --")
	if err == nil || !strings.Contains(err.Error(), "is not something this counts by") {
		t.Errorf("counting by an arbitrary column was refused with %v", err)
	}
}
