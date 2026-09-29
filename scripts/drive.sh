#!/bin/bash
# Starts a throwaway instance and a headless Chrome, runs the journey, tears
# both down. Kept apart from the Makefile because the cleanup matters: a browser
# left running holds a debugging port that the next run cannot bind.
set -euo pipefail

# A port nobody else is on, rather than the same two every time.
#
# Two passes running at once used to fight over 8099 and 9222 — the second one's
# browser would attach to the first one's instance, or the instance would fail
# to bind and the journey would read an empty page. Every "flaky" failure chased
# in this suite turned out to be that: two passes started by the same person in
# the same minute, not a fault in the product.
free_port() {
    candidate="$1"
    while ss -ltn 2>/dev/null | grep -q ":${candidate} "; do
        candidate=$((candidate + 1))
    done
    echo "$candidate"
}

PORT="${PORT:-$(free_port $((8100 + $$ % 400)))}"
CDP="${CDP:-$(free_port $((9300 + $$ % 400)))}"
WORK="$(mktemp -d)"

cleanup() {
    # Wait for the browser to actually exit before removing its profile: kill
    # returns as soon as the signal is sent, and Chrome is still writing to the
    # directory for a moment afterwards, so the rm fails and the run reports a
    # failure it did not have.
    if [ -n "${CHROME_PID:-}" ]; then
        kill "$CHROME_PID" 2>/dev/null || true
        wait "$CHROME_PID" 2>/dev/null || true
    fi
    if [ -n "${SERVER_PID:-}" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -rf "$WORK" 2>/dev/null || true
}
trap cleanup EXIT

# SEED=1 starts from an installation with history (scripts/seed) instead of an
# empty one; CHECK names the pass to run against it. The defaults are the main
# journey on an empty installation, which has to begin with the setup.
if [ -n "${SEED:-}" ]; then
    go run ./scripts/seed -db "$WORK/drive.db"
fi

LASTHELP_PORT="$PORT" LASTHELP_DB_FILE="$WORK/drive.db" ./bin/lasthelp > "$WORK/server.log" 2>&1 &
SERVER_PID=$!

# 1280x900, which is the size the documentation screenshots use and a size
# somebody actually has. Left to itself headless Chrome opens 800x600: the whole
# journey was being driven at a window nobody uses, and a front page of modules
# sized for a desktop came out cramped there and looked like a layout fault.
google-chrome --headless=new --disable-gpu --no-first-run --no-default-browser-check \
    --window-size=1280,900 \
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

PORT="$PORT" CDP="$CDP" bun run "scripts/${CHECK:-drive-check.ts}"
