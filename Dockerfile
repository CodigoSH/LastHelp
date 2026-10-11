# The house's image: both build stages run on the
# machine doing the build and cross-compile for the target, and the runtime
# stage runs no commands at all. That is what lets one runner publish amd64
# and arm64 without emulating anything — a RUN in the last stage would have to
# execute under QEMU for every architecture but the builder's own.

# ---- frontend -------------------------------------------------------------
# Pinned, and to the same version the CI installs: a floating tag means the
# image that built the release is not the one the tests ran against.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.3-alpine AS frontend
WORKDIR /src
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile
COPY tsconfig.json ./
COPY web ./web
RUN bun run build

# ---- backend --------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS backend
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
    mkdir -p /out/data

# ---- runtime --------------------------------------------------------------
# No RUN here, see the top. What the old image made with apk and adduser comes
# in as files instead: the CA bundle from the build stage (text, the same on
# every architecture), and a data directory already owned by the account the
# server runs as, which is named by number because there is no passwd entry to
# name it by.
FROM alpine:3.24

COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /out/lasthelp /usr/local/bin/lasthelp
COPY --from=backend /out/healthcheck /usr/local/bin/healthcheck
COPY --from=backend --chown=10001:10001 /out/data /data

USER 10001:10001
WORKDIR /data
VOLUME /data

ENV LASTHELP_PORT=8080 \
    LASTHELP_DB_FILE=/data/lasthelp.db

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

ENTRYPOINT ["/usr/local/bin/lasthelp"]
