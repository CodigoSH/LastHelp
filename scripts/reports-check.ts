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

  // The red dot on the panel's icon when an update waits: 8px of red in a 2px
  // ring, 12px in all, 4px from the
  // icon's top and end. Drawn here into the real icon, because whether an
  // update exists is up to the release server, not to this pass.
  const dot = await evaluate(`
    const toggle = document.querySelector("body > nav [data-panel-toggle]");
    const d = document.createElement("span");
    d.className = "notification-dot";
    toggle.append(d);
    const b = d.getBoundingClientRect(), t = toggle.getBoundingClientRect();
    const cs = getComputedStyle(d);
    const out = { size: [Math.round(b.width), Math.round(b.height)], border: cs.borderTopWidth,
                  top: Math.round(b.top - t.top), end: Math.round(t.right - b.right) };
    d.remove();
    return out;
  `);
  check(
    "the update dot on the panel icon is 12px, a 2px ring, 4px in",
    JSON.stringify(dot.size) === "[12,12]" && dot.border === "2px" && dot.top === 4 && dot.end === 4,
    JSON.stringify(dot),
  );

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
  // 53 reports in all: a page of 50, and the rest behind "Show more".
  const archive = await evaluate(`
    const rows = () => document.querySelectorAll('[data-what="report"]').length;
    const first = rows();
    document.querySelector(".rev-list [data-more]")?.click();
    for (let i = 0; i < 40 && rows() === first; i++) await new Promise(r => setTimeout(r, 100));
    return { first, all: rows() };
  `);
  check("the archive holds every report, whatever became of it, a page at a time",
    archive.first === 50 && archive.all === 53, JSON.stringify(archive));

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

  // The same reply rewritten where it stands: the pencil beside the bin,
  // Cancel leaving the words as they were, Save reaching the server. Both
  // tools are asked whether they can be pressed, not only whether they exist.
  const rewritten = await evaluate(`
    const id = ${JSON.stringify(before.id)};
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const last = () => [...document.querySelectorAll(".rev-detail .comments .comment")].pop();
    last()?.scrollIntoView({ block: "center" });
    await new Promise(r => setTimeout(r, 200));
    const pressable = (sel) => { const b = last()?.querySelector(sel); const r = b?.getBoundingClientRect();
      return !!r && r.width > 0 && document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)?.closest(sel) === b; };
    const tools = { pencil: pressable("[data-edit-comment]"), bin: pressable("[data-delete-comment]") };
    // Without a pencil there is nothing to drive: said, rather than thrown.
    if (!last()?.querySelector("[data-edit-comment]")) return { tools };
    last().querySelector("[data-edit-comment]").click();
    await wait(() => last()?.querySelector("form.comment-edit textarea"));
    const prefilled = last().querySelector("form.comment-edit textarea").value;
    last().querySelector("[data-comment-cancel]").click();
    const cancelled = !last().querySelector("form.comment-edit") && last().querySelector(".what").hidden === false
      && last().querySelector(".what").textContent === "Released in 1.4.3.";
    last().querySelector("[data-edit-comment]").click();
    await wait(() => last()?.querySelector("form.comment-edit textarea"));
    const form = last().querySelector("form.comment-edit");
    form.querySelector("textarea").value = "Released in 1.4.4.";
    form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await wait(() => last()?.querySelector(".what")?.textContent === "Released in 1.4.4." && !last()?.querySelector("form.comment-edit"));
    const server = await fetch("/api/items/" + id).then(r => r.json());
    return { tools, prefilled, cancelled, shown: last()?.querySelector(".what")?.textContent,
             server: (server.comments ?? []).map(c => c.body).pop() };
  `);
  check(
    "a reply is rewritten where it stands, beside the bin, and the server keeps the new words",
    rewritten.tools.pencil && rewritten.tools.bin && rewritten.prefilled === "Released in 1.4.3." &&
      rewritten.cancelled === true && rewritten.shown === "Released in 1.4.4." && rewritten.server === "Released in 1.4.4.",
    JSON.stringify(rewritten),
  );
  await shoot("comment-edited");

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
             gone: !(server.comments ?? []).some(c => c.body === "Released in 1.4.4.") };
  `);
  check(
    "and deleting it, asked in the product's dialog, takes it off the pane and the server",
    unsaid.thread === 1 && unsaid.server === 1 && unsaid.gone === true,
    JSON.stringify(unsaid),
  );

  // Answering it is the last thing done to it here: the conversation closes with
  // the status — for the team 48 hours later —, so the reply and the edits come first.
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

  const closedPane = await evaluate(`
    const pane = document.querySelector(".rev-detail");
    return { form: !!pane.querySelector("form.team-reply"), said: (pane.textContent ?? "").length > 0,
             until: !!pane.querySelector(".comment-window"), thread: pane.querySelectorAll(".comments .comment").length };
  `);
  check(
    "an answered report keeps its conversation and, for the team, its reply box with the time it lasts until",
    closedPane.form === true && closedPane.until === true && closedPane.thread === 1,
    JSON.stringify(closedPane),
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
    // The first report with a conversation, so the tools on its messages
    // are counted against something rather than against nothing.
    const rows = [...document.querySelectorAll('[data-what="report"]')];
    let comments = 0;
    for (const row of rows.slice(0, 15)) {
      row.click();
      await wait(() => document.querySelector(".rev-detail [data-rewrite]"));
      await new Promise(r => setTimeout(r, 300));
      comments = document.querySelectorAll(".rev-detail .comments .comment").length;
      if (comments > 0) break;
    }
    return {
      boards,
      rows: rows.length,
      server: (api.items ?? []).length,
      edit: !!document.querySelector(".rev-detail [data-rewrite]"),
      status: !!document.querySelector(".rev-detail #status-select"),
      bin: !!document.querySelector(".rev-detail [data-delete-item]"),
      comments,
      commentBins: document.querySelectorAll(".rev-detail [data-delete-comment]").length,
      commentPencils: document.querySelectorAll(".rev-detail [data-edit-comment]").length,
    };
  `);
  check(
    "a collaborator's archive is their repository's reports, worked on in the pane",
    JSON.stringify(ana.boards) === '["atlas"]' && ana.rows === ana.server && ana.edit && ana.status,
    JSON.stringify(ana),
  );
  check(
    "without the report's bin, which is the owners' and the administrator's, but with the pencil and the bin on every message",
    ana.bin === false && ana.comments > 0 && ana.commentBins === ana.comments && ana.commentPencils === ana.comments,
    JSON.stringify(ana),
  );

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

  // An owner deletes the organisation's repositories, and does not end the
  // organisation: the bin on every repository row, none for the whole.
  await goTo("/panel/acme", "form.org-settings");
  const owns = await evaluate(`return {
    repoBins: document.querySelectorAll("[data-delete-repo]").length,
    repos: document.querySelectorAll('a[href^="/panel/acme/repos/"]').length,
    orgBin: !!document.querySelector("[data-delete-org]"),
  }`);
  await goTo("/panel/acme/repos/atlas", "form.repo-settings");
  const ownsRepo = await evaluate(`return !!document.querySelector("[data-delete-repo], .settings-danger")`);
  check(
    "an owner may delete the organisation's repositories, and may not end the organisation",
    owns.repoBins === 2 && owns.orgBin === false && ownsRepo === true,
    JSON.stringify({ owns, ownsRepo }),
  );

  // 8b. A repository's icon, chosen in its settings: a real picture handed to
  //     the file input as somebody choosing one would, scaled down on the way,
  //     and then on the organisation's public page and the front page — and
  //     taken off again, back to the initials.
  await signIn("alex");
  await goTo("/panel/acme/repos/atlas", "form.repo-settings");
  const chosen = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 300;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = "#1e88e5"; ctx.fillRect(0, 0, 300, 300);
    ctx.fillStyle = "#ffffff"; ctx.fillRect(90, 90, 120, 120);
    const blob = await new Promise(r => canvas.toBlob(r, "image/png"));
    const input = document.querySelector("#icon-file");
    const dt = new DataTransfer();
    dt.items.add(new File([blob], "atlas.png", { type: "image/png" }));
    input.files = dt.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await wait(() => document.querySelector(".icon-preview img"));
    const value = document.querySelector('form.repo-settings input[name="icon"]').value;
    const img = new Image(); img.src = value; await img.decode();
    return { preview: !!document.querySelector(".icon-preview img"), clear: !document.querySelector("[data-clear-icon]").hidden,
             side: Math.max(img.naturalWidth, img.naturalHeight), bytes: value.length, isImage: value.startsWith("data:image/") };
  `);
  check(
    "a repository's icon is chosen in its settings, scaled to 128px on the way",
    chosen.preview && chosen.clear && chosen.isImage && chosen.side === 128 && chosen.bytes <= 24 * 1024,
    JSON.stringify(chosen),
  );
  await evaluate(`document.querySelector("form.repo-settings").dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }))`);
  await sleep(1200);
  await goTo("/acme", ".cards");
  const onOrg = await evaluate(`return [...document.querySelectorAll(".cards .card")].find(c => c.textContent.includes("Atlas"))?.querySelector(".card-mark img") ? true : false`);
  await goTo("/", ".lp");
  const onFront = await evaluate(`return [...document.querySelectorAll(".lp-card, .brief")].find(c => c.textContent.includes("Atlas"))?.querySelector(".lp-board-mark img") ? true : false`);
  check("and it is on the organisation's public page and on the front page", onOrg === true && onFront === true, JSON.stringify({ onOrg, onFront }));

  await goTo("/panel/acme/repos/atlas", "form.repo-settings");
  await evaluate(`
    document.querySelector("[data-clear-icon]").click();
    document.querySelector("form.repo-settings").dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  `);
  await sleep(1200);
  await goTo("/acme", ".cards");
  const back = await evaluate(`
    const card = [...document.querySelectorAll(".cards .card")].find(c => c.textContent.includes("Atlas"));
    return { img: !!card?.querySelector(".card-mark img"), initials: card?.querySelector(".card-mark")?.textContent?.trim() ?? "" };
  `);
  check("taken off, the repository shows its initials again", back.img === false && back.initials.length > 0, JSON.stringify(back));

  // 8c. The same for an organisation, from its settings: beside its name on
  //     its public page and in the panel's list — and taken off again.
  await goTo("/panel/acme", "form.org-settings");
  const orgChosen = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 256;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = "#43a047"; ctx.fillRect(0, 0, 256, 256);
    const blob = await new Promise(r => canvas.toBlob(r, "image/png"));
    const form = document.querySelector("form.org-settings");
    const input = form.querySelector("[data-icon-file]");
    const dt = new DataTransfer();
    dt.items.add(new File([blob], "acme.png", { type: "image/png" }));
    input.files = dt.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await wait(() => form.querySelector(".icon-preview img"));
    const value = form.querySelector('input[name="icon"]').value;
    return { preview: !!form.querySelector(".icon-preview img"), isImage: value.startsWith("data:image/") };
  `);
  if (SHOTS) { await evaluate(`document.querySelector("form.org-settings").scrollIntoView({ block: "center" })`); await sleep(400); }
  await shoot("org-icon-settings");
  check("an organisation's icon is chosen in its settings", orgChosen.preview && orgChosen.isImage, JSON.stringify(orgChosen));
  await evaluate(`document.querySelector("form.org-settings").dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }))`);
  await sleep(1200);
  await goTo("/acme", ".page-head");
  const orgHead = await evaluate(`
    const img = document.querySelector(".page-head .head-mark img");
    const box = img?.getBoundingClientRect();
    return { img: !!img, w: Math.round(box?.width ?? 0) };
  `);
  if (SHOTS) await sleep(900);
  await shoot("org-icon-public-page");
  await goTo("/panel/organizations", "[data-org]");
  if (SHOTS) await sleep(900);
  await shoot("org-icon-panel-list");
  const orgRow = await evaluate(`return !!document.querySelector('[data-org="acme"] .repo-si-mark img')`);
  check(
    "and it is beside its name on its public page and in the panel's list",
    orgHead.img === true && orgHead.w > 40 && orgRow === true,
    JSON.stringify({ orgHead, orgRow }),
  );
  await goTo("/panel/acme", "form.org-settings");
  await evaluate(`
    const form = document.querySelector("form.org-settings");
    form.querySelector("[data-clear-icon]").click();
    form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  `);
  await sleep(1200);
  await goTo("/acme", ".page-head");
  const orgBack = await evaluate(`return { mark: !!document.querySelector(".page-head .head-mark"), title: document.querySelector(".page-head h1")?.textContent?.trim() }`);
  check("taken off, the organisation's page is its name alone again", orgBack.mark === false && orgBack.title === "Acme", JSON.stringify(orgBack));

  // 9. The public list of a board: open first, a page at a time, closed on
  //    request, all of it in the address. The numbers are the server's at
  //    this point of the walk — earlier steps closed one report and published
  //    another — and the open side has to run past one page, or the paging
  //    would not be exercised.
  await goTo("/acme/atlas", ".items, .empty");
  const server = await evaluate(`
    const side = async (s) => (await fetch("/api/orgs/acme/repos/atlas/items?limit=1&state=" + s).then(r => r.json())).total;
    return { open: await side("open"), closed: await side("closed") };
  `);
  const wait = `const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };`;
  const closedWords = await evaluate(`
    const words = await fetch("/locales/" + document.documentElement.lang + ".json").then(r => r.json());
    return ["status.done", "status.declined", "status.closed", "status.not_reproducible"].map(k => words[k]);
  `);
  const openSide = await evaluate(`
    ${wait}
    const chip = (s) => Number(document.querySelector('[data-state="' + s + '"] .count-chip')?.textContent ?? -1);
    const rows = () => document.querySelectorAll(".items > li").length;
    const before = { on: document.querySelector("[data-state].on")?.dataset.state, open: chip("open"), closed: chip("closed"),
                     rows: rows(), foot: document.querySelector(".board-shown")?.textContent?.replace(/[\u2068\u2069]/g, "") ?? "" };
    document.querySelector("[data-more]")?.click();
    await wait(() => rows() > before.rows);
    return { ...before, after: rows(), footAfter: !!document.querySelector("[data-board-foot]"),
             states: [...document.querySelectorAll(".items .status")].map(e => e.textContent.trim()) };
  `);
  check(
    "a board opens on what is open, a page at a time, and says how much of it is shown",
    server.open > 20 && openSide.on === "open" && openSide.open === server.open && openSide.closed === server.closed &&
      openSide.rows === 20 && openSide.foot.includes("20") && openSide.foot.includes(String(server.open)) &&
      openSide.after === server.open && openSide.footAfter === false &&
      !openSide.states.some((w: string) => closedWords.includes(w)),
    JSON.stringify({ server, ...openSide, states: openSide.states.length }),
  );

  const closedSide = await evaluate(`
    ${wait}
    document.querySelector('[data-state="closed"]').click();
    await wait(() => document.querySelector('[data-state="closed"].on'));
    await wait(() => document.querySelectorAll(".items > li").length === ${server.closed});
    return { address: location.search, rows: document.querySelectorAll(".items > li").length,
             states: [...document.querySelectorAll(".items .status")].map(e => e.textContent.trim()) };
  `);
  check(
    "closed is one click away, in the address, and holds only what has an answer",
    closedSide.address.includes("state=closed") && closedSide.rows === server.closed &&
      closedSide.states.length === server.closed && closedSide.states.every((w: string) => closedWords.includes(w)),
    JSON.stringify({ ...closedSide, states: [...new Set(closedSide.states)] }),
  );

  await page.send("Page.reload");
  await waitFor(".items, .empty");
  const reloaded = await evaluate(`return { on: document.querySelector("[data-state].on")?.dataset.state, rows: document.querySelectorAll(".items > li").length }`);
  check("and a reload lands on the same side", reloaded.on === "closed" && reloaded.rows === server.closed, JSON.stringify(reloaded));

  const nothing = await evaluate(`
    ${wait}
    const words = await fetch("/locales/" + document.documentElement.lang + ".json").then(r => r.json());
    const box = document.querySelector(".search");
    box.value = "zzzz-nothing-says-this";
    box.dispatchEvent(new Event("input", { bubbles: true }));
    await wait(() => new URLSearchParams(location.search).get("q") && document.querySelector(".empty"));
    return { said: document.querySelector(".empty")?.textContent ?? "", expected: words["review.nothingMatches"] };
  `);
  check("a search that finds nothing says so, not that the board is empty", nothing.said.includes(nothing.expected), JSON.stringify(nothing));

  await goTo("/acme/orbit", ".items, .empty");
  const nextBoard = await evaluate(`return { on: document.querySelector("[data-state].on")?.dataset.state, q: document.querySelector(".search")?.value ?? "", address: location.search }`);
  check("another board starts afresh, not with the last one's filters", nextBoard.on === "open" && nextBoard.q === "" && nextBoard.address === "", JSON.stringify(nextBoard));

  // 11. Last, because it turns a report down: the administrator is offered a field
  //     for the word that gave it away, the word reaches the server's list, and
  //     an owner or collaborator is not offered the field at all.
  await signIn("alex");
  await goTo("/review?mode=pending", ".rev-row");
  const palabra = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    document.querySelector('[data-what="report"]').click();
    await wait(() => document.querySelector(".rev-detail [data-moderate='rejected']"));
    const field = document.querySelector(".rev-detail [name=add_word]");
    if (!field) return { field: false };
    const rows = document.querySelectorAll("[data-pick]").length;
    field.value = "Zzgiveaway";
    document.querySelector(".rev-detail [data-moderate='rejected']").click();
    await wait(() => document.querySelectorAll("[data-pick]").length === rows - 1);
    const listed = await fetch("/api/antiabuse").then(r => r.json());
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/antiabuse/words", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ words: [] }) });
    return { field: true, gone: document.querySelectorAll("[data-pick]").length === rows - 1,
      listed: listed.words.map(w => w.word) };
  `);
  check(
    "rejecting from the pane with a word adds it to the server's list, for the administrator",
    palabra.field === true && palabra.gone === true && palabra.listed.length === 1 && palabra.listed[0] === "Zzgiveaway",
    JSON.stringify(palabra),
  );
  await signIn("olga");
  await goTo("/review?mode=pending", ".rev-row, .empty");
  const sinCampo = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    document.querySelector('[data-what="report"]')?.click();
    await wait(() => document.querySelector(".rev-detail [data-moderate='rejected']") || !document.querySelector('[data-what="report"]'));
    return { pane: !!document.querySelector(".rev-detail [data-moderate='rejected']"), field: !!document.querySelector("[name=add_word]") };
  `);
  check("and an owner, who triages the board, is not offered the field", sinCampo.field === false, JSON.stringify(sinCampo));

  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));
  check("and the content policy refused nothing", refusedByPolicy.length === 0, refusedByPolicy.slice(0, 3).join(" | "));
  await close();
}

await main();

export {};
