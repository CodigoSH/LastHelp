/**
 * Renders assets/mockup.png — the image the project page and the README lead
 * with.
 *
 *   make build && bun scripts/docs-mockup.ts
 *
 * It takes its two screens itself, from an installation seeded with months of
 * history (scripts/seed), because the pictures in docs/ come from an instance
 * with three reports on it: right for explaining one screen, thin for showing
 * what the product looks like in use. The laptop shows the team's reports
 * screen; the phone shows the front page a visitor lands on.
 *
 * English, dark theme, house accent — the same as every other generated image.
 * A tick means a file was written, not that it is right: look at it.
 */
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { join } from "node:path";
import { tmpdir } from "node:os";

const OUT = "assets/mockup.png";
const WIDTH = 1600, HEIGHT = 900, DPR = 2;
const PORT = Number(process.env.PORT ?? 8187);
const CDP = Number(process.env.CDP ?? 9227);
const BASE = `http://127.0.0.1:${PORT}`;
const WORK = join(tmpdir(), "lasthelp-mockup");
const PASSWORD = "una contraseña larga de verdad";

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const dataUri = (bytes: Buffer, mime: string) => `data:${mime};base64,${bytes.toString("base64")}`;

const procs: ChildProcess[] = [];
process.on("exit", () => procs.forEach((p) => { try { p.kill(); } catch { /* gone */ } }));

let id = 0;
const pending = new Map<number, (v: any) => void>();
let ws: WebSocket;
function cdp(method: string, params: any = {}): Promise<any> {
  const mine = ++id;
  return new Promise((resolve) => { pending.set(mine, resolve); ws.send(JSON.stringify({ id: mine, method, params })); });
}

async function evaluate(expression: string) {
  const r = await cdp("Runtime.evaluate", { expression: `(async () => { ${expression} })()`, awaitPromise: true, returnByValue: true });
  if (r.result?.exceptionDetails) throw new Error(r.result.exceptionDetails.exception?.description ?? "evaluate failed");
  return r.result?.result?.value;
}

/** Until something is on the page — and loud when it never arrives. */
async function waitFor(selector: string, within = 15_000) {
  const until = Date.now() + within;
  while (!(await evaluate(`return !!document.querySelector(${JSON.stringify(selector)})`))) {
    if (Date.now() > until) throw new Error(`nothing matched ${selector}`);
    await sleep(100);
  }
}

async function until(ok: () => Promise<boolean>, what: string) {
  for (let i = 0; i < 80; i++) { if (await ok()) return; await sleep(250); }
  throw new Error(`${what} never came up`);
}

async function screen(path: string, marker: string, w: number, h: number, dpr: number, mobile: boolean) {
  await cdp("Emulation.setDeviceMetricsOverride", { width: w, height: h, deviceScaleFactor: dpr, mobile });
  await cdp("Page.navigate", { url: BASE + path });
  await waitFor(marker);
  // The entrance animation, so the picture is of the page and not of it arriving.
  await sleep(1200);
  const shot = await cdp("Page.captureScreenshot", { format: "png" });
  return Buffer.from(shot.result.data, "base64");
}

async function main() {
  rmSync(WORK, { recursive: true, force: true });
  mkdirSync(WORK, { recursive: true });
  const db = join(WORK, "mockup.db");

  const seeded = spawnSync("go", ["run", "./scripts/seed", "-db", db], { stdio: "inherit" });
  if (seeded.status !== 0) throw new Error("seeding failed");

  procs.push(spawn("./bin/lasthelp", [], {
    env: { ...process.env, LASTHELP_PORT: String(PORT), LASTHELP_DB_FILE: db, LASTHELP_OFFLINE: "true" },
    stdio: "ignore",
  }));
  procs.push(spawn("google-chrome", [
    "--headless=new", `--remote-debugging-port=${CDP}`, `--user-data-dir=${join(WORK, "profile")}`,
    "--no-first-run", "--hide-scrollbars", "--force-color-profile=srgb", "about:blank",
  ], { stdio: "ignore" }));
  await until(async () => (await fetch(`${BASE}/healthz`).catch(() => null))?.ok === true, "the server");
  await until(async () => (await fetch(`http://127.0.0.1:${CDP}/json/version`).catch(() => null))?.ok === true, "chrome");

  const list = await (await fetch(`http://127.0.0.1:${CDP}/json/list`)).json();
  ws = new WebSocket(list.find((t: any) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => { ws.onopen = () => r(null); });
  ws.onmessage = (e) => {
    const m = JSON.parse(String(e.data));
    if (m.id && pending.has(m.id)) { pending.get(m.id)!(m); pending.delete(m.id); }
  };
  await cdp("Page.enable");
  await cdp("Runtime.enable");
  await cdp("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "dark" }] });

  // English, and signed in as the administrator the seed creates.
  await cdp("Page.navigate", { url: BASE + "/" });
  await waitFor("#app > *");
  const signed = await evaluate(`
    localStorage.setItem("lasthelp_lang", "en");
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const r = await fetch("/api/session", { method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: JSON.stringify({ username: "alex", password: ${JSON.stringify(PASSWORD)} }) });
    return r.status;
  `);
  if (signed !== 200) throw new Error(`could not sign in: HTTP ${signed}`);

  // The team's screen: the reports waiting, the first one open beside them.
  const desktop = await screen("/review", ".rev-detail [data-rewrite]", 1280, 860, 2, false);

  // The visitor's: the front page on a phone, signed out.
  await evaluate(`
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf } });
  `);
  const mobile = await screen("/", ".lp", 390, 844, 3, true);

  // The screenshots are inlined rather than linked: a file:// page that goes
  // looking for them mid-render is a composition that sometimes ships with
  // empty frames.
  const html = readFileSync("scripts/docs-mockup.html", "utf8")
    .replace("__LOGO__", dataUri(readFileSync("assets/logo.png"), "image/png"))
    .replace("__DESKTOP__", dataUri(desktop, "image/png"))
    .replace("__MOBILE__", dataUri(mobile, "image/png"));
  const page = join(WORK, "mockup.html");
  writeFileSync(page, html);

  await cdp("Emulation.setDeviceMetricsOverride", { width: WIDTH, height: HEIGHT, deviceScaleFactor: DPR, mobile: false });
  await cdp("Page.navigate", { url: `file://${page}` });
  await sleep(1500);
  await cdp("Runtime.evaluate", { expression: "document.fonts.ready", awaitPromise: true });
  await sleep(400);

  const png = await cdp("Page.captureScreenshot", { format: "png" });
  await Bun.write(OUT, Buffer.from(png.result.data, "base64"));
  console.log(`✓ ${OUT} ${WIDTH * DPR}x${HEIGHT * DPR}`);
  process.exit(0);
}

main().catch((err) => { console.error("✗", err.message); process.exit(1); });
