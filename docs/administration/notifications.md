# Notifications

Two audiences with different needs, and one notifier for both would be wrong for
one of them.

## Telling your team

Without this the portal dies quietly: nobody opens a moderation queue every day
on the chance that it has something in it, so it fills, and a portal full of
people talking to nobody reads worse from outside than no portal at all.

Configured **per repository**, so one repository can go to one room and another somewhere
else. Nothing applies to every repository by itself: a channel everybody is
signed up to without choosing it is a channel everybody eventually silences.

Choose it in the panel, on the repository's page (**Organisations**, the
organisation, then the repository): pick a channel, and it asks only for what
that channel needs. The address and the recipient are shown again once saved,
so you can check them. A saved token is shown as dots: it is there, and which one
it is cannot be seen. Type a new token over the dots to replace it, or empty the
field to remove it; leave the dots as they are and the stored one is kept.

**Send a test** sends a message through what is on the form, using the stored
token if you left that field empty, and says why when it does not arrive: the
service refused the token, the address does not exist, it could not be reached,
or the server has no mail relay or is set to work offline.

Over the API, which is what the panel calls:

```sh
curl -X POST "$BASE/api/orgs/acme/repos/atlas/channel" \
  -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: $CSRF" -H 'Sec-Fetch-Site: same-origin' -b cookies.txt \
  -d '{"kind":"ntfy","url":"https://ntfy.sh/your-private-topic"}'
```

| `kind` | Needs | Notes |
|---|---|---|
| `ntfy` | `url`, optional `token` | The topic URL. Title and click-through go in headers |
| `gotify` | `url`, `token` | The server address; `/message` is appended |
| `webhook` | `url`, optional `token` | Posts `{"title","body","url"}` — point it at Matrix, a bridge, a script |
| `smtp` | `to` | Uses the relay configured on the server |
| `server` | — | A repository only: use the channel the administrator set for the server |
| *(empty)* | — | No notifications |

### One channel for the whole server

An administrator can set a channel once, under **Notifications** in the **This server**
group of the menu, and repositories that want it pick **The server's channel** in their own
list. It is still opt-in: a repository that does not choose it is not touched,
and one that does is told where its messages go. Change the server's channel
and every repository that chose it follows; remove it and they send nothing
until it is set again. A repository's own channel is never replaced by it.

Over the API, administrators only: `GET` and `PUT /api/notifications` (same
fields as above, and no `server` kind), and `POST /api/notifications/test`.
A repository's test is `POST /api/orgs/{org}/repos/{repo}/channel/test`.

The webhook shape is three fields and is meant to stay that way: it is somebody's
script.

A notification carries a nudge, not the report. One containing the whole
submission would deliver spam straight to wherever your team reads things, which
is the one place the moderation queue exists to keep it out of.

## Telling whoever reported something

**The Atom feed** is the channel that needs no configuration, no address and no
account, and stores nothing about whoever subscribes:

```
https://help.example.com/feed/acme/atlas/atom.xml
```

It carries published items only, through the same listing the public page uses,
so it cannot show something the page would not.

**Email** if you configure a relay. Then, and only then, the report form offers
an optional address field — with no relay it is not shown at all, because
collecting an address nothing can use is collecting personal data for nothing.

That address is never returned by any endpoint — not to the team, not through
the API — and is **deleted when the item closes**. It was collected for one
purpose and that purpose is over. It is stored as it was given, so it is in the
database file: whoever can read that file can read it.

Mail is plain text, marked `Auto-Submitted: auto-generated`, and nobody reads
replies to it.

One honest limitation: the mail is in English. The reporter's language is not
known, and asking for it would put another field on a form whose whole point is
being short.

## The relay

```
LASTHELP_SMTP_HOST=smtp.example.com
LASTHELP_SMTP_PORT=587
LASTHELP_SMTP_USERNAME=
LASTHELP_SMTP_PASSWORD=
LASTHELP_SMTP_FROM=lasthelp@example.com
LASTHELP_SMTP_STARTTLS=true
```

STARTTLS can be turned off, and that is not an oversight: a relay on localhost
or inside your own network is the common case for self-hosted mail.
