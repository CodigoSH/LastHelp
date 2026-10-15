#!/bin/sh
#
# Installs LastHelp on a Linux machine with systemd.
#
#   curl -fsSL https://git.codigosh.com/CodigoSH/LastHelp/raw/branch/main/install.sh | sudo sh
#
# It does five things and says what each one was: makes a user that owns
# nothing else, fetches a release and checks it, puts the binary in
# /opt/lasthelp/bin, writes a configuration file, and starts a hardened service.
#
# It is safe to run again. An existing installation is upgraded with its
# database backed up first, and an existing configuration is never overwritten —
# the one file an operator has edited by hand is the one thing a script must not
# touch.
#
#   sudo sh install.sh --uninstall            removes it and keeps the data
#   sudo sh install.sh --reinstall            installs it again even when it is already the latest
#   sudo sh install.sh --uninstall --purge    removes the data too
#
# The installer leaves no copy of itself, so when it arrives down a pipe the
# options go through `sh -s --`:
#   curl -fsSL https://git.codigosh.com/CodigoSH/LastHelp/raw/branch/main/install.sh | sudo sh -s -- --uninstall
#
# It refuses rather than guesses. A failed verification stops the install; it
# does not warn and carry on, because a warning in a scrolling terminal is a
# warning nobody reads.
#
# Reading it before running it is the correct instinct, and this is why it is
# short enough to read.
set -eu

# ============================================================ CONFIGURATION
#
# This is CodigoSH's installer, and the point of it is that it is the same one
# everywhere: what differs between projects is this block and nothing else, so a
# fix made in one project is a file copied to the others.
#
# It is POSIX sh, and that is not decoration. The command above pipes it into
# `sh`, which is dash on Debian and Ubuntu and BusyBox on Alpine — none of them
# has arrays, `local`, `[[` or `echo -e`. Check a change with `dash -n` or
# `busybox sh -n` before pushing it: a bashism here is not a subtle bug, it is a
# syntax error before the first line runs, on the three distributions most
# people self-host on.
PROJECT="LastHelp"
NAME="lasthelp"
ENV_PREFIX="LASTHELP"
TAGLINE="A feedback portal that speaks to your forge."
FORGE="https://git.codigosh.com/CodigoSH/LastHelp"
API_BASE="https://git.codigosh.com/api/v1/repos/CodigoSH/LastHelp"
# How this project names its binaries in a release: with dashes. It is the
# one thing a project cannot change without also changing its own updater.
artefact_for() { printf 'lasthelp-linux-%s' "$1"; }

# The public keys a release may be signed with, one per line.
#
# A list, because a release signed with a rotated key has to stay verifiable by
# an installer somebody downloaded last month. Append, never replace.
#
# A newline-separated string rather than an array: arrays are bash, and this
# file has to parse under dash and BusyBox.
RELEASE_KEYS='BxLbpqP9EKO0HqYHddiBwZ9Jt0sTJsMQ2OMGTcfCCQA='

# ============================================================ appearance
#
# Colour only when something is going to read it. Piped into a file or a log
# these become escape codes in the middle of the words, and the first thing
# anybody does with an installer that failed is paste its output somewhere.
if [ -t 1 ]; then
    RED=$(printf '\033[1;31m');  GREEN=$(printf '\033[1;32m')
    BLUE=$(printf '\033[1;34m'); GRAY=$(printf '\033[0;90m')
    BOLD=$(printf '\033[1m');    OFF=$(printf '\033[0m')
else
    RED= ; GREEN= ; BLUE= ; GRAY= ; BOLD= ; OFF=
fi

step()  { printf '%s  • %s%s\n' "$BLUE" "$OFF" "$1"; }
done_() { printf '%s  ✓ %s%s\n' "$GREEN" "$OFF" "$1"; }
note()  { printf '%s    %s%s\n' "$GRAY" "$1" "$OFF"; }
say()   { printf '%s\n' "$*"; }
die()   { printf '%s  ✗ %s%s\n' "$RED" "$1" "$OFF" >&2; exit 1; }

