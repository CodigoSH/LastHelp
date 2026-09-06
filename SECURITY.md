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
  rate limits are keyed on an address a caller cannot choose for themselves.
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
  have. An email address is stored only if a reporter asked to be notified, is
  visible only to your team, and is deleted when the item closes.

## Secure update delivery

Releases are published directly by us, with no intermediary.

- Every release ships a `SHA256SUMS` file in the format `sha256sum` already
  writes, so you can check it with the tools on your own machine, and a
  `SHA256SUMS.sig` signed with an ed25519 key.
- The public half of that key is compiled into the binary. A key the updater
  downloaded would be a key an attacker could substitute.
- Builds are reproducible: `-trimpath`, `CGO_ENABLED=0`, pinned `GOOS`/`GOARCH`,
  and a frontend stamp derived from the build's own output rather than the
  clock. Two builds of the same commit produce the same bytes, so you can
  rebuild a release yourself and compare hashes.

## Supported versions

While the project is pre-1.0, only the latest release receives fixes.
