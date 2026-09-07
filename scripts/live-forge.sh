#!/bin/bash
#
# Walks the forge bridge against a real Forgejo or Gitea, in a container.
#
# The adapters were written from documentation, and for Gitea and Forgejo from
# reading the structs they serialise — their webhook payload is not documented.
# Everything else that tests them uses a fake this repository also wrote, so both
# halves could be wrong in the same way and agree. This is the only check that
# can tell.
#
#   ./scripts/live-forge.sh            # Forgejo
#   FORGE_KIND=gitea ./scripts/live-forge.sh
#
# It needs podman or docker, and pulls an image the first time. Nothing is
# installed on this machine and nothing is left behind.
set -euo pipefail

FORGE_KIND="${FORGE_KIND:-forgejo}"
PORT="${PORT:-8099}"
CDP="${CDP:-9222}"
FORGE_PORT="${FORGE_PORT:-3000}"
PASSWORD="a good long passphrase"

# The version matters, and pinning one by eye is how a check ends up proving
# something about software nobody runs. The first version of this pinned Forgejo
# 11 while CodigoSH — and everybody else — was on 16.
#
# So the tag is asked for rather than assumed, with a floor that is only used
# when there is no network. FORGE_VERSION overrides it, which is how you test
# against the version an operator reported a problem on.
latest_forgejo() {
    curl -s --max-time 15 "https://codeberg.org/api/v1/repos/forgejo/forgejo/releases?limit=20" 2>/dev/null |
        python3 -c "
import sys, json
try:
    releases = [r['tag_name'].lstrip('v') for r in json.load(sys.stdin)
                if not r.get('prerelease') and not r.get('draft')]
    print(sorted(releases, key=lambda v: [int(p) for p in v.split('.')], reverse=True)[0])
except Exception:
    pass" 2>/dev/null
}

case "$FORGE_KIND" in
    forgejo)
        VERSION="${FORGE_VERSION:-$(latest_forgejo)}"
        VERSION="${VERSION:-16}"
        IMAGE="codeberg.org/forgejo/forgejo:$VERSION"
        ENV_PREFIX=FORGEJO; BIN=forgejo ;;
    gitea)
        # Gitea's "1" is a rolling tag on the current stable line, which is what
        # people actually run. It prints its real version on startup below.
        IMAGE="docker.io/gitea/gitea:${FORGE_VERSION:-1}"
        ENV_PREFIX=GITEA; BIN=gitea ;;
    gitlab)  IMAGE="docker.io/gitlab/gitlab-ce:${FORGE_VERSION:-latest}" ;;
    *) echo "FORGE_KIND must be forgejo, gitea or gitlab"; exit 2 ;;
esac

RUNTIME="$(command -v podman || command -v docker || true)"
[ -n "$RUNTIME" ] || { echo "podman or docker is required"; exit 1; }

WORK="$(mktemp -d)"
CONTAINER="lasthelp-live-$FORGE_KIND"

# The forge runs on the host network so it can deliver a webhook to LastHelp at
# 127.0.0.1. That also means every port the forge opens is a port on this
# machine, and a port already taken is a crash the forge reports as its own.
free_port() {
    python3 -c "import socket; s = socket.socket(); s.bind(('127.0.0.1', 0)); print(s.getsockname()[1]); s.close()"
}

port_is_free() {
    ! ss -lntH "sport = :$1" 2>/dev/null | grep -q .
}

# Chrome does not fail loudly when its debugging port is taken: it exits, and
# the wait below then finds somebody else's browser answering on that port and
# drives that instead. It happened — a Chrome left over from another session
# ran the whole check. The assertions still held, because they are about
# LastHelp and the forge, but the script was not driving what it thought it was.
for named in "PORT:$PORT" "CDP:$CDP"; do
    if ! port_is_free "${named#*:}"; then
        echo "${named%%:*}=${named#*:} is taken; set ${named%%:*} to a free port"
        exit 2
    fi
done

cleanup() {
    if [ -n "${CHROME_PID:-}" ]; then kill "$CHROME_PID" 2>/dev/null || true; wait "$CHROME_PID" 2>/dev/null || true; fi
    if [ -n "${SERVER_PID:-}" ]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
    "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
    # The forge's data directory is bind-mounted, and rootless podman writes it
    # as a subordinate uid this user cannot unlink. A plain rm fails, `|| true`
    # swallows the failure, and every run leaves the tree behind for ever.
    # RUNTIME holds a path, not a name — comparing it to "podman" is always
    # false, which is how the first version of this fix ran green while
    # cleaning nothing.
    if [ -d "$WORK/forge" ] && [ "$(basename "$RUNTIME")" = podman ]; then
        "$RUNTIME" unshare rm -rf "$WORK/forge" >/dev/null 2>&1 || true
    fi
    rm -rf "$WORK" 2>/dev/null || true
    if [ -d "$WORK" ]; then
        echo "note: could not remove $WORK — remove it by hand" >&2
    fi
}
trap cleanup EXIT