# ============================================================ settings
#
# Every path is overridable so this script can be run end to end against a
# temporary tree without touching the machine it is being tested on. Nothing
# else moves them.
VERSION="${LASTHELP_VERSION:-}"
BASE="${LASTHELP_RELEASES:-$FORGE/releases}"
API="${LASTHELP_API:-$API_BASE}"
PREFIX="${LASTHELP_PREFIX:-/opt/$NAME}"
DATA="${LASTHELP_DATA:-/var/lib/$NAME}"
ETC="${LASTHELP_ETC:-/etc/$NAME}"
USER_NAME="${LASTHELP_USER:-$NAME}"
UNIT="${LASTHELP_UNIT:-/etc/systemd/system/$NAME.service}"

PORT="${LASTHELP_PORT:-8080}"
# Answerable without a terminal, because otherwise the beta is uninstallable
# without one: the question below is skipped in a container or a cron job, and
# the default is `stable`.
CHANNEL="${LASTHELP_CHANNEL:-stable}"
PROXIED=no

UNINSTALL=no
PURGE=no
REINSTALL=no
for ARG in "$@"; do
    case "$ARG" in
        --uninstall) UNINSTALL=yes ;;
        --purge) PURGE=yes ;;
        --reinstall) REINSTALL=yes ;;
        --help|-h) sed -n '2,29p' "$0" 2>/dev/null | sed 's/^#\{0,1\} \{0,1\}//'; exit 0 ;;
        *) die "unknown option: $ARG" ;;
    esac
done

[ "$(id -u)" -eq 0 ] || die "run this as root (sudo sh install.sh)"

