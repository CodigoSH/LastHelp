package item

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

// TestAnAnsweredIdeaStopsTakingVotes.
//
// Once the team has said done, declined or closed, the count is a record of how
// many people wanted it when it was answered. If it kept moving it would stop
// being one. Taking a vote back is closed too, or the number the team answered
// to could still be edited afterwards.
func TestAnAnsweredIdeaStopsTakingVotes(t *testing.T) {
	for _, answer := range []Status{StatusDone, StatusDeclined} {
		t.Run(string(answer), func(t *testing.T) {
			s := store(t)
			idea, _ := report(t, s, Idea, "dark mode", "")
			if err := s.Vote(idea.ID, "voter-1", epoch); err != nil {
				t.Fatal(err)
			}
			if err := s.SetStatus(idea.ID, answer, epoch); err != nil {
				t.Fatal(err)
			}

			if err := s.Vote(idea.ID, "voter-2", epoch); !errors.Is(err, ErrVotingClosed) {
				t.Errorf("voting an answered idea gave %v, want ErrVotingClosed", err)
			}
			if err := s.Unvote(idea.ID, "voter-1"); !errors.Is(err, ErrVotingClosed) {
				t.Errorf("taking a vote back from an answered idea gave %v, want ErrVotingClosed", err)
			}
			loaded, err := s.ByID(idea.ID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.VoteCount != 1 {
				t.Errorf("the count moved to %d after the idea was answered", loaded.VoteCount)
			}
		})
	}
}

// TestAReopenedIdeaTakesVotesAgain: whether voting is open is read from the
// status each time, not remembered, so reopening needs nothing else.
func TestAReopenedIdeaTakesVotesAgain(t *testing.T) {
	s := store(t)
	idea, _ := report(t, s, Idea, "dark mode", "")
	if err := s.SetStatus(idea.ID, StatusDeclined, epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus(idea.ID, StatusPlanned, epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.Vote(idea.ID, "voter-1", epoch); err != nil {
		t.Errorf("a reopened idea refused a vote: %v", err)
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

// TestClosedSplitsBothVocabulariesWithoutGapOrOverlap.
//
// "Closed" is one decision, used by the status that stamps closed_at and by
// the public list's open and closed tabs. Every status of both kinds is on
// exactly one side, and the closed side is what has an answer.
func TestClosedSplitsBothVocabulariesWithoutGapOrOverlap(t *testing.T) {
	side := map[Status]string{}
	for _, st := range OpenStatuses() {
		side[st] += "open"
	}
	for _, st := range ClosedStatuses() {
		side[st] += "closed"
	}
	for _, k := range []Kind{Bug, Idea} {
		for _, st := range StatusesFor(k) {
			if side[st] != "open" && side[st] != "closed" {
				t.Errorf("%s is on %q, want exactly one side", st, side[st])
			}
		}
	}
	for _, st := range []Status{StatusDone, StatusDeclined, StatusClosed, StatusNotReproducible} {
		if !st.Closes() {
			t.Errorf("%s is an answer and does not close", st)
		}
	}
	for _, st := range []Status{StatusNew, StatusPlanned, StatusInProgress, StatusConfirmed, StatusInForge} {
		if st.Closes() {
			t.Errorf("%s still waits and closes", st)
		}
	}
}

// TestTallyCountsWhatListWouldShowWithoutItsLimit.
func TestTallyCountsWhatListWouldShowWithoutItsLimit(t *testing.T) {
	s := store(t)
	for i := 0; i < 7; i++ {
		it, _ := report(t, s, Idea, "an idea worth having "+strings.Repeat("x", i), "")
		if err := s.Approve(it.ID, epoch); err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			if err := s.SetStatus(it.ID, StatusDone, epoch); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, f := range map[string]Filter{
		"all":    {RepoID: 1},
		"closed": {RepoID: 1, Statuses: ClosedStatuses()},
		"open":   {RepoID: 1, Statuses: OpenStatuses()},
	} {
		n, err := s.Tally(f)
		if err != nil {
			t.Fatal(err)
		}
		f.Limit = 200
		list, err := s.List(f)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(list) {
			t.Errorf("%s: tally says %d, the list has %d", name, n, len(list))
		}
		f.Limit = 2
		if short, _ := s.List(f); len(short) > 2 {
			t.Errorf("%s: the limit did not hold", name)
		}
	}
	if n, _ := s.Tally(Filter{RepoID: 1, Statuses: ClosedStatuses()}); n != 3 {
		t.Errorf("closed: %d, want 3", n)
	}
}

// TestAnAnsweredReportTakesCommentsOnlyFromWhoAnsweredFor: a visitor's comment is
// refused at once; the reporter and the team keep writing for GraceAfterClosing,
// and then the conversation closes for everybody. It is the same for an idea and
// for a bug, and for each way of answering.
func TestAnAnsweredReportTakesCommentsOnlyFromWhoAnsweredFor(t *testing.T) {
	for _, tc := range []struct {
		kind   Kind
		answer Status
	}{
		{Idea, StatusDone}, {Idea, StatusDeclined},
		{Bug, StatusClosed}, {Bug, StatusNotReproducible},
	} {
		t.Run(string(tc.kind)+"/"+string(tc.answer), func(t *testing.T) {
			s := store(t)
			it, _ := report(t, s, tc.kind, "something", "")
			if _, err := s.Comment(it.ID, AuthorVisitor, "", "before", epoch); err != nil {
				t.Fatalf("an open report refused a comment: %v", err)
			}
			if err := s.SetStatus(it.ID, tc.answer, epoch); err != nil {
				t.Fatal(err)
			}

			// A visitor, never. The others, inside the window.
			if _, err := s.Comment(it.ID, AuthorVisitor, "", "after", epoch); !errors.Is(err, ErrConversationClosed) {
				t.Errorf("a visitor writing on an answered report gave %v, want ErrConversationClosed", err)
			}
			inside := epoch.Add(GraceAfterClosing - time.Second)
			for _, author := range []Author{AuthorReporter, AuthorTeam} {
				if _, err := s.Comment(it.ID, author, "x", "within the window", inside); err != nil {
					t.Errorf("the %s was refused inside the window: %v", author, err)
				}
			}
			// Outside it, nobody.
			for _, at := range []time.Time{epoch.Add(GraceAfterClosing), epoch.Add(GraceAfterClosing + time.Hour)} {
				for _, author := range []Author{AuthorVisitor, AuthorReporter, AuthorTeam} {
					if _, err := s.Comment(it.ID, author, "x", "late", at); !errors.Is(err, ErrConversationClosed) {
						t.Errorf("%s writing at +%s gave %v, want ErrConversationClosed", author, at.Sub(epoch), err)
					}
				}
			}
			all, err := s.Comments(it.ID, Pending)
			if err != nil {
				t.Fatal(err)
			}
			// The one from before and the reporter's from inside the window; the
			// team's is published and does not wait.
			if len(all) != 2 {
				t.Errorf("%d comments waiting, want 2: the one from before and the reporter's", len(all))
			}
		})
	}
}

// TestTheWindowIsCountedFromTheLastAnswer: answering again starts it again, and
// a report closed with no time on record has no window at all.
func TestTheWindowIsCountedFromTheLastAnswer(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	if err := s.SetStatus(it.ID, StatusClosed, epoch); err != nil {
		t.Fatal(err)
	}
	later := epoch.Add(40 * time.Hour)
	if err := s.SetStatus(it.ID, StatusNotReproducible, later); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Comment(it.ID, AuthorReporter, "", "still here", later.Add(40*time.Hour)); err != nil {
		t.Errorf("the window was not counted from the last answer: %v", err)
	}

	if _, err := s.DB.Exec(`UPDATE item SET closed_at = NULL WHERE id = ?`, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Comment(it.ID, AuthorTeam, "x", "no record", later); !errors.Is(err, ErrConversationClosed) {
		t.Errorf("an answered report with no closing time gave %v, want ErrConversationClosed", err)
	}
}

func TestAReopenedReportTakesCommentsAgain(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	if err := s.SetStatus(it.ID, StatusClosed, epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus(it.ID, StatusConfirmed, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Comment(it.ID, AuthorTeam, "x", "back", epoch); err != nil {
		t.Errorf("a reopened report refused a reply: %v", err)
	}
}

// TestNothingIsAddedToAReportThatIsNotOnAPage: a duplicate folded into another,
// or a report turned down, is on no public page. A vote or a comment given to it
// would be accepted and lost, which reads as having worked.
func TestNothingIsAddedToAReportThatIsNotOnAPage(t *testing.T) {
	s := store(t)
	rejected, _ := report(t, s, Idea, "a", "")
	survivor, _ := report(t, s, Idea, "b", "")
	duplicate, _ := report(t, s, Idea, "c", "")
	if err := s.Reject(rejected.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.Merge(duplicate.ID, survivor.ID, epoch); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"rejected": rejected.ID, "merged": duplicate.ID} {
		if err := s.Vote(id, "voter-1", epoch); !errors.Is(err, ErrNotFound) {
			t.Errorf("a vote on a %s report gave %v, want ErrNotFound", name, err)
		}
		if _, err := s.Comment(id, AuthorVisitor, "", "hello", epoch); !errors.Is(err, ErrNotFound) {
			t.Errorf("a comment on a %s report gave %v, want ErrNotFound", name, err)
		}
	}
	if got, _ := s.ByID(survivor.ID); got.VoteCount != 0 {
		t.Errorf("the survivor's count moved to %d", got.VoteCount)
	}
}

// TestTheLimitsCountCharactersNotBytes.
//
// They counted len(), which is bytes: a Japanese letter is three, so 20,000
// characters of English were accepted and about 6,600 of Japanese refused. The
// text here is not Latin, or the test passes with the bytes and with the
// characters alike.
func TestTheLimitsCountCharactersNotBytes(t *testing.T) {
	s := store(t)
	ja := func(n int) string { return strings.Repeat("ぜ", n) }

	if _, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: ja(maxTitle), Body: ja(maxBody)}, Pending, epoch); err != nil {
		t.Errorf("a title and description at their limits, in Japanese, were refused: %v", err)
	}
	if _, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: ja(maxTitle + 1), Body: "x"}, Pending, epoch); err == nil {
		t.Error("a title one character over its limit was accepted")
	}
	if _, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: "x", Body: ja(maxBody + 1)}, Pending, epoch); err == nil {
		t.Error("a description one character over its limit was accepted")
	}

	it, _ := report(t, s, Bug, "crash", "")
	if err := s.Edit(it.ID, ja(maxTitle), ja(maxBody), epoch); err != nil {
		t.Errorf("an edit at the limits, in Japanese, was refused: %v", err)
	}
	if err := s.Edit(it.ID, ja(maxTitle+1), "x", epoch); err == nil {
		t.Error("an edited title over its limit was accepted")
	}

	c, err := s.Comment(it.ID, AuthorTeam, "alex", ja(maxComment), epoch)
	if err != nil {
		t.Fatalf("a comment at its limit, in Japanese, was refused: %v", err)
	}
	if _, err := s.Comment(it.ID, AuthorTeam, "alex", ja(maxComment+1), epoch); err == nil {
		t.Error("a comment over its limit was accepted")
	}
	if err := s.EditComment(c.ID, ja(maxComment)); err != nil {
		t.Errorf("an edited comment at its limit was refused: %v", err)
	}
	if err := s.EditComment(c.ID, ja(maxComment+1)); err == nil {
		t.Error("an edited comment over its limit was accepted")
	}
}

// TestAnAnswerTooLongIsCutBetweenLettersNotThroughOne: cutting bytes at the
// limit left a third of a letter at the end of the stored text.
func TestAnAnswerTooLongIsCutBetweenLettersNotThroughOne(t *testing.T) {
	s := store(t)
	it, _, err := s.Create(New{RepoID: 1, Kind: Bug, Title: "crash", Body: "it closes",
		Fields: []Field{{Key: "steps", Label: "Steps", Value: strings.Repeat("ぜ", maxValue+500)}}}, Pending, epoch)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ByID(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fields) != 1 {
		t.Fatalf("%d answers stored", len(got.Fields))
	}
	v := got.Fields[0].Value
	if !utf8.ValidString(v) {
		t.Error("the stored answer is not valid text: a letter was cut through the middle")
	}
	if n := utf8.RuneCountInString(v); n != maxValue {
		t.Errorf("the stored answer is %d characters, want %d", n, maxValue)
	}
}
