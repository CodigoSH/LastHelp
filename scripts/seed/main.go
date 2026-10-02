// Command seed fills a database with an installation that has been in use for
// a while: an administrator, an owner and a collaborator, an organisation with
// two repositories, six months of reports in every state — with votes,
// comments and the team's replies — and some that are still waiting.
//
// It exists for the browser pass of the reports screen, and for trying the
// product by hand. A pass that runs against one pending report and no history
// never sees what a team sees on its second month: a list that scrolls, a
// search that has to find something, a filter that has to leave some out.
// Walking it once with content found a fault nothing else had — the pane asked
// the server three things per row, and holding an arrow key was refused.
//
// It writes through the same stores the server uses, never around them, so
// what it produces is something the server could have produced. It goes
// through no form: the public form is guarded against exactly this — many
// reports from one address in a few seconds — and bending it for a test would
// be testing a door with the lock taken off.
//
// Not part of any release: nothing in cmd/ imports it, and the Makefile builds
// cmd/lasthelp.
//
//	go run ./scripts/seed -db /path/to/new.db
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// Password is the one every seeded account signs in with.
const Password = "una contraseña larga de verdad"

// report is one seeded report: where, what, how old, and what became of it.
type report struct {
	repo       string
	kind       item.Kind
	title      string
	body       string
	daysAgo    int
	moderation item.Moderation
	status     item.Status
	votes      int
	// The conversation, oldest first: who said it and what.
	talk []line
}

type line struct {
	who  item.Author
	body string
	// A visitor's comment that is still waiting for the team.
	waiting bool
}

