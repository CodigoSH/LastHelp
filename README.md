<div align="center">
  <br />
  <img src="assets/logo.png" alt="LastHelp" width="150" />
  <br /><br />

  <h1>LastHelp</h1>
  <p><strong>The front door your forge does not have.</strong></p>

  <p>
    <a href="docs/index.md">
      <img src="https://img.shields.io/badge/Documentation-1a1a2e?style=for-the-badge&logo=readthedocs&logoColor=white" alt="Documentation" />
    </a>
    &nbsp;
    <img src="https://img.shields.io/badge/dynamic/json?url=https://git.codigosh.com/api/v1/repos/CodigoSH/LastHelp/releases&query=%24%5B0%5D.tag_name&label=Latest&style=for-the-badge&color=e8520e" alt="Latest release" />
    &nbsp;
    <img src="https://img.shields.io/badge/License-Apache_2.0-green?style=for-the-badge" alt="Apache 2.0" />
  </p>

  <br />

  <img src="assets/mockup.png" alt="A LastHelp board" width="92%" style="border-radius: 14px; box-shadow: 0 20px 60px rgba(0,0,0,0.3);" />

  <br /><br />

</div>

---

<div align="center">

![](https://img.shields.io/badge/🙋_No_account_to_report-1a1a2e?style=for-the-badge)
![](https://img.shields.io/badge/🔗_Issues_in_your_own_forge-1a1a2e?style=for-the-badge)
![](https://img.shields.io/badge/🔒_100%25_self--hosted-1a1a2e?style=for-the-badge)
![](https://img.shields.io/badge/💾_SQLite,_no_DB_setup-1a1a2e?style=for-the-badge)
![](https://img.shields.io/badge/✍️_Signed_releases-1a1a2e?style=for-the-badge)

</div>

---

📖 **[Read the Documentation](docs/index.md)** for detailed guides on installation, your first repository, connecting a forge, notifications and keeping the queue readable.

---

## Status

**Beta.** One release is published and signed, and this section says what is
actually behind it — a README describing a server nobody wrote is the first
thing that makes people stop trusting one.

Working end to end, each of them checked by driving a real browser before it ships:
organisations and boards, reporting without an account, the secret follow link,
the moderation queue, publishing, voting, comment threads, similar-item hints
while you type, the roadmap, the panel — overview, the cross-board listing and
triage from it — the anti-abuse layers, notifications in both directions, the
forge bridge for GitHub, GitLab, Gitea and Forgejo, installing, and updating in
place.

Not there yet: **Bitbucket**, and a **stable release** — the only release is a
prerelease, so answer `beta` when the installer asks which channel you want.

---

## Why LastHelp

Self-hosting your forge usually means turning registration off, and that closes the only door outsiders had for telling you something is broken. Opening registration to the world is not the answer; a separate front door is. The open-source feedback tools do not talk to any forge, and the ones that do are closed, paid and hosted by somebody else — so nobody serves the person who self-hosts their own forge, which is almost always the same person who would self-host a feedback portal. LastHelp was built with a simple rule: **anybody can tell you something without an account, and nothing reaches a public page until a person says so.**

- **Reporting without an account** — a bug or an idea, in one form, from anybody. What they get back is a secret link: the only way to their own report, and the reason no sign-up is needed at all.
- **A bridge to your forge** — GitHub, GitLab, Gitea and Forgejo, hosted anywhere including your own network. A triaged report becomes an issue, or is attached to one that already exists — which is what a project with four thousand open issues actually needs — and when that issue closes or reopens, the report follows.
- **A roadmap nobody maintains** — three columns built from the state of each report, so it cannot be out of date. Ideas gather votes, which is what turns "somebody asked for this once" into an order of work.
- **A queue that stays readable** — nothing is published without a person, and the decision sits beside what it is about — publish, reject, spam — with a key for each. One listing covers every board an account has a role in, filtered by state, kind, status, board or words, with the filters in the address so a narrowed view can be sent to somebody else.
- **An overview worth opening** — the last thirty days as a chart you can ask about day by day, the most wanted ideas, the newest reports and the work per board. Every number on it leads to the listing it counts.
- **Anti-abuse without a third party** — a signed single-use form token, a timing window, a honeypot, rate limits keyed on the address a caller cannot choose, duplicate and link heuristics, an operator word list, and a breaker that closes a board rather than hand somebody a queue they will not open. Every refusal is counted by layer, because a filter nobody can measure is a filter quietly turning away people who never complain.
- **Notifications in both directions** — the team is told through ntfy, Gotify, a webhook or mail, configured per repository. Whoever reported something follows an Atom feed that needs no address, no account and no configuration, or leaves an email if a relay is set up.
- **One binary, one file** — no database server, no runtime, no dependencies. SQLite and the whole interface are compiled in, and a backup is a copy of one file — or a button in the panel.
- **Twenty languages**, translated by hand — not machine-filled — with right-to-left laid out properly for Arabic, Persian and Urdu.
- **Installable as an app** — on a phone, a tablet or a desktop, from the browser's own menu, when the instance is served over HTTPS. It needs the network to report and vote, so it does not pretend to work offline.
- **Reproducible builds** — `-trimpath`, `CGO_ENABLED=0` and a frontend stamped with a digest of its own output, so two builds of the same commit produce the same bytes and you can check ours against yours.

Bitbucket is not bridged yet. Everything else above works end to end, and is checked by driving a real browser rather than by hoping.

---

## Security & Privacy

### Security

Passwords are hashed with Argon2id. Sessions are opaque tokens in an `HttpOnly`, `SameSite=Lax` cookie, marked `Secure` over HTTPS, and CSRF is a double-submit token checked alongside `Sec-Fetch-Site` — a header the browser writes from the real origins and a page cannot forge. Every response carries a Content-Security-Policy with `script-src 'self'`, so an injected script does not run, and `Referrer-Policy: no-referrer`, because the address of a report carries the token that opens it. Rate limits are keyed on the address our own proxy added rather than the first one the caller sent, because a proxy appends to `X-Forwarded-For` and reading the left-hand entry lets anybody choose their own identity. A stored forge token is never shown again and no endpoint returns one, request bodies are bounded per handler, and every release is signed.

For a detailed breakdown, including what the threat model does **not** cover, see the [Security Policy](SECURITY.md).

### Privacy

**Reporting is anonymous by design.** No account, no tracking, no analytics, and no telemetry of any kind: nothing about you or your instance is ever reported to us. The email address a reporter may leave is optional, asked for only when a mail relay exists to use it, never returned by any endpoint, and deleted when the report closes.

And no AI. Nothing anybody writes here is sent to a third party — the whole point is that it reaches one database and nowhere else.

---

## Installation Methods

### Quick Install (Linux)

Run this command to install or update LastHelp:

```bash
curl -fsSL https://git.codigosh.com/CodigoSH/LastHelp/raw/branch/main/install.sh | sudo sh
```

It asks which port to use, which release channel to follow and whether a reverse proxy is in front, and remembers all three the next time you run it. **Answer `beta` to the channel question today: there is no stable release yet**, and the default only ever picks a stable one, so nobody following this command lands on a prerelease by accident. Without a terminal to ask on — a container, a cron job — every answer is its default and `LASTHELP_CHANNEL=beta` supplies that one.

It verifies the signature over the checksums before anything reaches the disk, installs a systemd service, keeps the database in `/var/lib/lasthelp` and serves on port 8080 unless you answer otherwise when it asks. **The first account created is the administrator**: open the address it prints and claim the instance — a server with somebody in it cannot be claimed again.

---

### Docker

Images are built for `linux/amd64` and `linux/arm64` and published to our Forgejo registry with every release. **Use the `beta` tag today: there is no stable release yet**, and `latest` only ever moves with a stable one.

**Using Docker Compose:**
```yaml
services:
  lasthelp:
    image: git.codigosh.com/codigosh/lasthelp:beta
    container_name: lasthelp
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - lasthelp-data:/data
    environment:
      # Turn this on only when a reverse proxy really is in front. With it on
      # and no proxy there, the caller chooses the address we attribute their
      # requests to, and the rate limits stop meaning anything.
      LASTHELP_TRUST_PROXY: "false"
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

volumes:
  lasthelp-data:
```

**Using Docker CLI:**
```bash
docker run -d \
  --name lasthelp \
  -p 8080:8080 \
  -v lasthelp-data:/data \
  --restart unless-stopped \
  git.codigosh.com/codigosh/lasthelp:beta
```

The image runs as an unprivileged user (uid 10001), so a named volume just works; a directory bound from the host has to belong to that user first (`chown 10001:10001 ./data`). The binary inside refuses to update itself, because updating a container is pulling a new image: `docker compose pull && docker compose up -d`.

Every setting the binary reads is listed in [.env.example](.env.example) and explained under [Settings](docs/getting-started/install.md#settings).

---

## Updates

Updates are delivered directly from our own Forgejo instance — no proxy, no intermediary and no telemetry.

The updater checks the signature over the checksum list before it downloads anything, then the binary's own checksum, then makes the candidate prove it runs and report its schema, then refuses any binary older than the database, takes a backup, and only then replaces anything. None of those has a switch. In a container it says to pull an image instead of offering a button that would silently revert.

For detailed instructions see the [Updating guide](docs/administration/updating.md), or the [Security Policy](SECURITY.md) for how the signing works.

---

## Build

Requires [Go](https://go.dev) and [bun](https://bun.sh).

```sh
make build      # frontend + binaries into bin/
make run        # build and run
make test       # go test, the architecture rules and the frontend tests
make drive      # walk the whole journey in a real browser
make live-forge # and again against a real Forgejo, Gitea or GitLab in a container
make shots      # regenerate the images this repository shows
```

---

## Support & Recognition

We are an open-source initiative managed through [Open Collective Europe](https://opencollective.com/codigosh).

### Financial Support
If you find our tools useful, consider supporting our infrastructure through an unconditional donation.

[<img src="https://opencollective.com/codigosh/donate/button@2x.png?color=blue" width="250" />](https://opencollective.com/codigosh)

### Acknowledgments
Special thanks to everyone supporting our mission. 
<a href="https://opencollective.com/codigosh">
  <img src="https://opencollective.com/codigosh/tiers/backer.svg?width=890&button=false" />
</a>

### Organizations
We appreciate the organizations that support us as a gesture of goodwill to help maintain our infrastructure.
<a href="https://opencollective.com/codigosh">
  <img src="https://opencollective.com/codigosh/tiers/sponsor.svg?width=890&button=false" />
</a>

---

## Credits

- **Everything drawn here**: the mark, the icons and the charts are drawn by hand in this repository — no icon library, no chart library, no frontend framework. The whole frontend is TypeScript compiled by bun; its only dependencies are the type definitions and the TypeScript checker, both used while building and neither shipped.

Full attribution, including the Go modules compiled into the binaries, lives in [NOTICE](NOTICE).

---

A project by [CodigoSH](https://git.codigosh.com/CodigoSH) — built to last.
