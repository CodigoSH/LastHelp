**A picture beside every name, and a safer text for the forge**

### Added
- Replies and comments no longer draw initials. Everybody gets a picture beside their name: one figure in a light grey for whoever reported or commented, and the same figure with its colours turned round and a stronger grey for the team.
- A team member's own picture shows beside their replies on public pages, and in the panel's lists and menu. It is public, like their name already was; somebody who has chosen none keeps the default.

### Fixed
- What a stranger wrote is made inert before it reaches the forge. An issue body is read there as Markdown, so a mention notified people, an image loaded from a host the writer chose, a link hid its destination and, on one forge, a line starting with a slash could run a command. The report text and every answer now have those characters escaped, with code left exactly as written.
