# Connecting a forge

Four are supported: **GitHub** (including Enterprise Server), **GitLab** (hosted
or self-managed), **Gitea** and **Forgejo**.

Self-hosted instances on private addresses are supported deliberately. Pointing
at a Gitea on `192.168.1.20` with a self-signed certificate is the feature, not
the vulnerability — the URL comes from you, not from a stranger. Cloud metadata
addresses stay blocked regardless, and a redirect to a different host is refused.

## 1. A token

The token goes on the organisation, because that is as far as a forge will let
one reach:

| Forge | What to create | Scope |
|---|---|---|
| GitHub | A fine-grained personal access token | Resource owner = your org or user; **Issues: read and write** |
| GitLab | A group access token, or a project one | `api` |
| Gitea / Forgejo | An access token in your settings | `write:issue` |

On Gitea and Forgejo a write scope also grants the matching read routes, so
`write:issue` alone covers both creating an issue and reading one back — and a
scope is required even for a public repository.

A GitHub fine-grained token cannot span two organisations. That is why an
organisation here is one connection: two of them are two connections whether or
not anybody wanted a second level.

## 2. Store it

In the panel, open the organisation. If it has a forge, it asks for the token
and a webhook secret, and answers with the address to configure:

```
Deliver here: https://help.example.com/hooks/xO3f…
```

That address is opaque and stable, and it stays on that screen — a forge is
configured with it once, and somebody who lost it has no other way to find it.

The token is **never shown again**, by design: it is stored encrypted, and no
endpoint here returns a stored credential, because one that did would be a way
to read it through the panel.

Over the API, which is what the panel calls:

```sh
curl -X POST "$BASE/api/orgs/codigosh/credentials" \
  -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: $CSRF" -H 'Sec-Fetch-Site: same-origin' \
  -b cookies.txt \
  -d '{"token":"...","webhook_secret":"pick something long and random"}'
```

## 3. Point the forge at it

Add a webhook on the repository (or the organisation) with:

- **URL**: `https://help.example.com/hooks/xO3f…`
- **Content type**: JSON
- **Secret**: the `webhook_secret` from step 2
- **Events**: issues

| Forge | Where |
|---|---|
| GitHub | Repository → Settings → Webhooks |
| GitLab | Project → Settings → Webhooks (use the signing token, not the legacy one, if offered) |
| Gitea / Forgejo | Repository → Settings → Webhooks → Gitea/Forgejo |

## What happens then

- A triaged item becomes an issue, or is attached to one that already exists by
  pasting its number or URL. A project with four thousand open issues wants the
  second far more often than the first.
- When that issue is **closed** or **reopened**, the item follows: a bug becomes
  *closed* or *tracked*, an idea becomes *done* or *in progress*.
- The roadmap follows from that, which is why nobody has to maintain it.

Nothing calls your forge from inside a request. Work goes to a queue with
exponential backoff, so a forge having a bad afternoon delays an issue rather
than losing a report. A revoked token stops being retried immediately — there is
nothing to wait for — and the failed job stays visible rather than being dropped.

## Why the address is opaque

Forgejo sends GitHub's event and delivery headers alongside its own. A server
that decided what a delivery was by looking at its headers would let whoever
sent it choose which verification it faced — and the four forges do not verify
the same way. GitHub and Gitea sign the body; GitLab signs a composed message of
id, timestamp and body, and also still accepts a plain shared token, which is
weaker than the others.

So the verifier comes from the connection record and never from the request.
That is also why there is no generic verifier anywhere in this codebase: one
that hashed the body would reject GitLab outright, and one written to be lenient
would accept what it could not check.

## What has been checked against a real forge

Forgejo and Gitea are walked end to end against a real instance in a container,
by `make live-forge`: LastHelp creates the issue, the forge signs a webhook with
its own code, our receiver verifies it, and closing the issue there closes the
item here.

That check also verifies the token scope this page tells you to create: LastHelp
is handed one with **only `write:issue`** and the whole journey works with it.
Adding the webhook needs more, which is why a person does that part in the
forge's own settings.

GitHub and GitLab are covered by tests against a fake that signs the way each of
them documents, which is weaker: a fake written by the same hands as the adapter
can be wrong in the same way and agree.

## Running without a forge

Set `forge` to `none`, or simply do not add credentials. Everything except
turning an item into an issue works, and the screens say there is no forge
rather than offering buttons that fail.
