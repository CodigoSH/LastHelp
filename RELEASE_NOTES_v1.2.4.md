**Replies follow renamed accounts, History with open and closed, and the house colours**

### Added
- The archive tab, now called History, splits the published reports into All, Open and Closed, each with the number it shows. Under Closed, Done and Not done tell a delivery from a report the team turned down.
- In History, a repository's heading folds its reports away and opens them again, and the choice is kept in your browser.

### Changed
- The panel is laid on the page's own ground, with its menu and cards one step above it, in both themes, and the light background, the two text greys, the light border and the dark text are the values the rest of the family uses.
- Go 1.27.2 and x/net 0.60.0, which fix the standard-library and HTTP/2 advisories the vulnerability scan reports. The scan now runs before a release is tagged, so a release that reaches an advisory cannot be cut.
- The toolchain and dependencies are up to date: Bun 1.4.3, Docker CLI 29.9.0, buildx 0.38.0, and the Go modules crypto, image, net and SQLite.

### Fixed
- A team reply is signed with the name the account has now: renaming an account used to leave its earlier replies under the old name. Replies written before this version are matched to the account that holds their name today, and a rename claims the ones still signed with the name it leaves.
