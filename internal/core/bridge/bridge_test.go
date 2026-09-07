package bridge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/outbox"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var epoch = time.Unix(1_700_000_000, 0)

// stub is a forge that records what it was asked and answers what it was told.
type stub struct {
	created  []forge.NewIssue
	nextNum  int64
	failWith error
	issues   map[int64]forge.Issue
}

func (s *stub) CreateIssue(_ context.Context, _ forge.Repo, in forge.NewIssue) (forge.Issue, error) {
	if s.failWith != nil {
		return forge.Issue{}, s.failWith
	}
	s.created = append(s.created, in)
	s.nextNum++
	issue := forge.Issue{Number: s.nextNum, URL: "https://forge.example/issues/1", Title: in.Title, State: forge.Open}
	if s.issues == nil {
		s.issues = map[int64]forge.Issue{}
	}
	s.issues[issue.Number] = issue
	return issue, nil
}

func (s *stub) FindIssue(_ context.Context, _ forge.Repo, number int64) (forge.Issue, error) {
	if s.failWith != nil {
		return forge.Issue{}, s.failWith
	}
	issue, ok := s.issues[number]
	if !ok {
		return forge.Issue{}, forge.ErrNotFound
	}
	return issue, nil
}

func setup(t *testing.T, forgeKind org.Forge) (Bridge, *stub, item.Item) {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	orgs := org.Store{DB: db}
	organisation, err := orgs.CreateOrganization(org.Organization{
		Slug: "codigosh", Forge: forgeKind, BaseURL: "https://forge.example", ForgeOwner: "CodigoSH",
	}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	repoRow := org.Repository{OrgID: organisation.ID, Slug: "lastboard", AcceptsBugs: true, AcceptsIdeas: true}
	if forgeKind != org.ForgeNone {
		repoRow.ForgeRepo = "CodigoSH/Lastboard"
	}
	repo, err := orgs.CreateRepository(repoRow, epoch)
	if err != nil {
		t.Fatal(err)
	}

	items := item.Store{DB: db}
	created, _, err := items.Create(item.New{
		RepoID: repo.ID, Kind: item.Bug, Title: "crash on import", Body: "it closes",
		Fields: []item.Field{{Key: "version", Label: "Version", Value: "1.7.4"}},
	}, item.Published, epoch)
	if err != nil {
		t.Fatal(err)
	}

	f := &stub{}
	return Bridge{
		DB: db, Orgs: orgs, Items: items, Queue: outbox.Store{DB: db},
		Clients: func(org.Organization) (forge.Client, error) { return f, nil },
	}, f, created
}

func TestAnItemBecomesAnIssue(t *testing.T) {
	b, f, it := setup(t, org.ForgeForgejo)

	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	// Nothing has happened yet: the handler queued, it did not call.
	if len(f.created) != 0 {
		t.Fatal("the forge was called from the request rather than from the queue")
	}

	done, err := b.Run(context.Background(), epoch, 10)
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 || len(f.created) != 1 {
		t.Fatalf("ran %d jobs, created %d issues", done, len(f.created))
	}

	// The answers to the form travel with it: a bug with no version and no
	// steps is work for whoever picks it up rather than information.
	if body := f.created[0].Body; !contains(body, "Version") || !contains(body, "1.7.4") {
		t.Errorf("the form answers did not reach the issue:\n%s", body)
	}
	if body := f.created[0].Body; !contains(body, it.PublicID) {
		t.Error("the issue has no way back to the report it came from")
	}

	link, err := b.Link(it.ID)
	if err != nil {
		t.Fatalf("nothing was linked: %v", err)
	}
	if link.Number != 1 {
		t.Errorf("linked to issue %d", link.Number)
	}
}

// TestTheJobRunningTwiceCreatesOneIssue.
//
// A queue with retries will eventually run something twice — that is what
// retries are. Two issues for one report is the visible, noisy half of that.
func TestTheJobRunningTwiceCreatesOneIssue(t *testing.T) {
	b, f, it := setup(t, org.ForgeForgejo)

	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}
	// Queue the same work again, as a retry after an ambiguous failure would.
	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch.Add(time.Minute), 10); err != nil {
		t.Fatal(err)
	}

	if len(f.created) != 1 {
		t.Errorf("%d issues were created for one report", len(f.created))
	}
}

// TestARevokedTokenIsNotRetriedForEver.
//
// Being rate limited is worth waiting out. A credential that was revoked is
// not, and retrying it for a day and a half hides the one thing a person needs
// to be told.
func TestARevokedTokenIsNotRetriedForEver(t *testing.T) {
	b, f, it := setup(t, org.ForgeForgejo)
	f.failWith = forge.ErrDenied

	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}

	abandoned, err := b.Queue.Abandoned()
	if err != nil {
		t.Fatal(err)
	}
	if len(abandoned) != 1 {
		t.Fatalf("%d jobs were abandoned; a revoked token should stop being retried at once", len(abandoned))
	}
}

func TestBeingRateLimitedIsRetried(t *testing.T) {
	b, f, it := setup(t, org.ForgeForgejo)
	f.failWith = forge.ErrRateLimited

	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}

	abandoned, err := b.Queue.Abandoned()
	if err != nil {
		t.Fatal(err)
	}
	if len(abandoned) != 0 {
		t.Error("being rate limited was treated as permanent; the work is simply lost")
	}

	// And it comes back later, once the wait is over.
	f.failWith = nil
	done, err := b.Run(context.Background(), epoch.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 {
		t.Error("the job never came back after the wait")
	}
}

// TestWithoutAForgeNothingIsAttempted.
//
// Running with no forge is supported, not a broken state. The job stops rather
// than being retried for a day and a half against a connection that does not
// exist.
func TestWithoutAForgeNothingIsAttempted(t *testing.T) {
	b, f, it := setup(t, org.ForgeNone)

	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 0 {
		t.Error("an issue was created on an organisation with no forge")
	}
	abandoned, _ := b.Queue.Abandoned()
	if len(abandoned) != 1 {
		t.Error("the job is being retried against a forge that does not exist")
	}
}

// TestAttachingAnIssueThatAlreadyExists.
//
// Somebody with four thousand open issues wants to say "this is already
// #4213", not to create a four thousand and first.
func TestAttachingAnIssueThatAlreadyExists(t *testing.T) {
	b, f, it := setup(t, org.ForgeForgejo)
	f.issues = map[int64]forge.Issue{
		4213: {Number: 4213, URL: "https://forge.example/issues/4213", Title: "the same crash", State: forge.Open},
	}

	found, err := b.Attach(context.Background(), it.ID, 4213, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if found.Title != "the same crash" {
		t.Errorf("read back %+v; the issue is read rather than trusted so the title and state are real", found)
	}
	if len(f.created) != 0 {
		t.Error("attaching created a new issue")
	}

	link, err := b.Link(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if link.Number != 4213 {
		t.Errorf("linked to %d", link.Number)
	}
}

func TestAttachingSomethingThatIsNotThere(t *testing.T) {
	b, _, it := setup(t, org.ForgeForgejo)
	if _, err := b.Attach(context.Background(), it.ID, 9999, epoch); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound — the issue is read back precisely so a typo does not "+
			"link an item to nothing", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (len(needle) == 0 ||
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}())
}
