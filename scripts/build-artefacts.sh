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
build darwin amd64
build darwin arm64

# In the format sha256sum already writes, so anybody can check it with the tools
# already on their machine rather than one of ours.
echo "==> checksums"
( cd "$OUT" && sha256sum lasthelp-* > SHA256SUMS )
cat "${OUT}/SHA256SUMS"

echo
echo "To verify a release yourself:"
echo "  git checkout <tag> && VERSION=<tag> ./scripts/build-artefacts.sh"
echo "  sha256sum -c dist/SHA256SUMS"