# ============================================================ uninstall
#
# Nothing here runs `rm -rf` on a path it has not looked at first. Every one of
# them can be pointed somewhere else by an environment variable, and an empty or
# relative one turns a tidy-up into whatever happens to be in the working
# directory.
safe_to_remove() {
    case "$1" in
        /|/usr|/etc|/var|/opt|/home|/root|/boot) die "refusing to remove $1" ;;
        /*) [ ${#1} -gt 4 ] || die "refusing to remove $1" ;;
        *) die "refusing to remove a path that is not absolute: '$1'" ;;
    esac
}

if [ "$UNINSTALL" = yes ]; then
    safe_to_remove "$PREFIX"

    # Confirmed before anything is touched, not after. Somebody who types the
    # wrong word — or runs this where there is no terminal to type on — should
    # end up with the installation they started with, not half of one.
    if [ "$PURGE" = yes ]; then
        safe_to_remove "$DATA"
        safe_to_remove "$ETC"
        if (exec 3< /dev/tty) 2>/dev/null; then
            printf '  %sType the word DELETE to erase %s and %s: %s' "$BOLD" "$DATA" "$ETC" "$OFF" > /dev/tty
            IFS= read -r CONFIRM < /dev/tty 2>/dev/null || CONFIRM=
            [ "$CONFIRM" = "DELETE" ] || die "not confirmed. Nothing has been removed."
        else
            die "--purge needs a terminal to confirm on. Nothing has been removed."
        fi
    fi

    step "Stopping the service..."
    systemctl disable --now "$NAME" >/dev/null 2>&1 || true
    rm -f "$UNIT"
    systemctl daemon-reload >/dev/null 2>&1 || true

    step "Removing the binary..."
    rm -rf "$PREFIX"

    if [ "$PURGE" = yes ]; then
        rm -rf "$DATA" "$ETC"
        userdel "$USER_NAME" >/dev/null 2>&1 || true
        say ""
        done_ "Removed, data and all."
    else
        say ""
        done_ "Removed. The reports are still in $DATA"
        note "Run again with --purge to erase them, or reinstall to pick up where you left off."
    fi
    exit 0
fi

# ============================================================ what we need
command -v systemctl >/dev/null 2>&1 \
    || die "this installer needs systemd; see the documentation for other init systems"
for TOOL in curl sha256sum; do
    command -v "$TOOL" >/dev/null 2>&1 || die "$TOOL is needed and is not installed"
done

case "$(uname -s)" in
    Linux) ;;
    *) die "this installer sets up a systemd service and only runs on Linux" ;;
esac

case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "no build for $(uname -m). Building from source works: $FORGE" ;;
esac
ARTEFACT=$(artefact_for "$ARCH")

banner() {
    [ -t 1 ] || return 0
    clear
    printf '%s' "$BLUE"
    cat <<'ART'
 _              _   _   _      _
| |    __ _ ___| |_| | | | ___| |_ __
| |   / _` / __| __| |_| |/ _ \ | '_ \
| |__| (_| \__ \ |_|  _  |  __/ | |_) |
|_____\__,_|___/\__|_| |_|\___|_| .__/
                                |_|
ART
    printf '%s\n' "$OFF"
}

banner
say "  Welcome to $PROJECT. Let's get you started."
say ""

# ============================================================ what is here
#
# An existing installation answers the questions below by itself. Asking
# somebody to retype the port they chose last time is how an upgrade changes it
# by accident.
UPGRADE=no
INSTALLED=
# The unit, not the binary, is what says an installation is here.
#
# Looking for the binary in the place *this* version puts it means an
# installation made by an earlier one is invisible: the questions get asked
# again, and then the port check refuses because something is listening on
# 8080 — that something being the very service about to be upgraded. Which is
# what happened, on a real machine, the first time this ran against an install
# it had not made itself.
if [ -f "$UNIT" ]; then
    UPGRADE=yes

    if [ -f "$ETC/$NAME.env" ]; then
        OLD_PORT=$(sed -n "s/^${ENV_PREFIX}_PORT=//p" "$ETC/$NAME.env" | tail -1)
        OLD_PROXY=$(sed -n "s/^${ENV_PREFIX}_TRUST_PROXY=//p" "$ETC/$NAME.env" | tail -1)
    else
        # An installation from before the configuration lived in a file: the
        # settings are Environment= lines inside the unit. They are read out of
        # it here so an upgrade keeps the port somebody chose rather than
        # quietly moving their server to 8080.
        LEGACY=yes
        OLD_PORT=$(sed -n 's/.*Environment="'"${ENV_PREFIX}"'_PORT=\([0-9]*\)".*/\1/p' "$UNIT" | head -1)
        OLD_PROXY=$(sed -n 's/.*Environment="'"${ENV_PREFIX}"'_TRUST_PROXY=\([a-z]*\)".*/\1/p' "$UNIT" | head -1)
    fi

    [ -n "${OLD_PORT:-}" ] && PORT="$OLD_PORT"
    case "${OLD_PROXY:-}" in
        true|1|yes) PROXIED=yes ;;
    esac

    INSTALLED=$("$PREFIX/bin/$NAME" version 2>/dev/null \
        | sed -n 's/.*"version":"\([^"]*\)".*/\1/p' | head -1)
    if [ -n "$INSTALLED" ]; then
        step "Existing installation found (${BOLD}${INSTALLED}${OFF}) on port ${BOLD}${PORT}${OFF}."
        # Somebody on a beta who just presses Enter stays on the beta channel: a
        # default that quietly moved them to stable would be a downgrade they
        # never asked for, and where no stable release exists yet it finds nothing.
        # An explicit LASTHELP_CHANNEL always wins.
        if [ -z "${LASTHELP_CHANNEL:-}" ]; then
            case "$INSTALLED" in *-*) CHANNEL=beta ;; esac
        fi
    else
        step "Existing installation found on port ${BOLD}${PORT}${OFF}."
    fi
    say ""
fi

