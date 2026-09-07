#!/bin/bash
# Starts a throwaway instance and a headless Chrome, writes the documentation
# screenshots, tears both down.
set -euo pipefail

PORT="${PORT:-8099}"
CDP="${CDP:-9222}"
WORK="$(mktemp -d)"

cleanup() {
    if [ -n "${CHROME_PID:-}" ]; then kill "$CHROME_PID" 2>/dev/null || true; wait "$CHROME_PID" 2>/dev/null || true; fi
    if [ -n "${SERVER_PID:-}" ]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
    rm -rf "$WORK" 2>/dev/null || true
}
trap cleanup EXIT

LASTHELP_PORT="$PORT" LASTHELP_DB_FILE="$WORK/shots.db" \
    LASTHELP_PUBLIC_URL="http://127.0.0.1:$PORT" \
    ./bin/lasthelp > "$WORK/server.log" 2>&1 &
SERVER_PID=$!

google-chrome --headless=new --disable-gpu --no-first-run --no-default-browser-check \
    --remote-debugging-port="$CDP" --user-data-dir="$WORK/profile" \
    about:blank > "$WORK/chrome.log" 2>&1 &
CHROME_PID=$!

for _ in $(seq 1 80); do
    if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 &&
       curl -sf "http://127.0.0.1:$CDP/json/version" >/dev/null 2>&1; then break; fi
    sleep 0.25
done

PORT="$PORT" CDP="$CDP" bun run scripts/docs-shots.ts