wait_for_local_stack() {
    for _ in $(seq 1 80); do
        if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 &&
           curl -sf "http://127.0.0.1:$CDP/json/version" >/dev/null 2>&1; then return 0; fi
        sleep 0.25
    done
    echo
    echo "LastHelp or Chrome never came up:"
    curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 || echo "    no /healthz on $PORT"
    curl -sf "http://127.0.0.1:$CDP/json/version" >/dev/null 2>&1 || echo "    no debugger on $CDP"
    sed 's/^/    /' "$WORK/server.log" 2>/dev/null | tail -20
    sed 's/^/    /' "$WORK/chrome.log" 2>/dev/null | tail -20
    return 1
}


# GitLab is a different animal: minutes to boot, its own way of minting a token,
# and it refuses to deliver a webhook to a local address until an administrator
# says otherwise. All of that is set up here rather than left to the person
# running this, because a check nobody can run is a check nobody runs.
if [ "$FORGE_KIND" = gitlab ]; then
    if ! port_is_free "$FORGE_PORT"; then
        echo "port $FORGE_PORT is taken; set FORGE_PORT to a free one"; exit 2
    fi
    # Puma defaults to 127.0.0.1:8080, and on the host network that is a real
    # port on a real machine. On the one this was written on, 8080 was held by
    # Lastboard — so Puma could not bind, restarted for ever, and readiness
    # never went green. Nothing in GitLab's own logs pointed at the host.
    PUMA_PORT="$(free_port)"
    echo "==> starting gitlab ($IMAGE, puma on $PUMA_PORT; this takes several minutes)"
    "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
    "$RUNTIME" run -d --name "$CONTAINER" --network=host --shm-size=256m \
        -e GITLAB_OMNIBUS_CONFIG="external_url 'http://127.0.0.1:$FORGE_PORT'; gitlab_rails['initial_root_password'] = '$PASSWORD'; puma['worker_processes'] = 2; puma['port'] = $PUMA_PORT; sidekiq['max_concurrency'] = 5; prometheus_monitoring['enable'] = false;" \
        "$IMAGE" >/dev/null

    # Waiting without a failure branch is how the first attempt at this wasted
    # eighteen minutes: the loop ran out, printed "up", and everything after it
    # failed for reasons that looked like our bug. If it does not come up, say
    # so, and say what the container thought it was doing.
    READY=false
    for i in $(seq 1 900); do
        if curl -sf "http://127.0.0.1:$FORGE_PORT/-/readiness" >/dev/null 2>&1; then
            READY=true; break
        fi
        if [ $((i % 30)) -eq 0 ]; then
            echo "    still booting ($((i * 2))s) — $("$RUNTIME" exec "$CONTAINER" gitlab-ctl status 2>/dev/null |
                awk '{print $1}' | sort | uniq -c | tr '\n' ' ')"
        fi
        sleep 2
    done

    if [ "$READY" != true ]; then
        echo
        echo "gitlab never became ready. This is what it was doing:"
        "$RUNTIME" exec "$CONTAINER" gitlab-ctl status 2>&1 | sed 's/^/    /' || true
        echo "    --- puma ---"
        "$RUNTIME" exec "$CONTAINER" tail -n 40 /var/log/gitlab/puma/current 2>&1 | sed 's/^/    /' || true
        echo "    --- reconfigure ---"
        "$RUNTIME" logs --tail 40 "$CONTAINER" 2>&1 | sed 's/^/    /' || true
        exit 1
    fi
    echo "    up"

    echo "==> minting tokens and allowing local webhooks"
    # Without this GitLab refuses to deliver to 127.0.0.1, which would look
    # exactly like our receiver rejecting the delivery.
    "$RUNTIME" exec "$CONTAINER" gitlab-rails runner \
        "ApplicationSetting.current.update!(allow_local_requests_from_web_hooks_and_services: true)" >/dev/null 2>&1

    SETUP_TOKEN="$("$RUNTIME" exec "$CONTAINER" gitlab-rails runner \
        "t = User.find_by_username('root').personal_access_tokens.create!(scopes: ['api'], name: 'setup', expires_at: 1.year.from_now); t.set_token('glpat-lasthelpsetup00000'); t.save!; puts t.token" 2>/dev/null | tail -1)"
    ISSUE_TOKEN="$SETUP_TOKEN"

    PROJECT="$(curl -s -X POST "http://127.0.0.1:$FORGE_PORT/api/v4/projects" \
        -H "PRIVATE-TOKEN: $SETUP_TOKEN" -H 'Content-Type: application/json' \
        -d '{"name":"lastboard","path":"lastboard","visibility":"public","issues_enabled":true}' |
        python3 -c "import sys,json;print(json.load(sys.stdin).get('path_with_namespace',''))" 2>/dev/null)"
    echo "    project: $PROJECT"

    echo "==> starting LastHelp"
    LASTHELP_PORT="$PORT" LASTHELP_DB_FILE="$WORK/live.db" \
        LASTHELP_PUBLIC_URL="http://127.0.0.1:$PORT" \
        ./bin/lasthelp > "$WORK/server.log" 2>&1 &
    SERVER_PID=$!

    google-chrome --headless=new --disable-gpu --no-first-run --no-default-browser-check \
        --remote-debugging-port="$CDP" --user-data-dir="$WORK/profile" \
        about:blank > "$WORK/chrome.log" 2>&1 &
    CHROME_PID=$!

    wait_for_local_stack

    PORT="$PORT" CDP="$CDP" \
        FORGE_URL="http://127.0.0.1:$FORGE_PORT" \
        FORGE_TOKEN="$ISSUE_TOKEN" FORGE_OPERATOR_TOKEN="$SETUP_TOKEN" \
        FORGE_KIND=gitlab FORGE_OWNER=root FORGE_REPO=lastboard \
        bun run scripts/live-forge-check.ts
    exit $?
