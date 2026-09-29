# Changelog

Every released version, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

An entry is written when a stable is cut. A beta is not a released version: it
gets its own release page, with the same sections, and its changes reach this
file with the stable that carries them.

1.0.0 is the first stable release, so its entry describes the whole product
rather than a change from an earlier version.
## [1.0.0] - 2026-09-29

The first stable release of LastHelp, a self-hosted feedback portal that speaks to your forge. There is no earlier stable to compare it with, so these notes describe what LastHelp is rather than what changed. Installations that followed the betas update in place — from the panel, the installer or a new image — and keep everything they hold. Two things are not there yet: a bridge to Bitbucket, and a screen in the panel for the anti-abuse word list and the refusal counts, which work and are set and read as the documentation describes.

### Added
- **Reporting without an account.** Anybody can send a bug or an idea from one form. What they get back is a secret link to their own report, which is why no sign-up is needed.
- **Bugs and ideas as two journeys.** A bug is asked which version and how to see it happen, and is not voted. An idea is one sentence, gathers votes and ranks by them.
- **Similar reports while typing**, so the same problem gathers votes and comments instead of duplicates.
- **A public board per repository**, with open and closed reports apart, filters and search kept in the address, and a page at a time.
- **A roadmap nobody maintains.** Three columns built from the state of each report, so it cannot go out of date.
- **Reports, the team's one screen.** Every repository you work on in one list, with the report beside it: publish, reject or spam with a key each, edit its words, change its status, reply, edit or delete any comment, merge a duplicate, or open or link a forge issue. The same screen is the archive of every report ever sent.
- **An overview** of the last thirty days, the most wanted ideas, the newest reports and the work per repository, each number leading to the list it counts.
- **Organisations and repositories with two levels of trust.** A lead runs an organisation, its forge token and its repositories. A collaborator triages the repositories they are given and never sees the token. Both can carry an icon.
- **A front page you compose** from bands of modules — the repositories, the most voted ideas, what was resolved, statistics, links, and your own words and images — arranged on a grid in the page itself.
- **A bridge to GitHub, GitLab, Gitea and Forgejo**, including forges on your own network. A report becomes an issue or is attached to an existing one, and follows it when it closes or reopens. Work goes through a queue with backoff, so a forge that is down delays an issue instead of losing a report. Running without a forge is supported.
- **Notifications** for the team through ntfy, Gotify, a webhook or mail, per repository. Reporters can follow an Atom feed that needs no account, or leave an email when a mail relay is configured; the address is deleted when the report closes.
- **Anti-abuse without a third party.** A signed single-use form token, a timing window, a honeypot, rate limits by address and network block, duplicate and link heuristics, an operator word list, and a breaker that closes a repository for a while. Nothing is published without a person, and every refusal is counted by layer.
- **Signed releases and a verified updater.** Checksums are signed with Ed25519. The updater in the panel checks the signature and the checksum, makes the new binary prove it runs, refuses one older than the database and takes a backup before it swaps, showing each step.
- **One binary, one SQLite file.** A one-line installer with a hardened systemd service, `--uninstall` and `--purge`, and Docker images for `linux/amd64` and `linux/arm64`.
- **Twenty languages**, translated by hand, with right-to-left layout for Arabic, Persian and Urdu.
- **Installable as an app** from the browser when served over HTTPS.

