/**
 * The screenshots in docs/, generated.
 *
 * Never taken by hand: a hand-taken screenshot is one nobody can reproduce when
 * the interface changes, so the documentation quietly ends up showing a version
 * that no longer exists. This seeds an instance, drives a real browser and
 * writes the files.
 *
 * English, dark theme, house accent — the same three every time, so two runs
 * differ only where the interface did.
 *
 * A tick in this script's output means a file was written. It does not mean the
 * image is right: look at them.
 */
const PORT = Number(process.env.PORT ?? 8099);
const CDP = Number(process.env.CDP ?? 9222);
const OUT = process.env.OUT ?? "docs/images";
const BASE = `http://127.0.0.1:${PORT}`;

const WIDTH = 1280;
const HEIGHT = 860;

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
        close: () => ws.close(),
      });
  });
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

await Bun.$`mkdir -p ${OUT}`.quiet();

const page = await open((await target("about:blank")).webSocketDebuggerUrl);
await page.send("Page.enable");
await page.send("Runtime.enable");
await page.send("Emulation.setDeviceMetricsOverride", {
  width: WIDTH, height: HEIGHT, deviceScaleFactor: 2, mobile: false,
});
// The same three conditions every run: dark, English, house accent.
await page.send("Emulation.setEmulatedMedia", {
  features: [{ name: "prefers-color-scheme", value: "dark" }],
});

const evaluate = async (expression: string) => {
  const result = await page.send("Runtime.evaluate", {
    expression: `(async () => { ${expression} })()`,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result?.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? "failed");
  return result?.result?.value;
};

const go = async (path: string) => {
  await page.send("Page.navigate", { url: BASE + path });
  await sleep(1200);
  // English, whatever the browser asked for, so the images are comparable.
  await evaluate(`localStorage.setItem("lasthelp_lang", "en")`);
  await page.send("Page.reload");
  await sleep(1400);
};

const shoot = async (name: string) => {
  // Clipped to the content rather than to the viewport. A screenshot that is
  // two thirds empty background says nothing about the interface and makes
  // every image in the documentation look like a mistake.
  const height = await evaluate(`
    const main = document.querySelector("main");
    const bottom = main ? main.getBoundingClientRect().bottom + window.scrollY : 0;
    return Math.min(Math.max(Math.ceil(bottom) + 24, 320), 2000);
  `);

  const shot = await page.send("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
    clip: { x: 0, y: 0, width: WIDTH, height: Number(height), scale: 2 },
  });
  await Bun.write(`${OUT}/${name}.png`, Buffer.from(shot.data, "base64"));
  console.log(`  wrote ${OUT}/${name}.png (${WIDTH}x${height})`);
};

// The wizard, before anything is claimed. It has to be first: the moment the
// instance has an administrator this screen cannot be reached again.
await go("/setup");
await shoot("setup");

// --- seed, through the API, so the images show a repository with something in it
await go("/");
const seeded = await evaluate(`
  const csrf = () => document.cookie.split(";").map(c => c.trim())
    .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
  const post = (path, body) => fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
    body: JSON.stringify(body),
  }).then(r => r.status);

  // Refused silently for a while: this said "email", which the API stopped
  // taking. Nothing checked the answer, so setup failed, everything after it
  // ran with no session, and the images were of an unclaimed instance — with a
  // tick printed for each one.
  const claimed = await post("/api/setup", { username: "alex", password: "a good long passphrase" });
  if (claimed !== 201) throw new Error("could not claim the instance: HTTP " + claimed);

  await post("/api/instance", { name: "Acme", logo: "", accent: "" });
  await post("/api/organizations", { slug: "acme", name: "Acme", forge: "forgejo" });
  await post("/api/orgs/acme/repositories", {
    slug: "atlas", name: "Atlas", accepts_bugs: true, accepts_ideas: true,
  });

  const submit = async (kind, title, body, fields = {}) => {
    const form = await fetch("/api/orgs/acme/repos/atlas/form").then(r => r.json());
    // The form token refuses anything faster than a person types.
    await new Promise(r => setTimeout(r, 2200));
    const res = await fetch("/api/orgs/acme/repos/atlas/items", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ kind, title, body, fields, token: form.token }),
    });
    return (await res.json()).public_id;
  };

  const ids = [];
  ids.push(await submit("idea", "Dark mode for the dashboard", "The white is bright at night."));
  ids.push(await submit("idea", "Export the reports to CSV", "For a monthly report."));
  // Last, so it is the one left waiting: the picture of the queue is there to
  // show what a triage decision looks like, and a bug with its version and its
  // steps is the decision. An idea with one sentence shows nothing.
  ids.push(
    await submit("bug", "Crashes when importing a folder", "It closes with no message.", {
      // The two the form asks a bug, so the pictures of the queue and of a
      // report show what a usable one looks like rather than a bare sentence.
      version: "1.4.2",
      steps: "1. Open a board\\n2. Import a folder of photos\\n3. It closes",
    }),
  );

  // All but the last, so the queue screenshot has something in it. Publishing
  // everything left that page showing "nothing waiting" — a picture of the
  // triage screen with no triage on it, which is the one thing that page is
  // there to show.
  for (const id of ids.slice(0, -1)) {
    await post("/api/items/" + id + "/moderation", { state: "published" });
  }
  await post("/api/items/" + ids[0] + "/status", { status: "planned" });
  await post("/api/items/" + ids[1] + "/status", { status: "in_progress" });
  await post("/api/items/" + ids[0] + "/votes", {});
  return ids;
`);
if (!Array.isArray(seeded) || seeded.length !== 3) {
  console.error("seeding failed:", seeded);
  process.exit(1);
}

await go("/acme/atlas");
await shoot("board");

await go("/acme/atlas/new");
await shoot("report");

await go("/acme/atlas/roadmap");
await shoot("roadmap");

await go("/acme/atlas/queue");
await shoot("queue");

await go(`/i/${seeded[0]}`);
await shoot("item");

await go("/panel");
await shoot("panel");

await go("/panel/updates");
await shoot("update");

await go("/panel/organizations");
await shoot("organizations");

page.close();
console.log("\nlook at the images. A tick above says a file was written, nothing more.");
export {};
