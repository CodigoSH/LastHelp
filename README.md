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

**Early development.** The repository is being built in the open and there is no
release yet. What exists today is the skeleton: the server boots, serves its
embedded frontend and answers a health check. Nothing described above is
implemented yet, and this section will say so until it is — a README that
promises a server nobody wrote is the first thing that makes people stop
trusting one.

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
make test      # go test, the architecture rules, bun test
```

The frontend is stamped with a digest of its own output, so two builds of the
same commit produce the same bytes.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
