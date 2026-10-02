# Security policy

## Reporting a vulnerability

Email **admin@codigosh.com**.

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
- **The forge credential never comes back out.** Nothing echoes a stored token
  back — not the panel, not the API — and a collaborator, somebody trusted to
  triage one repository, cannot reach it at all. That is a test, not a promise:
  `TestTheTokenNeverComesBackOutOfTheAPI`.

  What we do **not** claim: it is not encrypted at rest. Earlier betas sealed it
  with a key kept beside the database, and that protected against nothing — the
  key travelled in the same directory as the data it was protecting, so anyone
  who could copy one could copy both, and the server had to be able to read it
  unattended anyway. The honest statement is the one above plus this one: **the
  database file is the boundary.** Whoever holds it holds your forge token.
  Back it up somewhere only you can read.

- **A webhook is authenticated by its signature and nothing else.** A forge
  cannot sign in, so that endpoint sits outside the session and CSRF checks. The
  address it delivers to is opaque and identifies the connection, and the
  connection is what says which verifier to use — never the request's headers.
  That matters: Forgejo sends GitHub's headers alongside its own, so a server
  that guessed from headers would let a caller choose which verification it
  faced, and one of the four paths is weaker than the others.

- **The browser is told what the page may do.** Every response carries a
  Content-Security-Policy, including the ones that refuse: `script-src 'self'`
  and nothing else, so an injected `<script>` — inline or from anywhere but this
  server — does not run. The shell loads one module by URL and has no inline
  script in it, which is why there is no nonce here: a nonce guards inline
  scripts, and there are none to guard.

  `style-src` keeps `'unsafe-inline'`, and that is deliberate rather than
  forgotten. The interface sets style attributes — the size of a mark, the width
  of a bar, where a tooltip lands — and **there is no nonce for an attribute**.
  Removing it would break the interface without closing anything, since the
  directive that matters for injected code is `script-src`. `img-src` allows
  `data:` because an avatar and an instance logo are stored that way. Everything
  else is `'self'` or `'none'`, and the whole product has no third party to make
  an exception for.

- **What is private is not indexable.** Everything under the panel, the screens
  that lead into it, and — the one that matters — any address carrying a follow
  token answer with `X-Robots-Tag: noindex, nofollow`, and `robots.txt` says the
  same for a crawler that reads it first. A follow link is the key to somebody's
  own report, and an indexed key is a published one. A published board and a
  published report stay indexable on purpose: being findable is what a feedback
  portal is for. That is `TestPrivatePagesStayOutOfSearchAndPublicOnesDoNot`,
  not a sentence.

- **HSTS, over HTTPS only, and no further than this host.** A request that
  arrived over TLS is told to remember it for a year. `includeSubDomains` and
  `preload` are deliberately absent: both are one-way doors over domains this
  server knows nothing about, and a self-hoster who tries either on a subdomain
  of their main site and changes their mind has months of browsers refusing
  plain HTTP for it.

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
  It is never returned by any endpoint and is deleted when the item closes —
  after they have been told about the closure, not before. That last part is
  `TestTheReporterIsToldAndThenForgotten`, not a sentence. Until then it sits in
  the database as it was given, which is the same boundary as everything else.

- **The feed asks for nothing.** Following a board over Atom stores nothing
  about you: no address, no account, no subscription. It is the notification
  channel this product leans on for exactly that reason.

## The list, and what keeps it true

Every control above is named in `internal/web/security_test.go` beside the test
that keeps it true, and a test in that file walks the repository to check each
one still exists. It is a test rather than a document on purpose: this policy
claimed nothing about a content policy for months because there was not one —
planned, never written, and invisible to everything. A list with no teeth would
have let it happen again.

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

## Updating

The update path is the most dangerous code in a self-hosted product: it runs as
whoever runs the server, fetches something from the internet, and puts it where
the operating system will execute it. So every step either proves something or
stops, and a refusal never falls back to a weaker check.

- **A build with no release keys will not update at all.** No key does not mean
  accept anything.
- **A missing signature aborts.** It never falls back to checksum-only — a host
  that could tamper with the binary could also delete the signature, and a
  fallback would turn that into a way to strip the check.
- Only a **verified** checksum file is allowed to say what the binary should
  hash to.
- The candidate is **executed before it replaces anything** and has to report
  what it is. That catches the wrong architecture, a truncated download, a
  missing library — every way a download can be intact and useless.
- It is **refused if it understands an older schema** than your database, and
  the database is **backed up** before the swap.
- Nothing is decompressed, because nothing is compressed: the release asset is a
  plain binary, so there is no decompression bomb to defend against.

`install.sh` does the same verification with `openssl`, and a test signs with the
release code and verifies with the exact command that script runs — otherwise
both halves could drift together and every release would verify here while none
of them installed.

That verification needs **openssl 3.0 or newer**: an ed25519 signature over raw
data is checked with `pkeyutl -rawin`, and 1.1.1 has no command-line way to do
it at all. The installer checks the version and refuses with that reason, rather
than letting openssl report an unknown option and leaving you to guess whether
the signature is bad.

## Supported versions

Only the latest stable release receives fixes. A fix ships as a new release, and updating — from the panel, the installer or a new image — is how it reaches you.
