# What a visitor leaves behind

A portal is public, and the people who use it are strangers. This page says what
LastHelp keeps about them, so that you can tell them. **You run the instance, so
you are the one who answers for it** — this is what the software does, not legal
advice.

## What is stored

| What | Where | For how long |
|---|---|---|
| What the reporter wrote: title, description, the answers to the form | The database | Until your team deletes it. Once approved it is public. |
| The reporter's **email** — *only if you configured a mail relay* | The database | Until the report closes, and the reporter has been told so. Then it is erased. |
| A hash of the secret link the reporter keeps | The database | As long as the report exists. It is a hash, so a copy of the database cannot be used to reply as the reporter. |
| A vote | The database, against a random identity, never an address | As long as the report exists. |
| A **picture** the reporter attached | The database, drawn again from its pixels, so the place and the camera a photo carries are not kept | Seen only by your team and the reporter until approved. Erased thirty days after the report closes, or at once if it is rejected, marked as spam or deleted. |

Without a mail relay (`LASTHELP_SMTP_HOST`) the "tell me when this changes"
option is not even offered, so **no email address is stored at all**.

Following a board with Atom stores nothing about the reader.

## What is *not* stored

- **The visitor's address.** It is never written to the database. The rate limits
  that slow a flood keep it in memory, grouped by network block (`/24` for IPv4,
  `/64` for IPv6), and it is gone when the server restarts. LastHelp writes no
  access log of its own.
- No account, no name, no profile, no analytics, no third party of any kind.

## Cookies

| Cookie | Who gets it | What it is for | Lasts |
|---|---|---|---|
| `lasthelp_csrf` | Every visitor | Refuses a form submitted from another site | The browser session |
| `lasthelp_voter` | Whoever votes | A random, signed identity, so one browser counts once | One year |
| `lasthelp_session` | Your team, after signing in | Keeps them signed in | Thirty days since last seen |

All three are needed for something the visitor asked for or for security, and
none carries personal data. In general that kind of cookie does not need a
consent banner; confirm it for your own country.

## What is yours to check

**The proxy in front of LastHelp logs more than LastHelp does.** nginx, Caddy,
Traefik and tunnels often write the visitor's address, the path and the time to
an access log, and keep it as long as they are configured to. That log is yours,
not LastHelp's. Look at how long it lives, and turn it off or shorten it if you
do not use it.

If your proxy does log, or you run a mail relay, say so to your visitors in one
sentence on your own page. For example: *"Reports are public once approved, with
their pictures, which are erased thirty days after the report closes. If you
leave an email it is used only to tell you when the report closes, and erased
then. Our server may record your address for a few days for security."*
