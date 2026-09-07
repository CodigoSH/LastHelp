// Package bridge is what connects an item to an issue, in both directions.
//
// Outwards: a triaged item becomes an issue on the operator's forge, or is
// attached to one that already exists. Inwards: when that issue is closed,
// reopened or relabelled, the item follows — which is what makes a roadmap
// nobody maintains by hand possible.
package bridge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/outbox"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// JobCreateIssue is the only kind of work this queue carries so far.
const JobCreateIssue = "create_issue"

// ErrNoForge means the organisation is not connected to one. It is not a
// failure state: running with no forge at all is supported, and everything
// except this bridge works without one.
var ErrNoForge = errors.New("this organisation has no forge")

// Clients builds a client for an organisation. It is a function rather than a
// map so the credential is decrypted at the moment of use rather than being
// carried around in memory.
type Clients func(o org.Organization) (forge.Client, error)

// Bridge carries what both directions need.
type Bridge struct {
	DB      database.Execer
	Orgs    org.Store
	Items   item.Store
	Queue   outbox.Store
	Clients Clients
}

type createIssuePayload struct {
	ItemID int64 `json:"item_id"`
}

// RequestIssue asks for an issue to be created, later.
//
// It does not call the forge. A handler that did would hang while somebody
// else's server thought about it, fail when that server was down, and lose the
// work — and the operator who clicked the button would have no idea whether it
// happened.
func (b Bridge) RequestIssue(itemID int64, now time.Time) error {
	return b.Queue.Enqueue(JobCreateIssue, createIssuePayload{ItemID: itemID}, now)
}

// Run drains what is due. It returns how many jobs it handled, which is what a
// test asserts on and what a log line reports.
func (b Bridge) Run(ctx context.Context, now time.Time, limit int) (int, error) {
	jobs, err := b.Queue.Due(now, limit)
	if err != nil {
		return 0, err
	}

	done := 0
	for _, job := range jobs {
		err := b.run(ctx, job, now)
		switch {
		case err == nil:
			if err := b.Queue.Done(job.ID); err != nil {
				return done, err
			}
			done++

		case permanent(err):
			// A revoked token or a repository that no longer exists will not
			// fix itself by being asked again in an hour.
			slog.Error("giving up on a forge job", "job", job.ID, "kind", job.Kind, "error", err)
			if err := b.Queue.Abandon(job.ID, err, now); err != nil {
				return done, err
			}

		case job.Attempts+1 >= outbox.MaxAttempts:
			slog.Error("a forge job ran out of attempts", "job", job.ID, "error", err)
			if err := b.Queue.Abandon(job.ID, err, now); err != nil {
				return done, err
			}

		default:
			if err := b.Queue.Retry(job.ID, job.Attempts, err, now); err != nil {
				return done, err
			}
		}
	}
	return done, nil
}

func permanent(err error) bool {
	return errors.Is(err, forge.ErrDenied) ||
		errors.Is(err, forge.ErrNotFound) ||
		errors.Is(err, ErrNoForge) ||
		errors.Is(err, item.ErrNotFound)
}