// The history, then what waits. Titles are distinct enough to be searched for
// one at a time.
var reports = []report{
	{"atlas", item.Bug, "Crash when importing a CSV with empty lines",
		"Importing a CSV that has a blank line in the middle stops the import and the app closes.\n\nExpected the blank line to be skipped.",
		170, item.Published, item.StatusClosed, 12,
		[]line{{item.AuthorVisitor, "Same here with files exported from a spreadsheet.", false},
			{item.AuthorTeam, "Fixed: blank lines are skipped now. Thanks for the file.", false}}},
	{"atlas", item.Idea, "Dark mode for the dashboard",
		"The white is very bright at night. A dark theme would help.",
		160, item.Published, item.StatusDone, 31,
		[]line{{item.AuthorTeam, "Shipped: the theme follows the system, or can be chosen.", false}}},
	{"atlas", item.Bug, "Dates shown in the wrong time zone",
		"Every date in the list is two hours off. The server is in UTC and I am in Madrid.",
		150, item.Published, item.StatusClosed, 4,
		[]line{{item.AuthorReporter, "It was my browser's setting, sorry.", false},
			{item.AuthorTeam, "No problem, closing it.", false}}},
	{"atlas", item.Idea, "Export the reports to CSV",
		"We would like to take the list into a spreadsheet once a month.",
		140, item.Published, item.StatusPlanned, 18, nil},
	{"atlas", item.Bug, "Search ignores accents",
		"Searching for 'cafe' does not find 'café'.",
		120, item.Published, item.StatusConfirmed, 9,
		[]line{{item.AuthorTeam, "Confirmed. Working on accent-insensitive search.", false}}},
	{"atlas", item.Idea, "Keyboard shortcuts for the list",
		"J and K to move, like in mail clients.",
		110, item.Published, item.StatusDeclined, 2,
		[]line{{item.AuthorTeam, "Not for now: the list is short enough to use with the mouse.", false}}},
	{"atlas", item.Bug, "Logo upload fails with large PNG files",
		"A 6 MB PNG gives a generic error.",
		95, item.Published, item.StatusNotReproducible, 1,
		[]line{{item.AuthorTeam, "Could not reproduce with a 6 MB file. Which browser?", false}}},
	{"atlas", item.Idea, "Weekly summary by email",
		"A short email every Monday with what changed.",
		80, item.Published, item.StatusNew, 7, nil},
	{"atlas", item.Bug, "Buy cheap watches online",
		"Best prices on watches, click the link in my profile.",
		75, item.Spam, item.StatusNew, 0, nil},
	{"atlas", item.Bug, "The page is broken",
		"It does not work.",
		70, item.Rejected, item.StatusNew, 0, nil},
	{"atlas", item.Bug, "Sort by votes forgets the chosen board",
		"After sorting by votes the board filter goes back to all boards.",
		60, item.Published, item.StatusConfirmed, 5,
		[]line{{item.AuthorVisitor, "Also happens when sorting by date.", false}}},
	{"atlas", item.Idea, "Attach screenshots to a report",
		"A picture explains a layout bug faster than a paragraph.",
		45, item.Published, item.StatusPlanned, 22,
		[]line{{item.AuthorVisitor, "Please, this would save so much back and forth.", false},
			{item.AuthorTeam, "Planned for the next minor version.", false},
			{item.AuthorVisitor, "Could it accept pasting from the clipboard too?", true}}},
	{"orbit", item.Bug, "Notifications arrive twice",
		"Every new report sends two identical notifications to our chat.",
		130, item.Published, item.StatusClosed, 6,
		[]line{{item.AuthorTeam, "Fixed in the last release.", false}}},
	{"orbit", item.Idea, "Show the version on the public page",
		"Visitors ask which version we run before reporting.",
		100, item.Published, item.StatusInProgress, 3, nil},
	{"orbit", item.Bug, "Mobile menu does not close",
		"On a phone the menu stays open after choosing a page.",
		50, item.Published, item.StatusConfirmed, 8,
		[]line{{item.AuthorVisitor, "Same on a tablet in portrait.", false},
			{item.AuthorVisitor, "And on Firefox for Android.", true}}},
	{"orbit", item.Idea, "Two columns on wide screens",
		"The list uses a third of a 27-inch screen.",
		30, item.Published, item.StatusNew, 4, nil},
	{"orbit", item.Bug, "Password field accepts spaces only",
		"A password of ten spaces is accepted at sign-up.",
		20, item.Published, item.StatusConfirmed, 2, nil},
	{"orbit", item.Idea, "Free money generator",
		"Visit my site for free money.",
		15, item.Spam, item.StatusNew, 0, nil},

	// Waiting for the team.
	{"atlas", item.Bug, "Backup restore hangs at 90 percent",
		"Restoring a 200 MB backup stops at 90% and never finishes.\n\nThe server log shows nothing.",
		3, item.Pending, item.StatusNew, 0, nil},
	{"atlas", item.Idea, "Pin a report to the top of the board",
		"Some announcements should stay first.",
		2, item.Pending, item.StatusNew, 0, nil},
	{"atlas", item.Bug, "Typo in the sign-in page",
		"It says 'Pasword' under the field.",
		1, item.Pending, item.StatusNew, 0, nil},
	{"orbit", item.Bug, "Export button does nothing on Safari",
		"Clicking Download on Safari 18 does nothing at all.",
		2, item.Pending, item.StatusNew, 0, nil},
	{"orbit", item.Idea, "Let visitors follow a report",
		"I would like to be told when a report I voted for changes.",
		1, item.Pending, item.StatusNew, 0, nil},
}

