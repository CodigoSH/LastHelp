**Two-step sign-in, passkeys, sign-in through a proxy and a new form to create a project**

### Added
- Two-step sign-in with an authenticator app and single-use recovery codes; the administrator can remove it from an account, and `lasthelp 2fa reset` does it from a terminal
- Passkeys, to sign in with a fingerprint, face or screen lock instead of the password, added from the profile
- Sign in through a proxy that already signs people in, such as Authelia, Authentik or oauth2-proxy
- One form to create a project and to change it: a use to start from, the options folded under it, and a summary; each project can turn votes off and keep its roadmap to the team
- A project's own questions on its report form, and a choice of whether its problem form asks for a version and for steps
- The profile lists where the account is signed in, and closes any of those sessions
- Help behind an "i" beside each field and setting, and a proper head on organization and project pages
- One plain vocabulary for any team: projects, problems and ideas
- What needs the administrator in one card, with the server's details one button away
- A proxy or tunnel in front is asked about plainly when installing, and noticed when nobody said so

### Fixed
- An installation that cannot replace its own program, such as an OCI image running on Proxmox VE, says to update the image instead of failing halfway
- A team is a list with Add on its card, and every action ends on the right edge
- A project's row says what it takes beside its address and what waits on the right, and a problem is a square with a warning
- A card's action drops below its title when the two do not fit
- On a phone the menu band keeps its place, with sign out pinned in its corner
- Inside an organization or one of its projects, Organizations is marked as the current place
