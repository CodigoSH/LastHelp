/**
 * What a stranger types never runs.
 *
 * A report, a follow-up and a reply are written by people the product does not
 * know, and read by the team in the panel and by everybody on the public
 * pages. This hangs a marked payload on every field one of them can write, then
 * visits every screen that shows it and looks for what escaping is supposed to
 * prevent: an element made from the text, an event attribute, a `javascript:`
 * link, or a script that ran.
 *
 * Each field has its own marker, so a failure names the field that leaked. The
 * control is as important as the zero: the text has to be on the page *as text*,
 * or an empty scan only means nothing was drawn.
 *
 * Run with `make drive-xss`, on `scripts/seed` so the screens have history.
 * Mutated on purpose, as every guard here is: with `escapeHTML` returning its
 * argument the scan reports leaks and the content policy refuses the inline
 * handlers, and either one fails the pass.
 */
import { BASE, connect, sleep } from "./drive-harness";

const PASSWORD = "una contraseña larga de verdad";

async function main() {
  const { page, evaluate, waitFor, goTo, check, close, consoleErrors, refusedByPolicy } = await connect();
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 860, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: BASE + "/" });
  await sleep(1200);

  // The payload for one field: an element with a marker id and a handler that
  // raises a flag, a closing quote and bracket for attribute breakouts, a script,
  // and a link that runs when followed. Short enough for the shortest field.
  const bite = (field: string) =>
    `"><img id=PWN_${field} src=x onerror=window.__xss_${field}=1><b id=PWB_${field}>`;
  const longBite = (field: string) =>
    `${bite(field)}<script id=PWS_${field}>window.__xss_${field}=1</script><a id=PWA_${field} href="javascript:window.__xss_${field}=1">x</a>`;

  // One report with every writable field marked, and a follow-up from a visitor
  // and from the reporter, all as a stranger.
  const sent = await evaluate(`
    const base = "/api/orgs/acme/repos/atlas";
    const csrf = document.cookie.split("; ").find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1] ?? "";
    const headers = { "Content-Type": "application/json", "X-CSRF-Token": csrf };
    const form = await fetch(base + "/form", { credentials: "same-origin" }).then(r => r.json());
    await new Promise(r => setTimeout(r, 2800));
    const report = await fetch(base + "/items", { method: "POST", credentials: "same-origin", headers,
      body: JSON.stringify({ kind: "bug", token: form.token, website: "", email: "",
        title: ${JSON.stringify(bite("title"))},
        body: ${JSON.stringify(longBite("body"))},
        fields: { version: ${JSON.stringify(bite("version"))}, steps: ${JSON.stringify(longBite("steps"))} } }) }).then(r => r.json());
    if (!report.public_id) return { error: report.error ?? "no id" };
    const id = report.public_id, token = report.follow_token;
    const visitor = await fetch("/api/items/" + id + "/comments", { method: "POST", headers,
      body: JSON.stringify({ body: ${JSON.stringify(longBite("visitor"))} }) });
    const reporter = await fetch("/api/items/" + id + "/comments", { method: "POST", headers,
      body: JSON.stringify({ body: ${JSON.stringify(longBite("reporter"))}, token }) });
    return { id, token, visitor: visitor.status, reporter: reporter.status };
  `);
  check("a stranger's report and two follow-ups are accepted, to be looked at", !!sent.id && sent.visitor === 202 && sent.reporter === 202, JSON.stringify(sent));
  const { id, token } = sent;

  // A scan of what is on the page now. Everything is copied to plain values at
  // the moment of measuring.
  const SCAN = `
    const flags = Object.keys(window).filter(k => k.startsWith("__xss_"));
    const handlers = [];
    for (const el of document.querySelectorAll("*")) {
      for (const a of el.attributes) if (a.name.startsWith("on")) handlers.push(el.tagName + "[" + a.name + "]");
    }
    const made = [...document.querySelectorAll("[id^=PWN_],[id^=PWB_],[id^=PWS_],[id^=PWA_]")].map(e => e.id);
    const links = [...document.querySelectorAll("a[href]")].filter(a => a.getAttribute("href").trim().toLowerCase().startsWith("javascript:")).length;
    return { made, handlers, links, flags, text: document.body.innerText };
  `;
  const scanned: string[] = [];
  const scan = async (label: string, expectText: string[] = []) => {
    const s = await evaluate(SCAN);
    scanned.push(label);
    const missing = expectText.filter((t) => !s.text.includes(t));
    check(
      `${label}: nothing a stranger wrote became an element, a handler, a link or a script`,
      s.made.length === 0 && s.handlers.length === 0 && s.links === 0 && s.flags.length === 0,
      JSON.stringify({ made: s.made, handlers: s.handlers, links: s.links, flags: s.flags }),
    );
    if (expectText.length) {
      check(`${label}: and the text is there, as text`, missing.length === 0, missing.length ? `missing ${missing.join(", ")}` : "");
    }
  };
  const asText = (...fields: string[]) => fields.map((f) => `PWN_${f}`);

  // As the reporter, who may see it before anybody has approved it.
  await goTo(`/i/${id}?k=${encodeURIComponent(token)}`, ".detail");
  await scan("the reporter's page", asText("title", "body", "version", "steps"));

  // As the team: approve the report and the two follow-ups, and answer.
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    const r = await fetch("/api/session", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ username: "alex", password: ${JSON.stringify(PASSWORD)} }) });
    if (!r.ok) throw new Error("sign-in refused: " + r.status);
  `);
  await page.send("Page.navigate", { url: BASE + "/panel" });
  await waitFor(".admin-main");

  const approved = await evaluate(`
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const headers = { "Content-Type": "application/json", "X-CSRF-Token": csrf };
    const publish = await fetch("/api/items/${id}/moderation", { method: "POST", headers, body: JSON.stringify({ state: "published" }) });
    const reply = await fetch("/api/items/${id}/reply", { method: "POST", headers, body: JSON.stringify({ body: ${JSON.stringify(longBite("reply"))} }) });
    // The two follow-ups wait for a person; this is the person, so they are on the page.
    const waiting = await fetch("/api/panel/comments").then(r => r.json());
    const mine = (waiting.comments ?? waiting.items ?? waiting ?? []).filter?.(c => String(c.body ?? "").includes("PWN_visitor") || String(c.body ?? "").includes("PWN_reporter")) ?? [];
    let moved = 0;
    for (const c of mine) {
      const r = await fetch("/api/comments/" + c.id + "/moderation", { method: "POST", headers, body: JSON.stringify({ state: "published" }) });
      if (r.ok) moved++;
    }
    return { publish: publish.status, reply: reply.status, followUps: mine.length, moved };
  `);
  check(
    "the team publishes it, answers with a marked reply and approves both follow-ups",
    approved.publish === 200 && approved.reply === 201 && approved.followUps === 2 && approved.moved === 2,
    JSON.stringify(approved),
  );

  await goTo("/review?mode=all&moderation=pending", ".rev-row, .empty");
  await scan("the team's queue");
  await goTo("/review?mode=all", ".rev-row");
  const picked = await evaluate(`
    const row = document.querySelector('[data-pick="${id}"]');
    if (!row) return false;
    row.click();
    for (let i = 0; i < 60 && !document.querySelector(".rev-detail .comments, .rev-detail article"); i++) await new Promise(r => setTimeout(r, 100));
    await new Promise(r => setTimeout(r, 500));
    return true;
  `);
  check("the report is found in the team's list and opened in its pane", picked === true, String(picked));
  await scan("the team's pane", asText("title", "body", "version", "steps"));

  await goTo(`/i/${id}`, ".detail");
  await scan("the team's page of the report", asText("title", "body", "steps", "visitor", "reporter", "reply"));
  await goTo("/acme/atlas", ".items, .empty");
  await scan("the board's list");
  await goTo("/panel", ".admin-main");
  await scan("the overview");
  await goTo("/panel/acme", ".admin-main");
  await scan("the organisation's page");

  // As a stranger again, now that it is public.
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
  `);
  await goTo(`/i/${id}`, ".detail");
  await scan("a visitor's page of the published report", asText("title", "body", "version", "steps", "visitor", "reporter", "reply"));
  await goTo("/acme/atlas", ".items, .empty");
  await scan("a visitor's board");

  // Every screen above has to have been scanned: a list that quietly shrank
  // would be a guard that watches less than it says.
  check("every screen on the list was scanned", scanned.length >= 9, String(scanned.length));

  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));
  check("and the content policy refused nothing", refusedByPolicy.length === 0, refusedByPolicy.slice(0, 3).join(" | "));
  await close();
}

await main();

export {};