fi

echo "==> starting $FORGE_KIND ($IMAGE)"
"$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
mkdir -p "$WORK/forge"
"$RUNTIME" run -d --name "$CONTAINER" --network=host \
    -e "${ENV_PREFIX}__server__HTTP_PORT=$FORGE_PORT" \
    -e "${ENV_PREFIX}__server__ROOT_URL=http://127.0.0.1:$FORGE_PORT/" \
    -e "${ENV_PREFIX}__security__INSTALL_LOCK=true" \
    -e "${ENV_PREFIX}__database__DB_TYPE=sqlite3" \
    -e "${ENV_PREFIX}__webhook__ALLOWED_HOST_LIST=*" \
    -v "$WORK/forge:/data:Z" \
    "$IMAGE" >/dev/null

for _ in $(seq 1 120); do
    curl -sf "http://127.0.0.1:$FORGE_PORT/api/v1/version" >/dev/null 2>&1 && break
    sleep 1
done
echo "    $(curl -s "http://127.0.0.1:$FORGE_PORT/api/v1/version")"

echo "==> creating a user, a repository and a token"
"$RUNTIME" exec -u git "$CONTAINER" "$BIN" admin user create \
    --username codigosh --password "$PASSWORD" --email dev@example.com \
    --admin --must-change-password=false >/dev/null 2>&1

mint() {
    curl -s -X POST "http://127.0.0.1:$FORGE_PORT/api/v1/users/codigosh/tokens" \
        -u "codigosh:$PASSWORD" -H 'Content-Type: application/json' \
        -d "{\"name\":\"$1\",\"scopes\":$2}" |
        python3 -c "import sys,json;print(json.load(sys.stdin)['sha1'])"
}

# Two tokens on purpose. The repository is created with a broad one, and
# LastHelp is handed one with only write:issue — which is what the documentation
# tells operators to create, so this is that claim being checked rather than
# repeated.
SETUP_TOKEN="$(mint setup '["write:user","write:repository","write:issue"]')"
curl -s -o /dev/null -X POST "http://127.0.0.1:$FORGE_PORT/api/v1/user/repos" \
    -H "Authorization: token $SETUP_TOKEN" -H 'Content-Type: application/json' \
    -d '{"name":"lastboard","auto_init":true,"private":false}'

ISSUE_TOKEN="$(mint lasthelp '["write:issue"]')"

echo "==> starting LastHelp"
LASTHELP_PORT="$PORT" LASTHELP_DB_FILE="$WORK/live.db" \
    LASTHELP_PUBLIC_URL="http://127.0.0.1:$PORT" \
    ./bin/lasthelp > "$WORK/server.log" 2>&1 &
SERVER_PID=$!

google-chrome --headless=new --disable-gpu --no-first-run --no-default-browser-check \
    --remote-debugging-port="$CDP" --user-data-dir="$WORK/profile" \
    about:blank > "$WORK/chrome.log" 2>&1 &
CHROME_PID=$!

wait_for_local_stack

PORT="$PORT" CDP="$CDP" \
    FORGE_URL="http://127.0.0.1:$FORGE_PORT" \
    FORGE_TOKEN="$ISSUE_TOKEN" FORGE_OPERATOR_TOKEN="$SETUP_TOKEN" \
    FORGE_KIND="$FORGE_KIND" \
    bun run scripts/live-forge-check.ts
