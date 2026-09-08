# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
