// Package forge is the shape every forge is made to fit.
//
// Four of them, and they disagree about nearly everything: what the body field
// is called, whether labels are a string or an array, where the issue number
// lives, what an event is called, and how a delivery is signed. The job of this
// package is to state what LastHelp needs, once, so the disagreements live in
// four small adapters instead of spreading through the application.
//
// It deliberately describes very little. Creating an issue, reading one back,
// and understanding a webhook is the whole of it — everything else a forge can
// do is something this product does not need, and an interface that grew to
// cover it would be four adapters' worth of code nobody calls.
package forge

import (
	"context"
	"errors"
	"net/http"
)

// Kind is which forge an organisation connects to.
type Kind string

const (
	GitHub  Kind = "github"
	GitLab  Kind = "gitlab"
	Gitea   Kind = "gitea"
	Forgejo Kind = "forgejo"
)

var (
	// ErrNotFound means the forge says there is no such repository or issue.
	ErrNotFound = errors.New("not found on the forge")

	// ErrDenied means the credential is wrong or lacks the permission. It is
	// separate from a transport failure because the answer is different: one is
	// retried, the other needs a person.
	ErrDenied = errors.New("the forge refused the credential")

	// ErrRateLimited means to come back later. GitHub warns about "secondary
	// rate limiting" specifically when creating content quickly, which is
	// exactly what a queue draining after an outage does.
	ErrRateLimited = errors.New("the forge is rate limiting")

	// ErrUnsupported means this forge cannot do what was asked.
	ErrUnsupported = errors.New("this forge does not support that")
)

// State is the only thing about an issue's lifecycle this product tracks.
//
// Two values, because that is what every one of the four agrees on. GitLab says
// "opened" where the others say "open", which is exactly the kind of difference
// that belongs in an adapter and not in a domain.
type State string

const (
	Open   State = "open"
	Closed State = "closed"
)

// Issue is a forge issue, reduced to what is used here.
type Issue struct {
	Number int64
	URL    string
	Title  string
	State  State
	Labels []string
}

// NewIssue is what gets created.
type NewIssue struct {
	Title  string
	Body   string
	Labels []string
}

// Repo names a repository on its forge, in that forge's own terms:
// "CodigoSH/Lastboard" for GitHub, Gitea and Forgejo, and either that or a
// numeric id for GitLab. Adapters encode it however their API wants.
type Repo string

// Client is what an adapter provides.
type Client interface {
	// CreateIssue opens one and returns it as created.
	CreateIssue(ctx context.Context, repo Repo, in NewIssue) (Issue, error)

	// FindIssue reads one back. It is how linking an existing issue works —
	// somebody pastes a URL, and this is what proves the issue is real, says
	// what state it is in, and gives the title to show.
	FindIssue(ctx context.Context, repo Repo, number int64) (Issue, error)
}

// EventKind is what a delivery means here, after the adapter has translated it.
type EventKind string

const (
	// EventIgnored is most of them. A forge sends deliveries for a great many
	// things, and this product cares about three.
	EventIgnored  EventKind = "ignored"
	EventOpened   EventKind = "opened"
	EventClosed   EventKind = "closed"
	EventReopened EventKind = "reopened"
	EventLabelled EventKind = "labelled"
)

// Event is a webhook delivery, translated.
//
// DeliveryID is what makes a redelivery — which every forge does, and which
// anybody who captured one can replay — into something recognisable.
type Event struct {
	Kind       EventKind
	Number     int64
	State      State
	Labels     []string
	DeliveryID string
}

// Receiver is the other half of an adapter: verifying and translating what
// arrives.
//
// Verify is separate from Parse on purpose. Nothing about a delivery may be
// read as meaning anything until its signature has been checked, and keeping
// them apart makes an implementation that forgets impossible to write by
// accident.
type Receiver interface {
	// Verify checks the signature. Every forge does this differently — GitHub
	// signs the body, GitLab signs a composed message of id, timestamp and
	// body, Gitea signs the body with a different header — so there is no
	// generic verifier, and writing one would accept what it should not.
	Verify(headers http.Header, body []byte, secret string) error

	// Parse translates a verified delivery.
	Parse(headers http.Header, body []byte) (Event, error)
}
