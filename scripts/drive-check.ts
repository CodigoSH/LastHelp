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

  /**
   * Wait until something is on the page, rather than for a length of time.
   *
   * Every navigation used to be followed by a fixed sleep, and a fixed sleep is
   * a bet that the page renders inside it. It is a bet that is won almost every
   * time, which is the problem: a pass that fails once in twenty is a pass
   * nobody believes, and the three checks that failed together one evening had
   * all read a page that had not finished arriving.
   *
   * Loud when it gives up. A check that quietly carried on with an empty page
   * would report the absence as a fault in the product.
   */
  const waitFor = async (selector: string, within = 10_000) => {
    const until = Date.now() + within;
    for (;;) {
      const there = await evaluate(
        `return !!document.querySelector(${JSON.stringify(selector)})`,
      );
      if (there) return;
      if (Date.now() > until) {
        throw new Error(`nothing matched ${selector} after ${within}ms`);
      }
      await sleep(80);
    }
  };

  /**
   * Go somewhere and wait for the thing that says you have arrived.
   *
   * Two conditions, and the second one is the one that is easy to miss: the
   * screen being left is still on the page while the next one fetches what it
   * needs, so a marker can match the page you are walking away from. That is
   * not hypothetical — waiting for "the queue or an empty state" matched the
   * empty state of the board screen before it, instantly, and the check read a
   * queue that was not there yet.
   *
   * So what is on the page now is tagged first, and the wait is for that tag to
   * be gone AND the marker to be there. The application replaces the whole of
   * #app when it draws a screen, so the tag cannot survive the change.
   */
  const goTo = async (path: string, marker: string) => {
    await evaluate(`
      const app = document.getElementById("app");
      app?.firstElementChild?.setAttribute("data-leaving", "1");
      history.pushState({}, "", ${JSON.stringify(path)});
      dispatchEvent(new PopStateEvent("popstate"));
    `);
    await waitFor("#app > :not([data-leaving])");
    await waitFor(marker);
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
  await goTo("/sign-in", "form.setup-preferences");
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
  await goTo("/panel/organizations", "[data-add-org]");
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
    // A row per organisation, as Lastboard lists its users.
    return [...document.querySelectorAll(".settings-list .si-name")].map(e => e.textContent.trim());
  `);
  check(
    "an organisation is added from a modal, not from a form left on the page",
    JSON.stringify(started) === '["CodigoSH"]',
    JSON.stringify(started),
  );

  await goTo("/panel/codigosh", "[data-add-repo]");
  const boards = await evaluate(`
    document.querySelector("[data-add-repo]").click();
    await new Promise(r => setTimeout(r, 300));
    const f = document.querySelector("#repo-modal form");
    f.querySelector('[name=name]').value = "Lastboard";
    f.querySelector('[name=slug]').value = "lastboard";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    return [...document.querySelectorAll(".repositories .si-name")].map(e => e.textContent.trim());
  `);
  check(
    "a repository is added the same way",
    JSON.stringify(boards) === '["Lastboard"]',
    JSON.stringify(boards),
  );

  // 5. Report something through the form, with no account.
  await goTo("/codigosh/lastboard/new", "form.report");
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
  await goTo("/codigosh/lastboard", ".items, .empty");
  const beforeApproval = await evaluate(`return document.querySelectorAll(".items li").length`);
  check("nothing waiting is public", beforeApproval === 0, `rows=${beforeApproval}`);

  // 7. The queue shows it, and publishing works from the panel. Through the
  //    board's old queue address, which is Review filtered to that board now:
  //    the address is in people's history, so it has to land somewhere that
  //    answers the same question.
  await goTo("/codigosh/lastboard/queue", ".rev-list");
  const queued = await evaluate(`
    const q = new URLSearchParams(location.search);
    return { path: location.pathname, board: q.get("board"), moderation: q.get("moderation"),
             rows: document.querySelectorAll('[data-what="report"]').length };
  `);
  check(
    "a board's old queue address opens Review filtered to that board, with it waiting",
    queued.path === "/review" && queued.board === "codigosh/lastboard" &&
      queued.moderation === "pending" && queued.rows === 1,
    JSON.stringify(queued),
  );

  // The detail is drawn after the list: wait for the button, not for a time.
  await waitFor(".rev-detail [data-act='published']");
  await evaluate(`document.querySelector(".rev-detail [data-act='published']").click()`);
  await sleep(900);
  const emptied = await evaluate(`return document.querySelectorAll('[data-what="report"]').length`);
  check("publishing removes it from the queue", emptied === 0, `rows=${emptied}`);

  // 8. Now it is public and votable.
  await goTo("/codigosh/lastboard", ".items, .empty");
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
    const inner = document.querySelector("body > nav .nav-inner").getBoundingClientRect();
    return {
      // The bar uses the window, the way the dashboard's does: its contents
      // start near the left edge and end near the right one. It used to share
      // the page's measure, which put both ends adrift in the middle of a wide
      // screen. Changed on purpose, so this asks the new question.
      desdeElBorde: Math.round(brand.left),
      hastaElBorde: Math.round(innerWidth - inner.right),
      offset: Math.round(Math.abs(brand.left - heading.left)),
      leads: [...new Set(leads)],
      titles: [...new Set(titles)],
    };
  `);
  check(
    "the bar uses the window rather than the measure of the page under it",
    grid.desdeElBorde <= 32 && grid.hastaElBorde <= 32,
    JSON.stringify({ izquierda: grid.desdeElBorde, derecha: grid.hastaElBorde }),
  );
  check(
    "and every row leads with the same tile, so the titles line up",
    grid.leads.length === 1 && grid.titles.length === 1,
    `leads=${JSON.stringify(grid.leads)} titles=${JSON.stringify(grid.titles)}`,
  );
  await page.send("Emulation.clearDeviceMetricsOverride");
  await sleep(300);

  const href = await evaluate(`return document.querySelector(".items a")?.getAttribute("href") ?? ""`);
  await goTo(String(href), "[data-vote]");
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
      tools: !!document.querySelector(".item-team"),
    };
  `);
  // For the team the way back is the panel's: the organisation and the
  // repository's page, where the rest of that board's work is. A visitor's
  // crumb leads to the public board, and is checked as the visitor below.
  check(
    "a report says which board it is on and leads back",
    JSON.stringify(detail.crumb) === '["/panel/organizations","/panel/codigosh","/panel/codigosh/repos/lastboard"]',
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
      // The bar stays on every screen now; what matters here is that somebody
      // arriving cold at a report inside the panel keeps the panel around it.
      barra: getComputedStyle(document.querySelector("body > nav")).display !== "none",
      tools: !!document.querySelector(".item-team"),
      // One place to write, and it is the team's. The public box posted a team
      // member's words as an anonymous visitor's, held for that same person to
      // approve.
      cajas: document.querySelectorAll(".admin-main textarea").length,
      publica: !!document.querySelector("form.comment-form"),
    };
  `);
  check(
    "and the team keeps their panel around it",
    inPanel.sidebar === true && inPanel.barra === true && inPanel.tools === true,
    JSON.stringify(inPanel),
  );
  check(
    "and has one place to write, the team's reply, not the public comment box",
    inPanel.cajas === 1 && inPanel.publica === false,
    JSON.stringify({ cajas: inPanel.cajas, publica: inPanel.publica }),
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
  //     Written as a visitor writes it, signed out on the public page: the
  //     team's page offers no public box, since the team's words are the
  //     team's. Signed back in afterwards, as the administrator this pass is.
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
  `);
  await goTo(String(href), "form.comment-form");
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
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ username: "alex", password: "una contraseña larga de verdad" }) });
    location.reload();
  `);
  await sleep(1500);
  await waitFor(".item-team");
  check("a comment is taken and held for a person", heldBack.told === true && heldBack.shown === false,
    JSON.stringify(heldBack));

  // The message waits in Review, the one screen for everything waiting. The
  // row has to name its board, or a decision is taken about a message from
  // nowhere.
  // Waiting for the list rather than for a row: if a message whose report is
  // not in the list were counted and not drawn, there would be no row to wait
  // for, and the pass would stop on a timeout instead of on this check.
  await goTo("/review", ".rev-list");
  const spanning = await evaluate(`
    const fila = document.querySelector('[data-what="comment"]');
    fila?.click();
    await new Promise(r => setTimeout(r, 500));
    return {
      filas: document.querySelectorAll('[data-what="comment"]').length,
      // Y se cuenta en lo mismo que se dibuja: el contador de la barra incluye
      // los mensajes porque las filas de mensaje están en la lista.
      enLaBarra: Number(document.querySelector(".review-pill b")?.textContent ?? 0),
      todasLasFilas: document.querySelectorAll("[data-pick]").length,
      elegido: !!document.querySelector('[data-what="comment"].on'),
    };
  `);
  check(
    "a message waiting is in the review screen too, counted with everything else",
    spanning.filas === 1 && spanning.elegido === true &&
      spanning.enLaBarra === spanning.todasLasFilas,
    JSON.stringify(spanning),
  );

  // And decided there. Approved from this screen rather than from the board's
  // own queue, so what follows — the comment appearing on the public page — is
  // the proof that the decision taken here is the same decision.
  await evaluate(`document.querySelector(".rev-detail [data-act='published']")?.click()`);
  await sleep(1100);
  const cleared = await evaluate(`
    const r = await fetch("/api/panel/comments", { headers: { accept: "application/json" } });
    const body = await r.json();
    return { onScreen: document.querySelectorAll('[data-what="comment"]').length,
             stillWaiting: (body.comments ?? []).length };
  `);
  check(
    "and approving it there leaves the queue empty on both sides",
    cleared.onScreen === 0 && cleared.stillWaiting === 0,
    JSON.stringify(cleared),
  );

  await goTo(String(href), ".count-chip");
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
  // Its own wait: the navigation above stops at the count, and the team's
  // controls arrive with a second read. Without this the check failed about one
  // run in ten, saying the product had lost a control it had not lost.
  await waitFor("#status-select");
  const statusControl = await evaluate(`return !!document.querySelector("#status-select")`);
  check("the team sees the status control", statusControl === true);

  await evaluate(`
    const select = document.querySelector("#status-select");
    select.value = "planned";
    select.dispatchEvent(new Event("change"));
  `);
  await sleep(600);

  await goTo("/codigosh/lastboard/roadmap", ".roadmap");
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
  await goTo(String(href), "[data-create-issue]");
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

  // 11. Updates live inside About now, as in Lastboard, and the old address
  //     is a door to it. What is checked is that the zone says what the server
  //     said — never nothing, and never green unless the release server was
  //     reached and answered. An empty answer where a reason belongs reads as
  //     "you are up to date". Compared with /api/update rather than with a
  //     premise about this build: this comment used to say the build had no
  //     release keys, and it has them.
  await goTo("/panel/updates", ".about-hero");
  await waitFor("[data-update-zone] > :not(.checking)");
  const updates = await evaluate(`
    const zone = document.querySelector("[data-update-zone]");
    const api = await fetch("/api/update").then(r => r.json());
    const alcanzado = (!!api.latest && !api.available) || api.reason === "error.update_none";
    return {
      alcanzado,
      razon: api.reason ?? "",
      ofreceInstalar: !!zone?.querySelector("[data-update]"),
      donde: location.pathname,
      version: document.querySelector(".about-version")?.textContent?.trim() ?? "",
      dice: zone?.textContent?.trim() ?? "",
      comprobando: !!zone?.querySelector(".checking"),
      verde: !!zone?.querySelector(".about-state.ok"),
      interruptor: !(document.querySelector("[data-beta]")?.disabled ?? true),
      enElMenu: [...document.querySelectorAll(".side-item")].map(a => a.getAttribute("href")),
    };
  `);
  check(
    "updates are part of About: the old address opens it, and the menu has one entry",
    updates.donde === "/panel/about" && updates.enElMenu.includes("/panel/about") &&
      !updates.enElMenu.includes("/panel/updates"),
    JSON.stringify(updates.enElMenu),
  );
  check(
    "About says what is running and what the release server said, never a green it did not earn",
    updates.version.length > 0 && updates.dice.length > 0 && updates.comprobando === false &&
      updates.verde === updates.alcanzado && updates.ofreceInstalar === false &&
      updates.interruptor === true,
    JSON.stringify(updates),
  );

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
  await goTo("/sign-in", ".signin-go");

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

  // 11b. About, which is the page somebody keeping a server running opens:
  //      the machine's health, each fact a number, and how full the disk is
  //      as a bar that is an attribute rather than a style.
  await goTo("/panel/about", ".about-stat");
  const about = await evaluate(`
    const tiles = [...document.querySelectorAll(".about-stat")];
    const meter = document.querySelector(".about-meter");
    const r = meter?.getBoundingClientRect();
    return {
      tiles: tiles.length,
      values: tiles.map(t => t.querySelector(".about-stat-value")?.textContent?.trim() ?? ""),
      meter: meter ? { value: meter.value, max: meter.max, ancho: Math.round(r.width), alto: Math.round(r.height) } : null,
      links: [...document.querySelectorAll(".about-links a")].map(a => a.getAttribute("href")),
      // Under the version, nothing about the build: the platform and the
      // compiler were a line nobody opening this page came for.
      plataforma: /linux|darwin|windows|go1\./.test(document.querySelector(".about-hero")?.textContent ?? ""),
      // The name sits under its mark. The panel's heading rule gave every h2
      // 36px above it and pushed the name half a card away from the logo.
      bajoElLogo: Math.round(
        (document.querySelector(".about-name")?.getBoundingClientRect().top ?? 999) -
        (document.querySelector(".about-logo")?.getBoundingClientRect().bottom ?? 0)),
    };
  `);
  check("About reports the state of the server", about.tiles >= 4, `${about.tiles} tiles`);
  check(
    "and every tile has a number to report",
    about.values.every((v: string) => v.length > 0),
    JSON.stringify(about.values),
  );
  check(
    "About sits together: the name under its mark, no build line, the disk as a bar with a real length, the links out",
    about.meter !== null && about.meter.max > 0 && about.meter.value >= 0 &&
      about.meter.value <= about.meter.max && about.meter.ancho > 100 && about.meter.alto >= 4 &&
      about.links.length === 2 && about.bajoElLogo >= 8 && about.bajoElLogo <= 20 &&
      about.plataforma === false,
    JSON.stringify({ meter: about.meter, links: about.links, bajoElLogo: about.bajoElLogo, plataforma: about.plataforma }),
  );

  // 11c. The panel on a phone: its menu is a strip across the top that scrolls
  //      sideways, every link on the one line. The links live inside a <nav>
  //      that stayed a block, so they stacked down the middle of the strip
  //      with "Public site" and "Sign out" adrift beside them, and "Sign out"
  //      broke over two lines.
  await page.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 1, mobile: true });
  // On a short screen, where the grid has room left over to share: that is
  // where the strip grew to 300px. A long one leaves nothing to share and
  // passes with the fault in place.
  await goTo("/panel/appearance", "#appearance-language");
  const franja = await evaluate(`
    const side = document.querySelector(".side");
    const fila = [...side.querySelectorAll(".side-item, .side-foot .link")];
    const tops = fila.map(e => Math.round(e.getBoundingClientRect().top + e.getBoundingClientRect().height / 2));
    const salir = side.querySelector(".side-foot .link")?.getBoundingClientRect();
    return {
      enlaces: fila.length,
      alturas: Math.max(...tops) - Math.min(...tops),
      franja: Math.round(side.getBoundingClientRect().height),
      salirEnUnaLinea: salir ? salir.height < 40 : false,
      desplazaDeLado: side.scrollWidth > side.clientWidth,
    };
  `);
  await page.send("Emulation.clearDeviceMetricsOverride", {});
  check(
    "on a phone the panel's menu is one strip across the top, every link on the one line",
    franja.enlaces >= 8 && franja.alturas <= 4 && franja.franja < 80 && franja.salirEnUnaLinea === true,
    JSON.stringify(franja),
  );

  // 11d. The panel wears Lastboard's settings recipe on every screen, measured
  //      on the page rather than read from the stylesheet: the button that
  //      commits is outlined, not filled; a card's name is 13px capitals; and
  //      no card carries an icon of its own. A panel restyled in pieces of its
  //      own read as a different product, and nothing but a screenshot said so.
  const receta: string[] = [];
  let piezas = 0;
  for (const ruta of ["/panel", "/panel/organizations", "/panel/codigosh", "/panel/people",
    "/panel/backups", "/panel/account", "/panel/appearance", "/panel/personalization",
    // About too: it was the one screen left in pieces of its own after the
    // rest had been ported, and nothing measured it.
    "/panel/about"]) {
    await goTo(ruta, ".admin-main > *");
    await sleep(250);
    const r = await evaluate(`
      const fallos = [];
      let n = 0;
      for (const b of document.querySelectorAll(".admin-main .button.primary")) {
        if (b.closest(".wizard, dialog")) continue;
        n++;
        const c = getComputedStyle(b);
        if (c.backgroundImage !== "none" || !/rgba\\(0, 0, 0, 0\\)|transparent/.test(c.backgroundColor) || c.borderTopWidth !== "2px" || c.textTransform !== "uppercase")
          fallos.push("button " + b.textContent.trim().slice(0, 20) + ": " + c.backgroundColor + " " + c.borderTopWidth);
      }
      for (const t of document.querySelectorAll(".admin-main .card-top h3, .admin-main .bento-card .card-head h3, .admin-main .dash-card > h3")) {
        n++;
        const c = getComputedStyle(t);
        if (c.fontSize !== "13px" || c.textTransform !== "uppercase") fallos.push("title " + t.textContent.trim().slice(0, 20) + ": " + c.fontSize);
      }
      for (const i of document.querySelectorAll(".admin-main .bento-card .card-head > .icon")) {
        if (i.getBoundingClientRect().width > 0) fallos.push("card icon shown");
      }
      // Laid on their sheet — one solid grey, not the public page's ground.
      const sonda = document.createElement("i");
      sonda.style.backgroundColor = "var(--sheet)";
      document.body.append(sonda);
      const hoja = getComputedStyle(sonda).backgroundColor;
      sonda.remove();
      n++;
      const fondo = getComputedStyle(document.querySelector(".admin")).backgroundColor;
      if (fondo !== hoja) fallos.push("panel ground " + fondo + " is not the sheet " + hoja);
      // And every language named beside its flag, as their picker does.
      for (const o of document.querySelectorAll("#appearance-language option")) {
        n++;
        if (!/^\\p{Regional_Indicator}{2} /u.test(o.textContent)) fallos.push("no flag: " + o.textContent);
      }
      return { n, fallos };
    `);
    piezas += r.n;
    for (const f of r.fallos) receta.push(`${ruta}: ${f}`);
  }
  check(
    "every panel screen wears Lastboard's settings recipe: its sheet, outlined buttons, quiet 13px titles, no card icons, flags beside languages",
    piezas > 15 && receta.length === 0,
    JSON.stringify({ piezas, receta: receta.slice(0, 6) }),
  );

  // 11e. The accent is chosen as in Lastboard: seven swatches and an eighth
  //      for any colour. A swatch writes the form's field and is the only one
  //      ringed; a colour of one's own paints the eighth; and its palette sits
  //      in the middle of it — it fell into the corner when a label rule made
  //      the swatch a block.
  await goTo("/panel/personalization", ".color-grid");
  const acento = await evaluate(`
    document.querySelector(".acc-blue").click();
    await new Promise(r => setTimeout(r, 200));
    const azul = { campo: document.querySelector("#accent").value,
      marcadas: [...document.querySelectorAll(".accent-swatch.on")].length,
      esAzul: document.querySelector(".acc-blue").classList.contains("on") };
    const propio = document.querySelector("[data-accent-custom]");
    propio.value = "#12ab34"; propio.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise(r => setTimeout(r, 200));
    const caja = document.querySelector(".acc-custom").getBoundingClientRect();
    const icono = document.querySelector(".acc-custom svg").getBoundingClientRect();
    return { azul, campo: document.querySelector("#accent").value,
      pintada: getComputedStyle(document.querySelector(".acc-custom")).backgroundColor,
      descentrado: Math.round(Math.abs((icono.x + icono.width / 2) - (caja.x + caja.width / 2)) +
        Math.abs((icono.y + icono.height / 2) - (caja.y + caja.height / 2))) };
  `);
  check(
    "the accent is chosen from Lastboard's swatches, and the eighth takes any colour",
    acento.azul.campo === "#228be6" && acento.azul.marcadas === 1 && acento.azul.esAzul === true &&
      acento.campo === "#12ab34" && acento.pintada === "rgb(18, 171, 52)" && acento.descentrado <= 2,
    JSON.stringify(acento),
  );

  const langPicker = await evaluate(`return !!document.querySelector("body > nav select");`);
  check("no language picker rides along in the top bar", langPicker === false);

  // The same control on the screens that are not the wizard, which is the half
  // that kept being forgotten: a change made in one place and nowhere else is
  // work handed back to whoever has to find the other three.
  const imagePickers: Array<[string, string]> = [
    ["profile", "/panel/account"],
    ["personalisation", "/panel/personalization"],
  ];
  for (const [name, path] of imagePickers) {
    await goTo(path, ".avatar-pick");
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
  await goTo("/panel", ".dash-card, .bento-card, .cards, .empty");
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
  await goTo("/", ".landing-grid");
  const front = await evaluate(`
    const hero = document.querySelector('[data-module^="hero"] .words-box');
    const mark = hero?.querySelector("img");
    return {
      title: hero?.querySelector("h1")?.textContent?.trim() ?? "",
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
  await goTo("/panel", ".dash-card, .bento-card, .cards, .empty");

  const sidebar = await evaluate(`
    return {
      items: [...document.querySelectorAll(".side-item")].map(a => a.textContent.trim()),
      current: document.querySelector('.side-item[aria-current="page"]')?.textContent?.trim(),
      // The bar keeps the panel's own control, which is what closes it.
      cierra: !!document.querySelector("body > nav [data-panel-toggle]"),
      who: document.querySelector(".who")?.textContent?.trim(),
      // The bar stays on every screen now, the way the dashboard's does. What
      // must not happen is two NAVIGATIONS: the bar carries modes — the panel
      // toggle, arranging the front page — and the places you can go are down
      // the side. A link to a page in both is the fault this was written for.
      barraVisible: getComputedStyle(document.querySelector("body > nav")).display !== "none",
      enlacesEnLaBarra: document.querySelectorAll("body > nav a[href^='/panel']").length,
      // One mark on screen. The menu used to open with the installation's
      // mark and name a few pixels under the bar's, the same thing twice.
      marcasALaVista: [...document.querySelectorAll(".brand-logo")]
        .filter(m => m.getBoundingClientRect().width > 0).length,
      // The whole sidebar fits the window, foot and all: it was a window tall
      // but hung below the bar, so "Sign out" sat under the fold.
      pieBajo: Math.round(document.querySelector(".side-foot .link")?.getBoundingClientRect().bottom ?? 99999),
      lateralBajo: Math.round(document.querySelector(".side")?.getBoundingClientRect().bottom ?? 99999),
      ventana: innerHeight,
    };`);
  check(
    "the administration shows its options down one side",
    sidebar.items.length >= 3 && Boolean(sidebar.current) && Boolean(sidebar.who),
    `${sidebar.items.join(" · ")} — on "${sidebar.current}" as ${sidebar.who}`,
  );
  check(
    "the whole sidebar fits the window, sign-out included, without scrolling",
    sidebar.pieBajo <= sidebar.ventana && sidebar.lateralBajo <= sidebar.ventana,
    JSON.stringify({ pie: sidebar.pieBajo, lateral: sidebar.lateralBajo, ventana: sidebar.ventana }),
  );
  // Pointed at, a menu row lights in the accent, as Lastboard's does — with a
  // real pointer through the protocol, because a synthetic mouseover does not
  // set :hover. It was a neutral grey here.
  const fila = await evaluate(`
    const it = [...document.querySelectorAll(".side-item")].find(a => !a.hasAttribute("aria-current"));
    const r = it.getBoundingClientRect();
    window.__fila = it;
    return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
  `);
  await page.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: 900, y: 600 });
  await sleep(150);
  await page.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: fila.x, y: fila.y });
  await sleep(400);
  const encima = await evaluate(`
    const sonda = document.createElement("i");
    sonda.style.backgroundColor = "var(--surface-hover)";
    document.body.append(sonda);
    const esperado = getComputedStyle(sonda).backgroundColor;
    sonda.remove();
    return { fondo: getComputedStyle(window.__fila).backgroundColor, esperado,
      acento: getComputedStyle(document.documentElement).getPropertyValue("--accent-rgb").trim() };
  `);
  await page.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: 900, y: 600 });
  check(
    "a menu row pointed at lights in the accent, as Lastboard's does",
    encima.fondo === encima.esperado && encima.fondo.replace(/\s/g, "").includes(encima.acento.replace(/\s/g, "")),
    JSON.stringify(encima),
  );
  check(
    "and does not stack two navigations, or two marks, on top of each other",
    sidebar.barraVisible === true && sidebar.enlacesEnLaBarra === 0 && sidebar.marcasALaVista === 1,
    JSON.stringify({ barra: sidebar.barraVisible, enlaces: sidebar.enlacesEnLaBarra, marcas: sidebar.marcasALaVista }),
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
  await goTo("/panel/new", ".forge-fields");
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

  // 12. Everything the panel can do, it can do from the screen.
  //
  //     Four actions were API-only for a while: the forge token, the
  //     notification channel, replying as the team and merging a duplicate.
  //     Each one was a screen naming something it could not do — the same
  //     failure as a panel that says nothing is configured and offers nothing
  //     to configure, which is how an operator ends up in a terminal.
  await goTo("/panel/codigosh", "[data-add-repo]");
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
  // Each repository's channel is on that repository's page: a picker of
  // repositories on the organisation's page was a second way to name one.
  check("and leaves each repository's channel to its own page", orgPanel.channel === false);
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
    const antes = document.querySelector("[data-connected]")?.dataset.connected ?? "";

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
      antes,
      despues: document.querySelector("[data-connected]")?.dataset.connected ?? "",
    };
  `);
  check("an organisation with a forge asks for its token", connected.form === true);
  check(
    "and shows where the forge should deliver",
    connected.shown === true && String(connected.text).includes("/hooks/"),
    String(connected.text).slice(0, 56),
  );
  check("and does not leave the token in the field", connected.tokenCleared === true);
  check(
    "and says whether it is connected, before and after the token is stored",
    connected.antes === "false" && connected.despues === "true",
    JSON.stringify({ antes: connected.antes, despues: connected.despues }),
  );

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

  await goTo(String(href), "form.team-reply");
  // Merging is offered as a pick from the board's other reports, so the form
  // is there exactly when there is another report to pick — and says so when
  // there is none, rather than asking for an identifier that cannot exist.
  const triage = await evaluate(`
    const id = location.pathname.split("/").pop();
    const all = await fetch("/api/reports?board=codigosh/lastboard&limit=200").then(r => r.json());
    const others = (all.items ?? []).filter(r => r.public_id !== id).length;
    return {
      reply: !!document.querySelector("form.team-reply"),
      merge: !!document.querySelector("form.merge"),
      opciones: document.querySelectorAll("#merge-into option").length,
      others,
    };
  `);
  check("the team can reply and merge from the item",
    triage.reply === true && triage.merge === (triage.others > 0) &&
      triage.opciones === triage.others,
    JSON.stringify(triage));

  // And rewrite it. A title that says "its broken" is the normal case, and
  // until now the only way to fix one was to ask the person who wrote it.
  // Checked on the page rather than through the API: the form has to be
  // reachable, prefilled with what is actually there, and the heading has to
  // end up saying the new thing.
  const rewritable = await evaluate(`
    // Behind the pencil, in the report card where the words stand.
    document.querySelector("[data-rewrite]")?.click();
    await new Promise(r => setTimeout(r, 200));
    const form = document.querySelector(".detail form.rewrite");
    return {
      form: !!form,
      prefilled: form?.querySelector("input[name=title]")?.value ?? "",
      heading: document.querySelector("h1.visually-hidden")?.textContent?.trim() ?? "",
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
  await goTo("/", ".landing-grid");
  const composing = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 600));

    const outlined = getComputedStyle(document.querySelector(".landing-cell")).borderStyle;
    const kebabs = document.querySelectorAll(".kebab-btn").length;

    // Add words, and the module opens its own dialog the moment it exists.
    document.querySelector("[data-add-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    document.querySelector('[data-open-modal="module-picker"]').click();
    await new Promise(r => setTimeout(r, 400));
    document.querySelector('[data-pick="text"]').click();
    await new Promise(r => setTimeout(r, 900));
    const opened = document.querySelector("#module-config")?.open === true;

    document.querySelector("#module-config [data-close]")?.click();
    await new Promise(r => setTimeout(r, 400));

    // And the words go in on the page, which is the whole point of the change:
    // the dialog has no field for them any more.
    const caja = [...document.querySelectorAll("[data-words-body]")].at(-1);
    caja.focus();
    caja.innerHTML = "<p>We read everything that arrives.</p>";
    caja.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise(r => setTimeout(r, 1200));
    const escrito = caja.textContent.trim();

    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 600));
    return { outlined, kebabs, opened, escrito };
  `);
  check(
    "the front page is arranged on the front page",
    composing.outlined === "dashed" && composing.kebabs >= 3 && composing.opened === true &&
      composing.escrito === "We read everything that arrives.",
    JSON.stringify(composing),
  );

  await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
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
    // Measured at the same instant as the ghost, not at the instant the drag
    // started: a scrollbar arriving between the two made the two boxes differ
    // by fifteen pixels and the check call it a fault.
    const ahora = first.getBoundingClientRect();
    const llevado = { alto: Math.round(ahora.height), ancho: Math.round(ahora.width) };

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
      document.querySelector("[data-add-menu]").click();
      await new Promise(r => setTimeout(r, 200));
      document.querySelector('[data-open-modal="module-picker"]').click();
      await new Promise(r => setTimeout(r, 400));
      document.querySelector('[data-pick="' + kind + '"]').click();
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
    document.querySelector("[data-add-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    document.querySelector('[data-open-modal="module-picker"]').click();
    await new Promise(r => setTimeout(r, 400));
    document.querySelector('[data-pick="call_to_action"]').click();
    await new Promise(r => setTimeout(r, 1100));
    const etiquetaOfrecida = !document.querySelector("[data-for-label]").hidden;
    const form = document.querySelector("form.module-config");
    form.querySelector("[name=label]").value = "Tell us what broke";
    form.requestSubmit();
    await new Promise(r => setTimeout(r, 1100));

    document.querySelector("[data-edit-done]")?.click();
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

  // The front page is the register that SHOWS things, not the one that asks for
  // one thing. The difference is not a matter of taste and it is measurable:
  // the sign-in stacks the mark above a display-sized name down the middle of
  // the window, and the front page used to do exactly that — it was the sign-in
  // without its card. In a module the mark sits BESIDE the name, on one line.
  const registro = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const escribir = (modules) => fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules }) });
    const dibujar = async () => {
      history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
      await new Promise(r => setTimeout(r, 1300));
    };

    // This block writes a page of its own, so it puts back the one it found.
    // Leaving a rewritten layout behind took the checks after it down — for the
    // third time in this file, which is why it is spelled out here.
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;

    await escribir([
      { id: "hero", kind: "hero", x: 1, y: 1, w: 12, h: 2 },
      { id: "block", kind: "text", x: 1, y: 3, w: 12, h: 2 },
    ]);
    await dibujar();

    const celda = document.querySelector('[data-module="hero"]');
    const nombre = celda.querySelector("h1");
    const bloque = document.querySelector('[data-module="block"]');
    const caja = bloque.querySelector(".words-box").getBoundingClientRect();
    const medido = {
      // Where the mark goes is the operator's decision now, so it is not
      // measured here. What is still the product's decision, and is what this
      // check exists for, is the register: a heading sized for a card that asks
      // for one thing, and modules whose content fills them rather than sitting
      // in a band of air.
      tamanoNombre: Math.round(parseFloat(getComputedStyle(nombre).fontSize)),
      // The box fills its module but for the gap asked for on each side.
      holgura: Math.round(bloque.getBoundingClientRect().height - caja.height),
    };
    await escribir(previa);
    await dibujar();
    return medido;
  `);
  check(
    "the front page shows things rather than asking for one, as the dashboard does",
    registro.tamanoNombre <= 34 && registro.holgura <= 14,
    JSON.stringify(registro),
  );

  // Nothing a module holds is cut off by the module.
  //
  // The tile clips rather than scrolls — that is the dashboard's rule and what
  // stops the resize handle fighting a scrollbar — so anything taller than its
  // tile loses its bottom silently. A box given the module's full height while
  // also carrying the page-flow top margin is taller by exactly that margin,
  // and the button module was losing its bottom padding to it.
  const recortes = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;
    const kinds = ["hero", "text", "call_to_action", "organisations", "boards",
      "most_voted", "recently_done", "recent_activity", "roadmap", "statistics"];
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: kinds.map((kind, i) => ({
        id: kind, kind, x: 1 + (i % 4) * 3, y: 1 + Math.floor(i / 4) * 2, w: 3, h: 2,
        ...(kind === "call_to_action" ? { scope: "codigosh/lastboard", body: "Tell us" } : {}),
      })) }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1500));
    const cortados = [...document.querySelectorAll(".landing-cell")]
      .map(c => ({ que: c.dataset.module, seSale: c.scrollHeight - c.clientHeight }))
      .filter(x => x.seSale > 0);
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: previa }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1300));
    return { cortados, mirados: kinds.length };
  `);
  check(
    "and no module has its content cut off by the tile it sits in",
    recortes.mirados === 10 && recortes.cortados.length === 0,
    JSON.stringify(recortes),
  );

  // Adding, the way the dashboard does it: a plus in the bar that opens a short
  // menu — a module, or a group — and the module one opens a picker of cards
  // saying what each kind draws. It was a dropdown and an Add button sitting on
  // the page: two controls that said nothing about what you were about to get,
  // in the middle of the thing every visitor sees.
  const anadir = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 900));

    const enLaPagina = !!document.querySelector("#app [data-add-kind], #app .landing-bar");
    const mas = document.querySelector("body > nav [data-add-menu]");
    mas.click();
    await new Promise(r => setTimeout(r, 300));
    // The menu is the dashboard's own, not the kebab's: monospaced items of
    // 11px with a 14px mark each, which is how somebody recognises it without
    // reading it. Measured, because "the same as theirs" is a number.
    const items = [...document.querySelectorAll("body > nav .add-menu .add-menu-item")];
    const menu = items.map(b => b.textContent.trim());
    const primera = items[0] ? getComputedStyle(items[0]) : null;
    const forma = {
      fuente: primera?.fontSize ?? "",
      mono: (primera?.fontFamily ?? "").includes("mono"),
      radio: getComputedStyle(document.querySelector("body > nav .add-menu")).borderRadius,
      marca: Math.round(items[0]?.querySelector("svg")?.getBoundingClientRect().width ?? 0),
      nowrap: primera?.whiteSpace === "nowrap",
    };
    // One line each, whatever the language — and English is not a test of that:
    // "Add Module" fits the starting width with or without the rule. The words
    // are swapped for the longest pair any catalogue carries, measured, and put
    // back, so this measures the menu rather than the language the journey runs
    // in. A wrapped item is the same words in twice the box: 46 against 31.
    const alto = (el) => Math.round(el.getBoundingClientRect().height);
    const original = items.map(i => i.querySelector("span")?.textContent ?? "");
    for (const i of items) {
      const span = i.querySelector("span");
      if (span) span.textContent = "Abschnitt hinzufügen";
    }
    await new Promise(r => setTimeout(r, 60));
    forma.unaLinea = items.every(i => alto(i) < 38);
    forma.conPalabrasLargas = items.map(alto);
    items.forEach((i, n) => {
      const span = i.querySelector("span");
      if (span) span.textContent = original[n];
    });

    document.querySelector('[data-open-modal="module-picker"]').click();
    await new Promise(r => setTimeout(r, 400));
    const dlg = document.querySelector("#module-picker");
    const tarjetas = [...dlg.querySelectorAll("[data-pick]")].map(c => c.dataset.pick);
    // The title has height. The class named picker here is the image control
    // elsewhere and carries a line-height of nought, which printed this title
    // and the line under it on top of each other at no height at all.
    const altoTitulo = Math.round(dlg.querySelector(".picker-head h2").getBoundingClientRect().height);
    const altoLead = Math.round(dlg.querySelector(".picker-head p").getBoundingClientRect().height);
    const conTexto = [...dlg.querySelectorAll(".picker-card")]
      .every(c => c.querySelector(".picker-name")?.textContent.trim() &&
                  c.querySelector(".picker-about")?.textContent.trim() &&
                  c.querySelector(".picker-icon svg"));
    const abierto = dlg.open === true;
    dlg.querySelector("[data-close]").click();
    await new Promise(r => setTimeout(r, 300));

    return { enLaPagina, menu, forma, tarjetas, conTexto, abierto, cerrado: dlg.open === false,
             // And the way out of arranging is still one click in the same bar.
             hecho: !!document.querySelector("body > nav [data-edit-done]") };
  `);
  check(
    "adding is a menu in the bar and a picker of cards, not two controls on the page",
    anadir.enLaPagina === false && anadir.menu.length === 2 &&
      anadir.abierto === true && anadir.cerrado === true && anadir.hecho === true &&
      // And it is shaped like theirs, not like the kebab it used to borrow.
      anadir.forma.fuente === "11px" && anadir.forma.mono === true &&
      anadir.forma.radio === "4px" && anadir.forma.marca === 14 &&
      anadir.forma.unaLinea === true && anadir.forma.nowrap === true,
    JSON.stringify({ enLaPagina: anadir.enLaPagina, menu: anadir.menu, forma: anadir.forma }),
  );
  check(
    "and the picker offers every module, each saying what it draws",
    // Ten, not eleven: the two that drew a paragraph did the same thing in two
    // shapes and are one block of words now.
    anadir.tarjetas.length === 10 && anadir.conTexto === true &&
      anadir.tarjetas.includes("roadmap") && anadir.tarjetas.includes("hero") &&
      // A group is not a module that draws something, so it is not in here.
      !anadir.tarjetas.includes("section"),
    JSON.stringify(anadir.tarjetas),
  );

  // Taking hold of a corner must not change the page around it. The handle is
  // positioned against the cell's edges, and while the cell was the thing that
  // scrolled, the handle pushed its scrollable area out: a scrollbar appeared,
  // the content narrowed, the handle moved, and it flickered without anything
  // being dragged.
  const esquina = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 900));

    const celda = document.querySelector(".landing-cell");
    const cuerpo = celda;
    const asa = celda.querySelector(":scope > [data-resize]");
    const foto = () => ({
      barras: cuerpo.scrollWidth > cuerpo.clientWidth || cuerpo.scrollHeight > cuerpo.clientHeight,
      caja: Math.round(celda.getBoundingClientRect().width) + "x" +
            Math.round(celda.getBoundingClientRect().height),
      asa: Math.round(asa.getBoundingClientRect().left) + "," +
           Math.round(asa.getBoundingClientRect().top),
    });

    const antes = foto();
    const caja = celda.getBoundingClientRect();
    const opts = { bubbles: true, cancelable: true, pointerId: 1, pointerType: "mouse" };
    asa.dispatchEvent(new PointerEvent("pointerdown", { ...opts,
      clientX: caja.right, clientY: caja.bottom }));
    await new Promise(r => setTimeout(r, 400));
    const agarrada = foto();
    await new Promise(r => setTimeout(r, 600));
    const quieta = foto();
    window.dispatchEvent(new PointerEvent("pointerup", { ...opts,
      clientX: caja.right, clientY: caja.bottom }));
    await new Promise(r => setTimeout(r, 600));
    return { antes, agarrada, quieta };
  `);
  check(
    "taking hold of a corner adds no scrollbars and moves nothing",
    esquina.antes.barras === false && esquina.agarrada.barras === false &&
      esquina.agarrada.caja === esquina.antes.caja &&
      esquina.quieta.caja === esquina.agarrada.caja &&
      esquina.quieta.asa === esquina.agarrada.asa,
    JSON.stringify(esquina),
  );

  // Sizes. Every module used to arrive twelve columns wide and refuse to go
  // below two, so a page was a stack of bands whatever the operator did. A
  // module is a tile like any other in this house: it arrives at a size that
  // suits it and can be dragged down to a single cell.
  const tamanos = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    const anadir = async (kind) => {
      document.querySelector("[data-add-menu]").click();
      await new Promise(r => setTimeout(r, 200));
      document.querySelector('[data-open-modal="module-picker"]').click();
      await new Promise(r => setTimeout(r, 400));
      document.querySelector('[data-pick="' + kind + '"]').click();
      await new Promise(r => setTimeout(r, 1100));
      document.querySelector("#module-config")?.close();
      await new Promise(r => setTimeout(r, 300));
      const capa = await fetch("/api/landing").then(r => r.json());
      const puesto = capa.modules.filter(m => m.kind === kind).pop();
      return { w: puesto.w, h: puesto.h, id: puesto.id };
    };

    const boton = await anadir("call_to_action");
    const cifras = await anadir("statistics");

    // And down to one column, from the corner.
    const celda = document.querySelector('[data-module="' + cifras.id + '"]');
    const caja = celda.getBoundingClientRect();
    const grid = document.querySelector(".landing-grid.editing").getBoundingClientRect();
    const col = (grid.width - 11 * 16) / 12;
    const asa = celda.querySelector(":scope > [data-resize]");
    const opts = { bubbles: true, cancelable: true, pointerId: 1, pointerType: "mouse" };
    asa.dispatchEvent(new PointerEvent("pointerdown", { ...opts, clientX: caja.right, clientY: caja.bottom }));
    window.dispatchEvent(new PointerEvent("pointermove", { ...opts,
      clientX: caja.right - 5 * (col + 16), clientY: caja.bottom - 2 * (col + 16) }));
    await new Promise(r => setTimeout(r, 160));
    window.dispatchEvent(new PointerEvent("pointerup", { ...opts,
      clientX: caja.right - 5 * (col + 16), clientY: caja.bottom - 2 * (col + 16) }));
    await new Promise(r => setTimeout(r, 1100));

    const despues = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === cifras.id);
    return { boton, cifras, encogido: { w: despues.w, h: despues.h } };
  `);
  check(
    "every module arrives three by two, whatever it is",
    tamanos.boton.w === 3 && tamanos.boton.h === 2 &&
      tamanos.cifras.w === 3 && tamanos.cifras.h === 2,
    JSON.stringify({ boton: tamanos.boton, cifras: tamanos.cifras }),
  );
  check(
    "and can be dragged down to a single cell",
    tamanos.encogido.w === 1 && tamanos.encogido.h === 1,
    JSON.stringify(tamanos.encogido),
  );

  // The bar carries icons and nothing but icons, and the panel arrives from the
  // side the reading direction ends on — the dashboard's drawer does exactly
  // that, and this pass runs in Arabic, where getting it wrong means sliding in
  // from the opposite edge of the screen.
  const entrada = await evaluate(`
    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 600));
    const bar = document.querySelector("body > nav");
    const antes = {
      palabras: bar.querySelector("[data-panel-toggle]").textContent.trim() !== "",
      icono: !!bar.querySelector("[data-panel-toggle] svg"),
      titulo: bar.querySelector("[data-panel-toggle]").getAttribute("title") ?? "",
      abierto: !!bar.querySelector(".panel-indicator.on"),
    };

    // Opened by its own control. Everything is read WHILE it happens: the
    // arriving class is taken off when the animation ends, so a probe that
    // waits for the page to settle finds nothing and reports the absence as a
    // fault. That is what the first version of this did.
    bar.querySelector("[data-panel-toggle]").click();
    let llegando = null;
    for (let i = 0; i < 40 && !llegando; i++) {
      const shell = document.querySelector(".admin.arriving");
      if (shell) {
        const hojas = [...document.styleSheets].flatMap(h => { try { return [...h.cssRules]; } catch { return []; } });
        const fotogramas = hojas.find(r => r.name === "panel-arrives");
        llegando = { nombre: getComputedStyle(shell).animationName,
                     direccion: document.documentElement.dir,
                     giro: getComputedStyle(document.documentElement).getPropertyValue("--rtl-flip").trim(),
                     sigueLaDireccion: (fotogramas?.cssText ?? "").includes("--rtl-flip") };
      }
      await new Promise(r => setTimeout(r, 25));
    }

    await new Promise(r => setTimeout(r, 500));
    const dentro = {
      enElPanel: document.body.classList.contains("admin-view"),
      indicador: !!document.querySelector("body > nav .panel-indicator.on"),
      barraVisible: getComputedStyle(document.querySelector("body > nav")).display !== "none",
    };

    // And the same control closes it, going back out the side it came in.
    document.querySelector("body > nav [data-panel-toggle]").click();
    let saliendo = false;
    for (let i = 0; i < 20 && !saliendo; i++) {
      saliendo = !!document.querySelector(".admin.leaving");
      await new Promise(r => setTimeout(r, 25));
    }
    await new Promise(r => setTimeout(r, 1200));
    const fuera = !document.body.classList.contains("admin-view");

    return { antes, llegando, dentro, saliendo, fuera };
  `);
  check(
    "the panel is an icon in the bar, with its name in a tooltip",
    entrada.antes.palabras === false && entrada.antes.icono === true &&
      entrada.antes.titulo !== "" && entrada.antes.abierto === false,
    JSON.stringify(entrada.antes),
  );
  check(
    "and that one icon opens it, says so, and closes it again",
    entrada.dentro.enElPanel === true && entrada.dentro.indicador === true &&
      entrada.dentro.barraVisible === true && entrada.saliendo === true &&
      entrada.fuera === true,
    JSON.stringify({ dentro: entrada.dentro, saliendo: entrada.saliendo, fuera: entrada.fuera }),
  );
  check(
    "and it arrives from the side the reading direction ends on",
    entrada.llegando?.nombre === "panel-arrives" &&
      entrada.llegando?.sigueLaDireccion === true &&
      (entrada.llegando?.direccion === "rtl"
        ? entrada.llegando?.giro === "-1"
        : entrada.llegando?.giro === "1"),
    JSON.stringify(entrada.llegando),
  );

  // The settings dialog, in the shape the dashboard uses for its own: the tabs
  // ARE the header, General and Customization, the close beside them, and one
  // action at the foot. Somebody who has configured a widget over there should
  // not have to learn this one.
  const ajustes = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));
    const celda = document.querySelector('.landing-cell[data-module^="boards-"]');
    celda.querySelector("[data-module-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    celda.querySelector("[data-edit-module]").click();
    await new Promise(r => setTimeout(r, 500));

    const form = document.querySelector("form.module-config");
    const pestanas = [...form.querySelectorAll(".modal-head.tabbed [data-tab]")]
      .map(b => b.textContent.trim());
    const enCabecera = !!form.querySelector(".modal-head.tabbed .modal-tabs [data-tab]");
    const primeraActiva = form.querySelector("[data-tab=general]").classList.contains("on");
    const generalVisible = !form.querySelector('[data-panel="general"]').hidden;
    const lookOculto = form.querySelector('[data-panel="look"]').hidden;

    // Its own name, and a placeholder saying what it is called without one.
    const nombre = form.querySelector("[name=name]");
    const pideNombre = !form.querySelector("[data-for-name]").hidden;
    const sugerencia = nombre.placeholder;
    nombre.value = "Our projects";

    // The second tab holds how it looks, and nothing else does.
    form.querySelector("[data-tab=look]").click();
    await new Promise(r => setTimeout(r, 200));
    const trasCambiar = {
      general: form.querySelector('[data-panel="general"]').hidden,
      look: !form.querySelector('[data-panel="look"]').hidden,
      posicion: !form.querySelector("[data-for-label-at]").hidden,
    };

    // What is actually in that tab. It held one sentence saying there was
    // nothing there; it holds the controls the dashboard's own does: ten
    // colours and a default, and where the name goes drawn as four little
    // diagrams rather than written as four words.
    //
    // Four widths, not three: none is a choice, and it has to be one that can
    // be told apart from nobody having touched the control — which is what an
    // empty field means everywhere else here.
    const aspecto = {
      colores: form.querySelectorAll("[data-colour]").length,
      grosores: form.querySelectorAll("[data-width]").length,
      sinBorde: !!form.querySelector('[data-width="0"]'),
      // And whether the module has anything behind it at all.
      relleno: !!form.querySelector("[name=clear]"),
      posiciones: form.querySelectorAll("[data-marks] [data-at]").length,
      marcas: [...form.querySelectorAll("[data-marks] [data-at]")].every(b => b.querySelector("svg")),
      // And it sits on the name, joined to the box you type it in — not in the
      // tab about how the box looks. Measured as boxes, not as markup: the
      // first version of this asked whether the two were siblings, which stays
      // true however they are laid out, so laying them one above the other left
      // it green.
      juntoAlNombre: (() => {
        const boton = form.querySelector("[data-open-marks]")?.getBoundingClientRect();
        const caja = form.querySelector("#module-name")?.getBoundingClientRect();
        if (!boton || !caja) return false;
        const mismaFila = Math.abs((boton.top + boton.bottom) / 2 - (caja.top + caja.bottom) / 2) < 4;
        const pegados = Math.abs(boton.right - caja.left) < 3;
        return mismaFila && pegados;
      })(),
      // Their close is an outline on the header, not a filled button sitting on
      // it.
      fondoDelCierre: getComputedStyle(form.querySelector(".modal-close")).backgroundColor,
      // Its own name, not one this stylesheet already uses for something else.
      // The class named tabs here is a pill-shaped segmented control elsewhere,
      // and borrowing the name drew that pill around this header. Invisible to
      // a test for duplicate rules; visible only by looking.
      pildoraDeOtro: getComputedStyle(form.querySelector(".modal-tabs")).borderStyle !== "none",
      // The chosen tab wears the accent and the other does not. Against the
      // other tab rather than against a fixed colour: the accent is the
      // operator's, so a literal here would be measuring this installation.
      acento: getComputedStyle(form.querySelector("[data-tab=look]")).color,
      apagada: getComputedStyle(form.querySelector("[data-tab=general]")).color,
      subrayado: getComputedStyle(form.querySelector("[data-tab=look]"), "::after").content !== "none",
    };

    form.requestSubmit();
    await new Promise(r => setTimeout(r, 1000));

    const guardado = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === celda.dataset.module);
    const enPantalla = document.querySelector(
      '[data-module="' + celda.dataset.module + '"] .module-header h2')?.textContent.trim();

    return { pestanas, enCabecera, primeraActiva, generalVisible, lookOculto,
             pideNombre, sugerencia, trasCambiar, aspecto,
             nombreGuardado: guardado.name ?? "", enPantalla,
             acciones: form.querySelectorAll(".modal-foot button").length };
  `);
  check(
    "a module's settings are two tabs in the header, the way the dashboard's are",
    ajustes.pestanas.length === 2 && ajustes.enCabecera === true &&
      ajustes.primeraActiva === true && ajustes.generalVisible === true &&
      ajustes.lookOculto === true && ajustes.trasCambiar.general === true &&
      ajustes.trasCambiar.look === true && ajustes.trasCambiar.posicion === true &&
      ajustes.acciones === 1,
    JSON.stringify({ pestanas: ajustes.pestanas, acciones: ajustes.acciones }),
  );
  check(
    "the customization tab carries the dashboard's own controls",
    ajustes.aspecto?.colores === 11 && ajustes.aspecto?.grosores === 4 &&
      ajustes.aspecto?.sinBorde === true && ajustes.aspecto?.relleno === true &&
      ajustes.aspecto?.posiciones === 4 && ajustes.aspecto?.marcas === true &&
      ajustes.aspecto?.juntoAlNombre === true &&
      (ajustes.aspecto?.fondoDelCierre ?? "").replace(/\s/g, "") === "rgba(0,0,0,0)" &&
      ajustes.aspecto?.acento !== ajustes.aspecto?.apagada &&
      ajustes.aspecto?.subrayado === true &&
      ajustes.aspecto?.pildoraDeOtro === false,
    JSON.stringify(ajustes.aspecto),
  );
  check(
    "and a module is called what its operator calls it, not what it draws",
    ajustes.pideNombre === true && ajustes.sugerencia !== "" &&
      ajustes.nombreGuardado === "Our projects" && ajustes.enPantalla === "Our projects",
    JSON.stringify({ sugerencia: ajustes.sugerencia, guardado: ajustes.nombreGuardado,
                     enPantalla: ajustes.enPantalla }),
  );

  // Every module can be configured for what it is: the hero carries its own
  // words and decides whether to draw the mark, and a module that lists things
  // is told how many. It drew a fixed name, a fixed line and a fixed mark, and
  // the lists always showed as many as they had.
  const opciones = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: [
        { id: "top", kind: "hero", x: 1, y: 1, w: 6, h: 2 },
        { id: "list", kind: "boards", x: 7, y: 1, w: 6, h: 3 },
      ] }) });
    // Two more boards, so that "show one" is a different number from "show
    // them all". With the single board the journey already has, a list that
    // ignored the count entirely would read the same as one that obeyed it.
    const extras = ["second-board", "third-board"];
    for (const slug of extras) {
      await fetch("/api/orgs/codigosh/repositories", { method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
        body: JSON.stringify({ slug, name: slug }) });
    }
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1300));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    const abrir = async (id) => {
      const celda = document.querySelector('[data-module="' + id + '"]');
      celda.querySelector("[data-module-menu]").click();
      await new Promise(r => setTimeout(r, 200));
      celda.querySelector("[data-edit-module]").click();
      await new Promise(r => setTimeout(r, 500));
      return document.querySelector("form.module-config");
    };

    // The block that opens the page is written on the page, so the dialog does
    // NOT ask for its words — a field there would be a second place to change
    // the same thing, and the one that wins would be whichever was touched
    // last. What the dialog still offers it is what it is: a name, a border.
    const bloque = document.querySelector('[data-module="top"] [data-words-body]');
    bloque.focus();
    bloque.innerHTML = "<h1>Acme Support</h1><p>Tell us what broke.</p>";
    bloque.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise(r => setTimeout(r, 1200));

    let form = await abrir("top");
    const heroPide = {
      nombre: !form.querySelector("[data-for-name]").hidden,
      palabras: !!form.querySelector("[data-for-text], [name=body]"),
      marca: !!form.querySelector("[data-for-mark]"),
      filas: !form.querySelector("[data-for-rows]").hidden,
    };
    form.querySelector("[data-close]").click();
    await new Promise(r => setTimeout(r, 400));

    // And it survives the round trip through the server, which is where the
    // markup policy runs.
    const guardado = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === "top");
    const heroEnPantalla = {
      titulo: document.querySelector('[data-module="top"] .words-box h1')?.textContent?.trim() ?? "",
      linea: document.querySelector('[data-module="top"] .words-box p')?.textContent?.trim() ?? "",
      enElServidor: (guardado?.body ?? "").includes("Acme Support"),
    };

    // A list: told how many, and it shows that many.
    form = await abrir("list");
    const listaPide = {
      filas: !form.querySelector("[data-for-rows]").hidden,
      marca: !!form.querySelector("[data-for-mark]"),
    };
    // The count is asked for the way a phone asks: minus, the number, plus,
    // and no browser spinners inside the field. At the ends the buttons say so
    // rather than doing nothing.
    const grupo = form.querySelector("[data-for-rows]");
    const caja = grupo.querySelector("input[name=rows]");
    const menos = grupo.querySelector('[data-step="-1"]');
    const mas = grupo.querySelector('[data-step="1"]');
    const izquierda = (el) => Math.round(el.getBoundingClientRect().x);
    caja.value = "0";
    caja.dispatchEvent(new Event("input", { bubbles: true }));
    const contador = {
      orden: izquierda(menos) < izquierda(caja) && izquierda(caja) < izquierda(mas),
      sinFlechas: getComputedStyle(caja).appearance === "textfield",
      suelo: menos.disabled === true,
    };
    mas.click(); mas.click();
    contador.subeDe2 = caja.value === "2" && menos.disabled === false;
    menos.click();
    contador.baja = caja.value === "1";

    const cuenta = () => document.querySelectorAll('[data-module="list"] .cards .card').length;
    const sinLimite = cuenta();
    if (listaPide.filas) form.querySelector("[name=rows]").value = "1";
    form.requestSubmit();
    await new Promise(r => setTimeout(r, 1000));
    const filasEnPantalla = cuenta();

    document.querySelector("[data-edit-done]")?.click();
    for (const slug of extras) {
      await fetch("/api/orgs/codigosh/repos/" + slug, { method: "DELETE",
        headers: { "X-CSRF-Token": csrf() } });
    }
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: previa }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));

    return { heroPide, heroEnPantalla, listaPide, contador, sinLimite, filasEnPantalla };
  `);
  check(
    "the block that opens the page is written on the page, not in a dialog",
    opciones.heroPide.nombre === false && opciones.heroPide.palabras === false &&
      opciones.heroPide.marca === false && opciones.heroPide.filas === false &&
      opciones.heroEnPantalla.titulo === "Acme Support" &&
      opciones.heroEnPantalla.linea === "Tell us what broke." &&
      opciones.heroEnPantalla.enElServidor === true,
    JSON.stringify({ pide: opciones.heroPide, enPantalla: opciones.heroEnPantalla }),
  );
  // No border and nothing behind it, which is what lets a block of words read
  // as part of the page rather than as a card sitting on it. Counting the
  // controls says they are there; this says they do something.
  const desnudo = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: [{ id: "naked", kind: "text", x: 1, y: 1, w: 6, h: 3 }] }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1400));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 800));

    const celda = document.querySelector('[data-module="naked"]');
    const antes = getComputedStyle(celda.querySelector(".words-box"));
    const vestido = { borde: antes.borderTopWidth, fondo: antes.backgroundColor };


    celda.querySelector("[data-module-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    celda.querySelector("[data-edit-module]").click();
    await new Promise(r => setTimeout(r, 500));
    const form = document.querySelector("form.module-config");
    form.querySelector('[data-width="0"]').click();
    form.querySelector("[name=clear]").checked = true;
    form.requestSubmit();
    await new Promise(r => setTimeout(r, 1300));

    // Waited for, not slept at: the save redraws the page, and a box read
    // before the redraw is a detached element whose computed style is a row of
    // empty strings — which reads exactly like a border that was removed.
    let caja = null;
    for (let i = 0; i < 40 && !caja; i++) {
      caja = document.querySelector('[data-module="naked"] .words-box');
      if (caja && !caja.isConnected) caja = null;
      if (!caja) await new Promise(r => setTimeout(r, 100));
    }
    // Read into plain strings HERE, not at the return. getComputedStyle hands
    // back a LIVE declaration: held onto across the two redraws below, it
    // belongs to an element that is no longer in the document by the time it is
    // read, and every value comes back as an empty string — which reads exactly
    // like a border that was successfully removed.
    const ahora = caja
      ? (() => {
          const c = getComputedStyle(caja);
          return { borde: c.borderTopWidth, fondo: c.backgroundColor, sombra: c.boxShadow };
        })()
      : { borde: "?", fondo: "?", sombra: "?" };
    const guardado = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === "naked");

    // And the dialog comes back saying what was chosen rather than empty: none
    // is a choice, and a control that forgets it is a control that says
    // nothing was chosen.
    document.querySelector('[data-module="naked"] [data-module-menu]').click();
    await new Promise(r => setTimeout(r, 200));
    document.querySelector('[data-module="naked"] [data-edit-module]').click();
    await new Promise(r => setTimeout(r, 500));
    const reabierto = {
      ancho: document.querySelector("form.module-config [name=borderWidth]").value,
      relleno: document.querySelector("form.module-config [name=clear]").checked,
    };
    document.querySelector("#module-config [data-close]")?.click();
    await new Promise(r => setTimeout(r, 300));

    document.querySelector("[data-edit-done]")?.click();
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: previa }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1300));
    return { vestido, desnudo: ahora,
             servidor: { bare: guardado?.bare === true, clear: guardado?.clear === true },
             reabierto };
  `);
  check(
    "a module can be asked for no border and nothing behind it",
    desnudo.vestido.borde !== "0px" &&
      desnudo.desnudo.borde === "0px" &&
      desnudo.desnudo.fondo === "rgba(0, 0, 0, 0)" && desnudo.desnudo.sombra === "none" &&
      desnudo.servidor.bare === true && desnudo.servidor.clear === true &&
      // Nought is a choice and comes back as one, not as an untouched control.
      desnudo.reabierto.ancho === "0" && desnudo.reabierto.relleno === true,
    JSON.stringify(desnudo),
  );

  // A picture in a block of words, from the file to the page and back out.
  //
  // The conversion is the point rather than a detail: what an operator picks is
  // a photograph off a phone, and what a front page can afford is a fraction of
  // it. It is done in the browser because encoding WebP on the server would
  // mean a C library and a binary that is no longer one static file.
  const retrato = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: [{ id: "shot", kind: "text", x: 1, y: 1, w: 8, h: 5 }] }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1400));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 800));

    // A picture with detail in it, so that the encoder has something to do and
    // the two sizes are a real comparison rather than two ways of writing a
    // flat rectangle.
    const c = document.createElement("canvas");
    c.width = 900; c.height = 600;
    const x = c.getContext("2d");
    for (let i = 0; i < 900; i += 3) {
      x.fillStyle = "hsl(" + (i % 360) + ",70%," + (40 + (i % 30)) + "%)";
      x.fillRect(i, 0, 3, 600);
    }
    const png = await new Promise(r => c.toBlob(r, "image/png"));
    const webp = await new Promise(r => c.toBlob(r, "image/webp", 0.9));

    const subir = (blob, type) => fetch("/api/landing/images", { method: "POST",
      headers: { "X-CSRF-Token": csrf(), "Content-Type": type }, body: blob });

    // What is not WebP is refused on the bytes, not on the header it declared.
    const mentira = await subir(png, "image/webp").then(r => r.status);
    const vacia = await subir(new Blob([], { type: "image/webp" }), "image/webp").then(r => r.status);

    const hecha = await subir(webp, "image/webp");
    const { url } = await hecha.json();

    const caja = document.querySelector('[data-module="shot"] [data-words-body]');
    caja.focus();
    caja.innerHTML = '<p><img src="' + url + '" width="480" alt="A shot"></p>';
    caja.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise(r => setTimeout(r, 1500));
    const guardado = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === "shot")?.body ?? "";

    // Asked of the server, never of the browser: these are served immutable for
    // a year, so a plain fetch answers out of the cache whatever the server
    // still holds. That is how this measurement first said the sweep did
    // nothing while the table was already empty.
    const preguntar = (at) => fetch(at, { cache: "no-store" });
    const servida = await preguntar(url);
    const cabeceras = { estado: servida.status, tipo: servida.headers.get("content-type"),
                        cache: servida.headers.get("cache-control") };

    // And a picture nothing refers to any more goes: it was uploaded while
    // somebody was writing, so undoing has to take it with it.
    caja.innerHTML = "<p>No picture now.</p>";
    caja.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise(r => setTimeout(r, 1600));
    const trasQuitarla = await preguntar(url).then(r => r.status);

    document.querySelector("[data-edit-done]")?.click();
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: previa }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1300));
    return { png: png.size, webp: webp.size, mentira, vacia,
             creada: hecha.status, guardado, cabeceras, trasQuitarla };
  `);
  check(
    "a picture is stored as WebP, served for a year, and swept when nothing shows it",
    // Lighter than what went in, which is the whole reason for the conversion.
    retrato.webp < retrato.png &&
      retrato.creada === 201 && retrato.mentira === 400 && retrato.vacia === 400 &&
      retrato.guardado.includes('width="480"') && retrato.guardado.includes('alt="A shot"') &&
      retrato.guardado.includes('loading="lazy"') &&
      retrato.cabeceras.estado === 200 && retrato.cabeceras.tipo === "image/webp" &&
      retrato.cabeceras.cache.includes("immutable") &&
      retrato.trasQuitarla === 404,
    JSON.stringify({ pesos: { png: retrato.png, webp: retrato.webp },
                     subida: { creada: retrato.creada, noEsWebp: retrato.mentira, vacia: retrato.vacia },
                     guardado: retrato.guardado, cabeceras: retrato.cabeceras,
                     trasQuitarla: retrato.trasQuitarla }),
  );

  // The writing bar, driven the way a hand drives it.
  //
  // Colour and alignment are the two that cannot be copied from the dashboard:
  // this page is served with style-src 'self' and no unsafe-inline, so an
  // inline style written by an editing command is dropped by the browser
  // without a word — the button would look like it worked and change nothing.
  // They write classes instead, and the check is that the class is there AFTER
  // the round trip through the server, because the server has its own list and
  // a class it does not know is a class it removes.
  const escribiendo = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: [{ id: "pen", kind: "text", x: 1, y: 1, w: 12, h: 3 }] }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1300));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    const caja = document.querySelector('[data-module="pen"] [data-words-body]');
    const barraAntes = document.querySelector(".words-bar")?.hidden !== false;
    caja.focus();
    await new Promise(r => setTimeout(r, 250));
    const barra = document.querySelector(".words-bar");
    const aparece = barra.hidden === false;
    const sigue = caja.parentElement.contains(barra);

    caja.innerHTML = "<p>Ours</p>";
    const elegir = () => {
      const r = document.createRange();
      r.selectNodeContents(caja.querySelector("p"));
      const sel = getSelection(); sel.removeAllRanges(); sel.addRange(r);
    };
    elegir(); barra.querySelector('[data-do="bold"]').click();
    await new Promise(r => setTimeout(r, 150));
    elegir(); barra.querySelector('[data-do="ink:ink-accent"]').click();
    await new Promise(r => setTimeout(r, 150));
    elegir(); barra.querySelector('[data-do="align:align-center"]').click();
    await new Promise(r => setTimeout(r, 1400));

    // Nothing was written as an inline style, which the policy would have
    // thrown away: what the page holds is classes.
    const enLaPagina = caja.innerHTML;
    const conEstilo = caja.querySelectorAll("[style]").length;
    const guardado = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === "pen")?.body ?? "";

    document.querySelector("[data-edit-done]")?.click();
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: previa }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));
    return { barraAntes, aparece, sigue, conEstilo, enLaPagina, guardado };
  `);
  check(
    "the bar follows the cursor, and colour and alignment survive the policy",
    escribiendo.barraAntes === true && escribiendo.aparece === true &&
      escribiendo.sigue === true && escribiendo.conEstilo === 0 &&
      escribiendo.guardado.includes("align-center") &&
      escribiendo.guardado.includes("ink-accent") &&
      // Bold arrives from the browser as <b> and is stored in one dialect.
      escribiendo.guardado.includes("<strong>"),
    JSON.stringify({ barra: { antesOculta: escribiendo.barraAntes, aparece: escribiendo.aparece,
                              dentro: escribiendo.sigue },
                     estilosEnLinea: escribiendo.conEstilo, guardado: escribiendo.guardado }),
  );

  // And every one of them, not just the two above. The browser kept its own
  // idea of which modules carry a name and which can be pointed at a board,
  // and it had fallen three behind the server's: recent activity, the roadmap
  // and the figures were drawn from a board the operator was never offered a
  // way to choose. Two predicates in two languages drift in silence, so the
  // table is asserted whole rather than per module.
  const todos = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const previa = (await fetch("/api/landing").then(r => r.json())).modules;
    const kinds = ["hero", "text", "call_to_action", "organisations",
      "boards", "most_voted", "recently_done", "recent_activity", "roadmap",
      "statistics", "section"];
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: kinds.map((kind, i) => ({
        id: kind, kind, x: 1 + (i % 4) * 3, y: 1 + Math.floor(i / 4) * 2, w: 3, h: 2,
      })) }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1400));
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    const ofrece = {};
    for (const kind of kinds) {
      const celda = document.querySelector('[data-module="' + kind + '"]');
      celda.querySelector("[data-module-menu]").click();
      await new Promise(r => setTimeout(r, 150));
      celda.querySelector("[data-edit-module]").click();
      await new Promise(r => setTimeout(r, 350));
      const form = document.querySelector("form.module-config");
      ofrece[kind] = ["name", "label", "scope", "rows"]
        .filter(f => !form.querySelector("[data-for-" + f + "]").hidden).join(",");
      form.querySelector("[data-close]").click();
      await new Promise(r => setTimeout(r, 250));
    }

    document.querySelector("[data-edit-done]")?.click();
    await fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules: previa }) });
    history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1200));
    return ofrece;
  `);
  check(
    "and it is asked the way a phone asks, with the ends saying so",
    opciones.contador.orden === true && opciones.contador.sinFlechas === true &&
      opciones.contador.suelo === true && opciones.contador.subeDe2 === true &&
      opciones.contador.baja === true,
    JSON.stringify(opciones.contador),
  );

  const esperado: Record<string, string> = {
    // A block of words is asked nothing in the dialog but how its box looks.
    // Not even a name: it says what it is in its own words, at whatever size
    // the operator chose, and a header drawn above that would be the same
    // thing said twice in two type sizes.
    hero: "",
    text: "",
    call_to_action: "label,scope",
    organisations: "name,scope,rows",
    boards: "name,scope,rows",
    most_voted: "name,scope,rows",
    recently_done: "name,scope,rows",
    recent_activity: "name,scope,rows",
    roadmap: "name,scope,rows",
    statistics: "name,scope",
    section: "name",
  };
  const desviados = Object.keys(esperado)
    .filter((kind) => todos[kind] !== esperado[kind])
    .map((kind) => `${kind}: ${todos[kind]} (se esperaba ${esperado[kind]})`);
  check(
    "and every module is asked about what it is, the whole table at once",
    desviados.length === 0 &&
      // Every module that draws something out of the installation is asked
      // about it. The two blocks of words are the exception and are named as
      // one, rather than the rule being loosened until it says nothing: what
      // they show is typed into them, so there is nothing left to ask.
      Object.entries(todos).every(
        ([kind, ofrecido]) => ofrecido !== "" || kind === "hero" || kind === "text",
      ),
    desviados.length === 0 ? JSON.stringify(todos) : desviados.join("; "),
  );

  check(
    "and a module that lists things is asked how many, and shows that many",
    opciones.listaPide.filas === true && opciones.listaPide.marca === false &&
      opciones.sinLimite >= 2 && opciones.filasEnPantalla === 1,
    JSON.stringify({
      pide: opciones.listaPide,
      sinLimite: opciones.sinLimite,
      filas: opciones.filasEnPantalla,
    }),
  );

  // Removing asks first. It used to go on the click, and a module somebody
  // spent a minute pointing at the right board is not something to lose to a
  // stray one.
  const borrar = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));
    const antes = (await fetch("/api/landing").then(r => r.json())).modules.length;
    const celda = document.querySelector('.landing-cell[data-module^="statistics-"]')
      || document.querySelector('.landing-cell[data-module^="call_to_action-"]');
    const id = celda.dataset.module;

    celda.querySelector("[data-module-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    celda.querySelector("[data-remove-module]").click();
    await new Promise(r => setTimeout(r, 500));

    const dlg = document.querySelector("#confirm-remove");
    const pregunta = {
      abierto: dlg?.open === true,
      titulo: dlg?.querySelector("[data-confirm-title]")?.textContent?.trim() ?? "",
      mensaje: dlg?.querySelector("[data-confirm-message]")?.textContent?.trim() ?? "",
      acciones: dlg ? dlg.querySelectorAll(".confirm-actions button").length : 0,
      sigue: (await fetch("/api/landing").then(r => r.json())).modules.length,
    };

    // And it is composed the way theirs is, not the way the settings dialog is.
    // It was wearing that one's furniture: no padding of its own, a header with
    // no rule under it, a bigger bolder title, a bigger message, and the action
    // stretched across the whole width. Five differences, one wrong
    // composition.
    const cabecera = dlg.querySelector(".confirm-head");
    const titulo = dlg.querySelector(".confirm-title");
    const mensaje = dlg.querySelector(".modal-message");
    const accion = dlg.querySelector(".confirm-actions button");
    const cs = (el) => getComputedStyle(el);
    const forma = {
      relleno: cs(dlg).padding,
      cabeceraCentrada: cs(cabecera).alignItems === "center",
      reglaBajoTitulo: cs(cabecera).borderBottomWidth,
      titulo: cs(titulo).fontSize + "/" + cs(titulo).fontWeight,
      mensaje: cs(mensaje).fontSize,
      // Right-aligned and its own width, not stretched: the full-width outlined
      // action belongs to the dialog whose verb is "save", not to this one.
      alFinal: cs(dlg.querySelector(".confirm-actions")).justifyContent,
      accionEstrecha:
        accion.getBoundingClientRect().width < dlg.getBoundingClientRect().width / 2,
    };

    // Backing out changes nothing.
    dlg.querySelector("[data-close]").click();
    await new Promise(r => setTimeout(r, 500));
    const trasCerrar = (await fetch("/api/landing").then(r => r.json())).modules.length;

    // And confirming removes it.
    celda.querySelector("[data-module-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    celda.querySelector("[data-remove-module]").click();
    await new Promise(r => setTimeout(r, 400));
    document.querySelector("#confirm-remove [data-confirm]").click();
    await new Promise(r => setTimeout(r, 1100));
    const despues = (await fetch("/api/landing").then(r => r.json()));

    return { antes, pregunta, forma, trasCerrar, despues: despues.modules.length,
             sigueAhi: despues.modules.some(m => m.id === id) };
  `);
  check(
    "removing asks first, and backing out changes nothing",
    borrar.pregunta.abierto === true && borrar.pregunta.titulo !== "" &&
      borrar.pregunta.mensaje !== "" && borrar.pregunta.acciones === 1 &&
      borrar.pregunta.sigue === borrar.antes && borrar.trasCerrar === borrar.antes,
    JSON.stringify(borrar.pregunta),
  );
  check(
    "and it is composed the way theirs is, not the way the settings dialog is",
    borrar.forma.relleno === "28px" && borrar.forma.cabeceraCentrada === true &&
      borrar.forma.reglaBajoTitulo === "1px" && borrar.forma.titulo === "18px/600" &&
      borrar.forma.mensaje === "14px" && borrar.forma.alFinal === "flex-end" &&
      borrar.forma.accionEstrecha === true,
    JSON.stringify(borrar.forma),
  );
  check(
    "and confirming is what removes it",
    borrar.despues === borrar.antes - 1 && borrar.sigueAhi === false,
    JSON.stringify({ antes: borrar.antes, despues: borrar.despues }),
  );

  // A module's dialog, and its three ways out. The × and Cancel had no listener
  // at all on this page: the wiring that gives every dialog in the product its
  // close buttons ran on three panel screens and not on this one, so the only
  // way out of a module's settings was the keyboard.
  const salidas = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));

    const abrir = async () => {
      document.querySelector('.landing-cell[data-module^="boards-"] [data-module-menu]').click();
      await new Promise(r => setTimeout(r, 200));
      document.querySelector('.landing-cell[data-module^="boards-"] [data-edit-module]').click();
      await new Promise(r => setTimeout(r, 400));
      return document.querySelector("#module-config");
    };
    const abierto = () => document.querySelector("#module-config")?.open === true;

    const dlg = await abrir();
    const seAbre = abierto();
    dlg.querySelector(".modal-head [data-close]").click();
    await new Promise(r => setTimeout(r, 300));
    const trasLaEquis = abierto();

    await abrir();
    document.querySelector("#module-config [type=submit]").click();
    await new Promise(r => setTimeout(r, 900));
    const trasGuardar = abierto();

    return { seAbre, trasLaEquis, trasGuardar,
             // The dashboard's dialog has no cancel: the cross is the way out
             // that changes nothing, and one action at the foot is the action.
             cancelar: !!document.querySelector(".modal-foot [data-close]") };
  `);
  check(
    "a module's dialog closes by the cross and by saving, and offers no third way",
    salidas.seAbre === true && salidas.trasLaEquis === false &&
      salidas.trasGuardar === false && salidas.cancelar === false,
    JSON.stringify(salidas),
  );

  // The four places a name can go, driven through the dialog. The pill is the
  // one that matters most: it is the mark the operator may CHOOSE, and the
  // whole point is that the product never hands it out by itself.
  const donde = await evaluate(`
    // Back into edit mode: the block above left it, and the kebab that opens a
    // module's dialog only exists while the page is being arranged.
    document.querySelector("[data-edit-page]")?.click();
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
        // And when the name IS the pill, whether it is whole.
        //
        // This is the most fragile piece of the system, because it lives half
        // outside its own box: it has been cut in two twice, once by
        // specificity and once by the tile clipping. A pill two pixels short
        // looks exactly like one that is fine, so it is measured rather than
        // looked at — the clearance to the module's edge, which has to be
        // NEGATIVE because the whole point is that it sits on the line.
        montada: (() => {
          const pill = cell.querySelector(".section-legend");
          if (!pill) return null;
          const pb = pill.getBoundingClientRect();
          return {
            holgura: Math.round(pb.top - cb.top),
            alto: Math.round(pb.height),
            recorta: getComputedStyle(cell).overflow,
          };
        })(),
      };
    };

    const poner = async (valor) => {
      document.querySelector('.landing-cell[data-module^="boards-"] [data-module-menu]').click();
      await new Promise(r => setTimeout(r, 200));
      document.querySelector('.landing-cell[data-module^="boards-"] [data-edit-module]').click();
      await new Promise(r => setTimeout(r, 400));
      const form = document.querySelector("form.module-config");
      const ofrecido = !document.querySelector("[data-for-label-at]").hidden;
      form.querySelector("[data-open-marks]").click();
      await new Promise(r => setTimeout(r, 200));
      form.querySelector('[data-marks] [data-at="' + valor + '"]').click();
      await new Promise(r => setTimeout(r, 150));
      form.requestSubmit();
      await new Promise(r => setTimeout(r, 900));
      return { ofrecido, ...leer() };
    };

    return { arriba: leer(), abajo: await poner("bottom"),
             enPildora: await poner("pill"), sinNombre: await poner("off") };
  `);
  check(
    "and a name worn on the border is whole, not cut in half by its own module",
    donde.enPildora.montada !== null &&
      // Above the line by roughly half of itself, which is what "on the
      // border" means. Cut off would read as nought or worse.
      donde.enPildora.montada.holgura < 0 &&
      donde.enPildora.montada.alto > 8 &&
      // And the module lets it out, which is the one exception to a tile that
      // clips everything.
      donde.enPildora.montada.recorta === "visible",
    JSON.stringify(donde.enPildora.montada),
  );
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

    // Out again — and aimed at a cell that is TAKEN, which is what a real page
    // looks like. Refusing a colliding drop is right everywhere else, and here
    // it left the module trapped in the section for good: there was nowhere
    // free to aim at. Leaving always works.
    const encima = document.querySelector('.landing-cell[data-module="hero"]').getBoundingClientRect();
    const salida = await arrastrar("loose", { clientX: encima.left + 40, clientY: encima.top + 20 });
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
  const seccionModal = await evaluate(`
    document.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));
    const celda = document.querySelector('.landing-cell[data-module="group"]');
    celda.querySelector("[data-module-menu]").click();
    await new Promise(r => setTimeout(r, 200));
    celda.querySelector("[data-edit-module]").click();
    await new Promise(r => setTimeout(r, 500));
    const form = document.querySelector("form.module-config");
    const nombre = form.querySelector("[name=name]");
    const pideNombre = !form.querySelector("[data-for-name]").hidden;
    const valor = nombre.value;
    const marcas = [...form.querySelectorAll("[data-marks] [data-at]")].map(b => ({
      at: b.dataset.at, puede: !b.disabled, elegida: b.classList.contains("on") }));
    nombre.value = "Our work";
    form.querySelector("[data-tab=general]").click();
    form.requestSubmit();
    await new Promise(r => setTimeout(r, 1000));
    const guardada = (await fetch("/api/landing").then(r => r.json()))
      .modules.find(m => m.id === "group");
    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 600));
    return { pideNombre, valor, marcas, nombreGuardado: guardada.body ?? "",
             enLaPildora: document.querySelector(
               '.landing-cell[data-module="group"] .section-legend')?.textContent.trim() ?? "" };
  `);
  check(
    "a section is named in the same field and placed with the same marks",
    seccionModal.pideNombre === true && seccionModal.valor === "My projects" &&
      seccionModal.marcas.length === 4 &&
      seccionModal.marcas.find((m: { at: string }) => m.at === "pill")?.puede === true &&
      seccionModal.marcas.find((m: { at: string }) => m.at === "pill")?.elegida === true &&
      seccionModal.marcas.find((m: { at: string }) => m.at === "top")?.puede === false &&
      seccionModal.nombreGuardado === "Our work" &&
      seccionModal.enLaPildora === "Our work",
    JSON.stringify(seccionModal),
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

  // The bar above the page, which is the only thing a signed-in operator sees
  // on every public screen. Two things had to come off it and one had to go on.
  const barra = await evaluate(`
    const bar = document.querySelector("body > nav");
    const leer = () => ({
      salir: !!bar.querySelector("[data-signout]"),
      lapiz: !!bar.querySelector("[data-edit-page]"),
      hecho: !!bar.querySelector("[data-edit-done]"),
      enLaPagina: !!document.querySelector("#app [data-edit-page], #app [data-edit-done]"),
      panel: !!bar.querySelector("[data-panel-toggle]"),
    });

    // Tolerant on purpose. A mutation that stops the control changing leaves no
    // "done" button at all, and a probe that assumes one throws and takes the
    // whole pass down instead of saying which half broke.
    const quieto = leer();
    bar.querySelector("[data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 900));
    const componiendo = leer();
    const hecho = bar.querySelector("[data-edit-done]");
    const acento = hecho ? getComputedStyle(hecho).color : "";
    hecho?.click();
    await new Promise(r => setTimeout(r, 900));
    return { quieto, componiendo, acento, vuelta: leer() };
  `);
  check(
    "leaving is not in the bar of every public page",
    barra.quieto.salir === false && barra.componiendo.salir === false &&
      barra.quieto.panel === true,
    JSON.stringify(barra.quieto),
  );
  check(
    "and arranging the page is one control in that bar, not a button on the page",
    barra.quieto.lapiz === true && barra.quieto.hecho === false &&
      barra.componiendo.hecho === true && barra.componiendo.lapiz === false &&
      barra.quieto.enLaPagina === false && barra.componiendo.enLaPagina === false &&
      barra.vuelta.lapiz === true,
    JSON.stringify({ quieto: barra.quieto.lapiz, componiendo: barra.componiendo.hecho,
                     enLaPagina: barra.componiendo.enLaPagina, vuelta: barra.vuelta.lapiz }),
  );
  check(
    "and it wears the accent while the page is being arranged",
    barra.acento.replace(/\s/g, "") === "rgb(232,82,14)",
    barra.acento,
  );

  // One mode at a time, as Lastboard's bar does it: arranging the front page,
  // the panel and the review screen each grey out the other two and leave
  // only their own control answering — and nothing leaves the bar, so no icon
  // moves under the pointer when a mode starts. The panel's icon disappeared
  // to nobody and Review stayed live inside the panel, which is what this
  // measures: which answer, and where each one is.
  const modos: Record<string, any> = {};
  const leerBarra = `
    const bar = document.querySelector("body > nav");
    const uno = (sel) => {
      const el = bar.querySelector(sel);
      if (!el) return null;
      const r = el.getBoundingClientRect();
      return { x: Math.round(r.left), vivo: !el.classList.contains("cannot") && !el.disabled &&
               getComputedStyle(el).pointerEvents !== "none" };
    };
    return { lapiz: uno("[data-edit-page], [data-edit-done]"), panel: uno("[data-panel-toggle]"),
             revision: uno("[data-review]"),
             doc: document.documentElement.scrollHeight + "/" + innerHeight + "/" + document.documentElement.clientWidth + "/" + innerWidth,
             admin: document.querySelector(".admin") ? Math.round(document.querySelector(".admin").getBoundingClientRect().bottom) : 0 };`;
  modos.portada = await evaluate(leerBarra);
  modos.editando = await evaluate(`
    document.querySelector("body > nav [data-edit-page]")?.click();
    await new Promise(r => setTimeout(r, 700));
    const leer = () => { ${leerBarra} };
    const v = leer();
    document.querySelector("body > nav [data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 700));
    return v;
  `);
  await goTo("/panel", ".dash-card, .bento-card, .cards, .empty");
  modos.panel = await evaluate(leerBarra);
  await goTo("/review", ".rev-list, .rev-empty");
  modos.revision = await evaluate(leerBarra);
  await goTo("/", ".landing-grid");
  const vivos = (m: any) =>
    ["lapiz", "panel", "revision"].filter((k) => m?.[k]?.vivo).join(",");
  const presentes = Object.values(modos).every((m: any) => m.lapiz && m.panel && m.revision);
  const quietos = ["panel", "revision"].every((k) =>
    new Set([modos.portada, modos.panel, modos.revision].map((m: any) => m[k]?.x)).size === 1);
  check(
    "one mode at a time in the bar: the others greyed, none gone, none moved",
    presentes && quietos &&
      vivos(modos.portada) === "lapiz,panel,revision" &&
      vivos(modos.editando) === "lapiz" &&
      vivos(modos.panel) === "panel" &&
      vivos(modos.revision) === "revision",
    JSON.stringify(Object.fromEntries(Object.entries(modos).map(([k, m]) =>
      [k, `${vivos(m)} @ ${m.lapiz?.x}/${m.panel?.x}/${m.revision?.x} doc=${m.doc} admin=${m.admin}`]))) +
      ` presentes=${presentes} quietos=${quietos}`,
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

  // The three that only make sense on an installation with some history: what
  // arrived lately, what is coming, and how it is going. The last of those is
  // the one worth being careful with — three zeroes on a fresh installation
  // tell the first visitor that nothing happens here.
  const historia = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const escribir = (modules) => fetch("/api/landing", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ modules }) }).then(r => r.status);
    const dibujar = async () => {
      history.pushState({}, "", "/"); dispatchEvent(new PopStateEvent("popstate"));
      await new Promise(r => setTimeout(r, 1300));
    };

    await escribir([
      { id: "lately", kind: "recent_activity", x: 1, y: 1, w: 6, h: 3 },
      { id: "soon", kind: "roadmap", x: 7, y: 1, w: 6, h: 3 },
      { id: "figures", kind: "statistics", x: 1, y: 4, w: 6, h: 2 },
    ]);
    await dibujar();

    const conDatos = {
      recientes: document.querySelectorAll('[data-module="lately"] .brief').length,
      columnas: [...document.querySelectorAll('[data-module="soon"] .column-name')]
        .map(el => el.textContent.trim()),
      cifras: [...document.querySelectorAll('[data-module="figures"] .figure-number')]
        .map(el => el.textContent.trim()),
      // The numbers have to agree with the reports the page itself lists, or
      // they are three numbers from somewhere else.
      recibidos: Number(document.querySelector('[data-module="figures"] .figure-number')?.textContent ?? "0"),
    };

    // Now the same three on an installation where nothing has been published.
    // The scope names a board that is not there, which is the same thing to
    // every one of them: no boards, so nothing counted.
    await escribir([
      { id: "lately", kind: "recent_activity", x: 1, y: 1, w: 6, h: 3, scope: "codigosh/gone" },
      { id: "soon", kind: "roadmap", x: 7, y: 1, w: 6, h: 3, scope: "codigosh/gone" },
      { id: "figures", kind: "statistics", x: 1, y: 4, w: 6, h: 2, scope: "codigosh/gone" },
    ]);
    await dibujar();

    const enBlanco = {
      recientesVacio: !!document.querySelector('[data-module="lately"] .empty'),
      roadmapVacio: !!document.querySelector('[data-module="soon"] .empty'),
      cifras: document.querySelectorAll('[data-module="figures"] .figure-number').length,
      // And the statistics module keeps its place instead of collapsing.
      cajaEstadisticas: Math.round(
        document.querySelector('[data-module="figures"]').getBoundingClientRect().height),
    };
    return { conDatos, enBlanco };
  `);
  check(
    "the front page can show what arrived lately and what is coming",
    historia.conDatos.recientes >= 1 &&
      historia.conDatos.columnas.length >= 1 &&
      historia.conDatos.cifras.length === 3,
    JSON.stringify(historia.conDatos),
  );
  check(
    "and the numbers count what was actually published",
    historia.conDatos.recibidos >= historia.conDatos.recientes,
    JSON.stringify({ recibidos: historia.conDatos.recibidos, listados: historia.conDatos.recientes }),
  );
  check(
    "with nothing to count, no numbers are shown at all",
    historia.enBlanco.cifras === 0 &&
      historia.enBlanco.recientesVacio === true && historia.enBlanco.roadmapVacio === true &&
      historia.enBlanco.cajaEstadisticas > 10,
    JSON.stringify(historia.enBlanco),
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


  // Against what is stored, not against a number written here. This counted a
  // fixed eight and had to be edited every time another block added a module,
  // which means it was measuring the journey rather than the page. What it is
  // for is that everything arranged is drawn, and nothing else is.
  const frontPage = await evaluate(`
    const stored = await fetch("/api/landing").then(r => r.json());
    return {
      cells: document.querySelectorAll(".landing-cell").length,
      arranged: stored.modules.length,
      words: document.body.textContent.includes("We read everything that arrives."),
      hero: !!document.querySelector('[data-module^="hero"] .words-box'),
      orgs: !!document.querySelector(".landing-scroll"),
    };
  `);
  check(
    "and what was arranged is what a visitor is shown",
    frontPage.cells === frontPage.arranged && frontPage.words === true && frontPage.hero === true && frontPage.orgs === true,
    JSON.stringify(frontPage),
  );


  // 13. Right to left, which is where a stylesheet written in left and right
  //     falls apart.
  // Changed where somebody actually changes it, now that the top bar carries no
  // picker: under Appearance, which is also the path that stores the choice on
  // the account rather than in this browser.
  await goTo("/panel/appearance", ".admin-main");
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
  await goTo("/codigosh/lastboard", ".items, .empty");
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
  await goTo("/panel", ".dash-card, .bento-card, .cards, .empty");

  const overview = await evaluate(`
    return {
      cards: document.querySelectorAll("[data-card]").length,
      draggable: document.querySelectorAll("[draggable=true]").length,
      arrange: !!document.querySelector("[data-edit-dash], [data-hide], [data-show]"),
      stored: (await fetch("/api/me/dashboard")).status,
      // Every row of the lists keeps its state inside its card, with a title
      // long enough to push. The lists were grids whose one column took the
      // width of the longest row, so a long title shoved the state out of a
      // card that clips. The title is lengthened, measured and put back, so
      // nothing after this reads a different screen.
      desbordan: (() => {
        const titulo = document.querySelector(".briefs .brief-title");
        if (!titulo) return ["no rows"];
        const antes = titulo.textContent;
        titulo.textContent = "A title long enough to push anything beside it out of the card ".repeat(3);
        const fuera = [...document.querySelectorAll(".briefs .brief")].filter(b => {
          const card = b.closest(".dash-card")?.getBoundingClientRect();
          const chip = (b.querySelector(".status") ?? b).getBoundingClientRect();
          return card && (chip.left < card.left - 1 || chip.right > card.right + 1);
        }).map(b => b.textContent.trim().slice(0, 30));
        titulo.textContent = antes;
        return fuera;
      })(),
    };
  `);
  check(
    "the overview is one screen, the same for everybody",
    overview.cards === 13 && overview.draggable === 0 && overview.arrange === false,
    JSON.stringify({ ...overview, desbordan: undefined }),
  );
  check(
    "and every row of its lists keeps its state inside the card, however long the title",
    overview.desbordan.length === 0,
    JSON.stringify(overview.desbordan),
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
  await goTo("/review", ".rev-row, .rev-empty");

  const bandeja = await evaluate(`
    const filas = [...document.querySelectorAll("[data-pick]")];
    return {
      filas: filas.length,
      // Cada fila lleva dos líneas de lo que dice. La pantalla que esto
      // sustituye no enseñaba ninguna: para decidir si algo era basura había
      // que abrirlo, uno por uno.
      conAvance: filas.filter(f => (f.querySelector(".rev-p")?.textContent ?? "").trim().length > 10).length,
      // Agrupada por tablero, porque moderar ya es cambiar de contexto.
      grupos: document.querySelectorAll(".rev-group").length,
      // Un solo contador: lo que dice la barra es lo que hay dentro. La
      // pantalla anterior decía 19 arriba, 16 en una pestaña y 13 en la
      // siguiente, sumando reportes y mensajes en un número que las pestañas
      // de debajo contradecían.
      enLaBarra: Number(document.querySelector(".review-pill b")?.textContent ?? 0),
      enLaPestana: Number(document.querySelector(".rev-modes .count-chip")?.textContent ?? 0),
      // Y empieza BAJO la barra, que se queda: el control que abre esto es
      // también la salida.
      empiezaEn: Math.round(document.querySelector(".review").getBoundingClientRect().top),
      barraSigue: !!document.querySelector("body > nav"),
      lista: Math.round(document.querySelector(".rev-list").getBoundingClientRect().width),
    };
  `);
  check(
    "what is waiting is one screen, one counter, and a row you can decide without opening",
    bandeja.filas >= 1 && bandeja.conAvance === bandeja.filas &&
      bandeja.grupos >= 1 &&
      bandeja.enLaBarra === bandeja.filas && bandeja.enLaPestana === bandeja.filas &&
      bandeja.empiezaEn === 48 && bandeja.barraSigue === true,
    JSON.stringify(bandeja),
  );

  // Y fuera de la pantalla de revisión, la píldora dice que hay algo esperando
  // con el color de «en espera». Se leía gris: la regla de los enlaces de la
  // barra le ganaba por especificidad, y el fichero decía lo contrario.
  // Comparado contra el valor calculado del token, no contra una cadena.
  await goTo("/panel", ".side-item");
  const pildora = await evaluate(`
    const p = document.querySelector(".review-pill");
    // The token resolved by the browser: an element coloured with it through
    // CSSOM, which the page's policy does not govern.
    const sonda = document.createElement("i");
    sonda.style.color = "var(--hue-hold)";
    document.body.append(sonda);
    const esperado = getComputedStyle(sonda).color;
    sonda.remove();
    return {
      tieneTrabajo: p?.classList.contains("has") ?? false,
      color: p ? getComputedStyle(p).color : "(sin píldora)",
      esperado,
    };
  `);
  check(
    "outside the review screen the bar's pill wears the waiting colour when something waits",
    pildora.tieneTrabajo === true && pildora.color === pildora.esperado,
    JSON.stringify(pildora),
  );
  await goTo("/review", ".rev-row");
  await waitFor(".rev-detail .rev-title");

  // La ficha, al lado y entera: el cuerpo completo sin abrir otra pantalla.
  const ficha = await evaluate(`
    const det = document.querySelector(".rev-detail");
    // Lo que separa cada parte de la siguiente. Las respuestas no llevaban
    // margen debajo y las tres decisiones salían pegadas a su caja.
    const partes = [...(det.querySelector("[data-detail]")?.children ?? [])];
    const huecos = partes.slice(1).map((p, i) =>
      Math.round(p.getBoundingClientRect().top - partes[i].getBoundingClientRect().bottom));
    return {
      huecos,
      conRespuestas: !!det.querySelector(".answers"),
      titulo: det.querySelector(".rev-title")?.textContent?.trim() ?? "",
      cuerpo: (det.querySelector(".rev-full")?.textContent ?? "").trim().length,
      decisiones: det.querySelectorAll("[data-act]").length,
      sinDesplazar: det.scrollHeight <= det.clientHeight,
    };
  `);
  check(
    "and the one chosen is read beside it, whole, with its decision there",
    ficha.titulo !== "" && ficha.cuerpo > 20 && ficha.decisiones === 3 && ficha.sinDesplazar === true &&
      // Y cada parte separada de la siguiente, respuestas incluidas.
      ficha.conRespuestas === true && ficha.huecos.length >= 3 && ficha.huecos.every((h: number) => h >= 12),
    JSON.stringify(ficha),
  );

  // El teclado, que es lo que convierte una cola en un trabajo que se acaba.
  // Con teclas de verdad por el protocolo, no con eventos sintéticos.
  await goTo("/review?mode=all", ".rev-row");
  const antesDeTeclas = await evaluate(`
    return { filas: document.querySelectorAll("[data-pick]").length,
             elegido: document.querySelector(".rev-row.on")?.dataset.pick ?? "" };
  `);
  await page.send("Input.dispatchKeyEvent", { type: "keyDown", key: "ArrowDown", code: "ArrowDown", windowsVirtualKeyCode: 40 });
  await page.send("Input.dispatchKeyEvent", { type: "keyUp", key: "ArrowDown", code: "ArrowDown", windowsVirtualKeyCode: 40 });
  await sleep(400);
  const trasBajar = await evaluate(`
    return { elegido: document.querySelector(".rev-row.on")?.dataset.pick ?? "",
             enLaDireccion: new URLSearchParams(location.search).get("at") ?? "" };
  `);
  check(
    "the keyboard moves the selection, and the address follows what is being read",
    antesDeTeclas.filas >= 2 &&
      trasBajar.elegido !== "" && trasBajar.elegido !== antesDeTeclas.elegido &&
      // Lo que miras es lo que puedes enviar a alguien.
      trasBajar.enLaDireccion === trasBajar.elegido,
    JSON.stringify({ antes: antesDeTeclas, trasBajar }),
  );

  // Y una decisión, con la tecla, sobre lo que de verdad está esperando: la
  // fila se va sin repintar la pantalla, y el servidor lo confirma.
  await goTo("/review", ".rev-row");
  const antesDeDecidir = await evaluate(`
    return { filas: document.querySelectorAll("[data-pick]").length,
             elegido: document.querySelector(".rev-row.on")?.dataset.pick ?? "" };
  `);
  await page.send("Input.dispatchKeyEvent", { type: "keyDown", key: "p", code: "KeyP", text: "p", windowsVirtualKeyCode: 80 });
  await page.send("Input.dispatchKeyEvent", { type: "keyUp", key: "p", code: "KeyP", windowsVirtualKeyCode: 80 });
  await sleep(1100);
  const trasDecidir = await evaluate(`
    const pub = await fetch("/api/reports?moderation=published").then(r => r.json());
    return { filas: document.querySelectorAll("[data-pick]").length,
             enElServidor: pub.items.some(i => i.public_id === ${"`"}${"$"}{${JSON.stringify("x")}}${"`"} ) };
  `);
  check(
    "and a decision taken with a key removes the row without redrawing the screen",
    antesDeDecidir.elegido !== "" && trasDecidir.filas === antesDeDecidir.filas - 1,
    JSON.stringify({ antes: antesDeDecidir, despues: trasDecidir }),
  );

  // La dirección vieja sigue abriendo esto. Está en el historial de la gente y
  // en enlaces que se mandaron: devolver un 404 sería romperlos por un cambio
  // de sitio nuestro.
  await goTo("/panel/reports", ".rev-row, .rev-empty");
  const viejaPuerta = await evaluate(`return { donde: location.pathname };`);
  check(
    "the address the queue used to live at still opens it",
    viejaPuerta.donde === "/review",
    JSON.stringify(viejaPuerta),
  );

  // Y ya no está en el menú del panel: lo pendiente es un trabajo, no un sitio.
  await goTo("/panel", ".side-item");
  const menu = await evaluate(`
    return {
      sigueEnElMenu: [...document.querySelectorAll(".side-item")]
        .some(a => (a.getAttribute("href") ?? "").startsWith("/panel/reports")),
      pildoraEnLaBarra: !!document.querySelector(".review-pill"),
      // Nunca escondida por estar vacía: un control que aparece solo cuando hay
      // trabajo es un control que nadie aprende, y quita la forma de ver que
      // has terminado.
      numero: document.querySelector(".review-pill b")?.textContent?.trim() ?? "(ninguno)",
    };
  `);
  check(
    "reports left the panel menu, and the way in is the bar",
    menu.sigueEnElMenu === false && menu.pildoraEnLaBarra === true,
    JSON.stringify(menu),
  );

  // 11d. The archive. "Everything" used to be a list of fifty with no way to
  //      find anything in it — the screen it replaced had tabs, selects and a
  //      search box, and they went when it did. They are back, in the address
  //      like everything else on this screen.
  await goTo("/review?mode=all", ".rev-filter");
  // Waiting for the screen to be redrawn, not for the select to look chosen:
  // if the narrowing were ignored it would never look chosen, and the pass
  // would stop on a timeout instead of on this check.
  await evaluate(`
    document.getElementById("app")?.firstElementChild?.setAttribute("data-leaving", "1");
    const s = document.querySelector('[data-f="kind"]');
    s.value = "bug";
    s.dispatchEvent(new Event("change"));
  `);
  await waitFor("#app > :not([data-leaving]) .rev-filter");
  const porTipo = await evaluate(`
    return {
      puesto: document.querySelector('[data-f="kind"]')?.classList.contains("set") ?? false,
      direccion: new URLSearchParams(location.search).get("kind"),
      filas: document.querySelectorAll('[data-what="report"]').length,
      noSonFallos: document.querySelectorAll('[data-what="report"] .rev-dot:not(.bug)').length,
    };
  `);
  check(
    "the archive narrows by type, and the address says so",
    porTipo.puesto === true && porTipo.direccion === "bug" && porTipo.filas >= 1 &&
      porTipo.noSonFallos === 0,
    JSON.stringify(porTipo),
  );

  // Typing into the search box redraws the screen from the address, and the
  // cursor has to survive it — otherwise every pause throws you out of the box.
  await evaluate(`document.querySelector("[data-q]").focus()`);
  await page.send("Input.insertText", { text: "accent" });
  await waitFor('#app .rev-filter [data-q][value="accent"]');
  await sleep(300);
  const buscando = await evaluate(`
    const filas = [...document.querySelectorAll('[data-what="report"]')];
    return {
      direccion: new URLSearchParams(location.search).get("q"),
      filas: filas.length,
      todasCasan: filas.every(f => /accent/i.test(f.textContent)),
      sigueEscribiendo: document.activeElement?.matches?.("[data-q]") ?? false,
      // Y el tipo no se perdió al buscar: las dos cosas se suman.
      tipo: new URLSearchParams(location.search).get("kind"),
    };
  `);
  check(
    "a search finds what it says, keeps the other narrowing, and keeps the cursor",
    buscando.direccion === "accent" && buscando.filas >= 1 && buscando.todasCasan &&
      buscando.sigueEscribiendo === true && buscando.tipo === "bug",
    JSON.stringify(buscando),
  );

  // Every select's label fits, in the longest of the twenty languages. The
  // pass reads English, where three to a row fitted; Greek and Japanese did
  // not, and a select cuts its label short without a word. Measured against
  // the catalogues rather than by switching language, so nothing downstream
  // changes.
  const labelsFit = `
    const langs = ["ar","bn","de","el","en","es","fa","fr","hi","id","it","ja","ko","nl","pl","pt","ru","tr","ur","zh"];
    const cats = {};
    for (const l of langs) cats[l] = await fetch("/locales/" + l + ".json").then(r => r.json());
    const keys = { kind: "review.anyKind", moderation: "review.anyModeration", status: "review.anyStatus" };
    const cortas = [];
    let medidas = 0;
    for (const [f, key] of Object.entries(keys)) {
      // The control people see: the house dropdown's trigger, not the select
      // it stands over, which is out of sight.
      const sel = document.querySelector('[data-f="' + f + '"]')?.closest(".sel")?.querySelector(".sel-trigger");
      if (!sel) { cortas.push(f + ": no dropdown"); continue; }
      const cs = getComputedStyle(sel);
      const ctx = document.createElement("canvas").getContext("2d");
      ctx.font = cs.fontWeight + " " + cs.fontSize + " " + cs.fontFamily;
      // The chevron and its gap take 24 pixels of what the padding leaves.
      const room = sel.clientWidth - parseFloat(cs.paddingInlineStart) - parseFloat(cs.paddingInlineEnd) - 24;
      for (const l of langs) {
        medidas++;
        const w = ctx.measureText(cats[l][key]).width;
        if (w > room) cortas.push(l + " " + f + " " + Math.round(w) + ">" + Math.round(room));
      }
    }
    return { medidas, cortas };
  `;
  const caben = await evaluate(labelsFit);
  check(
    "every archive filter's label fits in all twenty languages",
    caben.medidas === 60 && caben.cortas.length === 0,
    JSON.stringify(caben),
  );

  // The house dropdown, not the browser's. No select is left on screen, and
  // the one that is drawn opens a list under its trigger, inside the window,
  // with every option — and a choice made in it is the select's choice.
  const desplegable = await evaluate(`
    const selects = [...document.querySelectorAll("select")];
    const nativasALaVista = selects.filter(s => {
      const r = s.getBoundingClientRect();
      return r.width > 1 || r.height > 1;
    }).length;
    const trigger = document.querySelector('[data-f="moderation"]')?.closest(".sel")?.querySelector(".sel-trigger");
    trigger?.click();
    await new Promise(r => setTimeout(r, 350));
    const lista = document.querySelector(".sel-list.open");
    const t = trigger?.getBoundingClientRect();
    const l = lista?.getBoundingClientRect();
    return {
      selects: selects.length,
      nativasALaVista,
      abierta: !!lista,
      opciones: lista?.querySelectorAll('[role="option"]').length ?? 0,
      esperadas: document.querySelector('[data-f="moderation"]')?.options.length ?? -1,
      debajo: l && t ? Math.round(l.top - t.bottom) : null,
      mismoAncho: l && t ? Math.abs(l.width - t.width) < 1 : false,
      dentro: l ? l.bottom <= innerHeight && l.left >= 0 && l.right <= innerWidth : false,
    };
  `);
  check(
    "the archive's dropdowns are the house's: no browser select on screen, a list under its trigger",
    desplegable.selects >= 3 && desplegable.nativasALaVista === 0 && desplegable.abierta === true &&
      desplegable.opciones === desplegable.esperadas && desplegable.opciones >= 2 &&
      desplegable.debajo !== null && desplegable.debajo >= 0 && desplegable.debajo <= 12 &&
      desplegable.mismoAncho === true && desplegable.dentro === true,
    JSON.stringify(desplegable),
  );
  await evaluate(`
    document.getElementById("app")?.firstElementChild?.setAttribute("data-leaving", "1");
    // By value, not by words: by this point the pass reads Arabic.
    const nativa = document.querySelector('[data-f="moderation"]');
    const i = [...nativa.options].findIndex(o => o.value === "published");
    document.querySelector('.sel-list.open [data-i="' + i + '"]')?.click();
  `);
  await waitFor("#app > :not([data-leaving]) .rev-filter");
  const elegida = await evaluate(`
    return {
      direccion: new URLSearchParams(location.search).get("moderation"),
      rotulo: document.querySelector('[data-f="moderation"]')?.closest(".sel")?.querySelector(".sel-value")?.textContent?.trim() ?? "",
      esperado: document.querySelector('[data-f="moderation"]')?.selectedOptions[0]?.textContent?.trim() ?? "(none)",
      encendida: document.querySelector('[data-f="moderation"]')?.closest(".sel")?.classList.contains("set") ?? false,
    };
  `);
  check(
    "and choosing from its list narrows the archive like the select would",
    elegida.direccion === "published" && elegida.rotulo !== "" && elegida.rotulo === elegida.esperado &&
      elegida.encendida === true,
    JSON.stringify(elegida),
  );

  // Scrollbars: Lastboard's thin, border-coloured one inside the product, and
  // none on the public pages, where the page is the product and a gutter down
  // its edge belongs to the operating system.
  const barraDeLaLista = await evaluate(`
    const lista = document.querySelector(".rev-list");
    const sonda = document.createElement("i");
    sonda.style.color = "var(--border)";
    document.body.append(sonda);
    const borde = getComputedStyle(sonda).color;
    sonda.remove();
    const cs = getComputedStyle(lista);
    return { ancho: cs.scrollbarWidth, color: cs.scrollbarColor, borde };
  `);
  await goTo("/", "#app > *");
  const enPublico = await evaluate(`return getComputedStyle(document.documentElement).scrollbarWidth`);
  await goTo("/panel", ".side-item");
  const enElPanel = await evaluate(`return getComputedStyle(document.documentElement).scrollbarWidth`);
  check(
    "scrollbars are the house's: thin and border-coloured inside, none on the public pages",
    barraDeLaLista.ancho === "thin" && barraDeLaLista.color.startsWith(barraDeLaLista.borde) &&
      enPublico === "none" && enElPanel !== "none",
    JSON.stringify({ lista: barraDeLaLista, enPublico, enElPanel }),
  );
  await goTo("/review?mode=all", ".rev-filter");

  // A narrowing that finds nothing says so, and offers the way back.
  await goTo("/review?mode=all&q=zzzz-nothing-says-this", ".rev-empty");
  const nada = await evaluate(`
    return {
      filas: document.querySelectorAll("[data-pick]").length,
      quitar: !!document.querySelector("[data-unfilter]"),
    };
  `);
  check(
    "an archive search that finds nothing says so and offers to clear it",
    nada.filas === 0 && nada.quitar === true,
    JSON.stringify(nada),
  );

  // How long each row has waited, short — with the day in the title. A queue
  // is read for which has waited longest, and a full date in that column made
  // you do the subtraction.
  await goTo("/review?mode=all", ".rev-row");
  const edad = await evaluate(`
    const w = [...document.querySelectorAll(".rev-w")];
    return {
      filas: w.length,
      largoMaximo: Math.max(...w.map(e => e.textContent.trim().length)),
      conFecha: w.filter(e => (e.getAttribute("title") ?? "").length > 6).length,
    };
  `);
  check(
    "each row says how long it has waited, short, with the date on hover",
    edad.filas >= 1 && edad.largoMaximo <= 6 && edad.conFecha === edad.filas,
    JSON.stringify(edad),
  );

  // The bar while the screen is open: the icon that opened it is the way out
  // and goes back where you were; the panel's toggle is greyed, one mode at a
  // time, as Lastboard greys its bar while its status page is up. Opened from
  // a public page: inside the panel the icon is greyed, one mode at a time.
  await goTo("/codigosh/lastboard", ".items, .empty");
  await evaluate(`document.querySelector(".review-pill").click()`);
  await waitFor(".review");
  const barraEnRevision = await evaluate(`
    const t = document.querySelector("[data-panel-toggle]");
    return {
      panelApagado: t?.classList.contains("cannot") ?? false,
      noResponde: t ? getComputedStyle(t).pointerEvents === "none" : false,
      salida: document.querySelector(".review-pill")?.getAttribute("href") ?? "",
    };
  `);
  await evaluate(`document.querySelector(".review-pill").click()`);
  await waitFor(".items, .empty");
  const vuelta = await evaluate(`return location.pathname`);
  check(
    "while reviewing, the bar's icon is the way back and the panel toggle is greyed",
    barraEnRevision.panelApagado === true && barraEnRevision.noResponde === true &&
      barraEnRevision.salida === "/codigosh/lastboard" && vuelta === "/codigosh/lastboard",
    JSON.stringify({ barra: barraEnRevision, vuelta }),
  );

  // A phone. The detail has no room beside the list, so a tapped row opens it
  // as a drawer from the trailing edge, and Escape closes the drawer before it
  // closes the screen. Measured on the box, not on a class: a drawer that is
  // "open" and still off the edge is the failure this is for.
  await page.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 1, mobile: true });
  await goTo("/review?mode=all", ".rev-row");
  const cabenEnElMovil = await evaluate(labelsFit);
  check(
    "and on a phone",
    cabenEnElMovil.medidas === 60 && cabenEnElMovil.cortas.length === 0,
    JSON.stringify(cabenEnElMovil),
  );
  // Off the screen means past the TRAILING edge, which depends on the
  // direction: by this point the pass reads Arabic, and the drawer rightly
  // comes in from the left.
  const cerrado = await evaluate(`
    const r = document.querySelector(".rev-detail").getBoundingClientRect();
    return { izquierda: Math.round(r.left), derecha: Math.round(r.right), ancho: innerWidth,
             dir: document.documentElement.dir || "ltr" };
  `);
  const fuera = (c: { izquierda: number; derecha: number; ancho: number }) =>
    cerrado.dir === "rtl" ? c.derecha <= 0 : c.izquierda >= c.ancho;
  await evaluate(`document.querySelector(".rev-row").click()`);
  await sleep(450);
  // Tolerant probes: if the drawer never opened, Escape leaves the screen and
  // there is no detail left to measure — that has to read as this check
  // failing, not as a script error with no name on it.
  const abierto = await evaluate(`
    const d = document.querySelector(".rev-detail");
    const r = d?.getBoundingClientRect();
    return {
      izquierda: Math.round(r?.left ?? NaN), derecha: Math.round(r?.right ?? NaN), ancho: innerWidth,
      titulo: d?.querySelector(".rev-title")?.textContent?.trim() ?? "",
      visible: d ? getComputedStyle(d).visibility : "(no detail)",
      // The close button can be pressed: what is at its centre is the button
      // itself. Drawn from the window's top edge, it sat under the top bar —
      // there, measured right, and impossible to press.
      cerrarAlcanzable: (() => {
        const b = d?.querySelector(".rev-shut")?.getBoundingClientRect();
        if (!b || b.width === 0) return false;
        return !!document.elementFromPoint(b.x + b.width / 2, b.y + b.height / 2)?.closest(".rev-shut");
      })(),
    };
  `);
  await page.send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 });
  await page.send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 });
  await sleep(450);
  const trasEscape = await evaluate(`
    const r = document.querySelector(".rev-detail")?.getBoundingClientRect() ?? { left: NaN, right: NaN };
    return { izquierda: Math.round(r.left), derecha: Math.round(r.right), ancho: innerWidth,
             donde: location.pathname };
  `);
  await page.send("Emulation.clearDeviceMetricsOverride", {});
  check(
    "on a phone a tapped row opens the detail as a drawer, and Escape closes it first",
    fuera(cerrado) &&
      abierto.izquierda >= 0 && abierto.derecha <= abierto.ancho && abierto.titulo !== "" &&
      abierto.visible === "visible" && abierto.cerrarAlcanzable === true &&
      fuera(trasEscape) && trasEscape.donde === "/review",
    JSON.stringify({ cerrado, abierto, trasEscape }),
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
  await goTo("/panel", ".dash-card, .bento-card, .cards, .empty");

  const collaborator = await evaluate(`
    const me = await fetch("/api/me").then(r => r.json());
    const value = (card) => Number(document.querySelector('[data-card="' + card + '"] .value')?.textContent ?? -1);
    return {
      groups: [...document.querySelectorAll(".side-label")].map(e => e.textContent.trim()),
      items: [...document.querySelectorAll(".side-item")].map(e => e.textContent.trim()),
      equipo: !!document.querySelector(".dashboard.team"),
      personas: !!document.querySelector('[data-card="people"]'),
      pendientes: value("waiting"),
      servidor: me.pending?.reports ?? -1,
      tableros: [...document.querySelectorAll('[data-card="boards"] a')].map(a => a.getAttribute("href")),
      offersToAdd: !!document.querySelector("[data-open-modal]"),
    };`);
  // The administrator's summary, over their boards: the same screen, so the
  // two read as one product, and none of the server's own numbers. What waits
  // is the number the bar counts, and the work per board is their board.
  check(
    "a collaborator's overview is the summary of their boards, not the server's",
    collaborator.equipo === true && collaborator.personas === false &&
      collaborator.pendientes === collaborator.servidor &&
      JSON.stringify(collaborator.tableros) === '["/panel/reports?board=codigosh/lastboard"]',
    JSON.stringify(collaborator),
  );
  check(
    "and offers nothing they would be refused",
    collaborator.groups.length === 2 && collaborator.offersToAdd === false,
    `${collaborator.groups.join(" · ")} — ${collaborator.items.join(", ")}`,
  );

  // 12a. And the administrator's screens are not theirs by address either.
  //      The menu never offered them, but typing /panel/about drew the screen
  //      from the server's refusal: a version of "—" and "Could not reach the
  //      release server", which was false. Each one sends them to the
  //      overview now, and the organisation wizard with them.
  const ajenas: string[] = [];
  for (const ruta of ["/panel/about", "/panel/people", "/panel/personalization", "/panel/backups", "/panel/new"]) {
    await goTo(ruta, ".admin-main > *");
    await sleep(300);
    const r = await evaluate(`return { donde: location.pathname,
      falso: document.body.textContent.includes("Could not reach the release server") }`);
    if (r.donde !== "/panel" || r.falso) ajenas.push(`${ruta} → ${r.donde}${r.falso ? " (false message)" : ""}`);
  }
  // Adding a repository is the owner's: a collaborator on one board is sent
  // back to the organisation's page instead of into its wizard.
  await goTo("/panel/codigosh/new", ".admin-main > *");
  await sleep(300);
  const asistente = await evaluate(`return location.pathname`);
  if (asistente !== "/panel/codigosh") ajenas.push(`/panel/codigosh/new → ${asistente}`);
  check(
    "a collaborator who types an administrator's address is sent to their overview",
    ajenas.length === 0,
    JSON.stringify(ajenas),
  );

  // 12b. And their listing is theirs. The screen is the same one the
  //      administrator uses, and the whole question is whether what it shows
  //      comes from the roles this account holds rather than from the address
  //      it was asked for.
  await goTo("/review?mode=all", ".rev-row, .rev-empty");
  const theirs = await evaluate(`
    return {
      filas: document.querySelectorAll("[data-pick]").length,
      // Un solo grupo, porque tienen un solo tablero: lo que se ve sale de los
      // roles de la cuenta y no de la dirección que se pidió.
      grupos: [...document.querySelectorAll(".rev-group")].map(g => g.textContent.trim().replace(/\s+\d+$/, "")),
    };
  `);
  check(
    "a collaborator's review holds their board and nothing else",
    theirs.filas > 0 && theirs.grupos.length === 1 && theirs.grupos[0].includes("Lastboard"),
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

  // 12d. A repository's page in the panel, as the collaborator sees it: what
  //      is waiting and the way to it, and nothing they would be refused.
  //      Checked before the administrator's view, because the page is drawn
  //      from what the server says each reader may do — and a page that shows
  //      the owner's cards to everybody looks exactly right to the owner.
  await goTo("/panel/codigosh/repos/lastboard", ".admin-main .settings");
  const suyo = await evaluate(`
    return {
      tarjetas: document.querySelectorAll(".admin-main .settings-card").length,
      revision: document.querySelector("[data-repo-review]")?.getAttribute("href") ?? "",
      exportar: document.querySelector("[data-export]")?.getAttribute("href") ?? "",
      formularios: document.querySelectorAll(".admin-main form").length,
    };
  `);
  check(
    "a collaborator's repository page is what waits there and the way to it, and nothing else",
    suyo.tarjetas === 1 && suyo.formularios === 0 &&
      suyo.revision.includes("board=codigosh%2Flastboard") &&
      suyo.exportar === "/api/orgs/codigosh/repos/lastboard/export",
    JSON.stringify(suyo),
  );

  // 12d'. The organisation's page, as the same collaborator: where they work
  //       and the way into it, and none of the owner's sections.
  await goTo("/panel/codigosh", ".admin-main .settings");
  const suOrg = await evaluate(`
    return {
      tarjetas: document.querySelectorAll(".admin-main .settings-card").length,
      filas: [...document.querySelectorAll(".repositories .settings-item")].map(li => li.dataset.repo),
      formularios: document.querySelectorAll(".admin-main form").length,
      equipo: !!document.querySelector(".settings-list.team"),
    };
  `);
  check(
    "a collaborator's organisation page is the boards they work on, and nothing of the owner's",
    suOrg.tarjetas === 2 && JSON.stringify(suOrg.filas) === '["lastboard"]' &&
      suOrg.formularios === 0 && suOrg.equipo === false,
    JSON.stringify(suOrg),
  );

  // 12d''. The list of organisations, as the same collaborator: the ones they
  //        may open, with what they are there, and no dialog they could not
  //        send. The public list named every organisation, and a collaborator
  //        was offered pages that refuse them.
  await goTo("/panel/organizations", ".admin-main .settings");
  const suLista = await evaluate(`
    return {
      filas: [...document.querySelectorAll(".organizations .settings-item")].map(li => li.dataset.org),
      rol: document.querySelector('[data-org="codigosh"] [data-role]')?.dataset.role ?? "",
      acciones: document.querySelectorAll(".organizations .si-actions button").length,
      dialogos: document.querySelectorAll(".admin-main dialog, dialog form").length,
    };
  `);
  check(
    "a collaborator's list of organisations is the ones they may open, as what they are there",
    JSON.stringify(suLista.filas) === '["codigosh"]' && suLista.rol === "collaborator" &&
      suLista.acciones === 0 && suLista.dialogos === 0,
    JSON.stringify(suLista),
  );

  // 12e. And as whoever runs it: every card, reached from the organisation's
  //      row, and each one does what it says. The team is changed here and put
  //      back as it was, so nothing downstream counts a different number.
  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    await fetch("/api/session", { method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ username: "alex", password: "una contraseña larga de verdad" }) });
  `);
  // The list first: what waits in each organisation is the number Review
  // gives for its boards, and a forge with a token stored is not flagged.
  await goTo("/panel/organizations", ".organizations");
  const lista = await evaluate(`
    const review = await fetch("/api/reports?moderation=pending&limit=200").then(r => r.json());
    const enRevision = (review.items ?? []).filter(r => r.board?.org === "codigosh").length;
    const tag = document.querySelector('[data-org="codigosh"] [data-pending]');
    return {
      pendientes: tag ? Number(tag.dataset.pending) : 0,
      enRevision,
      rol: document.querySelector('[data-org="codigosh"] [data-role]')?.dataset.role ?? "",
      forja: document.querySelector('[data-org="withforge"] [data-forge]')?.dataset.forge ?? "",
      sinConectar: !!document.querySelector('[data-org="withforge"] [data-connected="false"]'),
    };
  `);
  check(
    "the list of organisations says what waits in each, as Review counts it, and which forge is connected",
    lista.pendientes === lista.enRevision && lista.rol === "owner" &&
      lista.forja === "forgejo" && lista.sinConectar === false,
    JSON.stringify(lista),
  );

  await goTo("/panel/codigosh", ".repositories");
  // The organisation in sections, in the order the work goes, and what is
  // waiting in each board said where the board is listed — the same number
  // the board's own page gives, or nothing when nothing waits.
  const secciones = await evaluate(`
    const tarjeta = (sel) => [...document.querySelectorAll(".admin-main .settings-card")]
      .findIndex(c => c.querySelector(sel));
    const panel = await fetch("/api/orgs/codigosh/repos/lastboard/panel").then(r => r.json());
    const tag = document.querySelector('[data-repo="lastboard"] [data-pending]');
    return {
      orden: [".repositories", ".settings-list.team", "form.org-settings", "[data-delete-org]"].map(tarjeta),
      pendientes: tag ? Number(tag.dataset.pending) : 0,
      servidor: panel.pending,
      alex: document.querySelector('.team [data-member="alex"] [data-role]')?.dataset.role ?? "",
      quitarseASiMismo: !!document.querySelector('[data-revoke-owner="alex"]'),
      ana: document.querySelector('.team [data-member="ana"] [data-role]')?.dataset.role ?? "",
    };
  `);
  check(
    "an organisation's page is in sections, says what waits in each board, and shows its team",
    JSON.stringify(secciones.orden) === "[1,2,3,4]" && secciones.pendientes === secciones.servidor &&
      secciones.alex === "owner" && secciones.quitarseASiMismo === false && secciones.ana === "collaborator",
    JSON.stringify(secciones),
  );

  // An owner added and taken off from the page, and the team back as it was.
  const duenos = await evaluate(`
    window.confirm = () => true;
    const f = document.querySelector("form.owner-add");
    f.querySelector("#owner-who").value = "ana";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    const dueña = !!document.querySelector('[data-revoke-owner="ana"]');
    document.querySelector('[data-revoke-owner="ana"]')?.click();
    await new Promise(r => setTimeout(r, 1100));
    return { dueña, vuelta: document.querySelector('.team [data-member="ana"] [data-role]')?.dataset.role ?? "" };
  `);
  check(
    "an owner is added and taken off from the organisation's page",
    duenos.dueña === true && duenos.vuelta === "collaborator",
    JSON.stringify(duenos),
  );
  // What a repository takes, as the marks of its kinds and not as words; each
  // named, because the picture is all a screen reader would otherwise get.
  const marcas = await evaluate(`
    const fila = [...document.querySelectorAll(".repositories .settings-item")]
      .find(li => li.textContent.includes("Lastboard"));
    return {
      marcas: [...(fila?.querySelectorAll("[data-accepts]") ?? [])].map(m => m.dataset.accepts + ":" + (m.getAttribute("aria-label") ? "named" : "unnamed")),
      conIcono: [...(fila?.querySelectorAll("[data-accepts]") ?? [])].every(m => !!m.querySelector("svg")),
      palabras: [...(fila?.querySelectorAll(".chip") ?? [])].length,
    };
  `);
  check(
    "a repository's row says what it takes with the kinds' marks, each one named",
    JSON.stringify(marcas.marcas) === '["bug:named","idea:named"]' && marcas.conIcono && marcas.palabras === 0,
    JSON.stringify(marcas),
  );
  await evaluate(`[...document.querySelectorAll(".repositories .si-main")]
    .find(a => a.textContent.includes("Lastboard"))?.click()`);
  await waitFor(".admin-main form.repo-settings");
  const pagina = await evaluate(`
    const rol = (u) => document.querySelector('[data-member="' + u + '"] [data-role]')?.dataset.role ?? "";
    // Each card by what it holds, not by its title: the language this pass
    // runs in at this point is not English.
    const tarjeta = (sel) => [...document.querySelectorAll(".admin-main .settings-card")]
      .findIndex(c => c.querySelector(sel));
    return {
      donde: location.pathname,
      tarjetas: document.querySelectorAll(".admin-main .settings-card").length,
      orden: ["[data-repo-review]", "form.repo-settings", ".settings-list.team", "form.channel", "[data-delete-repo]"]
        .map(tarjeta),
      alex: rol("alex"),
      ana: rol("ana"),
      quitarAna: !!document.querySelector('[data-revoke="ana"]'),
      quitarAlex: !!document.querySelector('[data-revoke="alex"]'),
    };
  `);
  check(
    "a repository's row opens its page, with its settings, its team, its channel and its danger zone",
    pagina.donde === "/panel/codigosh/repos/lastboard" &&
      pagina.tarjetas === 5 && JSON.stringify(pagina.orden) === "[0,1,2,3,4]" &&
      pagina.alex === "owner" && pagina.ana === "collaborator" &&
      pagina.quitarAna === true && pagina.quitarAlex === false,
    JSON.stringify(pagina),
  );

  // Taking a collaborator off and putting them back, from the page.
  const equipo = await evaluate(`
    window.confirm = () => true;
    document.querySelector('[data-revoke="ana"]').click();
    await new Promise(r => setTimeout(r, 1100));
    const fuera = !document.querySelector('[data-member="ana"]');
    const ofrecida = [...document.querySelectorAll("#team-who option")].map(o => o.value).includes("ana");
    const f = document.querySelector("form.team-add");
    f.querySelector("#team-who").value = "ana";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    return { fuera, ofrecida,
             vuelta: document.querySelector('[data-member="ana"] [data-role]')?.dataset.role ?? "" };
  `);
  check(
    "a collaborator is taken off and added back from the repository's page",
    equipo.fuera === true && equipo.ofrecida === true && equipo.vuelta === "collaborator",
    JSON.stringify(equipo),
  );

  // The channel: chosen here, said back as chosen, and never read back.
  const canal = await evaluate(`
    const f = document.querySelector("form.channel");
    const kind = f.querySelector("#chan-kind");
    kind.value = "ntfy";
    kind.dispatchEvent(new Event("change"));
    f.querySelector("#chan-url").value = "https://ntfy.example/drive-topic";
    f.querySelector("#chan-token").value = "tk_drive";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    const ahora = document.querySelector("[data-channel-now]")?.textContent ?? "";
    const elegido = document.querySelector("#chan-kind")?.value ?? "";
    const url = document.querySelector("#chan-url")?.value ?? "";
    // Back to none, so nothing afterwards tries to deliver anywhere.
    const csrf = document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/orgs/codigosh/repos/lastboard/channel", { method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: JSON.stringify({ kind: "" }) });
    return { ahora, elegido, url };
  `);
  check(
    "the channel is set from the repository's page and said back without what it stores",
    canal.ahora.includes("ntfy") && canal.elegido === "ntfy" && canal.url === "",
    JSON.stringify(canal),
  );

  // What it accepts, saved from the page and read back from the server.
  const acepta = await evaluate(`
    const f = document.querySelector("form.repo-settings");
    f.querySelector("[name=accepts_ideas]").checked = false;
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 1100));
    const r = await fetch("/api/orgs/codigosh/repos/lastboard/panel").then(r => r.json());
    const marcada = document.querySelector("form.repo-settings [name=accepts_ideas]")?.checked;
    const csrf = document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/orgs/codigosh/repos/lastboard", { method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
      body: JSON.stringify({ name: r.name, forge_repo: r.forge_repo, accepts_bugs: true, accepts_ideas: true }) });
    return { ideas: r.accepts_ideas, bugs: r.accepts_bugs, marcada };
  `);
  check(
    "what a repository accepts is saved from its page",
    acepta.ideas === false && acepta.bugs === true && acepta.marcada === false,
    JSON.stringify(acepta),
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
      crumb: [...document.querySelectorAll(".crumb a")].map(a => a.getAttribute("href")),
      comentar: !!document.querySelector("form.comment-form"),
      equipo: !!document.querySelector(".item-team, [data-rewrite], form.team-reply"),
    };
  `);
  check(
    "a visitor on the same address gets the page, not the application",
    visitor.sidebar === false && visitor.publicNav !== "none" && visitor.report === true,
    JSON.stringify(visitor),
  );
  check(
    "and a crumb to the public board, a box to comment in, and none of the team's controls",
    JSON.stringify(visitor.crumb) === '["/codigosh","/codigosh/lastboard"]' &&
      visitor.comentar === true && visitor.equipo === false,
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
