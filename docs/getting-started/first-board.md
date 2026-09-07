# Your first board

The structure is two levels, and they are the ones your forge already has:

- An **organisation** is a connection to a forge: its address, the owner, and
  the credentials. It is not an org chart — a token cannot span two
  organisations on any forge, so the boundary is imposed rather than invented.
- A **repository** is a public board. You register only the ones you want
  feedback for.

A project spread over twenty repositories does not have to register twenty. Most
register one and send everything there, because somebody reporting "the video
stutters on my phone" does not know which of your twenty repositories that lives
in, and should not have to.

## Creating them

From the panel: sign in, and the first screen asks for an organisation — a name,
an address, and which forge it connects to, or none. Then a board under it.

Or over the API, which is what the panel calls:

```sh
# The address of your instance, and a session from signing in.
BASE=http://localhost:8080

curl -X POST "$BASE/api/organizations" \
  -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: $CSRF" -H 'Sec-Fetch-Site: same-origin' \
  -b cookies.txt -c cookies.txt \
  -d '{"slug":"codigosh","name":"CodigoSH","forge":"forgejo",
       "base_url":"https://git.example.com","forge_owner":"CodigoSH"}'

curl -X POST "$BASE/api/orgs/codigosh/repositories" \
  -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: $CSRF" -H 'Sec-Fetch-Site: same-origin' \
  -b cookies.txt -c cookies.txt \
  -d '{"slug":"lastboard","name":"Lastboard"}'
```

The board is then at `/codigosh/lastboard`, its roadmap at
`/codigosh/lastboard/roadmap`, and its feed at
`/feed/codigosh/lastboard/atom.xml`.

`forge` may be `github`, `gitlab`, `gitea`, `forgejo` or `none`. **`none` is a
supported way to run**, not a placeholder: everything except turning an item into
an issue works without a forge, and the screens say there is none rather than
offering buttons that fail.

![The public board](../images/board.png)

## Bugs and ideas are two different journeys

They look the same on the form and are treated differently on purpose:

- **A bug is not voted.** Two hundred people agreeing that something crashes
  does not make it crash harder; what it needs is somebody able to reproduce it.
  Its statuses are *new, confirmed, tracked, closed, not reproducible*.
- **An idea is voted, and its value is not reaching your issue tracker yet.**
  That is what stops the tracker turning into a wishlist with a thousand open
  entries. Its statuses are *new, planned, in progress, done, declined*.

Both land on the same three roadmap columns, because to somebody reading it a
confirmed bug and a planned idea are the same news.

## Who can do what

| Level | Role | Can |
|---|---|---|
| Server | **Administrator** | Everything, and the only one who crosses organisations |
| Organisation | **Owner** | Its connection and **its token**, its repositories |
| Repository | **Collaborator** | Triage that board: moderate, change status, reply, create and link issues. **Never sees the token** |

That last line is the reason the two levels exist. Somebody trusted to triage
one repository is not automatically somebody trusted with a credential that can
write to every repository the organisation owns.

## Votes are a signal, not a ballot

A voter is a random identity in a signed cookie, never an address — behind a
household router or a mobile carrier the address is shared, so keying votes on
it would let one neighbour spend everybody else's vote and make a whole office
count as one person.

The honest consequence: clearing the cookie allows a second vote. That is a
documented limit rather than a defect. The alternative is asking people to
create an account, which is the thing this product exists not to do, so treat a
vote count as a signal of what is popular rather than as a count of people.

## Taking your data with you

Every board exports as JSON, from the organisation's screen in the panel or at
`/api/orgs/<org>/repos/<board>/export`. It carries everything the operator
decided — the published items, the queue, the rejections, the merged duplicates,
the comments and the issue links — because an export that showed only the
approved half would misrepresent the record.

It deliberately carries **no reporter email addresses and no follow tokens**.
Taking your data with you must not mean taking everybody's: those addresses are
personal data collected for one purpose, and a follow token is a credential.

## The roadmap

Both journeys land on the same three columns, and all three are always drawn —
showing only the ones with something in them leaves a reader working out which
two of three they are looking at.

![The roadmap](../images/roadmap.png)
