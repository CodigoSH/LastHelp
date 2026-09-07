# Contributing

LastHelp is open source and available for everyone to use, study, and build upon
under the terms of its license.

## How development works

Active development happens within the CodigoSH team. We do not follow the
standard open-source contribution model (fork → PR → merge). Instead,
development is coordinated internally among team members with direct repository
access.

This means:

- **The source code is fully open.** You can read it, fork it, run it, and learn
  from it.
- **We do not accept pull requests** from outside the team at this time.
- **Issues and bug reports are welcome.** Open one on our
  [Forgejo repository](https://git.codigosh.com/CodigoSH/LastHelp/issues) — we
  read everything.

There is an irony worth naming: this is a tool for collecting feedback from
people who cannot open an account on your forge, and it is itself developed on a
forge where you cannot open an account. That is exactly the problem it exists to
fix, and it will be fixed here first.

If you run a forge we do not test against — a self-managed GitLab, GitHub
Enterprise Server, an older Gitea — and something about it surprises us, that is
the most valuable report this project can get. `make live-forge` walks the whole
bridge against a real Forgejo or Gitea in a container; GitHub and GitLab are
covered only by a test double, and a double written by the same hands as the
adapter can be wrong in the same way and agree.

What is most useful to report, for this project in particular:

- **A forge that behaves differently from what we assumed.** The four we support
  disagree about what a webhook event is called, what field carries the issue
  number and how a delivery is signed. If your instance does something we did
  not expect, that report is worth more than a feature request.
- **Spam that got through.** Nothing is published without approval, so what gets
  through reaches a person's attention rather than a public page. Knowing what
  slipped past which layer is how the layers improve.
- **A translation that is wrong.** Twenty languages is a lot of surface, and a
  native speaker spotting a phrase nobody actually says is genuinely valuable.

## If you run a fork

The license lets you. If you find and fix something real, telling us about it is
appreciated, and so is not being told — that is what the license means.
