# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Public boards where anybody can report a bug or suggest an idea without an
  account, follow it with a secret link, and vote on ideas.
- A moderation queue: nothing reaches a public page without a person approving
  it.
- Similar-item hints while you type a title, and full-text search, both from one
  local index.
- A public roadmap fed by what actually happens rather than maintained by hand.
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
- Twenty languages including three that read right to left.
- Reproducible builds and signed checksums.
- Project skeleton: build, container image, release script and the architecture
  rules that the rest of the code is written against.
