**A redesigned public side, and a content policy on every response**

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
