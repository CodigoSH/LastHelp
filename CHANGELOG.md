# Changelog

Every released version, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

An entry is written when a stable is cut. A beta is not a released version: it
gets its own release page, with the same sections, and its changes reach this
file with the stable that carries them.

1.0.0 is the first stable release, so its entry describes the whole product
rather than a change from an earlier version.
## [1.2.1] - 2026-10-05

### Changed
- A saved token or secret — a notification channel's token, an organisation's forge token and its webhook secret — is shown as dots: it is there, and which one it is cannot be seen, and there is no eye to reveal it. Write over the dots to replace it, empty the field to remove it, or leave the dots to keep it. The checkbox that removed a channel's token is gone.
- An organisation's forge token and webhook secret are changed one at a time: replacing one leaves the other, and an empty field keeps what is stored. Removing the token disconnects the organisation from its forge.

### Fixed
- Saving only the forge token, as the setup wizard does, no longer empties a webhook secret that was already stored.

## [1.2.0] - 2026-10-05

### Added
- An **Anti-abuse** screen under *This server*: the word list as a form and a list, with edit and remove in each row, a search and pages of ten, how many reports each word has refused, and how many submissions each check turned away, by repository.
- When an administrator rejects a report from the queue, the word that gave it away can be added to the list in the same step.
- A notification channel for the whole server, set once under *Notifications* and chosen by each repository in its own list. A repository that does not choose it is not affected.
- A **Send a test** button on notification channels, which says why a message did not arrive: a refused token, an address that does not exist, a service that cannot be reached, no mail relay, or an installation working offline.
- **Ctrl+K** (⌘K on a Mac) goes to the search box on every screen.

### Changed
- The word list matches whole words, so "ass" no longer refuses "class", and finds a word anywhere in a sentence in Chinese, Japanese, Korean and Thai, which have no spaces. Capitals and accents do not matter. A list written for the old matching may need a word or two added.
- A repository's notification form shows the saved address and recipient after saving. The token is never shown; leave it empty to keep the stored one, or tick the box to remove it.
- An answered report stays open to its reporter and to the team for 48 hours after the answer, and the page says until when. Visitors still cannot comment as soon as it is answered.
- A board's card on the front page counts every report it has had, answered or not, instead of only the open ones.
- The reports screen's search shortcut is Ctrl+K instead of "/", and the board page's search uses the same box as the rest of the product.

### Fixed
- Usernames can no longer contain `<`, `>`, `"`, `'`, `&` or control characters. Accounts that already have them keep their names.
- The address a reporter leaves must be one plain mailbox.
- In a narrow reports pane, the sentence beside the approve, reject and spam buttons was squeezed into a column of a few letters.

## [1.1.0] - 2026-10-03

A screenshot is often the quickest way to show a bug, so reports can now carry them. Installations update in place — from the panel, the installer or a new image — and keep everything they hold; the update adds the table the pictures live in.

### Added
- **Pictures on reports.** The person reporting can attach up to five screenshots to a report and three to each follow-up, and the team can add three to each reply. Nothing is shown until the team has looked, and it can take out a single picture and approve the rest. The server draws every picture again, so the place and the camera a photo carries are not kept.
- A picture opens large over the page, with the arrows to move between them and Escape to go back.
- A switch per repository for pictures, in its settings.
- Each repository card on the front page shows how many bugs and ideas are open, and the two kinds wear their colours wherever they appear.

### Changed
- An answered idea or bug takes no more votes, comments or replies. The thread stays, and reopening the report opens it again.
- Pictures are erased 30 days after their report closes, and at once when a report or a message is rejected, marked as spam or deleted.
- The limits on the words of a report count characters, not bytes: a report in Japanese or Chinese was refused at about a third of what English was allowed.
- After updating, every repository takes pictures. Turn them off in its settings if you prefer.

### Fixed
- Pictures can be added to the front page from Safari on a Mac or an iPhone.
- Long words with no spaces, like a pasted address or a log, stay inside the report's card instead of running off it, most of all on a phone.
- A vote or a comment on a report that was folded into another or turned down is refused instead of being accepted and lost.
- A build made between two releases is no longer offered the release before it as an update.

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