func (b Bridge) run(ctx context.Context, job outbox.Job, now time.Time) error {
	switch job.Kind {
	case JobCreateIssue:
		var payload createIssuePayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("unreadable payload: %w", err)
		}
		return b.createIssue(ctx, payload.ItemID, now)
	default:
		return fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

func (b Bridge) createIssue(ctx context.Context, itemID int64, now time.Time) error {
	it, err := b.Items.ByID(itemID)
	if err != nil {
		return err
	}

	// Already linked: the job ran twice, which a queue with retries will always
	// eventually do. Creating a second issue would be the visible half of that
	// mistake, and the noisy one.
	if _, err := b.Link(itemID); err == nil {
		return nil
	}

	client, repo, orgID, err := b.clientFor(it.RepoID)
	if err != nil {
		return err
	}

	created, err := client.CreateIssue(ctx, repo, forge.NewIssue{
		Title: it.Title,
		Body:  IssueBody(it),
	})
	if err != nil {
		return err
	}

	return b.record(itemID, it.RepoID, created, orgID, now)
}

// IssueBody is what arrives on the forge.
//
// The answers to the form come first, in the order they were asked, because
// that is what makes a report usable by whoever picks it up — a bug with no
// version and no steps is work rather than information. The link back is last
// and always present: somebody reading the issue has to be able to reach the
// person who reported it.
func IssueBody(it item.Item) string {
	var out strings.Builder
	out.WriteString(it.Body)

	if len(it.Fields) > 0 {
		out.WriteString("\n\n")
		for _, f := range it.Fields {
			if strings.TrimSpace(f.Value) == "" {
				continue
			}
			fmt.Fprintf(&out, "**%s:** %s\n", f.Label, f.Value)
		}
	}

	fmt.Fprintf(&out, "\n\n---\nReported through LastHelp (%s)\n", it.PublicID)
	return out.String()
}

// Attach links an item to an issue that already exists.
//
// Somebody with four thousand open issues does not want a new one; they want to
// say "this is already #4213". This is the half of the bridge that makes the
// product usable on day one for a project that has been going for years.
func (b Bridge) Attach(ctx context.Context, itemID, number int64, now time.Time) (forge.Issue, error) {
	it, err := b.Items.ByID(itemID)
	if err != nil {
		return forge.Issue{}, err
	}

	client, repo, orgID, err := b.clientFor(it.RepoID)
	if err != nil {
		return forge.Issue{}, err
	}

	// Read it back rather than trusting the number. It proves the issue exists,
	// gives the title to show, and says what state it is in — which the item
	// then follows immediately rather than waiting for the next webhook.
	found, err := client.FindIssue(ctx, repo, number)
	if err != nil {
		return forge.Issue{}, err
	}
	if err := b.record(itemID, it.RepoID, found, orgID, now); err != nil {
		return forge.Issue{}, err
	}
	return found, nil
}

// Link is the issue an item is attached to, if any.
func (b Bridge) Link(itemID int64) (forge.Issue, error) {
	var (
		number int64
		url    string
		state  string
	)
	err := b.DB.QueryRow(`SELECT forge_number, forge_url, last_state FROM issue_link WHERE item_id = ?`,
		itemID).Scan(&number, &url, &state)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return forge.Issue{}, item.ErrNotFound
	case err != nil:
		return forge.Issue{}, fmt.Errorf("reading the issue link: %w", err)
	}
	return forge.Issue{Number: number, URL: url, State: forge.State(state)}, nil
}

func (b Bridge) record(itemID, repoID int64, issue forge.Issue, orgID int64, now time.Time) error {
	_, err := b.DB.Exec(`INSERT INTO issue_link
		(item_id, repo_id, forge_number, forge_url, last_state, last_synced_at, echo_fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (item_id) DO UPDATE SET
			forge_number = excluded.forge_number,
			forge_url = excluded.forge_url,
			last_state = excluded.last_state,
			last_synced_at = excluded.last_synced_at,
			echo_fingerprint = excluded.echo_fingerprint`,
		itemID, repoID, issue.Number, issue.URL, string(issue.State),
		now.Unix(), Fingerprint(issue))
	if err != nil {
		return fmt.Errorf("recording the issue link: %w", err)
	}
	_ = orgID
	return nil
}

func (b Bridge) clientFor(repoID int64) (forge.Client, forge.Repo, int64, error) {
	repo, err := b.Orgs.RepositoryByID(repoID)
	if err != nil {
		return nil, "", 0, err
	}
	organisation, err := b.Orgs.OrganizationByID(repo.OrgID)
	if err != nil {
		return nil, "", 0, err
	}
	if organisation.Forge == org.ForgeNone || repo.ForgeRepo == "" {
		return nil, "", 0, ErrNoForge
	}

	client, err := b.Clients(organisation)
	if err != nil {
		return nil, "", 0, err
	}
	return client, forge.Repo(repo.ForgeRepo), organisation.ID, nil
}

// Fingerprint is what an issue looked like the last time this server wrote it.
//
// It is the loop breaker. Without it: the team closes an item here, the bridge
// closes the issue there, the forge sends a delivery saying it closed, the
// bridge applies it, which writes again — and two systems tell each other the
// same news for ever. With it, a delivery describing the state we just pushed
// is recognised as our own echo and dropped.
func Fingerprint(issue forge.Issue) string {
	labels := append([]string(nil), issue.Labels...)
	sort.Strings(labels)
	sum := sha256.Sum256([]byte(string(issue.State) + "|" + strings.Join(labels, ",")))
	return hex.EncodeToString(sum[:8])
}
