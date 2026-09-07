# LastHelp

A self-hosted feedback portal that speaks to your forge.

Anyone can report a bug or suggest an idea **without creating an account**.
Ideas gather votes. You triage what arrives and turn what deserves it into a
real issue in your own forge — GitHub, GitLab, Forgejo or Gitea, hosted by
someone else or by you. When that issue is closed, the item's status follows on
its own.

Made for a developer or a small group of developers. One binary, one SQLite
file, no external services.

## Status

**Early development, and no release yet.** This section says what actually
works, because a README that describes a server nobody wrote is the first thing
that makes people stop trusting one.

Working today, end to end and driven through a real browser: organisations and
boards, reporting without an account, the secret follow link, the moderation
queue, publishing, voting on ideas, comment threads, similar-item hints while
you type, a public roadmap nobody maintains by hand, and the whole interface in
twenty languages including right-to-left.

Also working: the anti-abuse layers — a signed single-use form token, a timing
window, a honeypot, rate limits keyed on the address a caller cannot choose,
duplicate and link heuristics, an operator word list, and a circuit breaker that
closes a board rather than hand somebody a queue they will not open. Every
refusal is counted by layer, because a filter nobody can measure is a filter
quietly turning away people who never complain.

The forge bridge works too: GitHub, GitLab, Gitea and Forgejo, hosted anywhere
including your own network. A triaged item becomes an issue — or is attached to
one that already exists, which is what a project with four thousand open issues
actually needs — and when that issue is closed or reopened, the item follows.
That is what makes the roadmap something nobody has to maintain.

Notifications work in both directions they need to. The team is told something
arrived — ntfy, Gotify, a generic webhook, or mail — configured per board, so a
group can send one board to one room and another somewhere else. Whoever
reported something can follow the board's Atom feed, which needs no address, no
account and no configuration at all, or leave an email if a relay is set up:
that address is stored encrypted, is visible only to the team, and is deleted
when the item closes.

Not there yet: Bitbucket, and a first release — there is no tag, so there are no
binaries to download and no signing key in `internal/version/keys.go` yet.

## Why it exists

Self-hosting your forge usually means turning registration off, and that closes
the only door outsiders had for telling you something is broken. Opening
registration to the world is not the answer; a separate front door is.

The open-source feedback tools do not talk to any forge. The ones that do are
closed, paid, and hosted by someone else. Nobody serves the person who
self-hosts their own forge — which is, almost always, the same person who would
self-host a feedback portal.

## What it will not do

- No tracking, no telemetry, no analytics.
- No AI: nothing anybody writes here is sent to a third party.
- No paid tier. What exists is in the binary.
- No accounts for the public. Reporting is anonymous, and a secret link is what
  lets a reporter come back to their own report.

## Build

Requires Go and [bun](https://bun.sh).

```sh
make build     # frontend + binaries into bin/
make run       # build and run
make test      # go test and the architecture rules
make drive     # walk the whole journey in a real browser
make shots     # regenerate the documentation screenshots
```

The frontend is stamped with a digest of its own output, and the release
binaries are built with `-trimpath` and `CGO_ENABLED=0`, so two builds of the
same commit produce the same bytes — see [SECURITY.md](SECURITY.md) for how to
check that yourself.

## Documentation

[docs/](docs/) — installing, your first board, connecting a forge, notifications,
and keeping the queue readable.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
