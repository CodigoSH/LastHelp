# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **The form for reporting something was not laid out.** Its fields were
  stacked into a narrow column with the send button adrift beside them, on the
  one page the product exists to collect. Two different things shared a class
  name, so a rule written for a row in the moderation queue was being applied to
  the form.
- Notices were tinted with a hard-coded orange, so an operator who set their own
  accent colour got every other tinted surface in their colour and their notices
  in ours.
- On a report opened by the team, the merge block sat against the reply form's
  button and read as that form's caption.
- Upgrading over an installation made by an earlier installer is recognised as
  an upgrade again: it was asking every question a second time and then refusing
  because the port was busy — held by the very service it was about to replace.
  The port check now asks whose process it is rather than assuming.
- A key file that an instance run from a working copy had left behind was
  committed to this repository and stayed there for eight days. It is gone from
  the history, `*.key` and `*.pem` are ignored, and a check now asks git — not
  the filesystem — whether anything shaped like an instance secret is tracked.
  That key had sealed nothing: the database it belonged to had no forge
  connected and both encrypted columns were empty. Nothing had to be rotated.

### Changed

- **Secrets are stored as they are given.** The forge token, the webhook secret
  and a reporter's email address were sealed with a key kept in a file beside
  the database; they are now columns like any other. The key protected against
  nobody who could reach the machine — it travelled in the same directory as the
  data — while making a restore on another machine fail silently and adding a
  file that could leak, and did. Upgrading moves the sealed values across on the
  first start and says so in the log; the key file is then read by nothing and
  can be deleted. An installation whose key is already lost is told which values
  have to be entered again. Account passwords are unaffected: they are hashed
  with Argon2id and always were.

- The setup wizard and the sign-in screen are each one card — mark, heading,
  fields and the way on — instead of a mark floating above a card with the
  button below it.
- The theme is chosen by pointing at one of three tiles rather than by reading
  three words in a bar.
- Dark mode uses neutral greys rather than blue-black ones, and cards carry the
  light hairline along their top edge that keeps a dark card from reading as a
  hole in the page.
- Lists and groups — the organisations, the repositories, the board under its
  filter, each roadmap column — are drawn as a bordered section with its name on
  the border and the count beside it.
- The installer speaks the same words as the ones the other projects ship, and
  the command to install is the same everywhere: it asks which port, which
  release channel and whether a proxy is in front, and remembers the answers.

## [1.0.0-Beta.03] - 2026-09-09

### Added

- The installer can undo itself: `--uninstall` removes it and keeps the
  reports, `--uninstall --purge` erases them too after you type DELETE.
- A configuration file at `/etc/lasthelp/lasthelp.env`, written once on a fresh
  install and never overwritten afterwards — an upgrade keeps the port, the
  proxy setting and anything else you edited by hand.
- The service unit ships with the release and is listed in the signed
  `SHA256SUMS`, so the sandbox the service runs inside is verified like the
  binary rather than being printed by whatever copy of the installer you ran.
  It carries the full set of systemd restrictions the rest of the house uses.

### Changed

- The installer is POSIX `sh` rather than bash, so the documented
  `curl … | sudo sh` runs under dash on Debian and Ubuntu and under BusyBox on
  Alpine, not only where `/bin/sh` happens to be bash. Install with `sudo sh`;
  the beta channel is now `LASTHELP_CHANNEL=beta`.
- The binary lives at `/opt/lasthelp/bin/lasthelp`, owned by the service
  account, which is what lets the built-in updater replace it.

### Fixed

- The installer refuses a port that something else is already listening on,
  instead of installing a service that cannot start.

## [1.0.0-Beta.02] - 2026-09-09

### Added

- A content security policy on every response, with no `unsafe-inline` in it:
  scripts and styles load from this server or not at all. The interface keeps
  the handful of values only the browser knows — an accent colour, a tooltip
  position — through the CSSOM rather than through style attributes.
- HSTS over TLS, and nothing private in a search index: the panel, the sign-in
  page and any link carrying a follow token say so in their own headers.
- The list of security controls is a test rather than a document: each control
  names the test that keeps it true, and deleting one fails with the control's
  name in the message.
- A public side worth arriving at: the landing page, the boards, the roadmap,
  the report pages and every form, with a mark of its own and a sign-in page to
  match.
- An overview that says more than how many: the last thirty days as a chart that
  breaks a day down on hover, and cards that open the listing behind them.
- A listing of all the work, showing each person what their roles allow —
  waiting, open and closed — with the triage decision on the line of the state
  it changes.

### Changed

- The dashboard is one screen for the whole team rather than an arrangement each
  account keeps for itself.
- A report waiting for review no longer shows as open, and the state it does
  show is called what it is.
- Signing out says so, in red, and the panel's own vocabulary and highlight
  follow the rest of the house.
- Releases are built for what the installer can install: Linux on 64-bit Intel
  and ARM. The list is now checked against the installer during the build.
- The new screens are translated into all twenty languages.

### Fixed

- The page shown after sending a report was a dead end: it now leads back to the
  board and to the report itself, from opposite ends of the row.
- A report opened from the panel keeps the panel around it, so there is a way
  out of it for a signed-in member of the team.
- The moderation queue could not be decided from, and the decision now sits in a
  card of its own rather than among the tools below it.
- Signing in reads the field the form actually has, and finishing the setup
  wizard accepts the logo it asked for.
- Pictures are made to fit before they are stored, and every refusal reaches the
  page in words a person can read.

## [1.0.0-Beta.01] - 2026-09-08

### Added

- Public boards where anybody can report a bug or suggest an idea without an
  account, follow it with a secret link, and vote on ideas.
- A moderation queue: nothing reaches a public page without a person approving
  it.
- Similar-item hints while you type a title, and full-text search, both from one
  local index.
- A public roadmap fed by what actually happens rather than maintained by hand.
- A report page that stands on its own: the board it belongs to and the way back
  to it, its state, the vote, the thread, and the reporter's link to their own
  submission.
- Comment threads, held for a person to read and published from the same queue
  as the reports.
- A bug is asked which version and how to see it happen. Both travel with it:
  onto the public page, into the export and into the forge issue.
- A bridge to GitHub, GitLab, Gitea and Forgejo, hosted anywhere including a
  private network: turn an item into an issue or attach one that already exists,
  and the item follows when that issue closes or reopens.
- Notifications for the team over ntfy, Gotify, a generic webhook or mail,
  configured per board; an Atom feed and optional mail for whoever reported
  something.
- Seven layers between a public form and the moderation queue, with every
  refusal counted by the layer that made it.
- Accounts and roles: administrator of the server, owner of an organisation and
  its credentials, collaborator of one repository.
- An administration panel: a dashboard of cards each account arranges and hides
  for itself, the organisations and repositories, the people and what they may
  do, and the settings of the installation.
- Backups in one file, taken as a consistent snapshot rather than copied out
  from under a running server. Restoring checks the file before replacing
  anything and keeps the database it replaced.
- The name, the mark and the accent colour this installation wears, and a theme
  and a language each visitor chooses for themselves.
- Update checks against signed releases, on the stable channel or the beta one.
- Twenty languages including three that read right to left.
- Reproducible builds and signed checksums.
