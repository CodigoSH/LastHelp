**The house installer, in a shell it actually runs in**

### Added

- The installer can undo itself: `--uninstall` removes it and keeps the
  reports, `--uninstall --purge` erases them too after you type DELETE.
- A configuration file at `/etc/lasthelp/lasthelp.env`, written once on a fresh
  install and never overwritten afterwards — an upgrade keeps the port, the
  proxy setting and anything else you edited by hand.
- The service unit ships with the release and is listed in the signed
  `SHA256SUMS`, so the sandbox the service runs inside is verified like the
  binary rather than being printed by whatever copy of the installer you ran.
  It carries the full set of systemd restrictions the rest of the house uses.

### Changed

- The installer is POSIX `sh` rather than bash, so the documented
  `curl … | sudo sh` runs under dash on Debian and Ubuntu and under BusyBox on
  Alpine, not only where `/bin/sh` happens to be bash. Install with `sudo sh`;
  the beta channel is now `LASTHELP_CHANNEL=beta`.
- The binary lives at `/opt/lasthelp/bin/lasthelp`, owned by the service
  account, which is what lets the built-in updater replace it.

### Fixed

- The installer refuses a port that something else is already listening on,
  instead of installing a service that cannot start.
