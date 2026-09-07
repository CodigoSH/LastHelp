// Package forge_test holds the tests that are the same question asked of every
// adapter, plus the ones that are about the differences between them.
package forge_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/fake"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitea"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/github"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitlab"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

func gate() *outbound.Client { return outbound.New(false, 5*time.Second) }

func clients(server string) map[string]forge.Client {
	return map[string]forge.Client{
		"github": github.Client{BaseURL: server, Token: "t", Out: gate()},
		"gitlab": gitlab.Client{BaseURL: server, Token: "t", Out: gate()},
		"gitea":  gitea.Client{BaseURL: server, Token: "t", Out: gate()},
	}
}

// TestEveryAdapterCreatesAndReadsBack is the same question asked of all three.
//
// It runs against a fake that answers every forge's field spellings at once and
// gives them different values, so an adapter reading the wrong field — id
// instead of iid, body instead of description — comes back with the wrong
// answer rather than an empty one.
func TestEveryAdapterCreatesAndReadsBack(t *testing.T) {
	f := fake.New()
	defer f.Close()

	for name, client := range clients(f.URL()) {
		t.Run(name, func(t *testing.T) {
			created, err := client.CreateIssue(context.Background(), "owner/repo",
				forge.NewIssue{Title: "crash on import", Body: "it closes"})
			if err != nil {
				t.Fatalf("creating: %v", err)
			}
			if created.Number == 0 {
				t.Fatal("came back with issue number 0")
			}
			if created.Title != "crash on import" {
				t.Errorf("title came back as %q", created.Title)
			}
			if created.State != forge.Open {
				t.Errorf("a new issue is %q", created.State)
			}
			if created.URL == "" {
				t.Error("no URL came back; the item would link nowhere")
			}

			read, err := client.FindIssue(context.Background(), "owner/repo", created.Number)
			if err != nil {
				t.Fatalf("reading back: %v", err)
			}
			if read.Number != created.Number {
				t.Errorf("read back number %d, created %d", read.Number, created.Number)
			}
		})
	}
}

// TestGitLabUsesTheNumberPeopleSee.
//
// GitLab sends both `iid` and `id`. The first is what the interface shows and
// what a URL is built from; the second is global to the instance. The fake
// deliberately makes them different, so an adapter reading `id` links to
// somebody else's issue instead of failing.
func TestGitLabUsesTheNumberPeopleSee(t *testing.T) {
	f := fake.New()
	defer f.Close()

	client := gitlab.Client{BaseURL: f.URL(), Token: "t", Out: gate()}
	created, err := client.CreateIssue(context.Background(), "owner/repo", forge.NewIssue{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Number != 1 {
		t.Errorf("number came back as %d; the fake's iid is 1 and its id is 1001, so this is "+
			"reading the wrong field and every link it builds goes to the wrong issue", created.Number)
	}
}

// TestGitLabEscapesTheProjectPath.
//
// GitLab wants the path with every slash escaped. Unescaped, the request goes
// to a path that does not exist and comes back 404 — indistinguishable from a
// repository that is not there, which is a long afternoon.
func TestGitLabEscapesTheProjectPath(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.EscapedPath()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":1,"state":"opened","web_url":"u","title":"x"}`))
	}))
	defer server.Close()

	client := gitlab.Client{BaseURL: server.URL, Token: "t", Out: gate()}
	if _, err := client.CreateIssue(context.Background(), "group/sub/project", forge.NewIssue{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v4/projects/group%2Fsub%2Fproject/issues"; seen != want {
		t.Errorf("asked for %q, want %q", seen, want)
	}
}

// TestGitLabStateIsNormalised: it says "opened" where everybody else says
// "open", and that word must not reach the domain.
func TestGitLabStateIsNormalised(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":1,"state":"opened","web_url":"u","title":"x"}`))
	}))
	defer server.Close()

	client := gitlab.Client{BaseURL: server.URL, Token: "t", Out: gate()}
	created, err := client.CreateIssue(context.Background(), "o/r", forge.NewIssue{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if created.State != forge.Open {
		t.Errorf("state came back as %q; GitLab's \"opened\" has to be normalised in the adapter", created.State)
	}
}

// TestFailuresAreToldApart.
//
// A credential that is wrong needs a person; being rate limited needs a wait.
// Collapsing them into one error means the queue either gives up on something
// retryable or retries something that will never work.
func TestFailuresAreToldApart(t *testing.T) {
	f := fake.New()
	defer f.Close()

	for name, client := range clients(f.URL()) {
		t.Run(name+"/denied", func(t *testing.T) {
			f.Fail = http.StatusUnauthorized
			_, err := client.CreateIssue(context.Background(), "owner/repo", forge.NewIssue{Title: "x"})
			if !errors.Is(err, forge.ErrDenied) {
				t.Errorf("a rejected credential gave %v", err)
			}
		})
		t.Run(name+"/rate limited", func(t *testing.T) {
			f.Fail = http.StatusTooManyRequests
			_, err := client.CreateIssue(context.Background(), "owner/repo", forge.NewIssue{Title: "x"})
			if !errors.Is(err, forge.ErrRateLimited) {
				t.Errorf("being rate limited gave %v", err)
			}
		})
		t.Run(name+"/missing", func(t *testing.T) {
			_, err := client.FindIssue(context.Background(), "owner/repo", 9999)
			if !errors.Is(err, forge.ErrNotFound) {
				t.Errorf("a missing issue gave %v", err)
			}
		})
	}
}

// TestAdaptersGoThroughTheGate.
//
// Every adapter holds an *outbound.Client rather than an http.Client, and the
// architecture rule enforces that. This is the behavioural half: with the
// offline switch on, nothing gets out.
func TestAdaptersGoThroughTheGate(t *testing.T) {
	f := fake.New()
	defer f.Close()

	offline := outbound.New(true, time.Second)
	all := map[string]forge.Client{
		"github": github.Client{BaseURL: f.URL(), Token: "t", Out: offline},
		"gitlab": gitlab.Client{BaseURL: f.URL(), Token: "t", Out: offline},
		"gitea":  gitea.Client{BaseURL: f.URL(), Token: "t", Out: offline},
	}

	for name, client := range all {
		t.Run(name, func(t *testing.T) {
			_, err := client.CreateIssue(context.Background(), "owner/repo", forge.NewIssue{Title: "x"})
			if !errors.Is(err, outbound.ErrOffline) {
				t.Errorf("the offline switch did not stop it: %v", err)
			}
		})
	}
	if f.Issues() != 0 {
		t.Errorf("%d issues were created while offline", f.Issues())
	}
}
