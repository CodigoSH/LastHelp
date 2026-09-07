/**
 * Drives a real Chrome over CDP against a real instance and walks the whole
 * journey: claim, create a board, report without an account, moderate, publish,
 * vote, and then do it again in Arabic.
 *
 * Run it with `make drive`, which starts a server on a scratch database and a
 * headless Chrome, then tears both down.
 *
 * It is a verification tool: it is never imported by anything the binary ships,
 * and it needs a browser that is on the machine rather than in the repository.
 *
 * Two things it exists to catch, because nothing else does:
 *
 *   - Whether the interface is genuinely in the language it claims. The first
 *     version of this checked the brand in the nav, which is "LastHelp" in all
 *     twenty and therefore proved nothing.
 *   - Whether the layout mirrors. That is measured as geometry — is the brand
 *     now past the middle of the window — rather than compared as pixels, which
 *     drift for reasons that have nothing to do with the change under test.
 *
 * Not one-shot (--screenshot / --dump-dom): those terminate the page before an
 * asynchronous start has finished, and the symptom is identical to a real
 * failure. This attaches, waits on promises, and reads the DOM the way a person
 * would see it.
 */
const PORT = Number(process.env.PORT ?? 8099);
const CDP = Number(process.env.CDP ?? 9222);
const BASE = `http://127.0.0.1:${PORT}`;

