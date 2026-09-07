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

case "$FORGE_KIND" in
    forgejo) IMAGE="codeberg.org/forgejo/forgejo:11"; ENV_PREFIX=FORGEJO; BIN=forgejo ;;
    gitea)   IMAGE="docker.io/gitea/gitea:1";         ENV_PREFIX=GITEA;   BIN=gitea ;;
    gitlab)  IMAGE="docker.io/gitlab/gitlab-ce:latest" ;;
    *) echo "FORGE_KIND must be forgejo, gitea or gitlab"; exit 2 ;;
esac

RUNTIME="$(command -v podman || command -v docker || true)"
[ -n "$RUNTIME" ] || { echo "podman or docker is required"; exit 1; }

WORK="$(mktemp -d)"
CONTAINER="lasthelp-live-$FORGE_KIND"

cleanup() {
    if [ -n "${CHROME_PID:-}" ]; then kill "$CHROME_PID" 2>/dev/null || true; wait "$CHROME_PID" 2>/dev/null || true; fi
    if [ -n "${SERVER_PID:-}" ]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
    "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
    rm -rf "$WORK" 2>/dev/null || true
}
trap cleanup EXIT

# GitLab is a different animal: minutes to boot, its own way of minting a token,
# and it refuses to deliver a webhook to a local address until an administrator
# says otherwise. All of that is set up here rather than left to the person
# running this, because a check nobody can run is a check nobody runs.
if [ "$FORGE_KIND" = gitlab ]; then
    echo "==> starting gitlab (this takes several minutes)"
    "$RUNTIME" rm -f "$CONTAINER" >/dev/null 2>&1 || true
    "$RUNTIME" run -d --name "$CONTAINER" --network=host --shm-size=256m \
        -e GITLAB_OMNIBUS_CONFIG="external_url 'http://127.0.0.1:$FORGE_PORT'; gitlab_rails['initial_root_password'] = '$PASSWORD'; puma['worker_processes'] = 2; sidekiq['max_concurrency'] = 5; prometheus_monitoring['enable'] = false;" \
        "$IMAGE" >/dev/null

    for _ in $(seq 1 900); do
        curl -sf "http://127.0.0.1:$FORGE_PORT/-/readiness" >/dev/null 2>&1 && break
        sleep 2
    done
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

    for _ in $(seq 1 80); do
        if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 &&
           curl -sf "http://127.0.0.1:$CDP/json/version" >/dev/null 2>&1; then break; fi
        sleep 0.25
    done

    PORT="$PORT" CDP="$CDP" \
        FORGE_URL="http://127.0.0.1:$FORGE_PORT" \
        FORGE_TOKEN="$ISSUE_TOKEN" FORGE_OPERATOR_TOKEN="$SETUP_TOKEN" \
        FORGE_KIND=gitlab FORGE_OWNER=root FORGE_REPO=lastboard \
        bun run scripts/live-forge-check.ts
    exit $?
fi

echo "==> starting $FORGE_KIND"
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

for _ in $(seq 1 80); do
    if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 &&
       curl -sf "http://127.0.0.1:$CDP/json/version" >/dev/null 2>&1; then break; fi
    sleep 0.25
done

PORT="$PORT" CDP="$CDP" \
    FORGE_URL="http://127.0.0.1:$FORGE_PORT" \
    FORGE_TOKEN="$ISSUE_TOKEN" FORGE_OPERATOR_TOKEN="$SETUP_TOKEN" \
    FORGE_KIND="$FORGE_KIND" \
    bun run scripts/live-forge-check.ts
