**Two-step sign-in and passkeys, sign-in through a proxy, and projects with their own roadmap**

### Added
- **Two-step sign-in with an authenticator app.** Turn it on in Profile, scan the code or type the key, and confirm with a code from the app. Turning it on hands over ten single-use recovery codes. An administrator can remove the second step for a user from Users.
- **Passkeys.** Sign in with the device's own lock (fingerprint, face, PIN or security key) instead of the password. A passkey is named after the browser and system it was made on, and can be removed from Profile. Passkeys work on HTTPS by name, and on `localhost` while testing.
- **Sign-in through a proxy that already signs people in**, such as Authelia, Authentik, oauth2-proxy, or Caddy and Traefik with `forward_auth`. It is off until `LASTHELP_AUTH_PROXY_HEADER` and `LASTHELP_AUTH_PROXY_TRUSTED` are set, and it only ever opens sessions for accounts that already exist.
- **Active sessions.** Profile lists where the account is signed in, and any session can be closed from there. Sessions open in a dialog that shows which ones are in use now.
- **Projects with their own form and roadmap.** One form creates a project and changes it. Each project chooses whether it takes votes, offers a public roadmap of its own, asks its own questions on the report form, and whether its problem form asks for a version and for steps.
- **Lockout after repeated wrong passwords.** Five wrong passwords for one name within a minute lock that name for five minutes.
- **`lasthelp password reset <username>`** sets a forgotten password from a terminal on the server. It asks twice without showing what is typed.
- **Administrators are told when a newer version is out**, through a notification channel, with a switch that is off until it is turned on.
- **A sitemap** at `/sitemap.xml` lists the public organizations, boards and roadmaps. It needs `LASTHELP_PUBLIC_URL`; without it there is no sitemap.
- **Help behind an "i"** beside each field and setting, and a proper header on organization and project pages.
- **The server details in About** show the processor's temperature where the machine has a sensor, and the cores beside its use.
- **A proxy or tunnel in front is asked about during installation**, and noticed when nobody declared one. The notice says where to change the setting and what to run, with a button that copies each line.

### Changed
- **An update no longer closes the port while the server restarts.** The process being replaced hands its listening socket to the new one, so a proxy in front sees requests answered and not refused.
- **The copy of the database taken before an update** is a consistent snapshot, checked before it counts. The three newest are kept.
- **After an update, the binary it replaced stays beside the new one** as `<binary>.old`.
- **What needs the administrator is in one card in About**, and the server's details are one button away.
- **Plain HTTP is noticed only when somebody signed in uses it**, not for a probe.
- **Each language uses one plain vocabulary for any team**: projects, problems and ideas.
- **The team's archive is kept out of search**, and pages that are private say so in the page as well as in the header.
- **An address that leads nowhere answers 404**, with the Not found page, instead of 200.
- **Built with x/crypto 0.59.0**, which clears the last advisory `govulncheck` listed.

### Fixed
- **A project that takes no votes** no longer offers a way to rank by them or draws a count.
- **A link to a roadmap its team keeps private** is no longer sent to anybody who could not open it.
- **The version card** keeps the room of an update card, so its button sits at the same distance as the status.
- **The attention card's Enter** goes through the same key handling as every other key in the page.
- **On a phone, the menu band keeps its place**, with sign out pinned in its corner. A card's action drops below its title when the two do not fit.
- **A passkey** is drawn as a quiet key beside Sign in, not as an icon inside the username field.
- **An installation that cannot replace its own program** says to update the image instead of failing halfway.
- **Sizes are counted in thousands**, as the units they are written in.

### Security
- **Two-step sign-in and passkeys** protect the account even when the password leaks.
- **Lockout after repeated wrong passwords**, from any address.
- **Sessions can be closed one by one** from Profile.