const consoleErrors: string[] = [];

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
      if (msg.method === "Runtime.exceptionThrown") {
        consoleErrors.push(msg.params?.exceptionDetails?.exception?.description ?? "exception");
      }
      if (msg.method === "Runtime.consoleAPICalled" && msg.params?.type === "error") {
        consoleErrors.push(msg.params.args?.map((a: any) => a.value ?? a.description).join(" ") ?? "console.error");
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

async function main() {
  const t = await target(BASE + "/");
  const page = await open(t.webSocketDebuggerUrl);

  await page.send("Page.enable");
  await page.send("Runtime.enable");
  await page.send("Log.enable");

  const evaluate = async (expression: string) => {
    const result = await page.send("Runtime.evaluate", {
      // Top-level await throws, so everything is wrapped.
      expression: `(async () => { ${expression} })()`,
      awaitPromise: true,
      returnByValue: true,
    });
    if (result?.exceptionDetails) {
      throw new Error(result.exceptionDetails.exception?.description ?? "evaluate failed");
    }
    return result?.result?.value;
  };

  const check = (label: string, ok: boolean, detail = "") => {
    console.log(`  ${ok ? "✓" : "✗"} ${label}${detail ? ` — ${detail}` : ""}`);
    if (!ok) process.exitCode = 1;
  };

  await sleep(700);

  // 1. The shell actually renders something.
  const first = await evaluate(`return document.getElementById("app")?.textContent?.trim() ?? ""`);
  check("the page renders", String(first).length > 0, String(first).slice(0, 60));

  // 2. The document is stamped for the first paint.
  const lang = await evaluate(`return document.documentElement.lang`);
  const dir = await evaluate(`return document.documentElement.dir`);
  check("lang and dir are stamped", Boolean(lang) && Boolean(dir), `lang=${lang} dir=${dir}`);

  // 3. Sign in through the form, as a person would.
  await evaluate(`history.pushState({}, "", "/sign-in"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(400);
  const setupVisible = await evaluate(`return !!document.querySelector("form.auth")`);
  check("the setup form is on screen", setupVisible === true);

  await evaluate(`
    const f = document.querySelector("form.auth");
    f.querySelector('[name=email]').value = "k4wit0@codigosh.com";
    f.querySelector('[name=password]').value = "una contraseña larga de verdad";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  `);
  await sleep(800);
  const signedIn = await evaluate(`return location.pathname`);
  check("signing in lands on the panel", signedIn === "/panel", String(signedIn));

  // 4. Create an organisation and a board through the API the panel uses.
  const created = await evaluate(`
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const post = (path, body) => fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: JSON.stringify(body),
    }).then(r => r.status);
    const a = await post("/api/organizations", { slug: "codigosh", name: "CodigoSH" });
    const b = await post("/api/orgs/codigosh/repositories", { slug: "lastboard", name: "Lastboard" });
    return [a, b];
  `);
  check("organisation and board created", JSON.stringify(created) === "[201,201]", JSON.stringify(created));

  // 5. Report something through the form, with no account.
  await evaluate(`history.pushState({}, "", "/codigosh/lastboard/new"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(500);
  const formThere = await evaluate(`return !!document.querySelector("form.report")`);
  check("the report form is on screen", formThere === true);

  const fillIn = async () =>
    evaluate(`
      // An idea, not a bug: only ideas are voted, so this is what exercises the
      // vote button further down.
      document.querySelector('[data-choice="idea"]').click();
      const f = document.querySelector("form.report");
      f.querySelector('[name=title]').value = "dark mode for the dashboard";
      f.querySelector('[name=body]').value = "the white is bright at night";
      f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    `);

  // Submitting the instant the form appeared is refused, and should be: nobody
  // types that fast. Checking it here rather than only in the Go tests is what
  // proves the message reaches the screen instead of failing silently.
  await fillIn();
  await sleep(500);
  const tooFast = await evaluate(`
    const line = document.querySelector(".form-error");
    return { shown: line ? !line.hidden : false, text: line?.textContent?.trim() ?? "" };
  `);
  check("an instant submission is refused, visibly", tooFast.shown === true, tooFast.text.slice(0, 50));

  // Now wait the way a person filling in a form does.
  await sleep(2500);
  await fillIn();
  await sleep(900);
  const followLink = await evaluate(`return document.querySelector(".follow-link input")?.value ?? ""`);
  check("the follow link is shown once", String(followLink).includes("?k="), String(followLink).slice(0, 48));

  // 6. Nothing waiting is on the public board.
  await evaluate(`history.pushState({}, "", "/codigosh/lastboard"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(500);
  const beforeApproval = await evaluate(`return document.querySelectorAll(".items li").length`);
  check("nothing waiting is public", beforeApproval === 0, `rows=${beforeApproval}`);

  // 7. The queue shows it, and publishing works from the panel.
  await evaluate(`history.pushState({}, "", "/codigosh/lastboard/queue"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(500);
  const queued = await evaluate(`return document.querySelectorAll(".queue li").length`);
  check("the queue shows it", queued === 1, `rows=${queued}`);

  await evaluate(`document.querySelector('[data-act="published"]').click()`);
  await sleep(600);
  const emptied = await evaluate(`return document.querySelectorAll(".queue li").length`);
  check("publishing removes it from the queue", emptied === 0, `rows=${emptied}`);

  // 8. Now it is public and votable.
  await evaluate(`history.pushState({}, "", "/codigosh/lastboard"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(600);
  const afterApproval = await evaluate(`return document.querySelectorAll(".items li").length`);
  check("it is on the public board", afterApproval === 1, `rows=${afterApproval}`);

  const href = await evaluate(`return document.querySelector(".items a")?.getAttribute("href") ?? ""`);
  await evaluate(`history.pushState({}, "", ${JSON.stringify(String(href))}); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(600);
  await evaluate(`document.querySelector("[data-vote]")?.click()`);
  await sleep(500);
  const voteCount = await evaluate(`return document.querySelector("[data-vote] .count")?.textContent ?? ""`);
  check("voting updates the button in place", String(voteCount).trim() === "1", `count=${voteCount}`);

  // 9. The team's status control, and the roadmap it feeds.
  //
  //    The control is drawn only if the server says this person may use it, so
  //    this also checks that asking is the check rather than the drawing.
  const statusControl = await evaluate(`return !!document.querySelector("#status-select")`);
  check("the team sees the status control", statusControl === true);

  await evaluate(`
    const select = document.querySelector("#status-select");
    select.value = "planned";
    select.dispatchEvent(new Event("change"));
  `);
  await sleep(600);

  await evaluate(`history.pushState({}, "", "/codigosh/lastboard/roadmap"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(700);
  const roadmap = await evaluate(`
    return {
      columns: document.querySelectorAll(".roadmap .column").length,
      planned: document.querySelectorAll(".roadmap .column:first-child li").length,
    };
  `);
  check("the roadmap draws all three columns", roadmap.columns === 3, `columns=${roadmap.columns}`);
  check("what was just planned is on it", roadmap.planned === 1, `planned=${roadmap.planned}`);

  // 10. The bridge control. There is no forge connected in this run, so what
  //     is checked is that the control is there for the team and that asking
  //     for an issue answers rather than failing silently.
  await evaluate(`history.pushState({}, "", ${JSON.stringify(String(href))}); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(700);
  const bridgeControl = await evaluate(`return !!document.querySelector("[data-create-issue]")`);
  check("the team sees the bridge control", bridgeControl === true);

  await evaluate(`document.querySelector("[data-create-issue]").click()`);
  await sleep(600);
  const issueNote = await evaluate(`
    const note = document.querySelector(".issue-note");
    return { shown: note ? !note.hidden : false, text: note?.textContent?.trim() ?? "" };
  `);
  check("asking for an issue says something", issueNote.shown === true, issueNote.text.slice(0, 44));

  // 11. Right to left, which is where a stylesheet written in left and right
  //     falls apart.
  await evaluate(`
    const select = document.querySelector("nav .lang");
    select.value = "ar";
    select.dispatchEvent(new Event("change"));
  `);
  await sleep(900);
  const rtl = await evaluate(`return [document.documentElement.lang, document.documentElement.dir]`);
  check("switching to Arabic flips the document", JSON.stringify(rtl) === '["ar","rtl"]', JSON.stringify(rtl));

  // Back to the board, where a counter and a tab are on screen, and check for
  // strings only the Arabic catalogue has. Checking the brand would prove
  // nothing: "LastHelp" is the same word in all twenty.
  await evaluate(`history.pushState({}, "", "/codigosh/lastboard"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(700);
  const arabic = await evaluate(`
    const body = document.body.textContent;
    return { votes: body.includes("الأصوات"), everything: body.includes("الكل"), sample: body.slice(0, 60) };
  `);
  check("the counter is in Arabic", arabic.votes === true, String(arabic.sample));
  check("the tabs are in Arabic", arabic.everything === true);

  // And the layout actually mirrored: the nav's first child should now start at
  // the right-hand edge. Geometry, not a screenshot.
  const mirrored = await evaluate(`
    const brand = document.querySelector("nav .brand").getBoundingClientRect();
    return brand.right > window.innerWidth / 2;
  `);
  check("the layout mirrored", mirrored === true);

  // 12. Nothing exploded along the way.
  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));

  await fetch(`http://127.0.0.1:${CDP}/json/close/${t.id}`);
  page.close();
}

await main();

export {};
