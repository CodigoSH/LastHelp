#!/bin/bash
#
# LastHelp installer.
#
# Downloads a release, checks that it is ours, and sets it up as a service.
#
# It refuses rather than guesses. A failed verification stops the install; it
# does not warn and carry on, because a warning in a scrolling terminal is a
# warning nobody reads.
set -euo pipefail

REPO_API="https://git.codigosh.com/api/v1/repos/CodigoSH/LastHelp"
REPO_URL="https://git.codigosh.com/CodigoSH/LastHelp"

INSTALL_DIR="/opt/lasthelp"
BIN="/usr/local/bin/lasthelp"
DATA_DIR="/var/lib/lasthelp"
SERVICE="/etc/systemd/system/lasthelp.service"
USER_NAME="lasthelp"

GREEN='\033[1;32m'; RED='\033[1;31m'; BLUE='\033[1;34m'; GRAY='\033[0;90m'; NC='\033[0m'
step()  { echo -e "${BLUE}  • ${NC}$1"; }
ok()    { echo -e "${GREEN}  ✓ ${NC}$1"; }
die()   { echo -e "${RED}  ✗ ${NC}$1" >&2; exit 1; }
note()  { echo -e "${GRAY}    $1${NC}"; }

# The public keys this installer will accept a release from.
#
# A list, like the one compiled into the binary, and for the same reason: a
# release signed with a rotated key has to remain verifiable by an installer
# somebody downloaded last month. Empty means no release has been signed yet.
RELEASE_KEYS=(
    "BxLbpqP9EKO0HqYHddiBwZ9Jt0sTJsMQ2OMGTcfCCQA="
)

echo
echo "  LastHelp"
echo "  A feedback portal that speaks to your forge."
echo

[ "${EUID:-$(id -u)}" -eq 0 ] || die "This needs root to install a service. Re-run with sudo."

for tool in curl tar sha256sum; do
    command -v "$tool" >/dev/null || die "$tool is required and was not found."
done

case "$(uname -m)" in
    x86_64|amd64)  ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "No release is built for $(uname -m). Building from source works: see the README." ;;
esac
[ "$(uname -s)" = "Linux" ] || die "This installer sets up a systemd service and only runs on Linux."

# --- which release ---------------------------------------------------------

CHANNEL="${CHANNEL:-stable}"
step "Looking for the latest $CHANNEL release…"

if [ "$CHANNEL" = "beta" ]; then
    TAG=$(curl -fsSL "$REPO_API/releases?limit=1" | sed -n 's/.*"tag_name":"\([^"]*\)".*/\1/p' | head -1)
else
    TAG=$(curl -fsSL "$REPO_API/releases/latest" | sed -n 's/.*"tag_name":"\([^"]*\)".*/\1/p' | head -1)
fi
[ -n "${TAG:-}" ] || die "Could not find a release. Has one been published yet?"
ok "Found $TAG"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"

BASE="$REPO_URL/releases/download/$TAG"
BINARY="lasthelp-linux-$ARCH"

# --- verify before installing ----------------------------------------------
#
# The order matters and is the same as the in-app updater's: the checksum file
# is verified first, and only a verified checksum file is allowed to say what
# the binary should hash to. Downloading the binary first and then asking an
# unverified file whether it is right proves nothing.

step "Downloading the checksums…"
curl -fsSL -o SHA256SUMS "$BASE/SHA256SUMS" || die "Could not download SHA256SUMS."