# ============================================================ questions
#
# From /dev/tty rather than stdin, because stdin is the script itself when this
# arrives down a pipe — which is how the README says to run it. Without a
# terminal, every answer is its default and nothing blocks.
#
# Opened rather than tested for, and opened inside a subshell. `[ -c /dev/tty ]`
# only says the device node is there: it is there inside a container, under
# nohup and in a systemd unit, and openable in none of them. And `:` is a
# special built-in, so POSIX has a redirection error on it kill a
# non-interactive shell outright — even inside the condition of an `if`. The
# subshell contains that: it dies, the parent reads its status, the questions
# are skipped.
if (exec 3< /dev/tty) 2>/dev/null; then
    # The prompt goes to the terminal, not to stdout: this is called as
    # `IN=$(ask ...)` and command substitution would swallow a prompt printed on
    # stdout, leaving the installer waiting for an answer to a question nobody
    # can see.
    ask() {
        printf '%s' "$1" > /dev/tty
        IFS= read -r REPLY < /dev/tty 2>/dev/null || REPLY=
        printf '%s' "$REPLY"
    }

    # Port, then channel, then proxy: the order the rest of the house asks in.
    if [ "$UPGRADE" = no ]; then
        IN=$(ask "$(printf '  Desired Port [%s]: ' "$PORT")")
        [ -n "$IN" ] && PORT="$IN"
    fi

    IN=$(ask "$(printf '  Release Channel (stable/beta) [%s]: ' "$CHANNEL")")
    case "$IN" in beta|Beta|BETA) CHANNEL=beta ;; stable|Stable|STABLE) CHANNEL=stable ;; esac

    if [ "$UPGRADE" = no ]; then
        IN=$(ask '  Behind a reverse proxy? (nginx, Traefik, Caddy, Cloudflare...) (yes/no) [no]: ')
        case "$IN" in y|Y|yes|Yes) PROXIED=yes ;; esac
    fi
    say ""
fi

# The opening three, in the house's order and its words.
#
# They are what makes an install of any of these feel like the same product in
# the first seconds, which is the whole point of the installer being shared. The
# architecture was settled above — the artefact name depends on it — and this is
# where a person finds out, in the same place they find out in the others.
step "Checking system compatibility..."
step "Preparing environment..."

# A port has to be a number in range, and the answer to that question arrives
# from a person typing at three in the morning. A bad one here becomes a service
# that will not start and a log line about parsing an address.
case "$PORT" in
    ''|*[!0-9]*) die "the port must be a number, not \"$PORT\"" ;;
