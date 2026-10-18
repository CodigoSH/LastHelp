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

// The kinds of work this queue carries.
const (
	JobCreateIssue = "create_issue"

	// JobTellReporter tells the person who reported that something changed.
	JobTellReporter = "tell_reporter"

	// JobAnnounce tells the team something arrived. It goes through the queue
	// for the same reason the forge calls do: a slow relay or a chat server
	// having a bad afternoon must not be something the person reporting a bug
	// waits for, and must never be a reason to refuse their report.
	JobAnnounce = "announce"
)

// Announcer delivers a notification.
//
// A function rather than a dependency on the notification package, so the
// domain stays free of channels, tokens and HTTP: it says what happened, and
// something outside decides where that goes.
type Announcer func(ctx context.Context, repoID int64, title, body, url string) error

// Teller delivers to the person who reported something.
//
// It is handed the address, and it is the only thing that is. Nothing else in
// the domain asks for it: the address is read by id, at the moment a message is
// about to be sent, and it reaches no listing, no response and no log line
// because nothing else selects the column.
type Teller func(ctx context.Context, address, title, body, url string) error

// ErrNoForge means the organisation is not connected to one. It is not a
// failure state: running with no forge at all is supported, and everything
// except this bridge works without one.
var ErrNoForge = errors.New("this organisation has no forge")

// Clients builds a client for an organisation. It is a function rather than a
// map so the credential is read at the moment of use rather than being carried
// around in memory, where a panic would print it.
type Clients func(o org.Organization) (forge.Client, error)

// Bridge carries what both directions need.
type Bridge struct {
	DB      database.Execer
	Orgs    org.Store
	Items   item.Store
	Queue   outbox.Store
	Clients Clients

	// Announce is optional: an instance with no channel configured simply has
	// nothing here, and the job finishes doing nothing.
	Announce Announcer

	// Tell is optional in the same way. With no mail relay there is nothing to
	// tell anybody with, which is why the form never offers the address field
	// in that case rather than collecting one it cannot use.
	Tell Teller

	// PublicURL is where this instance is reachable, used to build the link in
	// a notification. Empty means the notification carries no link, which is
	// worse but not broken.
	PublicURL string
}

type createIssuePayload struct {
	ItemID int64 `json:"item_id"`
}

type announcePayload struct {
	ItemID int64 `json:"item_id"`
	// What happened, when it is not a new report. Empty for the original job,
	// which is what jobs queued before this field existed carry.
	What string `json:"what,omitempty"`
}

type tellPayload struct {
	ItemID int64  `json:"item_id"`
	Reason string `json:"reason"`
}

// RequestTellReporter asks for the reporter to be told, later.
//
// Nothing happens when there is no relay or no address, which is the common
// case: the job runs, finds nothing to do, and finishes.
func (b Bridge) RequestTellReporter(itemID int64, reason string, now time.Time) error {
	if b.Tell == nil {
		return nil
	}
	return b.Queue.Enqueue(JobTellReporter, tellPayload{ItemID: itemID, Reason: reason}, now)
}

// RequestAnnouncement asks for the team to be told, later.
func (b Bridge) RequestAnnouncement(itemID int64, now time.Time) error {
	if b.Announce == nil {
		return nil
	}
	return b.Queue.Enqueue(JobAnnounce, announcePayload{ItemID: itemID}, now)
}

// RequestCommentAnnouncement asks for the team to be told, later, that a
// message is waiting for somebody to read it before it shows.
func (b Bridge) RequestCommentAnnouncement(itemID int64, now time.Time) error {
	if b.Announce == nil {
		return nil
	}
	return b.Queue.Enqueue(JobAnnounce, announcePayload{ItemID: itemID, What: CommentWaiting}, now)
}

// CommentWaiting is the label a channel shows for a message awaiting review.
const CommentWaiting = "comment waiting for review"

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

	case JobTellReporter:
		var payload tellPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("unreadable payload: %w", err)
		}
		return b.tellReporter(ctx, payload.ItemID, payload.Reason)

	case JobAnnounce:
		var payload announcePayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return fmt.Errorf("unreadable payload: %w", err)
		}
		return b.announce(ctx, payload.ItemID, payload.What)

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
	out.WriteString(neutralise(it.Body))

	if len(it.Fields) > 0 {
		out.WriteString("\n\n")
		for _, f := range it.Fields {
			if strings.TrimSpace(f.Value) == "" {
				continue
			}
			fmt.Fprintf(&out, "**%s:** %s\n", f.Label, neutralise(f.Value))
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

// announce tells the team one thing arrived.
//
// Short on purpose: what it carries is a nudge to go and look, not a copy of
// the submission. A notification containing the whole report is a notification
// that delivers spam to wherever the team reads it — which is the one place the
// moderation queue was supposed to keep it out of.
func (b Bridge) announce(ctx context.Context, itemID int64, what string) error {
	if b.Announce == nil {
		return nil
	}

	it, err := b.Items.ByID(itemID)
	if err != nil {
		return err
	}

	link := ""
	if b.PublicURL != "" {
		link = strings.TrimSuffix(b.PublicURL, "/") + "/i/" + it.PublicID
	}
	if what == "" {
		what = string(it.Kind)
	}
	return b.Announce(ctx, it.RepoID, it.Title, what, link)
}

// tellReporter tells the person who reported that their item moved.
//
// And then, if the item is finished, forgets their address. It was collected
// for one purpose; that purpose is over, and keeping it afterwards is holding
// somebody's personal data for no reason at all. SECURITY.md says it is deleted
// when the item closes, and this is the code that has to make that true rather
// than the sentence that promises it.
func (b Bridge) tellReporter(ctx context.Context, itemID int64, reason string) error {
	if b.Tell == nil {
		return nil
	}

	it, err := b.Items.ByID(itemID)
	if err != nil {
		return err
	}

	address, err := b.Items.ReporterEmail(itemID)
	if err != nil {
		return err
	}
	if address == "" {
		// Nobody asked to be told. The common case, and not a failure.
		return b.forgetIfFinished(it)
	}

	link := ""
	if b.PublicURL != "" {
		link = strings.TrimSuffix(b.PublicURL, "/") + "/i/" + it.PublicID
	}
	if err := b.Tell(ctx, address, it.Title, reason, link); err != nil {
		return err
	}
	return b.forgetIfFinished(it)
}

func (b Bridge) forgetIfFinished(it item.Item) error {
	switch it.Status {
	case item.StatusDone, item.StatusClosed, item.StatusDeclined, item.StatusNotReproducible:
		return b.Items.ForgetEmail(it.ID)
	}
	return nil
}