if [ ${#RELEASE_KEYS[@]} -eq 0 ]; then
    die "This installer has no release keys compiled in, so it cannot verify anything.
    That is deliberate: no key does not mean accept anything. Get a newer installer."
fi

if ! command -v openssl >/dev/null; then
    die "openssl is required to check the release signature.
    Install it and run this again — an unverified binary is not worth the convenience."
fi

# Ed25519 signatures over raw data need `pkeyutl -rawin`, and that arrived in
# OpenSSL 3.0. On 1.1.1 there is no command-line way to verify one at all:
# without the flag openssl answers "operation not supported for this keytype",
# and with it, "Option unknown option -rawin". Neither says what is wrong, and
# both look like our signature is broken rather than their openssl being old.
openssl_major=$(openssl version 2>/dev/null | sed -nE 's/^OpenSSL ([0-9]+).*/\1/p')
if [ -z "$openssl_major" ] || [ "$openssl_major" -lt 3 ]; then
    die "openssl 3.0 or newer is required to verify an ed25519 signature.
    This machine has: $(openssl version 2>&1 | head -1)
    Older openssl cannot check this kind of signature at all, and installing
    without checking is not something this script will do for you."
fi

step "Checking the signature…"
curl -fsSL -o SHA256SUMS.sig "$BASE/SHA256SUMS.sig" || die "Could not download the signature."
base64 -d < SHA256SUMS.sig > signature.bin 2>/dev/null || die "The signature is not valid base64."

verified=false
for key in "${RELEASE_KEYS[@]}"; do
    # An ed25519 public key as a DER SubjectPublicKeyInfo is a fixed 12-byte
    # prefix followed by the 32 raw bytes, which is what lets openssl read a
    # key we ship as plain base64.
    printf 'MCowBQYDK2VwAyEA' > key.b64
    printf '%s' "$key" | tail -c 44 >> key.b64
    base64 -d < key.b64 > key.der 2>/dev/null || continue
    openssl pkey -pubin -inform DER -in key.der -out key.pem 2>/dev/null || continue
    if openssl pkeyutl -verify -pubin -inkey key.pem -rawin -in SHA256SUMS -sigfile signature.bin >/dev/null 2>&1; then
        verified=true
        break
    fi
done
$verified || die "The signature does not match any key this installer knows.
    Either this release was not signed by us, or something changed it on the way here.
    Not installing."
ok "Signed by CodigoSH"

step "Downloading $BINARY…"
# Bounded: a release binary is tens of megabytes, and an unbounded download from
# anywhere is a way to fill a disk.
curl -fsSL --max-filesize 209715200 -o "$BINARY" "$BASE/$BINARY" || die "Download failed."

step "Checking the binary against the signed checksums…"
grep " $BINARY\$" SHA256SUMS > expected.txt || die "$BINARY is not listed in SHA256SUMS."
sha256sum -c expected.txt >/dev/null 2>&1 || die "The binary does not match its checksum. Not installing."
ok "The binary is the one that was signed"

# --- install ---------------------------------------------------------------

UPDATING=false
[ -f "$SERVICE" ] && UPDATING=true

PORT="${PORT:-8080}"
if $UPDATING; then
    PORT=$(sed -n 's/.*LASTHELP_PORT=\([0-9]*\).*/\1/p' "$SERVICE" | head -1)
    PORT="${PORT:-8080}"
    step "Updating an existing installation on port $PORT…"
    systemctl stop lasthelp 2>/dev/null || true
fi

id -u "$USER_NAME" >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin "$USER_NAME"
install -d -o "$USER_NAME" -g "$USER_NAME" -m 750 "$INSTALL_DIR" "$DATA_DIR"

if $UPDATING && [ -f "$DATA_DIR/lasthelp.db" ]; then
    # Before replacing anything. An update that goes wrong with no backup is an
    # update nobody can undo.
    BACKUP="$DATA_DIR/lasthelp.db.before-$TAG"
    step "Backing up the database to $(basename "$BACKUP")…"
    cp "$DATA_DIR/lasthelp.db" "$BACKUP"
    chown "$USER_NAME:$USER_NAME" "$BACKUP"
fi

install -m 755 "$BINARY" "$BIN"
ok "Installed $BIN"

if ! $UPDATING; then
    cat > "$SERVICE" <<UNIT
[Unit]
Description=LastHelp
Documentation=$REPO_URL
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
Group=$USER_NAME
WorkingDirectory=$DATA_DIR
ExecStart=$BIN
Restart=on-failure
RestartSec=5

Environment="LASTHELP_PORT=$PORT"
Environment="LASTHELP_DB_FILE=$DATA_DIR/lasthelp.db"
Environment="LASTHELP_KEY_FILE=$DATA_DIR/lasthelp.key"
# Turn this on only when a reverse proxy really is in front. With it on and no
# proxy there, a caller chooses the address their requests are attributed to and
# the rate limits stop meaning anything.
Environment="LASTHELP_TRUST_PROXY=false"

NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload
    systemctl enable lasthelp >/dev/null 2>&1
    ok "Service installed"
fi

systemctl restart lasthelp
sleep 1

if systemctl is-active --quiet lasthelp; then
    ok "Running on port $PORT"
else
    die "The service did not start. Look at: journalctl -u lasthelp -n 40"
fi

echo
if $UPDATING; then
    echo "  Updated to $TAG."
else
    echo "  Open http://localhost:$PORT and claim the instance."
    note "The first account created is the administrator, and setup then refuses to run"
    note "again — so claim it before this is reachable from the internet."
fi
echo
note "Data:  $DATA_DIR/lasthelp.db"
note "Key:   $DATA_DIR/lasthelp.key   (back it up separately from the database)"
note "Logs:  journalctl -u lasthelp -f"
echo
