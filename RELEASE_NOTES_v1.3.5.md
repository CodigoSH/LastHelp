### Added
- The menu of filters is laid out as cards, one per group, all the same height and in one row, with a separator between options, a count of how many are ticked in each group, and a footer with Clear and Apply. On a phone it comes up from the bottom as a sheet that can be dragged down to close, like a native app.
- The team's archive can be narrowed by date: reports from the last day, the last week, the last month, or older.
- Several waiting reports can be decided at once: Select, tick the ones you want, and publish, reject or mark them as spam together. Rejecting and spam ask for confirmation once, only reports still waiting for review can be ticked, the ones you are not allowed to decide are skipped and counted, and a single request carries up to 100.

### Changed
- No team reply is a switch aligned with the other controls of the filter bar, instead of a button that looked like one more menu.
- Pending review is now Pending, short enough to read on one line in every language.
- Built with Go 1.27.4, which fixes GO-2026-6859, a crash of the HTTP server in the standard library.