esac
[ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || die "the port must be between 1 and 65535"
# Below 1024 needs a capability the service unit deliberately does not grant.
[ "$PORT" -ge 1024 ] \
    || die "ports below 1024 need privileges this service does not have. Put a reverse proxy in front instead."

# What somebody types into a browser, as opposed to what the process binds to.
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
[ -n "${IP:-}" ] || IP=localhost

# ============================================================ which release
#
# `releases/latest` on Forgejo means the latest *stable* one and skips
# pre-releases, so on a project whose only tag is a beta it is a 404 with no
# explanation. The channel is asked for above precisely so that answer can be
# "the newest thing there is", which is what somebody trying the beta wants.
if [ -z "$VERSION" ]; then
    step "Checking for latest $CHANNEL version..."
    if [ "$CHANNEL" = beta ]; then
        VERSION=$(curl -fsSL "$API/releases?limit=1" 2>/dev/null \
            | sed -n 's/.*"tag_name":"\([^"]*\)".*/\1/p' | head -1)
    else
        VERSION=$(curl -fsSL "$API/releases/latest" 2>/dev/null \
            | sed -n 's/.*"tag_name":"\([^"]*\)".*/\1/p' | head -1)
    fi
    [ -n "$VERSION" ] \
        || die "no $CHANNEL release found. Try the beta channel, or set ${ENV_PREFIX}_VERSION."
fi
RELEASE="$BASE/download/$VERSION"

# Nothing to do is an answer, not an error. Compared as text on purpose: the
# only question is "is it the one I would install", and ordering a beta against
# a stable is how somebody ends up never offered an update.
if [ "$UPGRADE" = yes ] && [ -n "$INSTALLED" ] && [ "$INSTALLED" = "$VERSION" ] && [ "$REINSTALL" != yes ]; then
    say ""
    done_ "You already have $VERSION. Nothing to do."
    say ""
    note "To install it again, to repair it or to apply other answers, run:"
    say "    ${BLUE}curl -fsSL https://git.codigosh.com/CodigoSH/LastHelp/raw/branch/main/install.sh | sudo sh -s -- --reinstall${OFF}"
    say ""
    exit 0
fi

step "Downloading latest version..."

# ============================================================ download
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT INT TERM

if [ -n "$INSTALLED" ] && [ "$INSTALLED" = "$VERSION" ]; then
    step "Reinstalling $VERSION..."
elif [ -n "$INSTALLED" ]; then
    step "Updating $INSTALLED → $VERSION..."
else
    step "Installing $CHANNEL: $VERSION"
fi
curl -fsSL "$RELEASE/SHA256SUMS"       -o "$WORK/SHA256SUMS"      || die "cannot reach $RELEASE"
curl -fsSL "$RELEASE/SHA256SUMS.sig"   -o "$WORK/SHA256SUMS.sig"  || die "the release has no signature"
curl -fsSL "$RELEASE/$NAME.service"    -o "$WORK/$NAME.service"   || die "the release has no service unit"
# Bounded: a release binary is tens of megabytes, and an unbounded download from
# anywhere is a way to fill a disk.
curl -fsSL --max-filesize 209715200 "$RELEASE/$ARTEFACT" -o "$WORK/$ARTEFACT" \
    || die "no build for $ARCH in this release"

# ============================================================ verify
#
# The order matters: the checksum file is verified first, and only a verified
# checksum file is allowed to say what the binary should hash to. Downloading
# the binary first and then asking an unverified file whether it is right proves
# nothing.
step "Verifying signature..."
[ -n "$RELEASE_KEYS" ] || die "this installer has no release keys compiled in, so it cannot verify
    anything. That is deliberate: no key does not mean accept anything. Get a newer installer."

command -v openssl >/dev/null 2>&1 \
    || die "openssl is needed to check the release signature, and is not installed"

# `pkeyutl -rawin` arrived in OpenSSL 3.0. On 1.1.1 there is no command-line way
# to verify ed25519 at all: without the flag openssl answers "operation not
# supported for this keytype", and with it, "Option unknown option -rawin".
# Neither says what is wrong, and both read as though our signature were broken
# rather than their openssl too old.
OPENSSL_MAJOR=$(openssl version 2>/dev/null | sed -n 's/^OpenSSL \([0-9]*\).*/\1/p')
[ -n "$OPENSSL_MAJOR" ] && [ "$OPENSSL_MAJOR" -ge 3 ] || die "openssl 3.0 or newer is required to
    verify an ed25519 signature; this machine has $(openssl version 2>&1 | head -1).
    Installing without checking is not something this script will do for you."

base64 -d < "$WORK/SHA256SUMS.sig" > "$WORK/signature.bin" 2>/dev/null \
    || die "the signature is not valid base64"

VERIFIED=no
printf '%s\n' "$RELEASE_KEYS" | while IFS= read -r KEY; do
    [ -n "$KEY" ] || continue
    # An ed25519 public key as a DER SubjectPublicKeyInfo is a fixed 12-byte
    # prefix followed by the 32 raw bytes, which is what lets openssl read a key
    # shipped as plain base64.
    printf '%s' "MCowBQYDK2VwAyEA$KEY" | base64 -d > "$WORK/key.der" 2>/dev/null || continue
    openssl pkey -pubin -inform DER -in "$WORK/key.der" -out "$WORK/key.pem" 2>/dev/null || continue
    if openssl pkeyutl -verify -pubin -inkey "$WORK/key.pem" -rawin \
        -in "$WORK/SHA256SUMS" -sigfile "$WORK/signature.bin" >/dev/null 2>&1; then
        # The loop is a subshell — a pipeline's last stage is, in every POSIX
        # shell — so a variable set here does not survive it. The answer leaves
        # through the filesystem, which does.
        : > "$WORK/verified"
        break
    fi
done
[ -f "$WORK/verified" ] && VERIFIED=yes
[ "$VERIFIED" = yes ] \
    || die "the signature matches no key this installer knows. Either this release was not signed
    by us, or something changed it on the way here. Not installing."
done_ "Signature verified."

step "Verifying checksum..."
( cd "$WORK" && grep -E " ($ARTEFACT|$NAME\.service)\$" SHA256SUMS | sha256sum -c - ) >/dev/null 2>&1 \
    || die "the download does not match its signed checksums. Do not run it."
done_ "Checksum verified."

chmod 0755 "$WORK/$ARTEFACT"
# It has to be able to say what it is before it goes anywhere near a database.
"$WORK/$ARTEFACT" version >/dev/null 2>&1 \
    || die "the downloaded binary does not run on this machine. Nothing was installed."

# ============================================================ the port
#
# Reported, never freed. Killing whatever holds the port is an outage caused by
# an installer on a machine where 8080 is somebody's other service. Saying what
# has it is enough: the person running this knows what their own machine does.
#
# And *whose* process it is, is asked rather than assumed. The first version of
# this stopped the service and took whatever was still listening to be a
# stranger; before that it did not even look, and refused an upgrade because the
# port was held — by the very service it was about to replace. Reading
# /proc/<pid>/exe answers the real question, does not depend on systemd knowing
# the unit by the name this script expects, and names the intruder in the
# message.
#
# No `ss`, no check: refusing an upgrade because the machine has no iproute2
# would be a worse failure than the one this prevents.
if command -v ss >/dev/null 2>&1; then
    FOREIGN=""
    for PID in $(ss -ltnp "sport = :$PORT" 2>/dev/null \
        | grep -o 'pid=[0-9]*' | cut -d= -f2 | sort -u); do
        HOLDER=$(readlink -f "/proc/$PID/exe" 2>/dev/null || true)
        case "$HOLDER" in
            # Ours, in either layout: the restart below replaces it.
            "$PREFIX/bin/$NAME"|/usr/local/bin/"$NAME") ;;
            *) FOREIGN="$FOREIGN    PID $PID  ${HOLDER:-unknown}
