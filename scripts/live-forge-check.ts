/**
 * Walks the forge bridge against a **real** forge, in a real browser.
 *
 * Everything else about the bridge is tested against a fake that this
 * repository also wrote, which means both halves could be wrong in the same way
 * and agree. The adapters were written from documentation and, for Gitea and
 * Forgejo, from reading the structs they serialise — because their webhook
 * payload is not documented at all. Until this runs, all of that is inference.
 *
 * What it proves that nothing else can:
 *
 *   - The issue really appears on the forge, with the body we built.
 *   - The forge's own webhook, signed by the forge, passes our verification —
 *     right header name, right algorithm, right bytes.
 *   - Closing the issue there moves the item here.
 *
 * Run it with `make live-forge`, which starts the forge and tears it down.
 */
const PORT = Number(process.env.PORT ?? 8099);
const CDP = Number(process.env.CDP ?? 9222);
const FORGE = process.env.FORGE_URL ?? "http://127.0.0.1:3000";
const FORGE_TOKEN = process.env.FORGE_TOKEN ?? "";
// A second token, standing in for the operator.
//
// LastHelp is handed one with only write:issue, which is what the documentation
// tells people to create — and creating a webhook needs more than that. In
// reality a person adds the webhook in the forge's own settings page; here that
// person is this token. Using LastHelp's token for it would be testing a
// permission the product does not need and does not ask for.
const OPERATOR_TOKEN = process.env.FORGE_OPERATOR_TOKEN ?? FORGE_TOKEN;
const FORGE_OWNER = process.env.FORGE_OWNER ?? "codigosh";
const FORGE_REPO = process.env.FORGE_REPO ?? "lastboard";
const KIND = process.env.FORGE_KIND ?? "forgejo";
const SECRET = "a long random webhook secret";
const BASE = `http://127.0.0.1:${PORT}`;

let failed = false;
const check = (label: string, ok: boolean, detail = "") => {
  console.log(`  ${ok ? "✓" : "✗"} ${label}${detail ? ` — ${detail}` : ""}`);
  if (!ok) failed = true;
};

const isGitLab = KIND === "gitlab";

/**
 * The forge's own API, in its own dialect.
 *
 * The three disagree about the header, the path shape and the vocabulary — a
 * GitLab project path is escaped into the URL, and closing an issue there is
 * `state_event: "close"` rather than `state: "closed"`. Absorbing that here
 * rather than writing three scripts is the same trade the adapters make.
 */
