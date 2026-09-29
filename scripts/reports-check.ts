/**
 * The reports screen, walked against an installation with history.
 *
 * The main journey starts from an empty installation — it has to, the setup is
 * part of it — so the reports screen only ever meets the handful of reports
 * that journey writes on its way. This one starts from `scripts/seed`: six
 * months of reports in every state, their votes and conversations, and some
 * still waiting, across two repositories and three kinds of account.
 *
 * Run with `make drive-reports`, which seeds a scratch database, starts the
 * server on it and a headless Chrome, and tears both down.
 *
 * What only content shows, and this is here to watch:
 *   - the door lands on the queue when something waits, and the counts agree;
 *   - holding an arrow key walks the list without the server refusing it;
 *   - the archive finds a report among many, by words and by filter;
 *   - the report is worked on in the pane: status, reply, deleting a comment
 *     and deleting the report, each asked in the product's own dialog;
 *   - each account sees the reports it may, and the controls it may use.
 */
import { BASE, connect, sleep } from "./drive-harness";

const PASSWORD = "una contraseña larga de verdad";
const SHOTS = process.env.SHOTS ?? "";

async function main() {
  const { page, evaluate, waitFor, goTo, check, key, close, consoleErrors, refusedByPolicy, requests } = await connect();

  await page.send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 860, deviceScaleFactor: 1, mobile: false });
  await sleep(700);

  const signIn = async (username: string) => {
    await evaluate(`
      const csrf = () => document.cookie.split(";").map(c => c.trim())
        .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
      await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
      const r = await fetch("/api/session", { method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
        body: JSON.stringify({ username: ${JSON.stringify(username)}, password: ${JSON.stringify(PASSWORD)} }) });
      if (!r.ok) throw new Error("sign-in refused for ${username}: " + r.status);
    `);
    await page.send("Page.navigate", { url: BASE + "/panel" });
    await waitFor(".admin-main");
  };

  const shoot = async (name: string) => {
    if (!SHOTS) return;
    const shot = await page.send("Page.captureScreenshot", { format: "png" });
    await Bun.write(`${SHOTS}/${name}.png`, Buffer.from(shot.data, "base64"));
  };

  // The dialog every deletion asks through, answered yes.
  const confirmYes = `
    await new Promise(r => setTimeout(r, 300));
    document.querySelector('#confirm-dialog [data-confirm="yes"]')?.click();
  `;

  const waiting = () =>
    evaluate(`
      const me = await fetch("/api/me").then(r => r.json());
      return (me.pending?.reports ?? 0) + (me.pending?.messages ?? 0);
    `);

  await signIn("alex");

  // The door is there for the administrator too, who holds no membership and
  // may triage every board: it used to hang off memberships alone.
  const door0 = await evaluate(`return !!document.querySelector("body > nav [data-review]")`);
  check("the administrator has the Reports icon in the bar", door0 === true, String(door0));

  // 1. The door. Seven things wait — five reports and two messages — and
  //    opening Reports lands on them, with the tab saying the bar's number.
  await goTo("/review", ".rev-list");
  const door = await evaluate(`
    return {
      mode: document.querySelector(".rev-modes .tab.on")?.dataset.mode ?? "",
      rows: document.querySelectorAll("[data-pick]").length,
      chip: Number(document.querySelector('[data-mode="pending"] .count-chip')?.textContent ?? -1),
    };
  `);
  const expected = await waiting();
  check(
    "with work waiting, Reports opens on it, and the list, the tab and the bar agree",
    door.mode === "pending" && expected === 7 && door.rows === expected && door.chip === expected,
    JSON.stringify({ ...door, bar: expected }),
  );
  await shoot("1-pending");

  // 2. Holding an arrow key. Each row read in the pane is three requests, and
  //    the server refuses one address past 240 a minute; walking the list
  //    must only ask for the row it stops on.
  // Four presses in a few milliseconds, as a held key repeats: past three
  // reports and onto the last one. Only that one may be asked for — one
  // request, which carries what this account may do to it — and none passed
  // over.
  const from = requests.length;
  for (let i = 0; i < 4; i++) await key("ArrowDown", "ArrowDown", 40);
  await sleep(1200);
  const stoppedOn = await evaluate(`return document.querySelector(".rev-row.on")?.dataset.pick ?? ""`);
  const sent = requests.slice(from).map((r) => r.url.replace(BASE, "")).filter((u) => u.startsWith("/api/items/"));
  const walk = {
    stoppedOn,
    kind: await evaluate(`return document.querySelector(".rev-row.on")?.dataset.what ?? ""`),
    pane: await evaluate(`return (document.querySelector(".rev-detail .detail-words h1")?.textContent ?? "").trim()`),
    sent,
  };
  check(
    "holding an arrow key walks the list and asks the server only for the report it stops on",
    walk.kind === "report" && walk.pane !== "" &&
      JSON.stringify(sent) === JSON.stringify([`/api/items/${stoppedOn}`]),
    JSON.stringify(walk),
  );


  // 3. The archive: everything, found among many.
  await goTo("/review?mode=all", ".rev-row");
  const archive = await evaluate(`return document.querySelectorAll('[data-what="report"]').length`);
  check("the archive holds every report, whatever became of it", archive === 23, `rows=${archive}`);

  await evaluate(`
    const box = document.querySelector("[data-q]");
    box.value = "accents";
    box.dispatchEvent(new Event("input", { bubbles: true }));
  `);
  const found = await evaluate(`
    for (let i = 0; i < 60; i++) {
      await new Promise(r => setTimeout(r, 100));
      if (new URLSearchParams(location.search).get("q") === "accents" &&
          document.querySelectorAll('[data-what="report"]').length === 1) break;
    }
    return [...document.querySelectorAll('[data-what="report"] .rev-t')].map(t => t.textContent.trim());
  `);
  check("searching the archive finds the one report among many", JSON.stringify(found) === '["Search ignores accents"]', JSON.stringify(found));

  // 4. Worked on in the pane: its status, a reply, and deleting that reply.
  await waitFor(".rev-detail #status-select");
  const before = await evaluate(`
    const pane = document.querySelector(".rev-detail");
    return {
      id: document.querySelector(".rev-row.on")?.dataset.pick ?? "",
      status: pane.querySelector("#status-select")?.value ?? "",
      thread: pane.querySelectorAll(".comments .comment").length,
      teamSaid: (pane.querySelector(".comments")?.textContent ?? "").includes("Working on accent-insensitive search"),
    };
  `);
  check(
    "and the pane is the report as the team works on it: its status and its conversation",
    before.status === "confirmed" && before.thread === 1 && before.teamSaid === true,
    JSON.stringify(before),
  );
  await shoot("2-archive-pane");

  const statusChange = await evaluate(`
    const id = ${JSON.stringify(before.id)};
    const pick = document.querySelector(".rev-detail #status-select");
    pick.value = "closed";
    pick.dispatchEvent(new Event("change", { bubbles: true }));
    for (let i = 0; i < 40; i++) {
      await new Promise(r => setTimeout(r, 100));
      const s = await fetch("/api/items/" + id).then(r => r.json());
      if (s.status === "closed") break;
    }
    const after = await fetch("/api/items/" + id).then(r => r.json());
    return { server: after.status, stillChosen: document.querySelector(".rev-row.on")?.dataset.pick === id };
  `);
  check(
    "changing the status from the pane reaches the server, and the row stays chosen",
    statusChange.server === "closed" && statusChange.stillChosen === true,
    JSON.stringify(statusChange),
  );

  const reply = await evaluate(`
    const id = ${JSON.stringify(before.id)};
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    await wait(() => document.querySelector(".rev-detail form.team-reply textarea"));
    const f = document.querySelector(".rev-detail form.team-reply");
    f.querySelector("textarea").value = "Released in 1.4.3.";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await wait(() => document.querySelectorAll(".rev-detail .comments .comment").length === 2);
    return { thread: document.querySelectorAll(".rev-detail .comments .comment").length,
             bins: document.querySelectorAll(".rev-detail [data-delete-comment]").length };
  `);
  check("a reply written in the pane joins the conversation there", reply.thread === 2 && reply.bins === 2, JSON.stringify(reply));

  const unsaid = await evaluate(`
    const id = ${JSON.stringify(before.id)};
    const bins = [...document.querySelectorAll(".rev-detail [data-delete-comment]")];
    bins[bins.length - 1].click();
    ${confirmYes}
    for (let i = 0; i < 40 && document.querySelectorAll(".rev-detail .comments .comment").length !== 1; i++)
      await new Promise(r => setTimeout(r, 100));
    const server = await fetch("/api/items/" + id).then(r => r.json());
    return { thread: document.querySelectorAll(".rev-detail .comments .comment").length,
             server: (server.comments ?? []).length,
             gone: !(server.comments ?? []).some(c => c.body === "Released in 1.4.3.") };
  `);
  check(
    "and deleting it, asked in the product's dialog, takes it off the pane and the server",
    unsaid.thread === 1 && unsaid.server === 1 && unsaid.gone === true,
    JSON.stringify(unsaid),
  );

  // A half-written edit survives choosing the same row again: drawing the
  // pane anew on every click threw away whatever was being typed in it.
  const kept = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    document.querySelector(".rev-detail [data-rewrite]").click();
    await wait(() => document.querySelector(".rev-detail form.rewrite"));
    document.querySelector(".rev-detail form.rewrite input[name=title]").value = "half written";
    document.querySelector(".rev-row.on").click();
    await new Promise(r => setTimeout(r, 800));
    const box = document.querySelector(".rev-detail form.rewrite input[name=title]");
    const typed = box?.value ?? "";
    document.querySelector(".rev-detail [data-rewrite-cancel]")?.click();
    return { typed };
  `);
  check("a half-written edit survives choosing the same row again", kept.typed === "half written", JSON.stringify(kept));

  // 5. A filter, and deleting a report from the pane.
  await goTo("/review?mode=all&moderation=rejected", ".rev-row");
  const rejected = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const rows = [...document.querySelectorAll('[data-what="report"]')];
    const id = rows[0]?.dataset.pick ?? "";
    const titles = rows.map(r => r.querySelector(".rev-t").textContent.trim());
    await wait(() => document.querySelector(".rev-detail [data-delete-item]"));
    document.querySelector(".rev-detail [data-delete-item]").click();
    ${confirmYes}
    await wait(() => !document.querySelector('[data-pick="' + id + '"]'));
    const server = await fetch("/api/items/" + id);
    return { titles, left: document.querySelectorAll('[data-what="report"]').length, status: server.status };
  `);
  check(
    "the moderation filter narrows the archive, and a report deleted from the pane leaves the list and the server",
    JSON.stringify(rejected.titles) === '["The page is broken"]' && rejected.left === 0 && rejected.status === 404,
    JSON.stringify(rejected),
  );

  // 6. Deciding what waits, from the pane and from the keyboard: the row goes,
  //    and the tab and the bar count one less each time.
  await goTo("/review?mode=pending", ".rev-row");
  const decided = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const first = document.querySelector('[data-what="report"]');
    first.click();
    await wait(() => document.querySelector(".rev-detail [data-moderate='published']"));
    const rows = document.querySelectorAll("[data-pick]").length;
    document.querySelector(".rev-detail [data-moderate='published']").click();
    await wait(() => document.querySelectorAll("[data-pick]").length === rows - 1);
    return { before: rows, after: document.querySelectorAll("[data-pick]").length };
  `);
  const afterOne = await waiting();
  check(
    "approving from the pane takes the row off, and the bar counts one less",
    decided.after === decided.before - 1 && afterOne === decided.after,
    JSON.stringify({ ...decided, bar: afterOne }),
  );

  await evaluate(`document.querySelector('[data-what="comment"]')?.click()`);
  await sleep(500);
  const beforeKey = await evaluate(`return document.querySelectorAll("[data-pick]").length`);
  await key("p", "KeyP", 80, "p");
  await sleep(1200);
  const afterKey = await evaluate(`return document.querySelectorAll("[data-pick]").length`);
  const barAfterKey = await waiting();
  check(
    "and a waiting message approved with P goes the same way",
    afterKey === beforeKey - 1 && barAfterKey === afterKey,
    JSON.stringify({ beforeKey, afterKey, bar: barAfterKey }),
  );

  // 7. The collaborator: Atlas and only Atlas, working on reports but not
  //    deleting them — that is the owners' and the administrator's.
  await signIn("ana");

  // From inside the panel the icon answers: the reports are the team's daily
  // work and the panel is where the team is. And the same icon takes them back.
  const fromPanel = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const pill = document.querySelector("body > nav [data-review]");
    const live = !!pill && !pill.classList.contains("cannot") && pill.getAttribute("aria-disabled") !== "true";
    pill?.click();
    await wait(() => location.pathname === "/review" && document.querySelector(".rev-list, .rev-empty"));
    const there = location.pathname;
    document.querySelector("body > nav [data-review]")?.click();
    await wait(() => location.pathname === "/panel");
    return { live, there, back: location.pathname };
  `);
  check(
    "from inside the panel the Reports icon answers, and takes the team back to the panel",
    fromPanel.live === true && fromPanel.there === "/review" && fromPanel.back === "/panel",
    JSON.stringify(fromPanel),
  );
  await goTo("/review?mode=all", ".rev-row");
  const ana = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const api = await fetch("/api/reports?limit=200").then(r => r.json());
    const boards = [...new Set((api.items ?? []).map(r => r.board?.repo))];
    document.querySelector('[data-what="report"]').click();
    await wait(() => document.querySelector(".rev-detail [data-rewrite]"));
    return {
      boards,
      rows: document.querySelectorAll('[data-what="report"]').length,
      server: (api.items ?? []).length,
      edit: !!document.querySelector(".rev-detail [data-rewrite]"),
      status: !!document.querySelector(".rev-detail #status-select"),
      bin: !!document.querySelector(".rev-detail [data-delete-item]"),
      commentBins: document.querySelectorAll(".rev-detail [data-delete-comment]").length,
    };
  `);
  check(
    "a collaborator's archive is their repository's reports, worked on in the pane",
    JSON.stringify(ana.boards) === '["atlas"]' && ana.rows === ana.server && ana.edit && ana.status,
    JSON.stringify(ana),
  );
  check("and without the bins, which are the owners' and the administrator's", ana.bin === false && ana.commentBins === 0, JSON.stringify(ana));

  // 8. The owner: both repositories, and the bin.
  await signIn("olga");
  await goTo("/review?mode=all", ".rev-row");
  const olga = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const api = await fetch("/api/reports?limit=200").then(r => r.json());
    document.querySelector('[data-what="report"]').click();
    await wait(() => document.querySelector(".rev-detail [data-rewrite]"));
    return {
      boards: [...new Set((api.items ?? []).map(r => r.board?.repo))].sort(),
      bin: !!document.querySelector(".rev-detail [data-delete-item]"),
    };
  `);
  check(
    "an owner's archive is the whole organisation, with the bin",
    JSON.stringify(olga.boards) === '["atlas","orbit"]' && olga.bin === true,
    JSON.stringify(olga),
  );

  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));
  check("and the content policy refused nothing", refusedByPolicy.length === 0, refusedByPolicy.slice(0, 3).join(" | "));
  await close();
}

await main();

export {};
