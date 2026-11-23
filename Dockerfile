# The house's image: the build stages run on the machine doing the build and
# cross-compile for the target, and the runtime stage runs no commands at all.
# That is what lets one runner publish amd64 and arm64 without emulating
# anything — a RUN in the last stage would have to execute under QEMU for every
# architecture but the builder's own.
#
# Where the binaries come from is a build argument:
#
#   BINARIES=source    (the default) compiles them here, from the source in the
#                      build context. `docker build .` from a checkout.
#   BINARIES=prebuilt  takes them, already built, from image-bin/ in the build
#                      context. The release does this: the image then carries
#                      the very binaries that are on the release page, signed
#                      in its SHA256SUMS, and not a second build of the same
#                      commit.
ARG BINARIES=source

# ---- frontend -------------------------------------------------------------
# Pinned, and to the same version the CI installs: a floating tag means the
# image that built the release is not the one the tests ran against.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.4-alpine AS frontend
WORKDIR /src
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile
COPY tsconfig.json ./
COPY web ./web
RUN bun run build

# ---- binaries, built here -------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS bins-source
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=frontend /src/internal/web/dist ./internal/web/dist

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown

# -buildvcs=false on purpose: .git is excluded from the build context, so the
# default VCS stamping would fail the build here. -trimpath and the explicit
# GOOS/GOARCH are what make the result reproducible.
#
# No signing key is passed, and none should be: the binary inside a container
# refuses to update itself, because updating a container is pulling a new
# image. The panel says so instead of offering a button.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
        -ldflags "-s -w -X git.codigosh.com/CodigoSH/LastHelp/internal/version.Current=${VERSION} -X git.codigosh.com/CodigoSH/LastHelp/internal/version.Commit=${COMMIT}" \
        -o /out/lasthelp ./cmd/lasthelp && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
        -ldflags "-s -w" \
        -o /out/healthcheck ./cmd/healthcheck && \
    mkdir -p /out/data && \
    cp /etc/ssl/certs/ca-certificates.crt /out/ca-certificates.crt

# ---- binaries, built by the release ---------------------------------------
# On the build platform too, so nothing here runs under emulation. Only what is
# named is copied: image-bin/ holds the server and the health check for each
# architecture, and the server is the file published on the release page.
FROM --platform=$BUILDPLATFORM alpine:3.24 AS bins-prebuilt
ARG TARGETARCH
COPY image-bin/lasthelp-linux-${TARGETARCH} /out/lasthelp
COPY image-bin/healthcheck-linux-${TARGETARCH} /out/healthcheck
RUN mkdir -p /out/data && \
    cp /etc/ssl/certs/ca-certificates.crt /out/ca-certificates.crt

FROM bins-${BINARIES} AS bins

# ---- runtime --------------------------------------------------------------
# No RUN here, see the top. What the old image made with apk and adduser comes
# in as files instead: the CA bundle (text, the same on every architecture), and
# a data directory already owned by the account the server runs as, which is
# named by number because there is no passwd entry to name it by.
FROM alpine:3.24

COPY --from=bins /out/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=bins /out/lasthelp /usr/local/bin/lasthelp
COPY --from=bins /out/healthcheck /usr/local/bin/healthcheck
COPY --from=bins --chown=10001:10001 /out/data /data

USER 10001:10001
WORKDIR /data
VOLUME /data

ENV LASTHELP_PORT=8080 \
    LASTHELP_DB_FILE=/data/lasthelp.db

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

ENTRYPOINT ["/usr/local/bin/lasthelp"]
