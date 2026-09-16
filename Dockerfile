# ---- frontend -------------------------------------------------------------
FROM oven/bun:1.3.11-alpine AS frontend
WORKDIR /src
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile
COPY tsconfig.json ./
COPY web ./web
RUN bun run build

# ---- backend --------------------------------------------------------------
FROM golang:1.26.2-alpine AS backend
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=frontend /src/internal/web/dist ./internal/web/dist

ARG VERSION=dev
ARG COMMIT=unknown

# -buildvcs=false on purpose: .git is excluded from the build context, so the
# default VCS stamping would fail the build here. -trimpath and the pinned
# GOOS/GOARCH are what make the result reproducible.
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false \
        -ldflags "-s -w -X git.codigosh.com/CodigoSH/LastHelp/internal/version.Current=${VERSION} -X git.codigosh.com/CodigoSH/LastHelp/internal/version.Commit=${COMMIT}" \
        -o /out/lasthelp ./cmd/lasthelp && \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
        -ldflags "-s -w" \
        -o /out/healthcheck ./cmd/healthcheck

# ---- runtime --------------------------------------------------------------
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -H -u 10001 lasthelp && \
    mkdir -p /data && chown lasthelp:lasthelp /data

COPY --from=backend /out/lasthelp /usr/local/bin/lasthelp
COPY --from=backend /out/healthcheck /usr/local/bin/healthcheck

USER lasthelp
WORKDIR /data
VOLUME /data

ENV LASTHELP_PORT=8080 \
    LASTHELP_DB_FILE=/data/lasthelp.db

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

ENTRYPOINT ["/usr/local/bin/lasthelp"]
