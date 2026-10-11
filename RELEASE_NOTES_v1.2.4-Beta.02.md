**The house colours on every screen, Go 1.27.2 and updated dependencies**

### Changed
- The panel is laid on the page's own ground, with its menu and cards one step above it, in both themes: nothing is the colour of what it sits on any more.
- The light background, the two text greys, the light border and the dark text are the values the rest of the family uses, so the product reads the same next to its siblings.
- Go 1.27.2 and x/net 0.60.0, which fix the standard-library and HTTP/2 advisories the scan reports.
- The toolchain and dependencies are up to date: Bun 1.4.3, Docker CLI 29.9.0, buildx 0.38.0, and the Go modules crypto, image, net and SQLite.
- The vulnerability scan runs before a release is tagged, so a release that reaches an advisory cannot be cut.
