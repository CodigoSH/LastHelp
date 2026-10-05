**Anti-abuse, notifications and a grace window**

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