func main() {
	path := flag.String("db", "", "the database file to create; must not exist")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "usage: seed -db <new database file>")
		os.Exit(2)
	}
	// Never over an installation somebody uses: a seed is a new database.
	if _, err := os.Stat(*path); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists; the seed only writes a new database\n", *path)
		os.Exit(2)
	}
	if err := seed(*path, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func seed(path string, now time.Time) error {
	db, err := database.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	start := now.AddDate(0, -7, 0)
	accounts := account.Store{DB: db}
	orgs := org.Store{DB: db}
	items := item.Store{DB: db}

	if err := (settings.Store{DB: db}).Set(settings.InstanceName, "Acme"); err != nil {
		return err
	}
	if _, err := accounts.Create("alex", Password, true, start); err != nil {
		return fmt.Errorf("administrator: %w", err)
	}
	owner, err := accounts.Create("olga", Password, false, start)
	if err != nil {
		return fmt.Errorf("owner: %w", err)
	}
	collaborator, err := accounts.Create("ana", Password, false, start)
	if err != nil {
		return fmt.Errorf("collaborator: %w", err)
	}

	acme, err := orgs.CreateOrganization(org.Organization{Slug: "acme", Name: "Acme", Forge: org.ForgeNone}, start)
	if err != nil {
		return fmt.Errorf("organisation: %w", err)
	}
	repos := map[string]org.Repository{}
	for _, r := range []org.Repository{
		{OrgID: acme.ID, Slug: "atlas", Name: "Atlas", AcceptsBugs: true, AcceptsIdeas: true},
		{OrgID: acme.ID, Slug: "orbit", Name: "Orbit", AcceptsBugs: true, AcceptsIdeas: true},
	} {
		made, err := orgs.CreateRepository(r, start)
		if err != nil {
			return fmt.Errorf("repository %s: %w", r.Slug, err)
		}
		repos[r.Slug] = made
	}
	if err := accounts.Grant(owner.ID, authz.Organization(acme.ID), authz.RoleOwner, start); err != nil {
		return err
	}
	atlas := repos["atlas"]
	if err := accounts.Grant(collaborator.ID, authz.Repository(atlas.ID, acme.ID), authz.RoleCollaborator, start); err != nil {
		return err
	}

	// Older history on Atlas, enough that its open side runs past one page of
	// the public list: every fourth one answered, the rest still open.
	all := append([]report(nil), reports...)
	for i := 0; i < 30; i++ {
		r := report{"atlas", item.Idea, fmt.Sprintf("Older idea %02d from the first months", i+1),
			"An idea from before most of the team arrived.", 200 - i, item.Published, item.StatusNew, i % 5, nil}
		if i%2 == 1 {
			r.kind, r.title, r.votes = item.Bug, fmt.Sprintf("Older bug %02d from the first months", i+1), 0
		}
		if i%4 == 0 {
			r.status = map[item.Kind]item.Status{item.Idea: item.StatusDone, item.Bug: item.StatusClosed}[r.kind]
		}
		all = append(all, r)
	}

	for _, r := range all {
		when := now.AddDate(0, 0, -r.daysAgo)
		fields := []item.Field{}
		for _, spec := range item.FieldsFor(r.kind) {
			spec.Value = map[string]string{"version": "1.4.2", "steps": "1. Open the page\n2. Do what the title says"}[spec.Key]
			if spec.Value != "" {
				fields = append(fields, spec)
			}
		}
		made, _, err := items.Create(item.New{RepoID: repos[r.repo].ID, Kind: r.kind, Title: r.title, Body: r.body, Fields: fields}, item.Pending, when)
		if err != nil {
			return fmt.Errorf("%q: %w", r.title, err)
		}
		later := when.Add(6 * time.Hour)
		switch r.moderation {
		case item.Published:
			err = items.Approve(made.ID, later)
		case item.Rejected:
			err = items.Reject(made.ID, later)
		case item.Spam:
			err = items.MarkSpam(made.ID, later)
		}
		if err != nil {
			return fmt.Errorf("%q: moderation: %w", r.title, err)
		}
		// The votes and the conversation come first, as they did: an answered
		// report takes no more of either, so the team answers after the people
		// have spoken.
		// Only ideas are voted on; a bug is confirmed, not liked.
		for v := 0; r.kind == item.Idea && v < r.votes; v++ {
			if err := items.Vote(made.ID, fmt.Sprintf("seed-voter-%d", v), later); err != nil {
				return fmt.Errorf("%q: vote: %w", r.title, err)
			}
		}
		for i, l := range r.talk {
			said := later.Add(time.Duration(i+1) * 36 * time.Hour)
			// The server writes the team member's name, and nothing for anybody
			// else: a visitor has no name here.
			ref := ""
			if l.who == item.AuthorTeam {
				ref = "alex"
			}
			c, err := items.Comment(made.ID, l.who, ref, l.body, said)
			if err != nil {
				return fmt.Errorf("%q: comment: %w", r.title, err)
			}
			if !l.waiting && c.Moderation == item.Pending {
				if err := items.ApproveComment(c.ID); err != nil {
					return fmt.Errorf("%q: comment: %w", r.title, err)
				}
			}
		}
		if r.status != item.StatusNew {
			answered := later.Add(time.Duration(len(r.talk)+1) * 36 * time.Hour)
			if err := items.SetStatus(made.ID, r.status, answered); err != nil {
				return fmt.Errorf("%q: status: %w", r.title, err)
			}
		}
	}
	return nil
}
