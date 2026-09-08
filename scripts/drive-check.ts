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
    const h = document.querySelector(".wizard-head h1").getBoundingClientRect();
    const box = document.querySelector(".wizard").getBoundingClientRect();
    return Math.abs((h.left - box.left) - (box.right - h.right)) < 2;
  `);
  check("the wizard's title is centred", centred === true);

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
  await evaluate(`document.querySelector("li[data-comment] [data-comment-act='published']").click()`);
  await sleep(700);

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
      mark: !!document.querySelector(".signin-head .brand-logo"),
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
    const same = await evaluate(`
      return {
        pick: !!document.querySelector(".avatar-pick"),
        old: [...document.querySelectorAll("button")]
          .filter(b => /Elige|Choose|Quitar|Remove/.test(b.textContent)).length,
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
  // 11b. The dashboard, arranged. It is the one screen somebody sees every day,
  //      so it is the one worth being able to change — and a saved arrangement
  //      that does not survive the next visit is worse than none.
  await evaluate(`history.pushState({}, "", "/panel"); dispatchEvent(new PopStateEvent("popstate"));`);
  await sleep(1300);

  const dashboard = await evaluate(`
    const before = [...document.querySelectorAll("[data-card]")].map(c => c.dataset.card);
    const draggableWhileReading = document.querySelectorAll("[data-card][draggable=true]").length;

    document.querySelector("[data-edit-dash]").click();
    await new Promise(r => setTimeout(r, 900));
    const draggableWhileEditing = document.querySelectorAll("[data-card][draggable=true]").length;

    // Drag the first card past the last one, the way a pointer would.
    const list = document.querySelector(".dashboard");
    const cards = [...list.querySelectorAll("[data-card]")];
    const first = cards[0], last = cards[cards.length - 1];
    const dt = new DataTransfer();
    first.dispatchEvent(new DragEvent("dragstart", { bubbles: true, dataTransfer: dt }));
    const box = last.getBoundingClientRect();
    last.dispatchEvent(new DragEvent("dragover", { bubbles: true, dataTransfer: dt, clientY: box.bottom - 2 }));
    list.dispatchEvent(new DragEvent("drop", { bubbles: true, dataTransfer: dt }));
    await new Promise(r => setTimeout(r, 1200));

    const saved = (await (await fetch("/api/me/dashboard")).json()).cards;
    return { before, draggableWhileReading, draggableWhileEditing, saved };
  `);

  check(
    "the dashboard is not draggable until you say so",
    dashboard.draggableWhileReading === 0 && dashboard.draggableWhileEditing > 0,
    `${dashboard.draggableWhileReading} reading, ${dashboard.draggableWhileEditing} editing`,
  );
  check(
    "a card dragged to the end stays there",
    dashboard.saved?.[dashboard.saved.length - 1] === dashboard.before[0],
    `${dashboard.before[0]} → last of ${JSON.stringify(dashboard.saved)}`,
  );

  // The slot a card would land in, painted green because the accent means
  // "selected" everywhere else in this interface and this one means "release".
  const ghost = await evaluate(`
    const cards = [...document.querySelectorAll("[data-card]")];
    const dt = new DataTransfer();
    cards[0].dispatchEvent(new DragEvent("dragstart", { bubbles: true, dataTransfer: dt }));
    const box = cards[2].getBoundingClientRect();
    cards[2].dispatchEvent(new DragEvent("dragover", { bubbles: true, cancelable: true, dataTransfer: dt,
      clientX: box.left + box.width * 0.9, clientY: box.top + box.height / 2 }));
    const el = document.querySelector(".dash-ghost");
    const style = el ? getComputedStyle(el) : null;
    const out = { shown: !!el, colour: style?.borderColor ?? "", style: style?.borderStyle ?? "",
                  sourceStays: cards[0].classList.contains("dragging") };
    document.querySelector(".dashboard").dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer: dt }));
    return out;
  `);
  check(
    "dragging shows the slot the card would land in",
    ghost.shown && ghost.colour.includes("34, 197, 94") && ghost.style === "dashed" && ghost.sourceStays,
    JSON.stringify(ghost),
  );

  const hidden = await evaluate(`
    document.querySelector("[data-hide]").click();
    await new Promise(r => setTimeout(r, 1200));
    return (await (await fetch("/api/me/dashboard")).json()).cards;
  `);
  check(
    "and a card hidden is remembered",
    Array.isArray(hidden) && hidden.length === dashboard.saved.length - 1,
    `${dashboard.saved.length} → ${hidden.length}`,
  );

  // 11c-bis. A dashboard arranged before the charts existed still gets them.
  //
  //          This is the house rule that a new feature has to reach the people
  //          who were already here: a saved layout is the list of visible
  //          cards, so a card added later is missing from it and would never
  //          be drawn again for anybody who had ever pressed Arrange. The
  //          payload below is what one of those layouts looks like — no record
  //          of what existed when it was saved — and the new cards have to
  //          appear at the end of it without the hidden one coming back.
  const inherited = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim())
      .find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/me/dashboard", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ cards: ["reports", "waiting", "published", "votes", "kinds", "statuses", "recent"] }),
    });
    history.pushState({}, "", "/panel");
    dispatchEvent(new PopStateEvent("popstate"));
    await new Promise(r => setTimeout(r, 1400));
    return [...document.querySelectorAll("[data-card]")].map(c => c.dataset.card);
  `);
  check(
    "a dashboard arranged before the charts existed still gets them",
    inherited.includes("activity") && inherited.includes("top") && inherited.includes("boards") &&
      !inherited.includes("moderation") && inherited[0] === "reports",
    JSON.stringify(inherited),
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

  // 11e. Every number leads to the list it counts. A dashboard whose tiles are
  //      not links is a dashboard that answers "how many" and then leaves you
  //      to find them yourself.
  // Out of arranging first: while the dashboard is being rearranged the tiles
  // are deliberately not links, because a card you meant to drag should not
  // navigate. Reading the href without leaving that mode measures the mode.
  // Out of arranging first: while the dashboard is being rearranged the tiles
  // are deliberately not links, because a card you meant to drag should not
  // navigate. Reading the href without leaving that mode measures the mode.
  //
  // And read as a set rather than by name: the checks above dragged one card to
  // the end and hid whichever was first, so naming a card here would be
  // asserting the result of the previous test.
  const tiles = await evaluate(`
    document.querySelector("[data-edit-done]")?.click();
    await new Promise(r => setTimeout(r, 900));
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
      rows: document.querySelectorAll(".report").length,
      waiting: document.querySelector('[data-moderation="pending"] .count-chip')?.textContent?.trim() ?? "",
      decisions: document.querySelectorAll(".report .row-actions").length,
      boards: [...new Set([...document.querySelectorAll(".report .chip")].map(c => c.textContent.trim()))],
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

  await evaluate(`document.querySelector('[data-moderation="pending"]').click()`);
  await sleep(1200);
  const narrowed = await evaluate(`
    return { search: location.search, rows: document.querySelectorAll(".report").length };
  `);
  check(
    "narrowing it goes into the address, so a view can be sent to somebody",
    narrowed.search.includes("moderation=pending") && narrowed.rows === 1,
    `${narrowed.search} → ${narrowed.rows} rows`,
  );

  await evaluate(`document.querySelector('.report [data-act="published"]').click()`);
  await sleep(1400);
  const triaged = await evaluate(`
    return { rows: document.querySelectorAll(".report").length,
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
      rows: document.querySelectorAll(".report").length,
      boards: [...new Set([...document.querySelectorAll(".report .chip")].map(c => c.textContent.trim()))],
      // One board, so there is nothing to choose between and no control for it.
      boardPicker: !!document.querySelector("[data-board]"),
    };
  `);
  check(
    "a collaborator's listing holds their board and nothing else",
    theirs.rows > 0 && theirs.boards.length === 1 && theirs.boards[0] === "Lastboard" && theirs.boardPicker === false,
    JSON.stringify(theirs),
  );

  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));

  await fetch(`http://127.0.0.1:${CDP}/json/close/${t.id}`);
  page.close();
}

await main();

export {};
