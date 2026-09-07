# Security policy

## Reporting a vulnerability

Email **security@codigosh.com**. We acknowledge within 48 hours.

Please include what you did, what happened, and what you expected. If you have a
proof of concept, keep it to a test instance of your own — never against someone
else's portal, which by design is full of other people's reports.

Do not open a public issue for a vulnerability.

## What the threat model assumes

LastHelp is self-hosted software for a developer or a small team. Two different
assumptions apply, and keeping them apart is most of the design:

- **The content comes from strangers.** Anyone on the internet can submit. So
  nothing is published without approval, every request body is bounded, and the
  rate limits are keyed on an address a caller cannot choose for themselves —
  the entry *our* proxy appended, never the leftmost one, and bucketed by /24
  and /64 because a single IPv6 customer holds eighteen quintillion addresses.

  What that protects is not the database — nothing reaches a public page
  unapproved — it is the operator's attention. A queue too full to open is how
  a portal dies. The layers are a signed single-use form token, a timing window,
  a honeypot, those rate limits, duplicate and link heuristics, a word list the
  operator controls, and a breaker that closes a board for a while rather than
  filling a queue nobody will read.

  Honestly: this stops the ambient automated flood. It does not stop somebody
  who has decided to target your project and is willing to work at it by hand.
  Against that the tools are the breaker, closing submissions for a while, and
  the fact that nothing is published without a person approving it.
- **The forge credential never leaves encrypted.** The token and webhook secret
  are sealed with a key kept in a file beside the database, not in it: a leaked
  backup is then a leak of hashes and public text rather than of write access to
  your forge. Nothing echoes a stored token back, and a collaborator — somebody
  trusted to triage one repository — cannot reach it at all.

- **A webhook is authenticated by its signature and nothing else.** A forge
  cannot sign in, so that endpoint sits outside the session and CSRF checks. The
  address it delivers to is opaque and identifies the connection, and the
  connection is what says which verifier to use — never the request's headers.
  That matters: Forgejo sends GitHub's headers alongside its own, so a server
  that guessed from headers would let a caller choose which verification it
  faced, and one of the four paths is weaker than the others.

- **The forge URL comes from the operator.** Pointing at a Gitea on
  `192.168.1.x` with a self-signed certificate is the feature, not the
  vulnerability, so private addresses are allowed there. Cloud metadata
  addresses stay blocked regardless, and nothing a member of the public writes
  ever becomes a URL this server fetches.

## What it does not do

- **No telemetry.** This server reports nothing about you or your instance to
  us, on any schedule, for any reason.
- **No third parties.** No CDN, no captcha service, no AI provider. What people
  write in your portal reaches your database and nowhere else.
- **No account for the public.** We cannot leak a password database we do not
  have. An email address is stored only if a reporter asked to be notified — and
  the field is not even offered unless a mail relay is configured, because
  collecting an address nothing can use is collecting personal data for nothing.
  It is encrypted with the same key as the forge credentials, is never returned
  by any endpoint, and is deleted when the item closes — after they have been
  told about the closure, not before. That last part is
  `TestTheReporterIsToldAndThenForgotten`, not a sentence.

- **The feed asks for nothing.** Following a board over Atom stores nothing
  about you: no address, no account, no subscription. It is the notification
  channel this product leans on for exactly that reason.

## Secure update delivery

Releases are published directly by us, with no intermediary.

- Every release ships a `SHA256SUMS` file in the format `sha256sum` already
  writes, so you can check it with the tools on your own machine, and a
  `SHA256SUMS.sig` signed with an ed25519 key.
- The public half of that key is compiled into the binary. A key the updater
  downloaded would be a key an attacker could substitute.
- Builds are reproducible: `-trimpath`, `CGO_ENABLED=0`, pinned `GOOS`/`GOARCH`,
  and a frontend stamp derived from the build's own output rather than the
  clock. Two builds of the same commit produce the same bytes:

  ```sh
  git checkout <tag>
  VERSION=<tag> ./scripts/build-artefacts.sh
  sha256sum -c dist/SHA256SUMS
  ```

  That is the same script our CI runs, and the release workflow builds twice and
  compares before publishing — because a promise nobody executes is one that
  quietly stops being true.

## Supported versions

While the project is pre-1.0, only the latest release receives fixes.
