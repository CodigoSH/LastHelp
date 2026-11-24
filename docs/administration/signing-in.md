# Signing in

Every account signs in with its username and password. Each person can add a
second step or a passkey from their own **Profile**, under **Your account**, and
the administrator can let a proxy that already signs people in do it for them.

## Two-step sign-in

With it on, the password is not enough: after it, LastHelp asks for the six-digit
code an authenticator app shows (Aegis, 2FAS, Google Authenticator, Bitwarden and
the like). The code changes every thirty seconds, and one from the thirty
seconds before or after is still accepted, so a clock a little off does not lock
anybody out.

**Turning it on.** In **Profile**, **Two-step sign-in** → **Turn on** shows a QR
code to scan, or a key to type in by hand, and asks for the code the app then
shows. Nothing changes until that code is right. The app files the account under
the name of this installation, so somebody with two of them can tell which is
which.

**Recovery codes.** Turning it on hands over ten recovery codes, shown once, to
copy or download. Each one signs in in place of a code from the app, once. The
card says how many are left; **New recovery codes** draws ten new ones and the
old ones stop working. Turning it off, and drawing new codes, both ask for the
password again.

**When signing in.** After the password comes a box for the code, or for a
recovery code. It has five minutes and five tries; after that the password has
to be typed again. The username and password are checked first and with the
usual limits (see [Passwords and locked accounts](passwords.md)).

**A lost phone.** Whoever kept a recovery code signs in with it. Otherwise the
administrator removes the second step from **Users**: the pencil on the account,
then **Remove two-step sign-in**, and the person signs in with the password
alone until they turn it on again. The list of users marks the accounts that
have it with **2FA**. The administrator's own account is cleared from a terminal
on the server:

```sh
lasthelp 2fa reset ana
```

## Passkeys

A passkey signs in with the device's own lock — a fingerprint, a face, a PIN, a
security key — instead of the password. It belongs to one account on one
installation, nothing secret leaves the device, and a page that only looks like
this one cannot use it.

**Adding one.** In **Profile**, **Passkeys** → **Add a passkey**, and the browser
asks for the device's lock. Each passkey is named after the browser and the
system it was made on (*Firefox · Linux*) and shows when it was added and last
used; **Remove** takes one away. A device that already has a passkey for the
account is not given a second.

**Signing in with one.** On the sign-in page, the key beside **Sign in** — *Use a
passkey* — asks the browser for it, with no username typed. Browsers that can
also offer the account's passkeys in the username box itself. A passkey signs in
on its own: it already proves the device and the person, so two-step sign-in is
not asked for after it. Failed tries count towards a lock, like wrong passwords.

**Where they work.** Browsers only allow passkeys on a page opened over
**HTTPS, by its name** — not by an IP address — and on `localhost` while
testing. Opened any other way, the sign-in page shows no key and the
**Passkeys** card says why, instead of offering something that would fail.
Behind a reverse proxy or a tunnel, the name the browser uses is the one the
proxy passes on, read only when `LASTHELP_TRUST_PROXY` is on (see
[About `LASTHELP_TRUST_PROXY`](../getting-started/install.md#about-lasthelp_trust_proxy)).
A passkey is tied to that name: one made at `help.example.com` does not work at
another address.

## Signing in through a proxy

A proxy that already signs people in — Authelia, Authentik, oauth2-proxy, Caddy
or Traefik with `forward_auth` — can sign them in to LastHelp too. It is off
unless both of the first two are set:

| Variable | What it is |
|---|---|
| `LASTHELP_AUTH_PROXY_HEADER` | The header in which the proxy says who is signed in, by username — `Remote-User` for Authelia and Authentik |
| `LASTHELP_AUTH_PROXY_TRUSTED` | The proxy's addresses or networks, comma-separated (`172.18.0.5`, `10.0.0.0/24`) |
| `LASTHELP_AUTH_PROXY_LOGOUT_URL` | Optional: where **Sign out** sends people, so the proxy signs them out too. Without it the proxy signs them straight back in |

What it does and what it never does:

- **The header is believed only from those addresses**, judged by the connection
  itself and never by `X-Forwarded-For`, which a caller writes. From anywhere
  else it is a header anybody could type, and it is ignored.
- **Only accounts that already exist sign in this way**, by their username, and
  not a disabled one. It never creates an account and never changes what one may
  do: the administrator still decides both, under **Users**.
- What it opens is an ordinary session, listed under **Active sessions** and
  closed like any other, that lasts a day unused. If the proxy names somebody
  else, that person's session takes over.
- Two-step sign-in is not asked for: the proxy is the sign-in.
- A setting that cannot be used — a header name that is not one, an address
  that is not one, a logout address that is not `http` or `https` — is left
  out and the log says why. With no usable trusted address it stays off, since
  the header would be believed from anybody.

**About** shows whether **Sign-in proxy** is set under the server's details.

Everybody can still sign in with their password, and the administrator should
keep that working: it is the way back in if the proxy is down.