" ;;
        esac
    done
    if [ -n "$FOREIGN" ]; then
        step "Port $PORT is held by something that is not $PROJECT:"
        printf '%s' "$FOREIGN"
        die "free that port, or choose another one, and run this again. Nothing has been changed."
    fi
fi

# ============================================================ account
if ! id "$USER_NAME" >/dev/null 2>&1; then
    step "Creating the $USER_NAME account..."
    # No login, no home to speak of, no shell. It owns its data directory and
    # nothing else on the machine.
    useradd --system --home-dir "$DATA" --shell /usr/sbin/nologin "$USER_NAME" 2>/dev/null \
        || useradd --system --home-dir "$DATA" --shell /sbin/nologin "$USER_NAME"
fi

# The binary's directory belongs to the service, and that is the whole of what
# makes the built-in updater possible: it replaces the binary by renaming a new
# one over it, which needs write permission on the *directory*. The unit grants
# ReadWritePaths on the same directory for the same reason — two halves of one
# decision, and either one alone is an updater that can only say "permission
# denied".
#
# Running this again repairs an installation that got it wrong: `install -d` on
# a directory that exists still sets its owner.
install -d -m 0755 -o root         -g root         "$PREFIX"
install -d -m 0755 -o "$USER_NAME" -g "$USER_NAME" "$PREFIX/bin"
install -d -m 0700 -o "$USER_NAME" -g "$USER_NAME" "$DATA"
install -d -m 0750 -o root         -g "$USER_NAME" "$ETC"

# ============================================================ configuration
if [ ! -f "$ETC/$NAME.env" ]; then
    step "Writing the configuration..."
    cat > "$ETC/$NAME.env" <<CONF
# $PROJECT configuration. Every setting has a default; these are the ones worth
# knowing about.

${ENV_PREFIX}_PORT=$PORT
${ENV_PREFIX}_DB_FILE=$DATA/$NAME.db

# The address people actually open, through your reverse proxy. Used in the
# links that go out by mail and in the feeds.
#${ENV_PREFIX}_PUBLIC_URL=https://help.example.org

