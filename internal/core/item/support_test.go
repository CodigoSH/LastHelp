package item

import (
	"testing"
	"time"
)

func publish(t *testing.T, s Store, kind Kind, title string, at time.Time) Item {
	t.Helper()
	it, _ := report(t, s, kind, title, "details")
	if err := s.setModeration(it.ID, Published, at); err != nil {
		t.Fatal(err)
	}
	return it
}

// TestAnAnswerIsAWordFromTheTeam.
//
// A visitor writing on a report is not an answer to the person who reported it,
// and a team reply is. The filter that finds what nobody has answered has to
// tell them apart or the list is empty the first time anybody comments.
func TestAnAnswerIsAWordFromTheTeam(t *testing.T) {
	s := store(t)
	answered := publish(t, s, Idea, "dark mode", epoch)
	chatted := publish(t, s, Idea, "csv export", epoch)
	silent := publish(t, s, Bug, "it crashes", epoch)

	if _, err := s.TeamReply(answered.ID, 0, "kiwi", "on it", epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Comment(chatted.ID, AuthorVisitor, "", "same here", epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(Filter{RepoID: 1, Unanswered: true})
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]bool{}
	for _, it := range got {
		titles[it.Title] = true
	}
	if titles["dark mode"] || !titles["csv export"] || !titles["it crashes"] || len(got) != 2 {
		t.Errorf("unanswered came back as %v, want the visitor-only and the silent one: %s %s", titles, chatted.Title, silent.Title)
	}
}

// TestTheFactsOfASetAreTheListsOwn.
func TestTheFactsOfASetAreTheListsOwn(t *testing.T) {
	s := store(t)
	old := publish(t, s, Idea, "old", epoch)
	publish(t, s, Idea, "newer", epoch.Add(time.Hour))
	if err := s.Vote(old.ID, "v1", epoch); err != nil {
		t.Fatal(err)
	}

	filter := Filter{RepoID: 1, Unanswered: true}
	facts, err := s.Facts(filter)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(filter)
	if facts.Count != len(list) || facts.Count != 2 {
		t.Errorf("the facts counted %d where the list shows %d", facts.Count, len(list))
	}
	if facts.Oldest != epoch.Unix() {
		t.Errorf("the oldest was %d, want %d", facts.Oldest, epoch.Unix())
	}
	if facts.MostVoted != 1 {
		t.Errorf("the most wanted had %d votes, want 1", facts.MostVoted)
	}

	none, err := s.Facts(Filter{RepoID: 1, Unanswered: true, Kind: Bug})
	if err != nil || none.Count != 0 || none.Oldest != 0 {
		t.Errorf("an empty set should be zeros, got %+v (%v)", none, err)
	}
}

// TestHowFastTheTeamAnswered.
func TestHowFastTheTeamAnswered(t *testing.T) {
	s := store(t)
	waits := []time.Duration{2 * time.Hour, 4 * time.Hour, 10 * time.Hour}
	for i, wait := range waits {
		it := publish(t, s, Idea, "idea "+string(rune('a'+i)), epoch)
		if _, err := s.TeamReply(it.ID, 0, "kiwi", "ok", epoch.Add(wait)); err != nil {
			t.Fatal(err)
		}
	}
	publish(t, s, Bug, "nobody wrote back", epoch)

	got, err := s.FirstReplies([]int64{1}, epoch.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if got.Published != 4 || got.Answered != 3 {
		t.Errorf("answered %d of %d, want 3 of 4", got.Answered, got.Published)
	}
	if got.Median != int64((4 * time.Hour).Seconds()) {
		t.Errorf("the median wait was %ds, want four hours", got.Median)
	}

	// Nothing published since: nothing to say, and no division by it.
	later, err := s.FirstReplies([]int64{1}, epoch.Add(24*time.Hour).Unix())
	if err != nil || later.Published != 0 || later.Median != 0 {
		t.Errorf("a window with nothing in it gave %+v (%v)", later, err)
	}

	// Nobody's boards: nothing, not everything.
	if none, _ := s.FirstReplies(nil, 0); none.Published != 0 {
		t.Errorf("no repositories matched %d reports", none.Published)
	}
}

// TestTheOldestWaitIsTheOldestOfReportsAndComments.
func TestTheOldestWaitIsTheOldestOfReportsAndComments(t *testing.T) {
	s := store(t)
	if oldest, err := s.OldestWaiting([]int64{1}); err != nil || oldest != 0 {
		t.Fatalf("nothing waiting should be zero, got %d (%v)", oldest, err)
	}

	waiting, _ := report(t, s, Bug, "waits", "b")
	if err := s.setModeration(waiting.ID, Pending, epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	published := publish(t, s, Idea, "talked about", epoch)
	if _, err := s.Comment(published.ID, AuthorVisitor, "", "a first word", epoch.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	oldest, err := s.OldestWaiting([]int64{1})
	if err != nil {
		t.Fatal(err)
	}
	// The report arrived at epoch (report() stamps it so); the comment half an hour later.
	if oldest != epoch.Unix() {
		t.Errorf("the oldest wait began at %d, want %d", oldest, epoch.Unix())
	}
}

// TestABugTheTeamAgreedOnNeedsAnIssue.
func TestABugTheTeamAgreedOnNeedsAnIssue(t *testing.T) {
	s := store(t)
	bug := publish(t, s, Bug, "confirmed and homeless", epoch)
	if err := s.SetStatus(bug.ID, StatusConfirmed, epoch); err != nil {
		t.Fatal(err)
	}
	idea := publish(t, s, Idea, "an idea is not a bug", epoch)
	_ = idea

	n, err := s.NotOnForge([]int64{1}, "none")
	if err != nil || n != 1 {
		t.Fatalf("counted %d (%v), want the one confirmed bug", n, err)
	}

	if _, err := s.DB.Exec(`INSERT INTO issue_link (item_id, repo_id, forge_number, forge_url) VALUES (?, 1, 1, 'u')`, bug.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.NotOnForge([]int64{1}, "none"); n != 0 {
		t.Errorf("a bug with an issue was still counted: %d", n)
	}

	// An organisation with no forge has nowhere to put an issue.
	if _, err := s.DB.Exec(`DELETE FROM issue_link`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE organization SET forge_kind = 'none'`); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.NotOnForge([]int64{1}, "none"); n != 0 {
		t.Errorf("a repository whose organisation has no forge was counted: %d", n)
	}
}
