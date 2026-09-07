# LastHelp

A self-hosted feedback portal that speaks to your forge.

Anyone reports a bug or suggests an idea **without an account**. Ideas gather
votes. You triage what arrives and turn what deserves it into a real issue on
GitHub, GitLab, Gitea or Forgejo — including one on your own network. When that
issue closes, the item follows, which is what makes the roadmap something nobody
has to maintain.

One binary, one SQLite file, no external services.

![A board](images/board.png)

- [Install](getting-started/install.md)
- [Your first board](getting-started/first-board.md)
- [Connecting a forge](forges/connecting.md)
- [Notifications](administration/notifications.md)
- [Keeping the queue readable](administration/anti-abuse.md)

## What it does not do

- No tracking, no telemetry, no analytics.
- No AI. Nothing anybody writes here is sent to a third party.
- No paid tier. What exists is in the binary.
- No accounts for the public. Reporting is anonymous, and a secret link is what
  lets a reporter come back to their own report.
