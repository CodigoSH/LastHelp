#!/bin/bash
# Builds the release binaries, reproducibly, and writes SHA256SUMS.
#
# A script rather than steps in a workflow: a workflow that rebuilt this by hand
# would be a second implementation of the release, and the two would drift. It
# runs locally too, which is how "rebuild it yourself and compare the hashes"
# stops being a promise nobody has tested.
set -euo pipefail

VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
OUT="${OUT:-dist}"
MODULE="git.codigosh.com/CodigoSH/LastHelp"

# The frontend first: the binary embeds it, so building the binary against a
# stale dist is how a release ships the previous version's interface. It has
# cost this house a whole round more than once.
echo "==> frontend"
bun install --frozen-lockfile
bun run build

rm -rf "$OUT"
mkdir -p "$OUT"

# What makes two builds of the same commit produce the same bytes:
#
#   -trimpath        removes the build machine's absolute paths
#   CGO_ENABLED=0    no C toolchain, so no toolchain to differ
#   GOOS/GOARCH      explicit, never inherited from whoever is building
#   -buildvcs=false  the VCS stamp includes state that is not the source
#
# The frontend contributes a stamp derived from its own output rather than from
# the clock, which is the other half of the same promise.
export CGO_ENABLED=0

build() {
    local goos=$1 goarch=$2
    local name="lasthelp-${goos}-${goarch}"
    echo "==> ${name}"
    GOOS="$goos" GOARCH="$goarch" go build \
        -trimpath -buildvcs=false \
        -ldflags "-s -w -X ${MODULE}/internal/version.Current=${VERSION} -X ${MODULE}/internal/version.Commit=${COMMIT}" \
        -o "${OUT}/${name}" ./cmd/lasthelp
}

build linux amd64
build linux arm64

# Linux, and only Linux, because that is all this product installs onto: the
# installer sets up a systemd unit and refuses anything else, and the container
# image is built for the same two architectures. A macOS binary was built and
# published here for one release — nothing asked for it, nothing tested it, and
# a binary on a downloads page is a promise of support whether or not anybody
# meant it as one.
#
# So the list is checked against the installer rather than kept in step by hand.
# The names install.sh can fetch are `lasthelp-linux-<arch>` for the
# architectures it accepts from uname; anything built that it cannot ask for is
# an asset nobody can install, and anything it asks for that is missing is a
# release that dies on a 404 in front of whoever ran the command.
echo "==> what the installer can fetch is what was built"
installable=$(grep -o 'ARCH=[a-z0-9]\{1,\}' install.sh | cut -d= -f2 | sort -u)
[ -n "$installable" ] || { echo "install.sh no longer states its architectures the way this reads them" >&2; exit 1; }
for arch in $installable; do
    [ -f "${OUT}/lasthelp-linux-${arch}" ] || {
        echo "install.sh downloads lasthelp-linux-${arch} and nothing built it" >&2; exit 1; }
done
for built in "${OUT}"/lasthelp-*; do
    name=$(basename "$built")
    case " $(for a in $installable; do printf 'lasthelp-linux-%s ' "$a"; done)" in
        *" ${name} "*) ;;
        *) echo "${name} was built and no installer can ask for it" >&2; exit 1 ;;
    esac
done

# In the format sha256sum already writes, so anybody can check it with the tools
# already on their machine rather than one of ours.
echo "==> checksums"
( cd "$OUT" && sha256sum lasthelp-* > SHA256SUMS )
cat "${OUT}/SHA256SUMS"

echo
echo "To verify a release yourself:"
echo "  git checkout <tag> && VERSION=<tag> ./scripts/build-artefacts.sh"
echo "  sha256sum -c dist/SHA256SUMS"
