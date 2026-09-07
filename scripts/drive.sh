#!/bin/bash
# Starts a throwaway instance and a headless Chrome, runs the journey, tears
# both down. Kept apart from the Makefile because the cleanup matters: a browser
# left running holds a debugging port that the next run cannot bind.
set -euo pipefail

PORT="${PORT:-8099}"
CDP="${CDP:-9222}"
WORK="$(mktemp -d)"

cleanup() {
    [ -n "${CHROME_PID:-}" ] && kill "$CHROME_PID" 2>/dev/null || true
    [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null || true
    rm -rf "$WORK"
}
trap cleanup EXIT

LASTHELP_PORT="$PORT" LASTHELP_DB_FILE="$WORK/drive.db" ./bin/lasthelp > "$WORK/server.log" 2>&1 &
SERVER_PID=$!

google-chrome --headless=new --disable-gpu --no-first-run --no-default-browser-check \
    --remote-debugging-port="$CDP" --user-data-dir="$WORK/profile" \
    about:blank > "$WORK/chrome.log" 2>&1 &
CHROME_PID=$!

for _ in $(seq 1 80); do
    if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 &&
       curl -sf "http://127.0.0.1:$CDP/json/version" >/dev/null 2>&1; then
        break
    fi
    sleep 0.25
done

PORT="$PORT" CDP="$CDP" bun run scripts/drive-check.ts
