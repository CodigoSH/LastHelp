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

// What the content policy refused, which is not the same list as the console's.
//
// A blocked script or style arrives as a Log entry from the security source,
// never as a console API call — so "no console errors" said nothing at all
// about the policy, and a policy that quietly breaks a screen would have walked
// straight past this file.
const refusedByPolicy: string[] = [];

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
      if (msg.method === "Log.entryAdded") {
        const entry = msg.params?.entry ?? {};
        if (entry.source === "security" || /Content Security Policy/i.test(entry.text ?? "")) {
          refusedByPolicy.push(String(entry.text ?? "").slice(0, 160));
        }
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
  await sleep(900);
  // A fresh instance opens the wizard rather than a bare login: the first
  // screen has to carry its own progress, or nobody knows how many more there
  // are. The dots are read from the markup, not from the shape.
  const firstStep = await evaluate(`
    return {
      path: location.pathname,
      prefs: !!document.querySelector("form.setup-preferences"),
      dots: document.querySelectorAll(".steps li").length,
      current: document.querySelector('.steps [aria-current="step"] .step-name')?.textContent?.trim(),
      theme: !!document.querySelector("[data-theme-choice]"),
    };`);
  // The mark, on the first screen anybody ever sees of this. There was none
  // anywhere in the product: a name in text and an empty space beside it.
  const mark = await evaluate(`
    const el = document.querySelector(".wizard-mark");
    return el ? { src: el.getAttribute("src"), drawn: Math.round(el.getBoundingClientRect().width) } : null;
  `);
  check("the wizard wears the mark", mark?.src === "/logo.svg" && mark.drawn > 40, JSON.stringify(mark));

  check(
    "a fresh instance opens on the wizard, asking language first",
    firstStep.path === "/setup" && firstStep.prefs === true && firstStep.dots === 3 && firstStep.theme === true,
    `${firstStep.path}, ${firstStep.dots} steps, on "${firstStep.current}"`,
  );

  // Choosing dark has to work on a light system too. A toggle that only ever
  // darkens is one that cannot be turned back, and the media query alone gives
  // exactly that.
  const themed = await evaluate(`
    document.querySelector('[data-theme-choice="dark"]').click();
    await new Promise(r => setTimeout(r, 400));
    const dark = document.documentElement.getAttribute("data-theme");
    document.querySelector('[data-theme-choice="light"]').click();
    await new Promise(r => setTimeout(r, 400));
    return { dark, light: document.documentElement.getAttribute("data-theme") };`);
  check(
    "the theme choice sticks in both directions",
    themed.dark === "dark" && themed.light === "light",
    JSON.stringify(themed),
  );

  await evaluate(`document.querySelector("form.setup-preferences").dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));`);
  await sleep(700);

  // The second box is the one thing standing between a typo and an instance
  // nobody can sign into: there is no reset here.
  const mismatch = await evaluate(`
    const f = document.querySelector("form.setup-account");
    f.querySelector('[name=username]').value = "alex";
    f.querySelector('[name=password]').value = "una contraseña larga de verdad";
    f.querySelector('[name=password_again]').value = "otra cosa completamente";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 500));
    const line = document.querySelector(".form-error");
    return { path: location.pathname, shown: line && !line.hidden, said: line?.textContent?.trim() };`);
  check(
    "two passwords that differ are refused before the server sees them",
    mismatch.path === "/setup/account" && mismatch.shown === true,
    String(mismatch.said),
  );

  // The picture, as one control and with nothing around it. Lastboard's, and
  // the same on purpose: a label, an upload button and a sentence saying it is
  // optional is three pieces of furniture for something you click or ignore.
  const picture = await evaluate(`
    return {
      pick: !!document.querySelector(".avatar-pick"),
      badge: !!document.querySelector(".avatar-pick-badge"),
      // A button may not contain interactive content, and an input inside it
      // takes the click the button forwards, which bubbles back, forever.
      inputInside: !!document.querySelector(".avatar-pick input"),
      // Every screen that picks an image uses this one control. Three of them
      // were a label, an upload button and a remove button in a row, because
      // it was written for the wizard and copied nowhere.
      oldButtons: [...document.querySelectorAll("button")]
        .filter(b => /Elige|Choose|Quitar|Remove/.test(b.textContent)).length,
      hints: document.querySelectorAll(".wizard-card .hint").length,
      lead: !!document.querySelector(".wizard-head .lead"),
    };
  `);
  check(
    "the picture is one control, with no furniture around it",
    picture.pick && picture.badge && !picture.inputInside && picture.hints === 1 && picture.oldButtons === 0,
    JSON.stringify(picture),
  );
  check("and the step does not introduce itself", picture.lead === false);

  // A heading hard against the left edge of a card centred on an empty page
  // reads as a fragment of a wider layout that is not there. Measured, because
  // text-align can be set and change nothing.
  const centred = await evaluate(`
    const h = document.querySelector(".wizard-head .wizard-where").getBoundingClientRect();
    const box = document.querySelector(".wizard").getBoundingClientRect();
    return Math.abs((h.left - box.left) - (box.right - h.right)) < 2;
  `);
  check("the wizard's title is centred", centred === true);

  // It is drawn as a small caption now, which is exactly how a heading stops
  // being one by accident. A setup screen with no <h1> is a page a screen
  // reader cannot navigate by headings.
  const heading = await evaluate(
    `return document.querySelector(".wizard-where")?.tagName ?? "";`,
  );
  check("and it is still the heading of the page", heading === "H1", heading);

  await evaluate(`
    const f = document.querySelector("form.setup-account");
    f.querySelector('[name=password_again]').value = "una contraseña larga de verdad";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  `);
  await sleep(1100);
  const afterAccount = await evaluate(`return location.pathname`);
  check("the account carries on into the wizard", afterAccount === "/setup/instance", String(afterAccount));

  // The wizard configures the installation and stops there. Organisations and
  // repositories are the work, and the work is done in the panel.
  await evaluate(`
    const f = document.querySelector("form.setup-instance");
    f.querySelector('[name=name]').value = "CodigoSH";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  `);
  await sleep(1100);
  const ending = await evaluate(`
    return { path: location.pathname, next: document.querySelector(".wizard-actions a")?.getAttribute("href") };`);
  check(
    "the wizard ends by handing over to the work",
    ending.path === "/setup/done" && ending.next === "/panel/new",
    `${ending.path} → ${ending.next}`,
  );

  // 4b. An organisation and a repository, made the way an operator makes them:
  //     from a modal on the screen that lists them.
  await evaluate(`history.pushState({}, "", "/panel/organizations"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1200);
  const started = await evaluate(`
    document.querySelector("[data-add-org]").click();
    await new Promise(r => setTimeout(r, 300));
    const dialog = document.querySelector("#org-modal");
    if (!dialog?.open) return "the modal did not open";
    const f = dialog.querySelector("form");
    f.querySelector('[name=name]').value = "CodigoSH";
    f.querySelector('[name=slug]').value = "codigosh";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    return [...document.querySelectorAll(".bento-card h3")].map(e => e.textContent.trim());
  `);
  check(
    "an organisation is added from a modal, not from a form left on the page",
    JSON.stringify(started) === '["CodigoSH"]',
    JSON.stringify(started),
  );

  await evaluate(`history.pushState({}, "", "/panel/codigosh"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1200);
  const boards = await evaluate(`
    document.querySelector("[data-add-repo]").click();
    await new Promise(r => setTimeout(r, 300));
    const f = document.querySelector("#repo-modal form");
    f.querySelector('[name=name]').value = "Lastboard";
    f.querySelector('[name=slug]').value = "lastboard";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    return [...document.querySelectorAll(".repositories h3")].map(e => e.textContent.trim());
  `);
  check(
    "a repository is added the same way",
    JSON.stringify(boards) === '["Lastboard"]',
    JSON.stringify(boards),
  );

  // 5. Report something through the form, with no account.
  await evaluate(`history.pushState({}, "", "/codigosh/lastboard/new"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(500);
  const formThere = await evaluate(`return !!document.querySelector("form.report")`);
  check("the report form is on screen", formThere === true);

  // The form is the narrowest point of the product: everything downstream is
  // fed by what a stranger is willing to type here once. So it has to say
  // where the report is going, and ask a bug the two questions that decide
  // whether it arrives as information or as homework.
  const form = await evaluate(`
    document.querySelector('[data-choice="bug"]').click();
    const asked = [...document.querySelectorAll('.kind-fields[data-for="bug"] label')].map(l => l.textContent.trim());
    document.querySelector('[data-choice="idea"]').click();
    return {
      crumb: [...document.querySelectorAll(".crumb a")].map(a => a.getAttribute("href")).join(","),
      cards: document.querySelectorAll(".choice-card").length,
      asked,
      hiddenForIdeas: document.querySelector('.kind-fields[data-for="bug"]').hidden,
    };
  `);
  check("it says where the report is going", form.crumb === "/codigosh,/codigosh/lastboard", form.crumb);
  check("the kind is a choice of two cards", form.cards === 2, String(form.cards));

  // Measured, because this was broken for an unknown length of time and nothing
  // said so: `.report` named both this form and a row in the moderation queue,
  // so the row's `display: flex; align-items: center` was applied here and
  // stacked the fields into a narrow column with the button adrift beside them.
  // The page still rendered, every other check still passed, and the
  // documentation screenshot showed it. A form is not on screen if it is not
  // laid out.
  const laidOut = await evaluate(`
    const form = document.querySelector("form.report").getBoundingClientRect();
    const title = document.querySelector("#report-title").getBoundingClientRect();
    const send = document.querySelector("form.report button[type=submit]").getBoundingClientRect();
    const cards = [...document.querySelectorAll(".choice-card")].map(c => c.getBoundingClientRect());
    return {
      titleFills: Math.round((title.width / form.width) * 100),
      sendFills: Math.round((send.width / form.width) * 100),
      // Side by side: two cards on one row have the same top and different lefts.
      choicesAbreast: cards.length === 2 && Math.abs(cards[0].top - cards[1].top) < 2
        && Math.abs(cards[0].left - cards[1].left) > 50,
    };
  `);
  check(
    "and the form is laid out, not stacked into a column",
    laidOut.titleFills > 60 && laidOut.sendFills > 60 && laidOut.choicesAbreast === true,
    JSON.stringify(laidOut),
  );
  // Compared against the catalogue the page is actually in, not against the
  // English words: this walk runs in whatever language the browser asks for,
  // and a probe carrying its own copy of the expected text only measures
  // whether the same words were typed twice.
  const catalogue = await Bun.file(
    `web/public/locales/${await evaluate(`return document.documentElement.lang`)}.json`,
  ).json();
  check(
    "a bug is asked what makes it usable",
    JSON.stringify(form.asked) === JSON.stringify([catalogue["form.version"], catalogue["form.steps"]]),
    JSON.stringify(form.asked),
  );
  check("and an idea is not asked it", form.hiddenForIdeas === true);

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

  // 5b. And the page after sending is not a dead end.
  //
  //     It said "it will appear once reviewed", offered an address to copy and
  //     nothing else: somebody who had just written a report was left on a
  //     screen with no way to their own report and no way back to the board.
  //     Both are checked by following one of them.
  const onward = await evaluate(`
    const links = [...document.querySelectorAll(".page .wizard-actions a")].map(a => a.getAttribute("href"));
    return { links };
  `);
  check(
    "and the page after sending leads somewhere",
    onward.links.length === 2 &&
      onward.links.some((h: string) => h.startsWith("/i/") && h.includes("?k=")) &&
      onward.links.includes("/codigosh/lastboard"),
    JSON.stringify(onward.links),
  );

  await evaluate(`document.querySelector(".wizard-actions a.button.primary").click()`);
  await sleep(900);
  const landed = await evaluate(`
    return { path: location.pathname, title: document.querySelector(".detail h1")?.textContent?.trim() ?? "" };
  `);
  check(
    "and the report itself opens from it",
    landed.path.startsWith("/i/") && landed.title === "dark mode for the dashboard",
    JSON.stringify(landed),
  );

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

  // 8a. The board, as a piece of design rather than as a list of strings.
  //
  //     Two things measured, both invisible in the markup. The bar carries the
  //     name of the installation and the page under it carries its first word,
  //     and if the two do not start on the same vertical line the screen reads
  //     as unaligned however correct it is — which is what a full-width bar
  //     over a centred column looked like. And every row leads with a tile of
  //     the same width, so the titles line up whatever kind they are: a score
  //     on an idea, a mark on a bug, nothing at all and the column staggers.
  //     Measured in a window wide enough for the page to be narrower than it.
  //     The browser opens at 800px, where a column capped at 70rem never
  //     reaches its cap and a bar pinned to the window starts at the same
  //     twenty pixels as the text — so the check passed with the bug in place.
  //     A probe that gives the same answer either way measures nothing.
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: 1280, height: 900, deviceScaleFactor: 1, mobile: false,
  });
  await sleep(300);
  const grid = await evaluate(`
    const brand = document.querySelector("body > nav .brand").getBoundingClientRect();
    const heading = document.querySelector("h1").getBoundingClientRect();
    const rows = [...document.querySelectorAll(".item")];
    const leads = rows.map(r => Math.round(r.firstElementChild.getBoundingClientRect().width));
    const titles = rows.map(r => Math.round(r.querySelector(".item-title").getBoundingClientRect().left));
    return {
      offset: Math.round(Math.abs(brand.left - heading.left)),
      leads: [...new Set(leads)],
      titles: [...new Set(titles)],
    };
  `);
  check(
    "the bar starts where the page starts",
    grid.offset <= 1,
    `${grid.offset}px apart`,
  );
  check(
    "and every row leads with the same tile, so the titles line up",
    grid.leads.length === 1 && grid.titles.length === 1,
    `leads=${JSON.stringify(grid.leads)} titles=${JSON.stringify(grid.titles)}`,
  );
  await page.send("Emulation.clearDeviceMetricsOverride");
  await sleep(300);

  const href = await evaluate(`return document.querySelector(".items a")?.getAttribute("href") ?? ""`);
  await evaluate(`history.pushState({}, "", ${JSON.stringify(String(href))}); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(600);
  await evaluate(`document.querySelector("[data-vote]")?.click()`);
  await sleep(500);
  const voteCount = await evaluate(`return document.querySelector("[data-vote] .count")?.textContent ?? ""`);
  check("voting updates the button in place", String(voteCount).trim() === "1", `count=${voteCount}`);

  // 8b. The report as a link somebody was sent: it has to say what board it is
  //     on and lead back to it, or a shared address is a dead end.
  const detail = await evaluate(`
    const crumb = [...document.querySelectorAll(".crumb a")].map(a => a.getAttribute("href"));
    const vote = document.querySelector(".vote-block");
    return {
      crumb,
      status: document.querySelector(".detail .status")?.className ?? "",
      vote: vote ? [...vote.children].map(c => c.className).join("+") : "",
      // The report card holds the report and the vote, and nothing else. The
      // team's controls used to be appended here, which made them columns of
      // its grid and squeezed the report into a strip a few words wide —
      // markup that read correctly and rendered wrong.
      cardChildren: [...document.querySelector("article.detail").children].map(c => c.classList[0]).join("+"),
      tools: !!document.querySelector(".team-tools .tools"),
    };
  `);
  check(
    "a report says which board it is on and leads back",
    JSON.stringify(detail.crumb) === '["/codigosh","/codigosh/lastboard"]',
    JSON.stringify(detail.crumb),
  );
  check("its status wears the colour the board uses", detail.status.includes("status-"), detail.status);

  // 8c. And for the team, the report keeps the panel around it.
  //
  //     The address is the public one and a stranger must not be shown an
  //     application, but for the team this is where the work is done — and
  //     opening one from the listing used to drop them out of the panel: no
  //     navigation, no way back to what they were working through. Measured on
  //     a cold load of the address, not a click inside the running page, which
  //     is how a report reaches somebody from a chat or a bookmark.
  const inPanel = await evaluate(`
    return {
      sidebar: !!document.querySelector(".side"),
      current: document.querySelector('.side-item[aria-current="page"]')?.textContent?.trim() ?? "",
      publicNav: getComputedStyle(document.querySelector("body > nav")).display,
      tools: document.querySelector(".team-tools") ? !document.querySelector(".team-tools").hidden : false,
    };
  `);
  check(
    "and the team keeps their panel around it",
    inPanel.sidebar === true && inPanel.publicNav === "none" && inPanel.tools === true,
    JSON.stringify(inPanel),
  );
  check("the vote is a count and a verb", detail.vote === "count+label", detail.vote);
  check(
    "the team's controls are not inside the report card",
    detail.cardChildren === "detail-main+vote-block" && detail.tools === true,
    `${detail.cardChildren} tools=${detail.tools}`,
  );

  // 8c. A comment, all the way: written in public, held back until somebody
  //     reads it, published from the queue, and only then on the page. Every
  //     step of this existed except the middle one, so the page promised a
  //     reading that could not happen.
  await evaluate(`
    const form = document.querySelector("form.comment-form");
    form.querySelector("textarea").value = "it happens on the phone too";
    form.requestSubmit();
  `);
  await sleep(700);
  const heldBack = await evaluate(`
    return {
      told: !document.querySelector(".comment-sent")?.hidden,
      shown: document.body.textContent.includes("it happens on the phone too"),
    };
  `);
  check("a comment is taken and held for a person", heldBack.told === true && heldBack.shown === false,
    JSON.stringify(heldBack));

  await evaluate(`history.pushState({}, "", "/codigosh/lastboard/queue"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(800);
  const queued2 = await evaluate(`return document.querySelectorAll("li[data-comment]").length`);
  check("it waits in the queue beside the reports", queued2 === 1, `rows=${queued2}`);
  // The same message, on the screen that does not make you name a board first.
  // That is the whole point of it: five boards used to mean asking five times
  // to find the one with something in it. The row has to name its board, or a
  // decision is taken about a message from nowhere.
  await evaluate(`history.pushState({}, "", "/panel/reports"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1400);
  const spanning = await evaluate(`
    const first = document.querySelector("li[data-comment]");
    return {
      rows: document.querySelectorAll("li[data-comment]").length,
      board: first?.querySelector(".chip")?.textContent?.trim() ?? "",
      approve: !!first?.querySelector('[data-comment-act="published"]'),
    };
  `);
  check(
    "a message waiting is on the reports screen too, naming its board",
    spanning.rows === 1 && spanning.board !== "" && spanning.approve === true,
    JSON.stringify(spanning),
  );

  // And decided there. Approved from this screen rather than from the board's
  // own queue, so what follows — the comment appearing on the public page — is
  // the proof that the decision taken here is the same decision.
  await evaluate(`document.querySelector("li[data-comment] [data-comment-act='published']").click()`);
  await sleep(900);
  const cleared = await evaluate(`
    const r = await fetch("/api/panel/comments", { headers: { accept: "application/json" } });
    const body = await r.json();
    return { onScreen: document.querySelectorAll("li[data-comment]").length, stillWaiting: (body.comments ?? []).length };
  `);
  check(
    "and approving it there leaves the queue empty on both sides",
    cleared.onScreen === 0 && cleared.stillWaiting === 0,
    JSON.stringify(cleared),
  );

  await evaluate(`history.pushState({}, "", ${JSON.stringify(String(href))}); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(800);
  const published = await evaluate(`
    return {
      shown: document.body.textContent.includes("it happens on the phone too"),
      counted: document.querySelector(".count-chip")?.textContent ?? "",
    };
  `);
  check("and appears once a person has published it", published.shown === true && published.counted === "1",
    JSON.stringify(published));

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
  // Three columns of the same height. Left to their contents they step down
  // like a bar chart, and a quiet board reads as a broken one.
  const columnHeights = await evaluate(`
    return [...new Set([...document.querySelectorAll(".roadmap .column")]
      .map(c => Math.round(c.getBoundingClientRect().height)))];
  `);
  check("and all three are the same height", columnHeights.length === 1, JSON.stringify(columnHeights));

  // 10. The bridge control. There is no forge connected in this run, so what
  //     is checked is that the control is there for the team and that asking
  //     for an issue answers rather than failing silently.
  await evaluate(`history.pushState({}, "", ${JSON.stringify(String(href))}); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(700);
  const bridgeControl = await evaluate(`return !!document.querySelector("[data-create-issue]")`);
  check("the team sees the bridge control", bridgeControl === true);

  // Every toast off the screen first. Reading whichever one happens to be
  // there measures the last action, not this one — and it passed that way,
  // reporting the status change from four steps earlier as the answer to a
  // button it never pressed.
  await evaluate(`document.querySelectorAll(".toast-close").forEach(b => b.click());`);
  await sleep(400);
  await evaluate(`document.querySelector("[data-create-issue]").click()`);
  await sleep(600);
  // It answers in a toast now, not in a line under the button. Which is the
  // point of checking it here: the message moved, and the only thing that
  // would have noticed is a probe that goes looking for it.
  const issueNote = await evaluate(`
    const t = document.querySelector(".toast");
    return { shown: !!t, text: t?.querySelector(".toast-message")?.textContent?.trim() ?? "" };
  `);
  check("asking for an issue says something", issueNote.shown === true, issueNote.text.slice(0, 44));

  // 11. The update panel. This build has no release keys, so what is checked is
  //     that it says so rather than showing nothing — an empty answer where a
  //     reason belongs reads as "you are up to date".
  // Updates have a section of their own in the sidebar now. They used to hang
  // off the bottom of the panel's front page, under a form, which is not where
  // anybody looks for them.
  await evaluate(`history.pushState({}, "", "/panel/updates"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1100);
  const updates = await evaluate(`
    const body = document.body.textContent;
    return {
      hasSection: !!document.querySelector(".running"),
      offersInstall: !!document.querySelector("[data-update]"),
      text: (document.querySelector(".notice")?.textContent ?? "").trim(),
    };
  `);
  check("the panel says what version is running", updates.hasSection === true);

  // 11a. Signing in through the form, which nothing here did.
  //
  // Every sign-in in this walk was a fetch with a payload written by hand, so
  // the form could send whatever it liked. It sent `email` — the name from
  // before the field was renamed to `username` — and every real sign-in was
  // answered "a username is needed", on a form where it was plainly filled in.
  // A hand-written payload tests the endpoint; this tests the page.
  await evaluate(`
    const csrf = () => document.cookie.split("; ").find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
  `);
  await evaluate(`history.pushState({}, "", "/sign-in"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(900);

  // One action, one word for it, the mark above it, and an eye on the password.
  // It was a heading, a button and a link in the bar all reading "Sign in".
  const signIn = await evaluate(`
    const word = document.querySelector(".signin-go").textContent.trim();
    return {
      times: (document.body.innerText.match(new RegExp(word, "g")) ?? []).length,
      // Asked of the card rather than of the header inside it: what this check
      // is for is that the entrance wears the installation's mark, not where in
      // the markup that mark happens to hang.
      mark: !!document.querySelector(".signin .brand-logo"),
      eye: !!document.querySelector(".pw-toggle"),
      navLink: !!document.querySelector("body > nav a[href='/sign-in']"),
      lead: !!document.querySelector(".signin-head .lead"),
    };
  `);
  check("the sign-in page says its one word once", signIn.times === 1 && !signIn.navLink && !signIn.lead,
    JSON.stringify(signIn));
  check("and carries the mark and an eye on the password", signIn.mark && signIn.eye, JSON.stringify(signIn));

  await evaluate(`
    const f = document.querySelector("form.auth");
    f.querySelector('[name=username]').value = "alex";
    f.querySelector('[name=password]').value = "una contraseña larga de verdad";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  `);
  await sleep(1400);
  const signedIn = await evaluate(`
    const line = document.querySelector(".form-error");
    return { path: location.pathname, said: line && !line.hidden ? line.textContent.trim() : "" };
  `);
  check(
    "signing in through the form works",
    signedIn.path === "/panel" && signedIn.said === "",
    `${signedIn.path} ${signedIn.said}`,
  );

  // 11b. About, which is the page somebody keeping a server running opens.
  await evaluate(`history.pushState({}, "", "/panel/about"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(900);
  const about = await evaluate(`
    return {
      cards: document.querySelectorAll(".bento-card").length,
      // The commit hash and the name of the installation used to be here. One
      // answers a question nobody asks on this page, and the other is a
      // setting that lives on Personalization.
      facts: [...document.querySelectorAll(".fact")].map(f => f.textContent.trim()),
    };
  `);
  check("About reports the state of the server", about.cards >= 4, `${about.cards} cards`);
  check(
    "and every card of it has a number to report",
    about.facts.length === about.cards && about.facts.every((f: string) => f.length > 0),
    JSON.stringify(about.facts),
  );

  const langPicker = await evaluate(`return !!document.querySelector("body > nav select");`);
  check("no language picker rides along in the top bar", langPicker === false);

  // The same control on the screens that are not the wizard, which is the half
  // that kept being forgotten: a change made in one place and nowhere else is
  // work handed back to whoever has to find the other three.
  for (const [name, path] of [["profile", "/panel/account"], ["personalisation", "/panel/personalization"]]) {
    await evaluate(`history.pushState({}, "", ${JSON.stringify(path)}); dispatchEvent(new PopStateEvent("popstate"));`);
    await sleep(900);
    // Counted inside the field that holds the image, and outside the common
    // control within it. It used to count every button on the page with that
    // wording, which caught the thing it was written for and also anything else
    // that ever says "Remove" — the front page editor says it once per module.
    // The question it is asking is "did this image picker get migrated", and
    // that question lives in that field.
    const same = await evaluate(`
      const pick = document.querySelector(".avatar-pick");
      const field = pick?.closest(".field") ?? document.body;
      return {
        pick: !!pick,
        old: [...field.querySelectorAll("button")]
          .filter(b => /Elige|Choose|Quitar|Remove/.test(b.textContent))
          .filter(b => !b.closest(".avatar-pick")).length,
      };
    `);
    check(`${name} picks an image the same way the wizard does`, same.pick === true && same.old === 0,
      JSON.stringify(same));
  }

  // The panel uses the window rather than a column on the left of it.
  await evaluate(`history.pushState({}, "", "/panel"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(900);
  const room = await evaluate(`
    const main = document.querySelector(".admin-main");
    const cards = [...document.querySelectorAll(".dash-card")].map(c => c.getBoundingClientRect());
    const box = main.getBoundingClientRect();
    return { width: Math.round(box.width),
             used: cards.length ? Math.round(Math.max(...cards.map(c => c.right)) - box.left) : 0 };
  `);
  check("the dashboard fills the width it is given", room.used > room.width * 0.9,
    `${room.used} of ${room.width}`);

  const wayOut = await evaluate(`return !!document.querySelector(".side-public");`);
  check("and there is a way to the public side from the panel", wayOut === true);

  // The bars on the overview, measured rather than looked at.
  //
  // They were spans with a width in a style attribute and no display, so every
  // fill on every card was zero pixels wide: the panel drew a row of empty grey
  // tracks and looked like a chart of nothing. On a screenshot an empty track
  // and a full one at low contrast are the same picture, which is why this
  // measures the box.
  const bars = await evaluate(`
    const fills = [...document.querySelectorAll(".bar-fill")];
    return { count: fills.length, drawn: fills.filter(f => f.getBoundingClientRect().width > 1).length };
  `);
  check(
    "the bars on the overview are actually drawn",
    bars.count > 0 && bars.drawn === bars.count,
    `${bars.drawn} of ${bars.count} have width`,
  );

  // The front door. It is the page a stranger arrives on, and what it announces
  // is the installation — its name and its mark — rather than the name of the
  // software running it, which is on the About screen where an operator looks
  // for it.
  await evaluate(`history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1100);
  const front = await evaluate(`
    const mark = document.querySelector(".hero-mark img");
    return {
      title: document.querySelector(".hero h1")?.textContent?.trim() ?? "",
      mark: mark ? Math.round(mark.getBoundingClientRect().width) : 0,
      points: document.querySelectorAll(".points > li").length,
      boards: document.querySelectorAll(".cards .card").length,
    };
  `);
  check(
    "the front page announces the installation, not the software",
    front.title === "CodigoSH" && front.mark > 30 && front.points === 3 && front.boards === 1,
    JSON.stringify(front),
  );

  // Back to the panel: what follows reads the sidebar, and a check that leaves
  // the walk on another screen breaks the next one rather than its own.
  await evaluate(`history.pushState({}, "", "/panel"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(900);

  const sidebar = await evaluate(`
    return {
      items: [...document.querySelectorAll(".side-item")].map(a => a.textContent.trim()),
      current: document.querySelector('.side-item[aria-current="page"]')?.textContent?.trim(),
      who: document.querySelector(".who")?.textContent?.trim(),
      publicNavHidden: getComputedStyle(document.querySelector("body > nav")).display === "none",
    };`);
  check(
    "the administration shows its options down one side",
    sidebar.items.length >= 3 && Boolean(sidebar.current) && Boolean(sidebar.who),
    `${sidebar.items.join(" · ")} — on "${sidebar.current}" as ${sidebar.who}`,
  );
  check(
    "and does not stack two navigations on top of each other",
    sidebar.publicNavHidden === true,
  );

  // The bar on the chosen row is a join between it and the panel it opened, so
  // it belongs on the edge where the two meet — and it went on the outer one
  // first, which nothing here noticed because a bar is a bar in a screenshot.
  //
  // Read as geometry and by the logical side, never as left or right: the walk
  // runs in Arabic further down, where the whole thing is mirrored and an
  // arithmetic answer in physical pixels is the opposite of the truth.
  //
  // And in a window this wide, because the override was cleared a few hundred
  // lines ago and what is left is whatever size the browser opened at. Below
  // 52rem the sidebar is a strip across the top and the bar is on its bottom
  // edge, which is correct there and would have passed this by measuring the
  // one layout where the mistake cannot appear.
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: 1280, height: 900, deviceScaleFactor: 1, mobile: false,
  });
  await sleep(300);
  const chosenRow = await evaluate(`
    const side = document.querySelector(".side");
    const item = document.querySelector('.side-item[aria-current="page"]');
    const other = document.querySelector('.side-item:not([aria-current="page"])');
    const style = getComputedStyle(item);
    const rtl = getComputedStyle(document.documentElement).direction === "rtl";
    const inner = (el) => rtl ? el.getBoundingClientRect().left : el.getBoundingClientRect().right;
    return {
      bar: parseFloat(style.borderInlineEndWidth),
      outer: parseFloat(style.borderInlineStartWidth),
      colour: style.borderInlineEndColor,
      accent: getComputedStyle(document.documentElement).getPropertyValue("--accent").trim(),
      toEdge: Math.abs(inner(side) - inner(item)),
      otherToEdge: Math.abs(inner(side) - inner(other)),
    };`);
  check(
    "the chosen row is barred on the side the content is on",
    chosenRow.bar === 2 && chosenRow.outer === 0 && chosenRow.toEdge <= 2 && chosenRow.otherToEdge > 8,
    JSON.stringify(chosenRow),
  );
  await page.send("Emulation.clearDeviceMetricsOverride");
  await sleep(200);

  // Measured as geometry, not as an attribute. `hidden` applies display:none at
  // the lowest specificity there is, so any class rule with a display of its own
  // beats it — and the element sits in the layout wearing a correct `hidden`.
  // Reading the attribute would say everything is fine.
  // The forge fields live in the organisation wizard now, not on the panel.
  await evaluate(`history.pushState({}, "", "/panel/new"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(600);
  const reallyHidden = await evaluate(`
    const fields = document.querySelector(".forge-fields");
    if (!fields) return "there are no forge fields";
    const select = document.querySelector("#org-forge");
    select.value = "none";
    select.dispatchEvent(new Event("change"));
    const box = fields.getBoundingClientRect();
    return { attribute: fields.hidden, takesSpace: box.height > 0 };
  `);
  check(
    "hidden fields take up no space",
    reallyHidden.attribute === true && reallyHidden.takesSpace === false,
    JSON.stringify(reallyHidden),
  );
  check(
    "a build that cannot verify says so instead of offering an update",
    updates.offersInstall === false && updates.text !== "",
    updates.text.slice(0, 50),
  );

  // 12. Everything the panel can do, it can do from the screen.
  //
  //     Four actions were API-only for a while: the forge token, the
  //     notification channel, replying as the team and merging a duplicate.
  //     Each one was a screen naming something it could not do — the same
  //     failure as a panel that says nothing is configured and offers nothing
  //     to configure, which is how an operator ends up in a terminal.
  await evaluate(`history.pushState({}, "", "/panel/codigosh"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(900);
  const orgPanel = await evaluate(`
    return {
      addBoard: !!document.querySelector("[data-add-repo]"),
      channel: !!document.querySelector("form.channel"),
      // No forge was chosen for this organisation, so the credentials form
      // should not be offered at all.
      credentials: !!document.querySelector("form.credentials"),
    };
  `);
  check("the organisation panel can add a board", orgPanel.addBoard === true);
  check("and can choose where the team is told", orgPanel.channel === true);
  check(
    "and does not ask for a token when there is no forge",
    orgPanel.credentials === false,
    JSON.stringify(orgPanel),
  );

  // And the positive case, which is the one that matters: an organisation with
  // a forge has to be connectable from here. Checking only the negative would
  // pass with the form never rendering at all.
  const connected = await evaluate(`
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/organizations", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: JSON.stringify({ slug: "withforge", name: "With a forge", forge: "forgejo",
                             base_url: "https://git.example.com", forge_owner: "CodigoSH" }),
    });
    history.pushState({}, "", "/panel/withforge");
    dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 900));

    const form = document.querySelector("form.credentials");
    if (!form) return { form: false };

    form.querySelector("[name=token]").value = "a-token";
    form.querySelector("[name=webhook_secret]").value = "a long random secret";
    form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 900));

    const path = document.querySelector("[data-webhook]");
    return {
      form: true,
      shown: path ? !path.hidden : false,
      text: (path?.textContent ?? "").trim(),
      tokenCleared: form.querySelector("[name=token]").value === "",
    };
  `);
  check("an organisation with a forge asks for its token", connected.form === true);
  check(
    "and shows where the forge should deliver",
    connected.shown === true && String(connected.text).includes("/hooks/"),
    String(connected.text).slice(0, 56),
  );
  check("and does not leave the token in the field", connected.tokenCleared === true);

  // Coming back later still shows the address: it is configured once on the
  // forge, and somebody who lost it has no other way to find it.
  const revisit = await evaluate(`
    history.pushState({}, "", "/panel"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 600));
    history.pushState({}, "", "/panel/withforge"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 900));
    const path = document.querySelector("[data-webhook]");
    return path && !path.hidden ? (path.textContent ?? "").trim() : "";
  `);
  check("and it is still there on the next visit", String(revisit).includes("/hooks/"), String(revisit).slice(0, 56));

  await evaluate(`history.pushState({}, "", ${JSON.stringify(String(href))}); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(800);
  const triage = await evaluate(`
    return {
      reply: !!document.querySelector("form.team-reply"),
      merge: !!document.querySelector("form.merge"),
    };
  `);
  check("the team can reply and merge from the item", triage.reply === true && triage.merge === true,
    JSON.stringify(triage));

  // And rewrite it. A title that says "its broken" is the normal case, and
  // until now the only way to fix one was to ask the person who wrote it.
  // Checked on the page rather than through the API: the form has to be
  // reachable, prefilled with what is actually there, and the heading has to
  // end up saying the new thing.
  const rewritable = await evaluate(`
    const form = document.querySelector("form.rewrite");
    return {
      form: !!form,
      prefilled: form?.querySelector("input[name=title]")?.value ?? "",
      heading: document.querySelector(".detail h1")?.textContent?.trim() ?? "",
    };
  `);
  check(
    "and is offered the report's own words, as they are",
    rewritable.form === true && rewritable.prefilled !== "" && rewritable.prefilled === rewritable.heading,
    JSON.stringify(rewritable),
  );

  await evaluate(`
    const form = document.querySelector("form.rewrite");
    form.querySelector("input[name=title]").value = "Importing a folder crashes on the second run";
    form.requestSubmit();
  `);
  await sleep(1600);
  const rewritten = await evaluate(`
    return { heading: document.querySelector(".detail h1")?.textContent?.trim() ?? "" };
  `);
  check(
    "and rewriting it changes what the page says",
    rewritten.heading === "Importing a folder crashes on the second run",
    JSON.stringify(rewritten),
  );

  // The front page, composed. Two halves: the default is the page everybody
  // already had — proved elsewhere by comparing the pixels — and composing it
  // actually changes what a visitor sees. Without the second half this is a
  // setting that saves and does nothing.
  // The front page, arranged on the front page. Not by calling the API: the
  // whole point of this is that somebody does it with the page in front of
  // them, and an editor tested through fetch is an editor nobody has used.
  await evaluate(`history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1400);
  const composing = await evaluate(`
    document.querySelector("[data-edit-page]").click();
    await new Promise(r => setTimeout(r, 600));

    const outlined = getComputedStyle(document.querySelector(".landing-cell")).borderStyle;
    const kebabs = document.querySelectorAll(".kebab-btn").length;

    // Add words, and the module opens its own dialog the moment it exists.
    document.querySelector("[data-add-kind]").value = "text";
    document.querySelector("[data-add-module]").click();
    await new Promise(r => setTimeout(r, 900));
    const opened = document.querySelector("#module-config")?.open === true;

    const form = document.querySelector("form.module-config");
    form.querySelector("[name=body]").value = "We read everything that arrives.";
    form.requestSubmit();
    await new Promise(r => setTimeout(r, 900));

    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 600));
    return { outlined, kebabs, opened };
  `);
  check(
    "the front page is arranged on the front page",
    composing.outlined === "dashed" && composing.kebabs >= 3 && composing.opened === true,
    JSON.stringify(composing),
  );

  await evaluate(`
    document.querySelector("[data-edit-page]").click();
    await new Promise(r => setTimeout(r, 600));
  `);
  // The ghost, which is the whole of the feedback while something is being
  // carried: green where it would land, red where it would sit on top of
  // another module. Driven with real drag events rather than by calling the
  // handler, because what is being checked is that the page answers a gesture.
  const carried = await evaluate(`
    const grid = document.querySelector(".landing-grid.editing");
    const first = grid.querySelector(".landing-cell");
    const box = first.getBoundingClientRect();
    const gridBox = grid.getBoundingClientRect();

    const dt = new DataTransfer();
    first.dispatchEvent(new DragEvent("dragstart", { bubbles: true, dataTransfer: dt,
      clientX: box.left + 10, clientY: box.top + 10 }));

    // Somewhere empty, well below everything that is placed.
    grid.dispatchEvent(new DragEvent("dragover", { bubbles: true, cancelable: true, dataTransfer: dt,
      clientX: gridBox.left + 10, clientY: gridBox.top + 1400 }));
    await new Promise(r => setTimeout(r, 120));
    const ghost = document.querySelector(".landing-ghost");
    const ghostBox = ghost.getBoundingClientRect();
    const free = { shown: !ghost.hidden, invalid: ghost.classList.contains("invalid"),
                   border: getComputedStyle(ghost).borderTopColor,
                   // The ghost stands for the module being carried, so it has to
                   // be that module's size. Reading only its colour let it pass
                   // while it drew as a four-pixel line: an empty grid item with
                   // align-items:start above it sizes to the nothing it holds.
                   alto: Math.round(ghostBox.height), ancho: Math.round(ghostBox.width) };
    const llevado = { alto: Math.round(box.height), ancho: Math.round(box.width) };

    // And on top of another module, which is a no.
    const other = grid.querySelectorAll(".landing-cell")[1];
    const otherBox = other.getBoundingClientRect();
    grid.dispatchEvent(new DragEvent("dragover", { bubbles: true, cancelable: true, dataTransfer: dt,
      clientX: otherBox.left + 12, clientY: otherBox.top + 12 }));
    await new Promise(r => setTimeout(r, 120));
    const taken = { invalid: ghost.classList.contains("invalid"),
                    border: getComputedStyle(ghost).borderTopColor };

    first.dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer: dt }));
    return { free, llevado, taken, hiddenAfter: ghost.hidden };
  `);
  check(
    "what is being carried shows where it would land, and where it would not",
    carried.free.shown === true && carried.free.invalid === false &&
      carried.free.border === "rgb(0, 255, 0)" &&
      carried.free.alto === carried.llevado.alto &&
      carried.free.ancho === carried.llevado.ancho &&
      carried.taken.invalid === true && carried.taken.border === "rgb(255, 0, 0)" &&
      carried.hiddenAfter === true,
    JSON.stringify(carried),
  );

  // And where it actually lands, down the page. Colour alone never saw this:
  // the vertical arithmetic divided by the column width, so a module dropped
  // one row lower landed several rows lower. Rows are 100px, columns are about
  // 45, and the two axes needed their own divisor.
  const dropped = await evaluate(`
    const grid = document.querySelector(".landing-grid.editing");
    const before = await fetch("/api/landing").then(r => r.json());
    const was = before.modules.slice().sort((a, b) => (b.y + b.h) - (a.y + a.h))[0];
    const cellEl = grid.querySelector('[data-module="' + was.id + '"]');
    const box = cellEl.getBoundingClientRect();
    const dt = new DataTransfer();

    cellEl.dispatchEvent(new DragEvent("dragstart", { bubbles: true, dataTransfer: dt,
      clientX: box.left + 10, clientY: box.top + 10 }));
    // Exactly one row further down: one row plus one gap. A row is as tall as a
    // column is wide — the cells are square, which is what lets the drag use one
    // divisor for both axes — so the step is measured, never written down.
    const gridBox = grid.getBoundingClientRect();
    const fila = (gridBox.width - 11 * 16) / 12;
    // One row down and a third of a row into it. Landing on the exact boundary
    // put the pointer at 8.0000 cells and let floating point pick the row: the
    // same gesture read as row eight or row nine depending on the numbers. A
    // hand never aims at the seam, and neither should this.
    const lower = { clientX: box.left + 10, clientY: box.top + 10 + fila + 16 + fila / 3 };
    grid.dispatchEvent(new DragEvent("dragover", { bubbles: true, cancelable: true, dataTransfer: dt, ...lower }));
    await new Promise(r => setTimeout(r, 60));
    const fantasma = getComputedStyle(document.querySelector(".landing-ghost")).gridRow;
    const arriba = Math.round(box.top - grid.getBoundingClientRect().top);
    grid.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer: dt, ...lower }));
    await new Promise(r => setTimeout(r, 500));
    cellEl.dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer: dt }));

    const after = await fetch("/api/landing").then(r => r.json());
    const now = after.modules.find(m => m.id === was.id);
    return { id: was.id, antes: was.y, esperado: was.y + 1, ahora: now.y,
             fantasma, ocupado: after.modules.map(m => m.id + "@" + m.y + "+" + m.h).join(" ") };
  `);
  check(
    "a module dropped one row lower is one row lower",
    dropped.ahora === dropped.esperado,
    JSON.stringify(dropped),
  );

  // Dragging the bottom-right corner, the way Lastboard does it. This one goes
  // all the way to the stored layout: a corner that only moves a ghost and
  // forgets on reload would pass any check that stopped at the screen.
  const corner = await evaluate(`
    const grid = document.querySelector(".landing-grid.editing");
    const before = await fetch("/api/landing").then(r => r.json());
    // The bottom-most module, because growing downwards into an occupied cell is
    // refused — correctly. Picking the top one measured the refusal, not the
    // corner, and the layout came back unchanged for the right reason.
    const was = before.modules.slice().sort((a, b) => (b.y + b.h) - (a.y + a.h))[0];
    const id = was.id;
    const cellEl = grid.querySelector('[data-module="' + id + '"]');

    const handle = cellEl.querySelector("[data-resize]");
    const box = cellEl.getBoundingClientRect();
    const gridBox = grid.getBoundingClientRect();
    const colWidth = (gridBox.width - 11 * 16) / 12;
    const fila = colWidth;
    const opts = { bubbles: true, cancelable: true, pointerId: 1, pointerType: "mouse" };

    handle.dispatchEvent(new PointerEvent("pointerdown", { ...opts,
      clientX: box.right, clientY: box.bottom }));
    // Exactly one cell shorter and one row taller: a module that spans the whole
    // grid cannot grow sideways, so shrinking is the move that proves the axis.
    window.dispatchEvent(new PointerEvent("pointermove", { ...opts,
      clientX: box.right - (colWidth + 16), clientY: box.bottom + (fila + 16) }));
    await new Promise(r => setTimeout(r, 120));
    const ghost = document.querySelector(".landing-ghost");
    const ghostBox = ghost.getBoundingClientRect();
    const mientras = { alto: Math.round(ghostBox.height), visible: !ghost.hidden };
    window.dispatchEvent(new PointerEvent("pointerup", { ...opts,
      clientX: box.right - (colWidth + 16), clientY: box.bottom + (fila + 16) }));
    await new Promise(r => setTimeout(r, 400));

    const after = await fetch("/api/landing").then(r => r.json());
    const now = after.modules.find(m => m.id === id);
    return { id, antes: { w: was.w, h: was.h }, ahora: { w: now.w, h: now.h },
             mientras, altoEsperado: Math.round((was.h + 1) * fila + was.h * 16) };
  `);
  check(
    "a module is resized from its bottom-right corner, and stays resized",
    corner.ahora.w === corner.antes.w - 1 && corner.ahora.h === corner.antes.h + 1 &&
      corner.mientras.visible === true && corner.mientras.alto === corner.altoEsperado,
    JSON.stringify(corner),
  );

  // The two modules that draw out of the database. Added the way an operator
  // adds them — the picker and the button, not the API — because what is being
  // checked is that somebody can compose this page, not that the server can
  // store it.
  const conDatos = await evaluate(`
    // What the dialog offered, captured while it is open. Read afterwards it is
    // always closed and always hidden: submitting redraws the page, and the
    // dialog comes back in its initial state.
    let opciones = [];
    let ofrecido = false;
    const add = async (kind) => {
      document.querySelector("[data-add-kind]").value = kind;
      document.querySelector("[data-add-module]").click();
      await new Promise(r => setTimeout(r, 1100));
      opciones = [...document.querySelectorAll("#module-scope option")].map(o => o.value);
      ofrecido = !document.querySelector("[data-for-scope]").hidden;
      // Each one opens its own dialog. Closed without changing anything: the
      // default is every board, which is what this checks first.
      document.querySelector("#module-config")?.close();
      await new Promise(r => setTimeout(r, 300));
    };
    await add("boards");
    await add("most_voted");
    await add("recently_done");

    // The button, with a label typed into its dialog rather than left default:
    // what is being checked is that the words reach the button, not that a
    // fallback exists.
    document.querySelector("[data-add-kind]").value = "call_to_action";
    document.querySelector("[data-add-module]").click();
    await new Promise(r => setTimeout(r, 1100));
    const etiquetaOfrecida = !document.querySelector("[data-for-label]").hidden;
    const form = document.querySelector("form.module-config");
    form.querySelector("[name=label]").value = "Tell us what broke";
    form.requestSubmit();
    await new Promise(r => setTimeout(r, 1100));

    document.querySelector("[data-edit-done]").click();
    await new Promise(r => setTimeout(r, 800));

    const tableros = [...document.querySelectorAll(".landing-cell .cards .card")]
      .map(a => a.getAttribute("href"));
    const votadas = [...document.querySelectorAll(".landing-cell .briefs .brief")]
      .map(a => ({ titulo: a.querySelector(".brief-title")?.textContent?.trim() ?? "",
                   href: a.getAttribute("href"),
                   votos: a.querySelector(".count")?.textContent?.trim() ?? "" }));
    const boton = document.querySelector(".landing-action a");
    const hechas = document.querySelector('.landing-cell[data-module^="recently_done-"]');
    return { opciones, ofrecido, tableros, votadas, etiquetaOfrecida,
             boton: boton ? { href: boton.getAttribute("href"),
                              palabras: boton.textContent.trim() } : null,
             sinHechas: !!hechas?.querySelector(".empty"),
             hechasRotas: hechas ? hechas.getBoundingClientRect().height < 10 : true };
  `);
  check(
    "the front page can list the boards themselves, without the organisation in the way",
    conDatos.tableros.includes("/codigosh/lastboard"),
    JSON.stringify(conDatos.tableros),
  );
  // That the list holds ideas and only ideas is proved in Go, not here: this
  // journey has one published idea and no published bug at this point, so the
  // module shows the same single row whether it filters by kind or not. Trying
  // it here meant seeding a bug, and that moved the row counts three later
  // checks depend on.
  check(
    "and the ideas people asked for most, with their count",
    conDatos.votadas.length === 1 && conDatos.votadas[0].votos === "1" &&
      conDatos.votadas[0].href.startsWith("/i/"),
    JSON.stringify(conDatos.votadas),
  );
  check(
    "a data module is asked which board, and offered all of them first",
    conDatos.ofrecido === true && conDatos.opciones[0] === "" &&
      conDatos.opciones.includes("codigosh/lastboard"),
    JSON.stringify(conDatos.opciones),
  );

  check(
    "the page can carry the button that is the point of it, in the operator's words",
    conDatos.etiquetaOfrecida === true && conDatos.boton !== null &&
      conDatos.boton.href === "/codigosh/lastboard/new" &&
      conDatos.boton.palabras.includes("Tell us what broke"),
    JSON.stringify(conDatos.boton),
  );
  // Nothing has been finished at this point in the journey, which is the state
  // every installation starts in and the one least likely to be looked at. The
  // module has to say so and still occupy its box: a module that collapses to
  // nothing reads as a page that failed to load.
  check(
    "a module with nothing to show yet says so, and still holds its place",
    conDatos.sinHechas === true && conDatos.hechasRotas === false,
    JSON.stringify({ vacio: conDatos.sinHechas, roto: conDatos.hechasRotas }),
  );

  // How a module names itself, which is a question about two products rather
  // than about this page. The pill mounted on a border means "a container you
  // drag items into" in the dashboard this grid comes from, so a module wearing
  // one reads as a container with its contents missing — and reads that way
  // hardest to the people most likely to recognise it. A module is that
  // dashboard's widget, so it wears a widget's header: inside the box, at the
  // top, and still there after the module is scrolled.
  const nombre = await evaluate(`
    const cell = document.querySelector('.landing-cell[data-module^="boards-"]');
    const head = cell.querySelector(".module-header");
    const cb = cell.getBoundingClientRect();
    // Tolerant on purpose: a mutation that brings the container pill back
    // leaves no header at all, and a probe that assumes one throws and takes
    // the whole pass down instead of naming what changed.
    if (!head) return { pildora: !!cell.querySelector(".section-legend"), cabecera: false };
    const antes = Math.round(head.getBoundingClientRect().top - cb.top);
    cell.querySelector(".landing-scroll")?.scrollTo(0, 400);
    await new Promise(r => setTimeout(r, 120));
    return {
      cabecera: true,
      pildora: !!cell.querySelector(".section-legend"),
      dentro: antes >= 0,
      texto: head.querySelector("h2")?.textContent?.trim() ?? "",
      icono: !!head.querySelector(".module-icon svg"),
      trasDesplazar: Math.round(head.getBoundingClientRect().top - cb.top),
    };
  `);
  check(
    "a module names itself the way a widget does, not the way a container does",
    nombre.cabecera === true && nombre.pildora === false &&
      nombre.dentro === true && nombre.icono === true &&
      nombre.texto !== "" && nombre.trasDesplazar >= 0 && nombre.trasDesplazar < 8,
    JSON.stringify(nombre),
  );

  // The four places a name can go, driven through the dialog. The pill is the
  // one that matters most: it is the mark the operator may CHOOSE, and the
  // whole point is that the product never hands it out by itself.
  const donde = await evaluate(`
    // Back into edit mode: the block above left it, and the kebab that opens a
    // module's dialog only exists while the page is being arranged.
    document.querySelector("[data-edit-page]").click();
    await new Promise(r => setTimeout(r, 600));

    const leer = () => {
      const cell = document.querySelector('.landing-cell[data-module^="boards-"]');
      const head = cell.querySelector(".module-header");
      const cb = cell.getBoundingClientRect();
      return {
        cabecera: !!head,
        pildora: !!cell.querySelector(".section-legend"),
        alPie: cell.querySelector(".landing-scroll")?.classList.contains("label-bottom") ?? false,
        // Where the header actually sits inside the module, not which class it
        // carries: a class says what was asked for, a box says what happened.
        mitad: head ? (head.getBoundingClientRect().top - cb.top) > cb.height / 2 : null,
      };
    };

    const poner = async (valor) => {
      document.querySelector('.landing-cell[data-module^="boards-"] [data-module-menu]').click();
      await new Promise(r => setTimeout(r, 200));
      document.querySelector('.landing-cell[data-module^="boards-"] [data-edit-module]').click();
      await new Promise(r => setTimeout(r, 400));
      const form = document.querySelector("form.module-config");
      const ofrecido = !document.querySelector("[data-for-label-at]").hidden;
      form.querySelector("[name=labelAt]").value = valor;
      form.requestSubmit();
      await new Promise(r => setTimeout(r, 900));
      return { ofrecido, ...leer() };
    };

    return { arriba: leer(), abajo: await poner("bottom"),
             enPildora: await poner("pill"), sinNombre: await poner("off") };
  `);
  check(
    "a module's name goes where the operator puts it, in four places",
    donde.arriba.cabecera === true && donde.arriba.pildora === false && donde.arriba.mitad === false &&
      donde.abajo.ofrecido === true && donde.abajo.alPie === true && donde.abajo.mitad === true &&
      donde.enPildora.pildora === true && donde.enPildora.cabecera === false &&
      donde.sinNombre.pildora === false && donde.sinNombre.cabecera === false,
    JSON.stringify(donde),
  );


  // Sections: a box that holds modules, ported from the dashboard. Four things
  // have to be true for one to be worth having — something can be dropped in,
  // what is in it is placed in ITS grid and not the page's, it can be taken
  // back out, and shrinking one puts out what no longer fits.
  //
  // The page it runs on is written through the API first. That is setup, not
  // the thing being checked: the default page is all full-width modules, so
  // there is nowhere on it to drop anything, and a drag that has nowhere to
  // land proves nothing about dragging.
  const seccion = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const capa = () => fetch("/api/landing").then(r => r.json());
    const escribir = (modules) => fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules }) }).then(r => r.status);

    // The page as the checks after this one expect to find it. A journey is one
    // story with accumulated state: writing a layout of my own here and leaving
    // it took three later checks down with it.
    window.__paginaAntes = (await capa()).modules;

    const puesto = await escribir([
      { id: "hero", kind: "hero", x: 1, y: 1, w: 12, h: 3 },
      { id: "group", kind: "section", x: 1, y: 4, w: 6, h: 3, body: "My projects" },
      { id: "loose", kind: "boards", x: 7, y: 4, w: 4, h: 2 },
    ]);

    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    const conPildora = !!document.querySelector(
      '.landing-cell[data-module="group"] .section-legend');

    // The point is worked out by the caller, in page coordinates. Aiming at a
    // fraction of some element kept landing on top of another module, and a
    // drop that collides is correctly refused — so the check was measuring the
    // collision rule, not the drag.
    const arrastrar = async (id, punto) => {
      const celda = document.querySelector('[data-module="' + id + '"]');
      const caja = celda.getBoundingClientRect();
      const dt = new DataTransfer();
      celda.dispatchEvent(new DragEvent("dragstart", { bubbles: true, dataTransfer: dt,
        clientX: caja.left + 8, clientY: caja.top + 8 }));
      const grid = document.querySelector(".landing-grid.editing");
      grid.dispatchEvent(new DragEvent("dragover", { bubbles: true, cancelable: true, dataTransfer: dt, ...punto }));
      await new Promise(r => setTimeout(r, 140));
      const senales = {
        resaltado: !!document.querySelector(".landing-group.drop-target"),
        fantasmaDentro: !!document.querySelector(".landing-section .landing-ghost"),
        valido: !document.querySelector(".landing-ghost")?.classList.contains("invalid"),
        celda: (() => {
          const g = document.querySelector(".landing-ghost");
          const cs = getComputedStyle(g);
          return cs.gridColumn + " / " + cs.gridRow;
        })(),
      };
      grid.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer: dt, ...punto }));
      await new Promise(r => setTimeout(r, 900));
      celda.dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer: dt }));
      return senales;
    };

    // In, aimed at the section's own first cell.
    const dentroDe = document.querySelector('.landing-section[data-section="group"]').getBoundingClientRect();
    const senales = await arrastrar("loose",
      { clientX: dentroDe.left + 10, clientY: dentroDe.top + 10 });
    const hijo = (await capa()).modules.find(m => m.id === "loose");
    const dibujadoDentro = !!document.querySelector(
      '.landing-section[data-section="group"] [data-module="loose"]');

    // Shrink the section under it, through the API rather than the corner: the
    // corner is checked elsewhere, and what is being measured here is what the
    // page does with a child that no longer fits.
    await escribir([
      { id: "hero", kind: "hero", x: 1, y: 1, w: 12, h: 3 },
      { id: "group", kind: "section", x: 1, y: 4, w: 6, h: 3, body: "My projects" },
      { id: "loose", kind: "boards", x: 1, y: 1, w: 4, h: 2, parent: "group" },
    ]);
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    // Out again, into the empty columns beside the section — the only free
    // space on this page, and the reason the page was written this way.
    const caja = document.querySelector('.landing-cell[data-module="group"]').getBoundingClientRect();
    const salida = await arrastrar("loose", { clientX: caja.right + 30, clientY: caja.top + 10 });
    const fuera = (await capa()).modules.find(m => m.id === "loose");

    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 700));
    return { puesto, conPildora, ...senales, dibujadoDentro,
             dentro: { parent: hijo.parent ?? "", x: hijo.x, y: hijo.y },
             fuera: { parent: fuera.parent ?? "", x: fuera.x, y: fuera.y, ...salida } };
  `);
  check(
    "a module dropped into a section is placed in the section's own grid",
    seccion.puesto === 200 && seccion.dentro.parent === "group" &&
      seccion.dentro.x === 1 && seccion.dentro.y === 1 && seccion.dibujadoDentro === true,
    JSON.stringify(seccion.dentro),
  );
  check(
    "and the page says where it is going before it is let go",
    seccion.resaltado === true && seccion.fantasmaDentro === true && seccion.valido === true,
    JSON.stringify({ resaltado: seccion.resaltado, fantasma: seccion.fantasmaDentro }),
  );
  check(
    "a section wears its name on its border, or wears none",
    seccion.conPildora === true,
    JSON.stringify({ pildora: seccion.conPildora }),
  );
  check(
    "and a module can be taken back out of a section",
    seccion.fuera.parent === "",
    JSON.stringify(seccion.fuera),
  );

  // One of three blocks that write their own page: the section checks capture
  // what was here, this one and the next write over it, and the last one puts
  // it back. They are a sequence, not three independent checks — moving one out
  // of the middle leaves the page rewritten for everything after it, which is
  // how three later checks went down the first time.
  //
  // A module pointed at one thing IS that thing, rather than a list of one row
  // inside a box with a heading over it. This is what makes eight boards in a
  // section read as eight boards, and it is the reason sections were worth
  // building at all.
  const singular = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: [
        { id: "group", kind: "section", x: 1, y: 1, w: 8, h: 2, body: "My projects" },
        { id: "one", kind: "boards", x: 1, y: 1, w: 4, h: 2, scope: "codigosh/lastboard", parent: "group" },
        { id: "many", kind: "boards", x: 1, y: 3, w: 6, h: 3 },
        { id: "org", kind: "organisations", x: 7, y: 3, w: 6, h: 2, scope: "codigosh" },
      ] }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1300));

    const uno = document.querySelector('[data-module="one"]');
    const muchos = document.querySelector('[data-module="many"]');
    const unaOrg = document.querySelector('[data-module="org"]');
    return {
      // One board: the card is the module, with no heading and no list around it.
      unoEsTarjeta: !!uno.querySelector(".landing-one"),
      unoSinCabecera: !uno.querySelector(".module-header"),
      unoEnlaza: uno.querySelector(".landing-one")?.getAttribute("href") ?? "",
      unoDentroDeSeccion: !!document.querySelector(
        '.landing-section[data-section="group"] [data-module="one"]'),
      // All of them: still the list it was, which is what an installation that
      // never chose anything keeps showing.
      muchosEsLista: !!muchos.querySelector(".cards") && !!muchos.querySelector(".module-header"),
      orgEsTarjeta: !!unaOrg.querySelector(".landing-one"),
      orgEnlaza: unaOrg.querySelector(".landing-one")?.getAttribute("href") ?? "",
    };
  `);
  check(
    "a module pointed at one board is that board, not a list holding it",
    singular.unoEsTarjeta === true && singular.unoSinCabecera === true &&
      singular.unoEnlaza === "/codigosh/lastboard" && singular.unoDentroDeSeccion === true,
    JSON.stringify(singular),
  );
  check(
    "and one pointed at everything is still the listing it was",
    singular.muchosEsLista === true,
    JSON.stringify({ lista: singular.muchosEsLista }),
  );
  check(
    "the same for an organisation",
    singular.orgEsTarjeta === true && singular.orgEnlaza === "/codigosh",
    JSON.stringify({ tarjeta: singular.orgEsTarjeta, enlace: singular.orgEnlaza }),
  );

  // Shrinking a section under what is inside it. The dashboard this comes from
  // compares HOW MANY children there are against how many cells are left, so a
  // section with one child shrunk under that child ejects nothing and draws it
  // outside the box meant to contain it. Here the question is where each child
  // is, so this is the case that separates the two.
  const encoger = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: [
        { id: "hero", kind: "hero", x: 1, y: 1, w: 12, h: 3 },
        { id: "group", kind: "section", x: 1, y: 4, w: 6, h: 3, body: "My projects" },
        { id: "loose", kind: "boards", x: 3, y: 1, w: 4, h: 2, parent: "group" },
      ] }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    // One child, sitting at the section's third column and four wide, so it
    // ends at the sixth. Take three columns away and it no longer fits — and
    // counting children would say one child against nine cells and keep it.
    const grid = document.querySelector(".landing-grid.editing");
    const gridBox = grid.getBoundingClientRect();
    const col = (gridBox.width - 11 * 16) / 12;
    const celda = document.querySelector('.landing-cell[data-module="group"]');
    const caja = celda.getBoundingClientRect();
    // The section's OWN corner. Without :scope this finds the child's handle
    // first — it is a descendant too — and the drag resized the child from four
    // columns to one while the section stayed exactly as it was.
    const asa = celda.querySelector(":scope > [data-resize]");
    const opts = { bubbles: true, cancelable: true, pointerId: 1, pointerType: "mouse" };

    asa.dispatchEvent(new PointerEvent("pointerdown", { ...opts,
      clientX: caja.right, clientY: caja.bottom }));
    window.dispatchEvent(new PointerEvent("pointermove", { ...opts,
      clientX: caja.right - 3 * (col + 16), clientY: caja.bottom }));
    await new Promise(r => setTimeout(r, 140));
    window.dispatchEvent(new PointerEvent("pointerup", { ...opts,
      clientX: caja.right - 3 * (col + 16), clientY: caja.bottom }));
    await new Promise(r => setTimeout(r, 1100));

    const capa = await fetch("/api/landing").then(r => r.json());
    const seccion = capa.modules.find(m => m.id === "group");
    const hijo = capa.modules.find(m => m.id === "loose");
    const dentroDelDibujo = !!document.querySelector(
      '.landing-section[data-section="group"] [data-module="loose"]');

    // And the page goes back to what it was, for whatever runs after this.
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: window.__paginaAntes }) });
    document.querySelector("[data-edit-done]")?.click();
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));

    return { ancho: seccion.w, hijo: { parent: hijo.parent ?? "", x: hijo.x, y: hijo.y, w: hijo.w },
             dentroDelDibujo,
             devuelta: (await fetch("/api/landing").then(r => r.json())).modules.length };
  `);
  check(
    "shrinking a section puts out what no longer fits in it",
    encoger.ancho === 3 && encoger.hijo.parent === "" && encoger.dentroDelDibujo === false &&
      encoger.devuelta > 3,
    JSON.stringify(encoger),
  );

  // Pointed at a board that is not there any more — which is what an operator
  // is left with after deleting one they had composed a page around. The module
  // has to come back empty, not quietly show every board instead: showing
  // everything would be the page answering a question nobody asked.
  const huerfano = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const layout = await fetch("/api/landing").then(r => r.json());
    const modules = layout.modules.map(m =>
      m.kind === "boards" ? { ...m, scope: "codigosh/gone" } : m);
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));
    // Inside the boards module and nowhere else. Counting every .card on the
    // page counted the organisations module's two as well, so this passed and
    // failed for reasons that had nothing to do with the scope.
    const suyo = document.querySelector('.landing-cell[data-module^="boards-"]');
    return { tarjetas: suyo.querySelectorAll(".cards .card").length,
             vacio: !!suyo.querySelector(".empty") };
  `);
  check(
    "a module pointed at a board that is gone shows nothing, not everything",
    huerfano.tarjetas === 0 && huerfano.vacio === true,
    JSON.stringify(huerfano),
  );

  await evaluate(`
    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 500));
  `);


  const frontPage = await evaluate(`
    return {
      cells: document.querySelectorAll(".landing-cell").length,
      words: document.body.textContent.includes("We read everything that arrives."),
      hero: !!document.querySelector(".hero"),
      orgs: !!document.querySelector(".section"),
    };
  `);
  check(
    "and what was arranged is what a visitor is shown",
    frontPage.cells === 8 && frontPage.words === true && frontPage.hero === true && frontPage.orgs === true,
    JSON.stringify(frontPage),
  );


  // 13. Right to left, which is where a stylesheet written in left and right
  //     falls apart.
  // Changed where somebody actually changes it, now that the top bar carries no
  // picker: under Appearance, which is also the path that stores the choice on
  // the account rather than in this browser.
  await evaluate(`history.pushState({}, "", "/panel/appearance"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(900);
  await evaluate(`
    const select = document.querySelector("#appearance-language");
    select.value = "ar";
    select.dispatchEvent(new Event("change"));
  `);
  await sleep(1200);
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

  // 14. Nothing exploded along the way.
  // 11b. The overview, which is one screen and the same one for everybody.
  //
  //      It used to be arrangeable per account — drag, hide, and a layout
  //      saved per person — and that is gone on purpose: two people looking at
  //      the same installation were looking at two different screens, so
  //      nothing could be pointed at by where it is. What is checked is that
  //      every card is drawn, that there is nothing to arrange with, and that
  //      the endpoint that stored a layout is not there any more. A feature
  //      removed from a screen but left in the server is a feature that comes
  //      back the next time somebody wires a button.
  await evaluate(`history.pushState({}, "", "/panel"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1300);

  const overview = await evaluate(`
    return {
      cards: document.querySelectorAll("[data-card]").length,
      draggable: document.querySelectorAll("[draggable=true]").length,
      arrange: !!document.querySelector("[data-edit-dash], [data-hide], [data-show]"),
      stored: (await fetch("/api/me/dashboard")).status,
    };
  `);
  check(
    "the overview is one screen, the same for everybody",
    overview.cards === 13 && overview.draggable === 0 && overview.arrange === false,
    JSON.stringify(overview),
  );
  check(
    "and the layout it used to save is gone from the server too",
    overview.stored === 404,
    `GET /api/me/dashboard → ${overview.stored}`,
  );

  // 11d. The chart. A number says how many; only this says whether it is
  //      getting busier, and it is drawn by hand from the series the server
  //      sends — so what is checked is that there is a line with a point per
  //      day on it, not that a box is on screen.
  const chart = await evaluate(`
    const line = document.querySelector('[data-card="activity"] .chart-line');
    const d = line?.getAttribute("d") ?? "";
    return { points: (d.match(/[ML]/g) ?? []).length, comma: d.includes(","), axis: !!document.querySelector(".chart-axis") };
  `);
  check(
    "the overview draws thirty days rather than one more number",
    chart.points === 30 && chart.comma === false && chart.axis === true,
    JSON.stringify(chart),
  );

  // And it answers which day was which, under a real pointer. A dispatched
  // MouseEvent does not drive hover — this house has been caught by that — so
  // the pointer is moved with the browser's own input, onto the last point and
  // then off the chart entirely.
  const where = await evaluate(`
    // Into view first: the pointer is dispatched in the window's own
    // coordinates, and this browser opens at 600px tall — the chart is below
    // the fold, so the first version of this check moved the pointer past the
    // bottom of the window and measured a page nothing had touched.
    document.querySelector(".chart-wrap").scrollIntoView({ block: "center" });
    await new Promise(r => setTimeout(r, 300));
    const box = document.querySelector(".chart-wrap svg").getBoundingClientRect();
    return { x: Math.round(box.left + box.width * 0.98), y: Math.round(box.top + box.height * 0.6),
             away: Math.round(box.top - 40) };
  `);
  await page.send("Input.dispatchMouseEvent", {
    type: "mouseMoved", x: where.x, y: where.y, pointerType: "mouse",
  });
  await sleep(400);
  const asked = await evaluate(`
    const tip = document.querySelector(".chart-tip");
    const dot = document.querySelector(".chart-dot.on");
    const digits = (el) => Number((el?.textContent ?? "").replace(/[^0-9]/g, ""));
    return {
      // Compared against the point it landed on rather than against a fixed
      // string: by now the walk has changed language once, and a check that
      // expects English would be checking the language.
      day: dot ? Number(dot.dataset.day) : 0,
      count: dot ? Number(dot.dataset.count) : -1,
      dayLine: (tip?.querySelector(".tip-day")?.textContent ?? "").trim(),
      total: digits(tip?.querySelector(".tip-total .count")),
      // The breakdown has to add up to what the day was, or it is decoration.
      parts: [...(tip?.querySelectorAll(".tip-row .count") ?? [])].map(digits),
      shown: tip ? !tip.hidden : false,
      cursor: !document.querySelector(".chart-cursor").hasAttribute("hidden"),
    };
  `);
  await page.send("Input.dispatchMouseEvent", {
    type: "mouseMoved", x: where.x, y: where.away, pointerType: "mouse",
  });
  await sleep(300);
  const gone = await evaluate(`
    return { tip: document.querySelector(".chart-tip").hidden, lit: document.querySelectorAll(".chart-dot.on").length };
  `);
  const added = (asked.parts as number[]).reduce((sum: number, n: number) => sum + n, 0);
  check(
    "and says which day was which, and of what, under the pointer",
    asked.shown === true && asked.cursor === true && asked.day > 0 &&
      asked.dayLine.length > 0 && asked.total === asked.count && added === asked.count &&
      gone.tip === true && gone.lit === 0,
    `${JSON.stringify(asked)} → ${JSON.stringify(gone)}`,
  );

  // 11e. Every number leads to the list it counts. A dashboard whose tiles are
  //      not links is a dashboard that answers "how many" and then leaves you
  //      to find them yourself.
  // Out of arranging first: while the dashboard is being rearranged the tiles
  // are deliberately not links, because a card you meant to drag should not
  // navigate. Reading the href without leaving that mode measures the mode.
  // Read as a set rather than by name, so this says what it means — every tile
  // is a link, and more than one of them goes into the listing — rather than
  // pinning the order the cards happen to be in.
  const tiles = await evaluate(`
    const cards = [...document.querySelectorAll('.dash-card:has(.stat)')];
    return {
      stats: cards.length,
      links: cards.map(c => c.querySelector("a.stat")?.getAttribute("href") ?? ""),
    };
  `);
  check(
    "and every number on it leads to the list it counts",
    tiles.stats > 0 &&
      tiles.links.every((href: string) => href.length > 0) &&
      tiles.links.filter((href: string) => href.startsWith("/panel/reports")).length >= 2,
    JSON.stringify(tiles.links),
  );

  // 11f. The listing itself: everything this account may triage, wherever it
  //      lives. The panel had a queue per board and nothing else, so somebody
  //      looking after four repositories had no screen that answered "what is
  //      waiting for me".
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const form = await fetch("/api/orgs/codigosh/repos/lastboard/form").then(r => r.json());
    // The form token refuses anything faster than a person types.
    await new Promise(r => setTimeout(r, 2400));
    await fetch("/api/orgs/codigosh/repos/lastboard/items", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ kind: "bug", title: "search ignores accents",
        body: "searching for a word with an accent returns nothing",
        fields: { version: "1.4.2", steps: "1. search for a word with an accent" }, token: form.token }),
    });
  `);
  await evaluate(`history.pushState({}, "", "/panel/reports"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1400);

  const listing = await evaluate(`
    return {
      rows: document.querySelectorAll(".queue-row").length,
      waiting: document.querySelector('[data-moderation="pending"] .count-chip')?.textContent?.trim() ?? "",
      decisions: document.querySelectorAll(".queue-row .row-actions").length,
      boards: [...new Set([...document.querySelectorAll(".queue-row .chip")].map(c => c.textContent.trim()))],
    };
  `);
  check(
    "the panel lists every report it may triage, published ones included",
    listing.rows >= 2 && listing.waiting === "1",
    JSON.stringify(listing),
  );
  check(
    "and offers a decision only on what is actually waiting",
    listing.decisions === 1,
    `${listing.decisions} of ${listing.rows} rows carry the buttons`,
  );

  // The same number, in the menu. A count the server works out correctly and
  // the page renders into a zero-width element is the failure that reading the
  // DOM cannot see: the text is there and nothing is on screen. So this measures
  // the box, and that it sits at the far end rather than against the label.
  const badge = await evaluate(`
    const reports = [...document.querySelectorAll(".side-item")]
      .find(a => a.getAttribute("href") === "/panel/reports");
    const pill = reports?.querySelector(".side-count");
    const box = pill?.getBoundingClientRect();
    return {
      text: pill?.textContent?.trim() ?? "",
      label: pill?.getAttribute("aria-label") ?? "",
      width: Math.round(box?.width ?? 0),
      height: Math.round(box?.height ?? 0),
      // At the INLINE end of the item, which is the right one reading left to
      // right and the left one in Arabic. Asking for "on the right" would have
      // been asking the wrong question — this pass runs in Arabic, and there
      // the badge is correctly on the left.
      atEnd: (() => {
        if (!box || !reports) return false;
        const item = reports.getBoundingClientRect();
        return getComputedStyle(reports).direction === "rtl"
          ? box.left - item.left < 24
          : item.right - box.right < 24;
      })(),
      dir: reports ? getComputedStyle(reports).direction : "",
    };
  `);
  check(
    "what is waiting is a number beside Reports, with a box around it",
    badge.text === listing.waiting && badge.width > 8 && badge.height > 8 &&
      badge.atEnd === true && badge.label !== "",
    JSON.stringify(badge),
  );

  // A report nobody outside the team can see is not open, and its chip used to
  // say so: the status is what a published report is in, and until then the
  // state is what it is waiting for.
  const states = await evaluate(`
    return [...document.querySelectorAll(".queue-row")].map(r => ({
      decide: !!r.querySelector("[data-act]"),
      state: r.querySelector(".status")?.className ?? "",
    }));
  `);

  // The decision sits on the line of the state it would change — a queue of ten
  // was ten double-height rows — and it is a group beside that state rather
  // than four things heaped against the edge. Measured as a relationship
  // between the two distances, not as a number of pixels: the point is that
  // the gap outside the group is the larger one.
  //
  // In a window wide enough for one line, because at the 800px this browser
  // opens at the row deliberately stacks — and a stacked row answers this
  // question with the distances of a layout that is not the one under test.
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: 1280, height: 900, deviceScaleFactor: 1, mobile: false,
  });
  await sleep(400);
  const grouped = await evaluate(`
    const row = [...document.querySelectorAll(".queue-row")].find(r => r.querySelector(".row-actions"));
    const plain = [...document.querySelectorAll(".queue-row")].find(r => !r.querySelector(".row-actions"));
    const buttons = [...row.querySelectorAll(".row-actions .button")].map(b => b.getBoundingClientRect());
    const chip = row.querySelector(".status").getBoundingClientRect();
    // The distance between two boxes on a line, whichever way the line runs.
    // By this point the walk is in Arabic, so "the one on the right" is the
    // first of them — and a subtraction written for English answers with a
    // negative number that still satisfies a "bigger than" comparison. Which
    // is a check that passes on geometry it never measured.
    const between = (a, b) => Math.round(Math.max(b.left - a.right, a.left - b.right));
    return {
      dir: getComputedStyle(document.documentElement).direction,
      inside: between(buttons[0], buttons[1]),
      outside: between(buttons[buttons.length - 1], chip),
      taller: Math.round(row.getBoundingClientRect().height - plain.getBoundingClientRect().height),
    };
  `);
  check(
    "the decision is a group beside the state, and costs the row no height",
    grouped.outside > grouped.inside * 2 && grouped.taller === 0,
    JSON.stringify(grouped),
  );
  await page.send("Emulation.clearDeviceMetricsOverride");
  await sleep(400);
  check(
    "and nothing waiting for review is called open",
    states.length > 1 &&
      states.every((r: { decide: boolean; state: string }) =>
        r.decide ? r.state.includes("status-pending") : !r.state.includes("status-pending")),
    JSON.stringify(states),
  );

  await evaluate(`document.querySelector('[data-moderation="pending"]').click()`);
  await sleep(1200);
  const narrowed = await evaluate(`
    return { search: location.search, rows: document.querySelectorAll(".queue-row").length };
  `);
  check(
    "narrowing it goes into the address, so a view can be sent to somebody",
    narrowed.search.includes("moderation=pending") && narrowed.rows === 1,
    `${narrowed.search} → ${narrowed.rows} rows`,
  );

  // Reading it before deciding about it, which is the whole point of opening
  // one. The public page answered "not here" to everybody who was not its
  // author — the team included — so a waiting report could only be triaged by
  // trusting its title. And the decision is on the report now, not one screen
  // away from the reading.
  await evaluate(`document.querySelector(".queue-row a.item").click()`);
  await sleep(1400);
  const reading = await evaluate(`
    const decide = document.querySelector(".decide");
    return {
      title: document.querySelector(".detail h1")?.textContent?.trim() ?? "",
      // The decision is a card of its own, above the team's tools rather than
      // stacked in with the status select and the issue form: it is the one
      // thing on this screen the screen cannot undo.
      state: decide?.querySelector("h3")?.textContent?.trim() ?? "",
      apart: decide ? !decide.closest(".team-tools") : false,
      decide: document.querySelectorAll("[data-moderate]").length,
      // And said once: the notice above is for the reporter, who is told by it
      // what this card tells the team.
      notices: document.querySelectorAll(".notice").length,
      answers: document.querySelectorAll(".answers dd").length,
    };
  `);
  check(
    "a report waiting for review can be read, and decided where it is read",
    reading.title === "search ignores accents" && reading.state.length > 0 &&
      reading.apart === true && reading.decide === 3 && reading.notices === 0 &&
      reading.answers === 2,
    JSON.stringify(reading),
  );

  await evaluate(`history.pushState({}, "", "/panel/reports?moderation=pending"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1300);
  await evaluate(`document.querySelector('.queue-row [data-act="published"]').click()`);
  await sleep(1400);
  const triaged = await evaluate(`
    return { rows: document.querySelectorAll(".queue-row").length,
             waiting: document.querySelector('[data-moderation="pending"] .count-chip')?.textContent?.trim() ?? "" };
  `);
  check(
    "and publishing from the row empties what was waiting",
    triaged.rows === 0 && triaged.waiting === "0",
    JSON.stringify(triaged),
  );

  // 11c. The owner, who is the role the panel used to get wrong. It asked
  //      whether you were an administrator, which is stricter than the rules:
  //      an owner may add repositories to their organisation, rename it and
  //      store its forge token, and the screens offered none of it.
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const post = (path, body) => fetch(path, { method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify(body) }).then(r => r.json());

    await post("/api/people", { username: "olivia", display_name: "Olivia", password: "una contraseña larga de verdad" });
    const people = await fetch("/api/people").then(r => r.json());
    const olivia = people.people.find(p => p.username === "olivia");
    // The organisation that has a forge, so there is a token to be offered.
    // "codigosh" was made without one, and a card asking for a credential that
    // cannot be used would be the wrong thing to check for.
    await post("/api/people/" + olivia.id + "/grants", { organization: "withforge", role: "owner" });
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    await post("/api/session", { username: "olivia", password: "una contraseña larga de verdad" });
  `);
  await evaluate(`history.pushState({}, "", "/panel/withforge"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1500);

  const owner = await evaluate(`
    return {
      groups: [...document.querySelectorAll(".side-label")].map(e => e.textContent.trim()).length,
      addRepo: !!document.querySelector("[data-add-repo]"),
      deleteRepo: !!document.querySelector("[data-delete-repo]"),
      token: !!document.querySelector("form.credentials"),
    };`);
  check(
    "an owner is offered what the server would allow them",
    owner.addRepo === true && owner.token === true,
    `add=${owner.addRepo} token=${owner.token}`,
  );
  check(
    "and not what it would refuse",
    owner.deleteRepo === false && owner.groups === 2,
    `delete=${owner.deleteRepo}, ${owner.groups} sidebar groups`,
  );

  // 12. The other account. Every check above ran as the administrator, which is
  //     the comfortable one: it can reach everything, so none of them ever saw
  //     what somebody with one role sees. A collaborator's panel is most of the
  //     product for most of the people who will ever use it.
  await evaluate(`
    localStorage.setItem("lasthelp_lang", "en");
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const post = (path, body) => fetch(path, { method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify(body) }).then(r => r.json());

    // Back to the administrator: the previous block left an owner signed in,
    // and creating an account is not an owner's to do.
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    await post("/api/session", { username: "alex", password: "una contraseña larga de verdad" });

    await post("/api/people", { username: "ana", display_name: "Ana", password: "una contraseña larga de verdad" });
    const people = await fetch("/api/people").then(r => r.json());
    const ana = people.people.find(p => p.username === "ana");
    await post("/api/people/" + ana.id + "/grants",
      { organization: "codigosh", repository: "lastboard", role: "collaborator" });
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    await post("/api/session", { username: "ana", password: "una contraseña larga de verdad" });
  `);
  await evaluate(`history.pushState({}, "", "/panel"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1400);

  const collaborator = await evaluate(`
    return {
      groups: [...document.querySelectorAll(".side-label")].map(e => e.textContent.trim()),
      items: [...document.querySelectorAll(".side-item")].map(e => e.textContent.trim()),
      cards: [...document.querySelectorAll(".admin-main .bento-card h3")].map(e => e.textContent.trim()),
      offersToAdd: !!document.querySelector("[data-open-modal]"),
    };`);
  check(
    "a collaborator's panel shows the places they were given",
    JSON.stringify(collaborator.cards) === '["Lastboard"]',
    JSON.stringify(collaborator.cards),
  );
  check(
    "and offers nothing they would be refused",
    collaborator.groups.length === 2 && collaborator.offersToAdd === false,
    `${collaborator.groups.join(" · ")} — ${collaborator.items.join(", ")}`,
  );

  // 12b. And their listing is theirs. The screen is the same one the
  //      administrator uses, and the whole question is whether what it shows
  //      comes from the roles this account holds rather than from the address
  //      it was asked for.
  await evaluate(`history.pushState({}, "", "/panel/reports"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1400);
  const theirs = await evaluate(`
    return {
      rows: document.querySelectorAll(".queue-row").length,
      boards: [...new Set([...document.querySelectorAll(".queue-row .chip")].map(c => c.textContent.trim()))],
      // One board, so there is nothing to choose between and no control for it.
      boardPicker: !!document.querySelector("[data-board]"),
    };
  `);
  check(
    "a collaborator's listing holds their board and nothing else",
    theirs.rows > 0 && theirs.boards.length === 1 && theirs.boards[0] === "Lastboard" && theirs.boardPicker === false,
    JSON.stringify(theirs),
  );

  // 12c. And the count beside Reports is drawn, and drawn with a box. A number
  //      the server computed correctly and the page renders into nothing is the
  //      failure this cannot see any other way — reading the DOM says the text
  //      is there while the element measures zero.
  // And nothing waiting draws nothing. A badge that says 0 is a badge people
  // learn to stop reading, and this account has an empty queue of its own —
  // which is also the proof that the number is theirs and not the server's.
  const noBadge = await evaluate(`
    const r = await fetch("/api/me", { headers: { accept: "application/json" } });
    const me = await r.json();
    const reports = [...document.querySelectorAll(".side-item")]
      .find(a => a.getAttribute("href") === "/panel/reports");
    return {
      pending: me.pending,
      pill: !!reports?.querySelector(".side-count"),
    };
  `);
  check(
    "and an account with nothing waiting is shown no number at all",
    noBadge.pending.reports === 0 && noBadge.pending.messages === 0 && noBadge.pill === false,
    JSON.stringify(noBadge),
  );

  // 13. And a stranger, on the same address, is shown a page rather than an
  //     application. The panel around a report is for whoever has a role in it;
  //     dropping the sidebar in front of a visitor would be showing them the
  //     controls of a building they are visiting.
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    location.href = ${JSON.stringify(String(href))};
  `);
  await sleep(2000);
  const visitor = await evaluate(`
    return {
      sidebar: !!document.querySelector(".side"),
      publicNav: getComputedStyle(document.querySelector("body > nav")).display,
      report: !!document.querySelector("article.detail"),
    };
  `);
  check(
    "a visitor on the same address gets the page, not the application",
    visitor.sidebar === false && visitor.publicNav !== "none" && visitor.report === true,
    JSON.stringify(visitor),
  );

  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));
  check(
    "and the content policy refused nothing the product needs",
    refusedByPolicy.length === 0,
    refusedByPolicy.slice(0, 3).join(" | "),
  );

  await fetch(`http://127.0.0.1:${CDP}/json/close/${t.id}`);
  page.close();
}

await main();

export {};
