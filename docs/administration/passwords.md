# Passwords and locked accounts

A forgotten password is set again from a terminal on the machine the server runs on, because an instance that sends no email has no link to send. Five wrong guesses at one name lock that name for a few minutes.

## Setting a forgotten password

```sh
lasthelp password reset ana
```

It asks for the new password twice, without showing it, so it never reaches the shell's history or the process list. It needs at least ten characters, the same floor as the sign-in form, and nothing is changed if the two do not match.

It does what changing a password from the panel does: the account's sessions are closed, so whoever had the old password is signed out. The other accounts are not touched.

It reads the same `LASTHELP_DB_FILE` as the server, and it can run while the server does. In a container it needs a terminal:

```sh
docker exec -it lasthelp lasthelp password reset ana
```

This is the way back for the administrator too: nobody else can reset that account.

The second step of signing in is cleared the same way, for an account whose phone and recovery codes are both gone:

```sh
lasthelp 2fa reset ana
```

It signs in with the password alone until two-step sign-in is turned on again. For anybody else the administrator can do it from **Users** — see [Signing in](signing-in.md#two-step-sign-in).

## Seeing where you are signed in

**Profile**, under **Your account**, says how many browsers the account is signed in on, and **View sessions** opens them in a window: one line for each, with the browser and the system (*Firefox · Linux*), the address it was last used from and when. A green dot marks one used in the last five minutes; yours is marked **This device**.

**Close** signs one of the others out and **Close all other sessions** signs out every one but yours; both ask first. Signing out ends the session on the server too, so a copy of its cookie stops working, and a session nobody uses for thirty days ends by itself.

Behind a reverse proxy or a tunnel the address is only the visitor's when `LASTHELP_TRUST_PROXY` says the proxy is there; otherwise every line shows the proxy's. See [About `LASTHELP_TRUST_PROXY`](../getting-started/install.md#about-lasthelp_trust_proxy).

## When an account is locked

Five wrong passwords for one name inside a minute lock that name for five minutes. The lock is on the name, whichever addresses the guesses came from, and it refuses the right password too while it lasts, because a lock that opens for the correct guess only tells the guesser when to stop. A name with no account is counted the same way, so the refusal does not say which names exist.

The sign-in form answers *Too many requests* and the response carries `Retry-After`. Waiting out the five minutes is enough; restarting the server also clears it, since the count is held in memory.

Password hashing is limited to two at a time. Each one takes 64 MiB, and a form anybody can post to must not turn a burst of requests into a burst of memory. A sign-in under load waits its turn instead of failing.