# Turn this on only when a reverse proxy really is in front. With it on and no
# proxy there, a caller chooses the address their requests are attributed to and
# the rate limits stop meaning anything.
${ENV_PREFIX}_TRUST_PROXY=$( [ "$PROXIED" = yes ] && echo true || echo false )
# How many proxies are in front, when there is more than one.
#${ENV_PREFIX}_TRUST_PROXY_HOPS=1

# Mail, for notifying whoever reported something. Without it, nothing is sent
# and the rest of the product works.
#${ENV_PREFIX}_SMTP_HOST=smtp.example.org
#${ENV_PREFIX}_SMTP_PORT=587
#${ENV_PREFIX}_SMTP_USERNAME=
#${ENV_PREFIX}_SMTP_PASSWORD=
#${ENV_PREFIX}_SMTP_FROM=help@example.org
CONF
    chmod 0640 "$ETC/$NAME.env"
    chown root:"$USER_NAME" "$ETC/$NAME.env"
else
    # The one file somebody has edited by hand. Left exactly as it is.
    step "Keeping the existing configuration..."
fi

# ============================================================ install
if [ "$UPGRADE" = yes ]; then
    # Stopped here rather than up at the port check, which is the only place it
    # was needed for: a copy of a database taken from under a running server is
    # a copy of a database mid-write.
    systemctl stop "$NAME" >/dev/null 2>&1 || true
fi

if [ "$UPGRADE" = yes ] && [ -f "$DATA/$NAME.db" ]; then
    # Before replacing anything. An update that goes wrong with no backup is an
    # update nobody can undo.
    BACKUP="$DATA/$NAME.db.before-$VERSION"
    step "Backing up the database to $(basename "$BACKUP")..."
    cp "$DATA/$NAME.db" "$BACKUP"
    chown "$USER_NAME:$USER_NAME" "$BACKUP"
fi

step "Installing the binary..."
# Owned by the service, like the directory holding it. Root-owning the file
# would buy nothing — anybody who can write the directory can replace the file
# whatever it says — and this way a fresh install and an updated one look the
# same on disk.
install -m 0755 -o "$USER_NAME" -g "$USER_NAME" "$WORK/$ARTEFACT" "$PREFIX/bin/$NAME"

# An earlier version of this installer put the binary in /usr/local/bin. The new
# unit runs the one in $PREFIX/bin, so the old copy is not what serves any more
# — and leaving an outdated binary on the PATH under the same name is how
# somebody debugging at midnight ends up running the version before last.
if [ "${LEGACY:-no}" = yes ] && [ -f /usr/local/bin/"$NAME" ]; then
    step "Removing the binary the previous layout left in /usr/local/bin..."
    rm -f /usr/local/bin/"$NAME"
fi

# ============================================================ service
step "Configuring system service..."
install -m 0644 "$WORK/$NAME.service" "$UNIT"
systemctl daemon-reload
systemctl enable "$NAME" >/dev/null 2>&1

if [ "$UPGRADE" = yes ]; then
    systemctl restart "$NAME"
else
    systemctl start "$NAME"
fi

# Waited for rather than assumed. "Installed successfully" followed by a service
# that died on its first line is the worst possible ending.
sleep 2
if ! systemctl is-active --quiet "$NAME"; then
    say ""
    say "  ${RED}The service did not start. What it said:${OFF}"
    journalctl -u "$NAME" -n 20 --no-pager || true
    die "the installation finished and the service is not running"
fi

# ============================================================ and then
#
# Three lines, like the others. What ended up here as well was a warning that
# the first account created is the administrator and should be claimed before
# the server is reachable — true, and important, and still the wrong place for
# it: that belongs to the setup this product is about to grow, not to the tail
# of an installer somebody is already scrolling past. The documentation is where
# the rest lives.
say ""
done_ "Installation complete!"
say ""
say "  💬 Open $PROJECT at: ${BLUE}${BOLD}http://$IP:$PORT${OFF}"
say ""
