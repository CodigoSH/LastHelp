**A notice when a new version is out, a way back for a forgotten password, and a sitemap for the public pages**

### Added
- Administrators can be told when a newer version is out. Under Notifications, in the card for the server's channel, the switch New versions (off until you turn it on) tells them through that channel: push to the administrators' own devices, in the language of each account, or ntfy, Gotify, mail or a webhook. The server looks every hour while it is on and as soon as you turn it on, says each version once, and counts a version as told only when the channel took it.
- `lasthelp password reset <username>` sets a forgotten password from a terminal on the machine the server runs on. It asks twice without showing what is typed, needs at least ten characters, and closes that account's sessions. In a container: `docker exec -it lasthelp lasthelp password reset <username>`.
- Five wrong passwords for one name inside a minute lock that name for five minutes, from whichever addresses they come, and the right password is refused while it lasts. At most two passwords are checked at once, so a burst of sign-in attempts cannot become a burst of memory.
- `/sitemap.xml` lists the public organizations, boards and roadmaps, and `robots.txt` points at it. It needs `LASTHELP_PUBLIC_URL`; without it there is no sitemap. What is private is not in it.
- After an update, the binary it replaced stays beside the new one as `<binary>.old`.

### Changed
- An update no longer closes the port while the server restarts: the process being replaced hands its listening socket to the new one, so a proxy in front sees requests answered and not refused.
- The copy of the database taken before an update is a consistent snapshot, checked before it counts, and the three newest are kept. It used to be a copy of the file, which could miss what was written since the last checkpoint and still look like a database. Restoring one now says to remove the old `-wal` and `-shm` files.
- Push notifications about a report ask the push service to deliver them at once.
- The team's archive at `/review` is kept out of search, the pages that are private say so in the page as well as in the header, and `robots.txt` no longer forbids the addresses that carry a noindex, which is what stops a crawler from ever reading it.
- An address that leads nowhere answers 404, with the same page that draws the Not found message, instead of 200.
- Each language uses one form of address and one word for a report, and the place where the forge is asked for names the hosts it means: GitHub, GitLab or Gitea.
- Built with x/crypto 0.59.0, which clears the last advisory `govulncheck` listed.

### Fixed
- Words added to the word list in the same moment are all kept. Twenty added at once used to leave one to four.
- The Enter that confirms a word typed with an input method no longer acts as a key press.
- The Open and Closed tabs of the team's archive no longer cut the word Geschlossen in a phone's width.
