**Private boards, push notifications, a new overview and roadmap, and a board page that reads in lanes**

### Added
- An organisation or a repository can be private. A private board answers the public with the same "not found" as one that does not exist, on every page, form, feed, vote, comment, picture and search; the administrator, the organisation's owners and the repository's collaborators still see it. Lists mark what is private, and nothing already published changes: every existing board stays public.
- Push notifications for the team. Each person turns them on for their own browser in their profile, and a new report or a comment waiting for review arrives as a notification written in the language of that account. It reaches the administrator, the organisation's owners and the repository's collaborators, and nobody else. Push goes through the browser vendor's push service, the only notification that passes through a third party, and it is off until somebody turns it on.
- A new front page for the panel: what waits and for how long, who got no answer, how fast the team answers, a chart with a line per kind and a range of a week, a month or a quarter, and each repository's own detail, each of them opening in a dialog.
- A new roadmap page: four tiles with their numbers, In progress, Planned, Shipped and Declined, a filter by kind, a search, Planned ordered by votes, Shipped by month, and Declined shut until it is asked for. Every lane can be opened whole in a dialog with its own search, filter and order.
- A new repository page. The reports read in lanes, ideas and bugs side by side, or done and not done when looking at what is closed, each lane limited to a few rows with a dialog to read, search and order all of it. The tiles at the top hold the numbers, open and closed with what was done and what was not, and the roadmap is a tile that leads to its page.

### Changed
- A notification channel the whole team reads says what arrived with a mark, a report or a comment, instead of a sentence in one language, and its title is the board's name. A webhook receives that mark as a `kind` code.
- Ideas and bugs share one vocabulary of states, including In progress and Declined for bugs, and every screen says it the same way.
- What a board accepts, and whether an organisation or a board is private, are switches on a row, like the rest of the panel's settings.
- The list of organisations no longer carries a rename icon: opening an organisation edits its name with the rest.
- Go 1.27.3, Bun 1.4.4 and SQLite 1.61.0, with the x/net module updated.

### Fixed
- The overview never cuts a label, lists a state even when nobody is in it, and is read in each of the twenty languages without a plural that does not agree.
- A month in an age is no longer written like a minute.

### Upgrading
- The database is migrated twice when the new version starts, once to add the private marker and once for the push devices. As with every update, the server keeps a copy of the database first.