const forgeAPI = async (path: string, init: RequestInit = {}, asOperator = false) => {
  const token = asOperator ? OPERATOR_TOKEN : FORGE_TOKEN;
  const url = isGitLab ? `${FORGE}/api/v4${path}` : `${FORGE}/api/v1${path}`;

  const res = await fetch(url, {
    ...init,
    headers: {
      ...(isGitLab ? { "PRIVATE-TOKEN": token } : { "Authorization": `token ${token}` }),
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });
  const text = await res.text();
  return { status: res.status, body: text ? JSON.parse(text) : null };
};

/** Where this forge keeps a project's issues. */
const project = isGitLab
  ? `/projects/${encodeURIComponent(`${FORGE_OWNER}/${FORGE_REPO}`)}`
  : `/repos/${FORGE_OWNER}/${FORGE_REPO}`;

/** The issue number people see. GitLab sends both, and only one is right. */
const numberOf = (issue: any) => (isGitLab ? issue.iid : issue.number);
// GitLab calls the issue body "description". Reading .body there yields
// undefined, and String(undefined) fails the assertion with a message that
// looks like the text never travelled — which is exactly how this read for a
// while, against a product that had been sending the right field all along.
const bodyOf = (issue: any) => String((isGitLab ? issue.description : issue.body) ?? "");

async function target(url: string) {
  const res = await fetch(`http://127.0.0.1:${CDP}/json/new?${encodeURIComponent(url)}`, { method: "PUT" });
  return (await res.json()) as { id: string; webSocketDebuggerUrl: string };
}

function open(url: string) {
  return new Promise<{ send: (m: string, p?: unknown) => Promise<any>; close: () => void }>((resolve, reject) => {
    const ws = new WebSocket(url);
    let next = 1;
    const pending = new Map<number, (v: any) => void>();
    ws.onerror = reject;
    ws.onmessage = (event) => {
      const msg = JSON.parse(String(event.data));
      if (msg.id && pending.has(msg.id)) {
        pending.get(msg.id)!(msg.result);
        pending.delete(msg.id);
      }
    };
    ws.onopen = () =>
      resolve({
        send: (method, params) =>
          new Promise((ok) => {
            const id = next++;
            pending.set(id, ok);
            ws.send(JSON.stringify({ id, method, params }));
          }),
        // Closed at the end, or this script never exits.
        close: () => ws.close(),
      });
  });
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

const page = await open((await target(BASE + "/")).webSocketDebuggerUrl);
await page.send("Page.enable");
await page.send("Runtime.enable");

const evaluate = async (expression: string) => {
  const result = await page.send("Runtime.evaluate", {
    expression: `(async () => { ${expression} })()`,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result?.exceptionDetails) {
    throw new Error(result.exceptionDetails.exception?.description ?? "evaluate failed");
  }
  return result?.result?.value;
};

await sleep(1200);

// --- set the instance up, through the browser ------------------------------
const version = await (async () => {
  try {
    if (isGitLab) {
      const res = await forgeAPI("/version", {}, true);
      return String(res.body?.version ?? "unknown");
    }
    const res = await fetch(`${FORGE}/api/v1/version`);
    return String((await res.json()).version ?? "unknown");
  } catch {
    return "unknown";
  }
})();

// Printed, always. The first run of this checked Forgejo 11 while everybody —
// including the operator who asked for it — was on 16, and nothing in the
// output said so.
console.log(`\nAgainst a real ${KIND} ${version} at ${FORGE}\n`);

const setup = await evaluate(`
  const csrf = () => document.cookie.split(";").map(c => c.trim())
    .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
  const post = async (path, body) => {
    const res = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify(body),
    });
    return { status: res.status, body: await res.json().catch(() => null) };
  };

  await post("/api/setup", { email: "you@example.com", password: "a good long passphrase" });
  await post("/api/organizations", {
    slug: "codigosh", name: "CodigoSH", forge: ${JSON.stringify(KIND)},
    base_url: ${JSON.stringify(FORGE)}, forge_owner: ${JSON.stringify(FORGE_OWNER)},
  });
  await post("/api/orgs/codigosh/repositories", {
    slug: "board", name: "Board", forge_repo: ${JSON.stringify(`${FORGE_OWNER}/${FORGE_REPO}`)},
  });
  const creds = await post("/api/orgs/codigosh/credentials", {
    token: ${JSON.stringify(FORGE_TOKEN)}, webhook_secret: ${JSON.stringify(SECRET)},
  });
  return creds.body;
`);
check("the instance is connected to the forge", Boolean(setup?.webhook_path), setup?.webhook_path ?? "");

// --- the forge delivers to us ----------------------------------------------
const hookURL = `${BASE}${setup.webhook_path}`;
const hook = await forgeAPI(`${project}/hooks`, {
  method: "POST",
  body: JSON.stringify(
    isGitLab
      ? { url: hookURL, issues_events: true, push_events: false, token: SECRET }
      : {
          type: KIND === "gitea" ? "gitea" : "forgejo",
          active: true,
          events: ["issues"],
          config: { url: hookURL, content_type: "json", secret: SECRET },
        },
  ),
}, true);
check("the forge accepted a webhook pointing here", hook.status === 201, `HTTP ${hook.status}`);

// --- a report becomes an issue ---------------------------------------------
const reported = await evaluate(`
  const csrf = () => document.cookie.split(";").map(c => c.trim())
    .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
  const post = async (path, body) => {
    const res = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify(body ?? {}),
    });
    return { status: res.status, body: await res.json().catch(() => null) };
  };

  const form = await fetch("/api/orgs/codigosh/repos/board/form").then(r => r.json());
  // The form token refuses anything faster than a person types.
  await new Promise(r => setTimeout(r, 2400));

  const sent = await post("/api/orgs/codigosh/repos/board/items", {
    kind: "bug", title: "Crashes when importing a folder",
    body: "It closes with no message.", token: form.token,
  });
  const id = sent.body.public_id;

  await post("/api/items/" + id + "/moderation", { state: "published" });
  await post("/api/items/" + id + "/issue", {});
  return id;
`);
check("a report was taken and queued for the forge", Boolean(reported), String(reported));

// The queue drains on its own timer. Waiting is the honest way to test it:
// reaching in to run the worker would test a code path nobody uses.
console.log("  … waiting for the queue to drain");
let issue: any = null;
for (let attempt = 0; attempt < 20; attempt++) {
  await sleep(2000);
  const list = await forgeAPI(`${project}/issues${isGitLab ? "" : "?state=all"}`);
  issue = (list.body ?? []).find((i: any) => i.title === "Crashes when importing a folder");
  if (issue) break;
}
check("the issue really exists on the forge", Boolean(issue), issue ? `#${numberOf(issue)}` : "never appeared");

if (issue) {
  check(
    "the report's own text travelled with it",
    bodyOf(issue).includes("It closes with no message"),
    (bodyOf(issue).split("\n")[0] ?? "").slice(0, 44),
  );
  check(
    "and a way back to the report",
    bodyOf(issue).includes(String(reported)),
    "carries the item's identifier",
  );

  // --- and now the direction nothing else can test -------------------------
  const closed = await forgeAPI(`${project}/issues/${numberOf(issue)}`, {
    method: isGitLab ? "PUT" : "PATCH",
    body: JSON.stringify(isGitLab ? { state_event: "close" } : { state: "closed" }),
  });
  check("the issue was closed on the forge", closed.status === 201 || closed.status === 200, `HTTP ${closed.status}`);

  console.log("  … waiting for the forge's own webhook to arrive");
  let status = "";
  for (let attempt = 0; attempt < 15; attempt++) {
    await sleep(1000);
    const item = await evaluate(`
      const res = await fetch("/api/items/" + ${JSON.stringify(reported)});
      const body = await res.json();
      return body.status;
    `);
    status = String(item ?? "");
    if (status === "closed") break;
  }
  check(
    "closing it there closed the item here",
    status === "closed",
    `status=${status}`,
  );

  // Delivered by the forge, verified by us. If the header name, the algorithm
  // or the bytes we sign over were wrong, the delivery would have been refused
  // and the status above would still be "new".
  const hooks = await forgeAPI(`${project}/hooks`, {}, true);
  check(
    "the webhook is still there on the forge",
    Array.isArray(hooks.body) && hooks.body.length > 0,
    `${(hooks.body ?? []).length} hook(s)`,
  );

  // GitLab's own record of what it sent. If our receiver had refused the
  // delivery — wrong header, wrong algorithm, wrong bytes — the forge would
  // have recorded the failure, and the status check above would have been the
  // only sign.
  if (isGitLab && hooks.body?.[0]) {
    const detail = await forgeAPI(`${project}/hooks/${hooks.body[0].id}`, {}, true);
    check(
      "the forge did not record a failed delivery",
      !detail.body?.disabled_until && detail.body?.alert_status !== "disabled",
      String(detail.body?.alert_status ?? "no status"),
    );
  }
}

await fetch(`http://127.0.0.1:${CDP}/json/close/${(await target("about:blank")).id}`).catch(() => undefined);
page.close();

console.log();
process.exit(failed ? 1 : 0);
export {};
