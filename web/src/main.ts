/**
 * The application shell: routing, fetching, and putting the templates on screen.
 *
 * Rendering is deliberately per-view rather than one big redraw. Reassigning the
 * whole container on every interaction destroys and recreates every child — the
 * entrance animations replay, a focused field loses focus, and a list that was
 * scrolled jumps back to the top. This house has measured that three times.
 */
import { api, ApiError } from "./api";
import { askConfirm } from "./confirm";
import type {
  ItemDetail,
  ItemSummary,
  Me,
  OrganizationSummary,
  RepositorySummary,
  Status,
  Submitted,
} from "./api";
import { i18n } from "./services/i18n";
import { availableLocales } from "./locales/metadata";
import { applyTheme, currentTheme, restoreTheme, type Theme } from "./services/theme";
import { escapeAttr, escapeHTML } from "./escape";
import { toast, toastError } from "./services/toast";
import { type Fit, ICON, shrink, TooLarge } from "./services/image";
import { PICTURES_ON_A_MESSAGE, PICTURES_ON_A_REPORT, removePicture, sendPictures, wireTray } from "./services/reportPictures";
import { attachPictureViewer } from "./services/pictureViewer";
import { attachChartHover } from "./services/chartHover";
import { setRule } from "./services/dynamicStyles";
import { watchSelects } from "./services/select";
import { attachPasswordToggles } from "./services/password";
import * as views from "./views/templates";
import * as landingViews from "./views/landing";
import { LandingEditor } from "./landingEditor";
import { openUpdateDialog, type UpdateInfo } from "./updateModal";
import { icons } from "./views/icons";

const text: views.Text = {
  t: (key, params) => i18n.t(key, params),
  markup: (key, markup) => i18n.markup(key, markup),
  date: (value) => i18n.date(value),
  dateTime: (value) => i18n.dateTime(value),
  number: (value) => i18n.number(value),
  bytes: (value) => i18n.bytes(value),
  since: (seconds) => i18n.since(seconds),
  age: (seconds) => i18n.age(seconds),
};

const root = document.getElementById("app");
const nav = document.getElementById("nav");

let me: Me | null = null;

/** The address last drawn, so the review screen knows where it was opened from. */
let lastRendered = "";

/** Where somebody was before opening the review screen: leaving goes back there. */
let reviewCameFrom = "/panel";

/**
 * What this installation calls itself.
 *
 * Read once when a panel page is drawn and kept: it is on every screen, and
 * asking the server for it on each navigation would be a request for a string
 * that changes about once in the life of an install.
 */
let instanceName = "";

/** The logo beside it, if the operator chose one. */
let instanceLogo = "";

/** The accent this installation chose, applied to the document root. */
let instanceAccent = "";

// --- routing ---------------------------------------------------------------

/** Navigate without a reload, so the language and the scroll survive. */
function go(path: string): void {
  history.pushState({}, "", path);
  void render();
}

document.addEventListener("click", (event) => {
  // A link something else already answered — the front page editor keeps the
  // links on the page it is editing from going anywhere.
  if (event.defaultPrevented) return;
  const link = (event.target as HTMLElement | null)?.closest?.("a");
  if (!link) return;
  const href = link.getAttribute("href");
  // The feed is served by the server, not by this router: intercepting it
  // would show a blank page where a reader expects XML.
  // The feed and the export are served by the server, not by this router:
  // intercepting either shows a blank page where a file was expected.
  if (
    !href?.startsWith("/") ||
    href.startsWith("/feed/") ||
    href.startsWith("/api/") ||
    link.hasAttribute("target")
  ) {
    return;
  }
  event.preventDefault();
  go(href);
});

window.addEventListener("popstate", () => void render());

// A picture on a report opens large over the page; see pictureViewer.ts.
attachPictureViewer({
  markup: () => views.pictureViewer(text),
  count: (n, of) => text.t("pictures.alt", { n: text.number(n), count: text.number(of) }),
});

// --- the shell -------------------------------------------------------------

/**
 * What the last update check found, for an administrator.
 *
 * Checked once, a moment after the panel's owner arrives,
 * and when something newer exists the bar's panel icon carries a red dot and
 * Info in the menu says so — the update is noticed without going to look.
 */
let updateFound: UpdateInfo | null = null;
let updateChecked = false;

function watchForUpdate(): void {
  if (updateChecked || me?.admin !== true) return;
  updateChecked = true;
  setTimeout(async () => {
    const status = await api.get<views.UpdateStatus>("/api/update").catch(() => null);
    if (status) noteUpdate(status);
  }, 2000);
}

/** Records a check's answer and shows it wherever the house shows it. */
function noteUpdate(status: views.UpdateStatus): void {
  updateFound = {
    current: status.current ?? "",
    latest: status.latest,
    available: status.available === true,
    container: status.container === true,
  };
  renderNav();
  const info = root?.querySelector<HTMLAnchorElement>('.side-item[href="/panel/about"]');
  const badge = info?.querySelector(".side-update");
  if (updateFound.available && info && !badge) info.insertAdjacentHTML("beforeend", views.sideUpdateBadge(text));
  if (!updateFound.available) badge?.remove();
}

function renderNav(): void {
  if (!nav) return;
  watchForUpdate();

  // No language picker here. A visitor who has not chosen one is already
  // reading their browser's language, and somebody who wants a different one
  // sets it once — in the wizard on the first run, or under Appearance — rather
  // than being offered a list of twenty on every page of the product.
  /**
   * Arranging the front page, from the bar above it.
   *
   * One control that turns into the other: a pencil while the page is just a
   * page, a tick in the accent
   * colour while it is being arranged. It used to be a button inside the page
   * itself, which put a control for the operator in the middle of the thing
   * every visitor sees.
   *
   * Only on the front page, and only for somebody who can change it.
   */
  const composing = me?.admin === true && location.pathname === "/" && landingEditing;
  // Arranging the front page, the panel and the review screen are exclusive:
  // while one is on, every other control in the bar is shown and greyed out, and only
  // the one that leads back out answers. Shown, not removed — a bar whose
  // icons come and go moves under the pointer, and has to be read again.
  const inPanel = document.body.classList.contains("admin-view");
  const inReview = location.pathname === "/review";
  // The pencil arranges the front page, so it answers there and nowhere else;
  // on any other page it stays in its place, greyed.
  const pencilOff = inPanel || inReview || location.pathname !== "/";
  const compose =
    me?.admin === true
      ? composing
        ? // While arranging: a plus that opens a short menu, and the tick that
          // finishes. Adding is two decisions,
          // a module or a group, and only the first needs a picker.
          `<div class="add-wrap">
             <button class="icon-button" data-add-menu
                     title="${escapeAttr(i18n.t("landing.add"))}"
                     aria-label="${escapeAttr(i18n.t("landing.add"))}"
                     aria-haspopup="menu" aria-expanded="false">${icons.add(16)}</button>
             <div class="add-menu" role="menu" hidden>
               <button type="button" class="add-menu-item" role="menuitem" data-editor-add="module">
                 ${icons.module(14)}<span>${escapeHTML(i18n.t("landing.addModule"))}</span></button>
               <button type="button" class="add-menu-item" role="menuitem" data-editor-add="section">
                 ${icons.section(14)}<span>${escapeHTML(i18n.t("landing.addSection"))}</span></button>
             </div>
           </div>
           <button class="icon-button composing" data-edit-done
                   title="${escapeAttr(i18n.t("action.done"))}"
                   aria-label="${escapeAttr(i18n.t("action.done"))}">${icons.check(16)}</button>`
        : `<button class="icon-button${pencilOff ? " cannot" : ""}" data-edit-page
                   ${pencilOff ? "disabled" : ""}
                   title="${escapeAttr(i18n.t("landing.compose"))}"
                   aria-label="${escapeAttr(i18n.t("landing.compose"))}">${icons.edit(16)}</button>`
      : "";

  // The panel is somewhere to go, so it is a button. Leaving is NOT here: the
  // panel's own sidebar carries it, and a red control in the bar of every
  // public page is the one thing on that bar nobody came for.
  // The panel's own control, which opens it and closes it.
  //
  // A box whose side column fills while the thing is open, so the icon says which state you are in rather than what
  // clicking will do. Disabled while the front page is being arranged — one
  // mode at a time, and a control that does something else mid-gesture is a
  // control somebody hits by accident.
  const panelOff = composing || inReview;
  const panel = me
    ? `<button class="icon-button panel-toggle${panelOff ? " cannot" : ""}" data-panel-toggle
               ${panelOff ? "disabled" : ""}
               aria-pressed="${inPanel}"
               title="${escapeAttr(i18n.t("nav.panel"))}"
               aria-label="${escapeAttr(i18n.t("nav.panel"))}">
         <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
              aria-hidden="true">
           <rect x="3" y="3" width="18" height="18" rx="2" stroke-width="1.5" />
           <rect class="panel-indicator${inPanel ? " on" : ""}" x="15" y="3" width="6" height="18" />
           <path d="M15 3V21" stroke-width="1.5" />
         </svg>
         ${me.admin && updateFound?.available ? '<span class="notification-dot" aria-hidden="true"></span>' : ""}
       </button>`
    : "";

  /**
   * What is waiting, and the way into it.
   *
   * Shown to whoever may triage something — a collaborator on a repository,
   * the owner of an organisation, and the administrator, who may triage every
   * board. It once hung off memberships alone, so that the person who
   * moderates daily was not left out for not being the administrator — and
   * that left out the administrator instead, who holds no membership: the
   * screen that is now where every report is worked on had no door for them.
   *
   * Never hidden when the queue is empty. Hiding it when a feature is off
   * would be a decision of installation; a control that comes and goes with
   * the work is one nobody learns is there,
   * and it takes away the only way to see that you are finished.
   *
   * The count is the server's, already narrowed to the boards this account may
   * open — the page has no way to know which those are, and a number it worked
   * out itself would be a second opinion on a permission.
   */
  const canTriage = me?.admin === true || (me?.memberships?.length ?? 0) > 0;
  const waiting = (me?.pending?.reports ?? 0) + (me?.pending?.messages ?? 0);
  const review =
    me && canTriage
      ? // While the screen is open this is its way out, the way the status
        // icon closes a full-screen page: it goes back where you were.
        // Greyed while the front page is arranged, with no address to follow:
        // a link that only looks disabled still opens from the keyboard.
        //
        // Live inside the panel, the one departure from one mode at a time,
        // decided with the user: the reports are the team's daily
        // work and the panel is where the team spends the day, so the door
        // to them answers from there, leaving the panel as it opens.
        `<a class="review-pill${inReview ? " on" : ""}${waiting > 0 ? " has" : ""}${composing ? " cannot" : ""}"
             ${composing ? 'aria-disabled="true" tabindex="-1"' : `href="${escapeAttr(inReview ? reviewCameFrom : "/review")}"`} data-review
             title="${escapeAttr(i18n.t("review.title"))}"
             aria-label="${escapeAttr(i18n.t("review.title"))}">
           ${icons.queue(16)}${waiting > 0 ? `<b>${escapeHTML(text.number(waiting))}</b>` : ""}
         </a>`
      : "";

  // The queue first, then the page's own controls beside the panel's: editing
  // the front page and opening the panel are both about this installation, so
  // they sit together, and what is waiting stands a step apart from them.
  const account = me
    ? `${review}${compose}${panel}`
    : // Not on the sign-in page itself: a link to where you already are is the
      // third thing on that screen saying the same word.
      location.pathname === "/sign-in"
      ? ""
      : `<a class="button small" href="/sign-in">${escapeHTML(i18n.t("nav.signIn"))}</a>`;

  // The name the operator chose, on the public side too. Personalising an
  // installation and then seeing "LastHelp" on the page everybody else opens is
  // a setting that did not take.
  // The bar's contents share the measure of the page under it. Pinned to the
  // window's own edges, the brand floated a long way from the first word of the
  // page it names, which is most of what made these screens read as unaligned.
  nav.innerHTML = `<div class="nav-inner">
      <a class="brand" href="/">
        ${views.brandMark(instanceLogo)}
        <span>${escapeHTML(instanceName || i18n.t("app.name"))}</span>
      </a>
      <div class="nav-right">${account}</div>
    </div>`;

  nav.querySelector<HTMLButtonElement>("[data-edit-page]")?.addEventListener("click", () => void startEditing());
  nav.querySelector<HTMLButtonElement>("[data-edit-done]")?.addEventListener("click", () => void stopEditing());
  const adder = nav.querySelector<HTMLButtonElement>("[data-add-menu]");
  const menu = nav.querySelector<HTMLElement>(".add-menu");
  adder?.addEventListener("click", (event) => {
    event.stopPropagation();
    if (!menu) return;
    menu.hidden = !menu.hidden;
    adder.setAttribute("aria-expanded", String(!menu.hidden));
    if (!menu.hidden) {
      document.addEventListener(
        "click",
        () => {
          menu.hidden = true;
          adder.setAttribute("aria-expanded", "false");
        },
        { once: true },
      );
    }
  });
  nav.querySelectorAll<HTMLButtonElement>("[data-editor-add]").forEach((item) =>
    item.addEventListener("click", () => {
      if (menu) menu.hidden = true;
      editor?.addFromBar(item.dataset.editorAdd === "section" ? "section" : "module");
    }),
  );

  nav.querySelector<HTMLButtonElement>("[data-signout]")?.addEventListener("click", async () => {
    await api.del("/api/session").catch(() => undefined);
    me = null;
    go("/");
  });
}

/**
 * The front page, and whether it is being arranged.
 *
 * Kept in module scope because editing is a mode the page is in, not a thing
 * one render knows about: what the editor changes has to survive a redraw.
 */
let landing: landingViews.LandingPage = { bands: [], settings: {} };
let landingContent: landingViews.LandingContent = {};
let landingEditing = false;
let editor: LandingEditor | null = null;

/** Opens the editor on the front page. */
async function startEditing(): Promise<void> {
  if (editor || !root) return;
  landingEditing = true;
  document.body.classList.remove("admin-view");
  editor = new LandingEditor({
    root,
    text,
    instance: () => ({ name: instanceName, logo: instanceLogo }),
    report,
    toast: (message) => toast(message),
    toastError,
    renderNav,
  });
  try {
    await editor.start();
  } catch (err) {
    editor = null;
    landingEditing = false;
    report(err);
    await fetchLanding();
    drawLanding();
  }
}

/**
 * Leaves the editor. What was typed last is saved first, so the page drawn
 * afterwards is the one just edited — the same page visitors get.
 */
async function stopEditing(redraw = true): Promise<void> {
  const leaving = editor;
  editor = null;
  landingEditing = false;
  await leaving?.stop();
  if (!redraw) return;
  await fetchLanding();
  drawLanding();
}

/**
 * What this installation's front page is, and what goes in it.
 *
 * The published page for everybody; the draft, when there is one, for whoever
 * is arranging it. Failing softly: an unreachable page leaves the default
 * rather than an error, because this is the one screen a stranger sees first.
 */
async function fetchLanding(): Promise<void> {
  type Composed = landingViews.LandingPage & {
    content?: landingViews.LandingContent;
    boards?: landingViews.LandingBoard[];
  };
  const composed = await api
    .get<Composed>("/api/landing")
    .catch((): Composed => ({ bands: [], settings: {} }));
  landing = { bands: composed.bands ?? [], settings: composed.settings ?? {} };
  landingContent = composed.content ?? {};
}

function drawLanding(): void {
  show(
    landingViews.landingPage(
      landing,
      { instance: { name: instanceName, logo: instanceLogo }, content: landingContent, editing: landingEditing },
      text,
    ),
  );
  // The bar carries the control that turns arranging on and off, so it is
  // redrawn when that changes.
  renderNav();
  wireLandingSearch();
}

/**
 * The search on the front page: reports that already exist, as somebody types,
 * and always a last line to write a new one — so finding nothing is a step
 * forward rather than a dead end.
 *
 * Waits a moment after the last key rather than asking on every one, and drops
 * an answer that arrives after a newer question: typing fast must not end with
 * the results of three letters ago.
 */
function wireLandingSearch(): void {
  root?.querySelectorAll<HTMLFormElement>("[data-landing-search]").forEach((form) => {
    const input = form.querySelector<HTMLInputElement>("input[name=q]");
    const results = form.querySelector<HTMLElement>(".lp-results");
    if (!input || !results) return;
    let waiting: ReturnType<typeof setTimeout> | undefined;
    let asked = 0;

    const close = () => {
      results.hidden = true;
      results.innerHTML = "";
    };
    form.addEventListener("submit", (e) => e.preventDefault());
    input.addEventListener("keydown", (e) => {
      if (e.key === "Escape") close();
    });
    input.addEventListener("input", () => {
      clearTimeout(waiting);
      const q = input.value.trim();
      if (q.length < 2) return close();
      waiting = setTimeout(async () => {
        const mine = ++asked;
        const found = await api
          .get<{ items: landingViews.LandingSearchHit[]; boards: landingViews.LandingBoard[] }>(
            `/api/search?q=${encodeURIComponent(q)}`,
          )
          .catch(() => ({ items: [], boards: [] }));
        if (mine !== asked) return;
        results.innerHTML = landingViews.searchResults(found.items ?? [], found.boards ?? [], q, text);
        results.hidden = false;
      }, 250);
    });
    document.addEventListener("click", (e) => {
      if (!form.contains(e.target as Node)) close();
    });
  });
}

function show(html: string): void {
  document.body.classList.remove("admin-view");
  // And leaving it: the toggle has to stop saying the panel is open.
  renderNav();
  if (root) root.innerHTML = html;
  // Every password field on whatever was just drawn, rather than a toggle
  // written into the one form somebody asked about and missing from the rest.
  attachPasswordToggles(root);
}

/**
 * Draw a panel page inside the administration shell.
 *
 * The sidebar needs the organisations and, when you are inside one, its boards,
 * so this fetches them once for every panel page rather than each screen
 * building its own navigation and drifting from the others.
 */
async function showInShell(content: string, path: string): Promise<void> {
  const { organizations } = await api
    .get<{ organizations: OrganizationSummary[] }>("/api/orgs")
    .catch(() => ({ organizations: [] as OrganizationSummary[] }));

  const orgs: views.SideOrganization[] = organizations ?? [];
  const inside = orgs.find((o) => path.startsWith(`/panel/${o.slug}`) || path.startsWith(`/${o.slug}/`));
  if (inside) {
    const body = await api
      .get<{ repositories: RepositorySummary[] }>(`/api/orgs/${encodeURIComponent(inside.slug)}/repos`)
      .catch(() => ({ repositories: [] as RepositorySummary[] }));
    inside.repos = (body.repositories ?? []).map((r) => ({ slug: r.slug, name: r.name }));
  }

  if (root) {
    root.innerHTML = views.adminShell(
      {
        path,
        organizations: orgs,
        who: me?.username ?? "",
        role: views.accountRole(me, text),
        isAdmin: me?.admin === true,
        mayAddOrganization: me?.admin === true,
        avatar: me?.avatar,
        waiting: me?.pending,
        updateAvailable: updateFound?.available === true,
        content,
      },
      text,
    );
  }
  // Arriving at the panel, it comes in from the side the reading direction ends
  // on. Only on arrival: moving
  // between panel screens is not an arrival, and replaying it on every click
  // would be a shudder rather than an entrance.
  const arriving = !document.body.classList.contains("admin-view");
  document.body.classList.add("admin-view");
  renderNav();
  if (arriving) {
    const shell = root?.querySelector<HTMLElement>(".admin");
    shell?.classList.add("arriving");
    shell?.addEventListener("animationend", () => shell.classList.remove("arriving"), {
      once: true,
    });
  }
  attachPasswordToggles(root);
  wireShell();
}

/**
 * The name, the mark and the colour this installation wears.
 *
 * Read once and kept: it is on every screen, public and private, and asking on
 * every navigation would be a request for three strings that change about once
 * in the life of an install.
 */
let instanceLoaded = false;

async function loadInstance(): Promise<void> {
  if (instanceLoaded) return;
  instanceLoaded = true;
  const instance = await api
    .get<{ name: string; logo: string; accent: string }>("/api/instance")
    .catch(() => null);
  if (!instance) return;
  instanceName = instance.name;
  instanceLogo = instance.logo;
  instanceAccent = instance.accent;
  applyAccent(instanceAccent);
}

/** The controls that live in the shell rather than in any one screen. */
function wireShell(): void {
  root?.querySelector<HTMLButtonElement>(".side [data-signout]")?.addEventListener("click", async () => {
    await api.del("/api/session").catch(() => undefined);
    me = null;
    instanceName = "";
    go("/");
  });
}

/**
 * A chosen image, scaled and encoded into something that fits in a row.
 *
 * It used to measure the file and refuse it, which failed twice over: no
 * photograph a phone takes is under the limit, and the number it compared was
 * the file's while the server bounds the data URL — about a third longer — so a
 * file that passed here could still be refused there.
 */
async function readAsDataURL(file: File | undefined, fit?: Fit): Promise<string | null> {
  if (!file) return null;
  try {
    // Scaled and re-encoded rather than measured and refused. Every photograph
    // a phone takes is over the limit, and telling the person setting up a
    // server to go and find a smaller picture is not a feature.
    return await shrink(file, fit);
  } catch (err) {
    // A file this browser cannot decode — an HEIC from an iPhone, say — is not
    // a file that is too large, and saying so sends somebody off to shrink an
    // image that was never the problem.
    showFormError(new ApiError(400, err instanceof TooLarge ? "error.image_too_big" : "error.image_unreadable"));
    return null;
  }
}

/**
 * A repository's icon, chosen in whichever form is on the page — the one that
 * creates a repository or the one that edits it. The picture goes into the
 * form's hidden `icon` field and the preview changes in place; the form then
 * sends it with the rest. Until an icon is chosen the preview is the initials
 * of the name being typed, which is what the repository will show.
 */
function wireIconPicker(changed?: (icon: string) => void): void {
  for (const button of root?.querySelectorAll<HTMLButtonElement>("[data-pick-icon]") ?? []) {
    const box = button.closest(".icon-field");
    const form = button.closest("form");
    const file = box?.querySelector<HTMLInputElement>("[data-icon-file]");
    const value = box?.querySelector<HTMLInputElement>('input[name="icon"]');
    const clear = box?.querySelector<HTMLButtonElement>("[data-clear-icon]");
    const name = form?.querySelector<HTMLInputElement>('input[name="name"]');
    const paint = (icon: string) => {
      if (value) value.value = icon;
      if (clear) clear.hidden = icon === "";
      const preview = button.querySelector(".icon-preview");
      if (preview) preview.outerHTML = views.markOf(name?.value.trim() || "?", icon, "card-mark icon-preview");
      changed?.(icon);
    };
    button.addEventListener("click", () => file?.click());
    file?.addEventListener("change", async () => {
      const picked = await readAsDataURL(file.files?.[0], ICON);
      file.value = "";
      if (picked !== null) paint(picked);
    });
    clear?.addEventListener("click", () => paint(""));
    name?.addEventListener("input", () => {
      if (!value?.value) paint("");
    });
  }
}

/**
 * The modal plumbing, in one place.
 *
 * <dialog> already does the hard parts — focus, Escape, inert background — so
 * this is only opening, closing and the submit. Every screen wiring its own
 * would be four copies of the same three listeners, and the fourth would be the
 * one that forgot to close on cancel.
 */
/**
 * The accent an operator chose, as a variable on the root.
 *
 * A style attribute rather than a stylesheet, because the value comes from the
 * database and a stylesheet with a hole in it needs a nonce, a request and a
 * cache story. The server only ever stores six hex digits, which is why this
 * can be written into a property at all.
 */
function applyAccent(accent: string): void {
  // Written as a rule rather than onto the element: the page is served under a
  // policy with no 'unsafe-inline' in style-src, and setting a property on
  // documentElement.style is an inline style as far as the browser is
  // concerned. A rule in the sheet this document adopted is not.
  if (!accent) {
    setRule("accent");
    return;
  }
  const [r, g, b] = [1, 3, 5].map((at) => parseInt(accent.slice(at, at + 2), 16));
  setRule(
    "accent",
    `:root { --accent: ${accent}; --accent-rgb: ${r}, ${g}, ${b}; --accent-alt: ${accent}; }`,
  );
}

function openModal(id: string): void {
  root?.querySelector<HTMLDialogElement>("#" + id)?.showModal();
}

/**
 * Opening and closing a dialog, wired once for the whole product.
 *
 * It used to be a function each screen called after drawing itself, and the
 * comment on it said screens had forgotten before. One did again: the front
 * page never called it, so a module's settings had a cross and a cancel that
 * did nothing and the only way out was the keyboard.
 *
 * Delegated from the document instead, so there is nothing left to forget. It
 * is attached once, not per draw, or every redraw would stack another copy and
 * a single click would close the dialog as many times as the page had been
 * drawn.
 */
/**
 * The one control that opens the panel and closes it.
 *
 * Delegated, because the bar is redrawn whenever what it shows changes and a
 * listener bound to the button it drew would be listening to a button that is
 * no longer there.
 *
 * Closing plays the arrival backwards before leaving, so the panel goes out the
 * side it came in from rather than blinking away.
 */
function wirePanelToggle(): void {
  document.addEventListener("click", (event) => {
    const button = (event.target as HTMLElement | null)?.closest("[data-panel-toggle]");
    if (!button) return;

    if (!document.body.classList.contains("admin-view")) {
      go("/panel");
      return;
    }

    const shell = root?.querySelector<HTMLElement>(".admin");
    if (!shell) {
      go("/");
      return;
    }
    shell.classList.add("leaving");
    shell.addEventListener("animationend", () => go("/"), { once: true });
    // And if the animation is off — somebody asked for less motion — the click
    // still has to go somewhere.
    setTimeout(() => {
      if (document.body.classList.contains("admin-view")) go("/");
    }, 400);
  });
}

function wireDialogs(): void {
  document.addEventListener("click", (event) => {
    const target = event.target as HTMLElement | null;

    const opener = target?.closest<HTMLElement>("[data-open-modal]");
    if (opener) {
      openModal(opener.dataset.openModal ?? "");
      return;
    }

    // closest, not the target itself: these buttons carry an icon, and a click
    // lands on the svg inside them.
    const closer = target?.closest<HTMLElement>("[data-close]");
    if (closer) closer.closest("dialog")?.close();
  });
}

/**
 * Submit handling for a form, whether it is in a modal or on the page.
 *
 * A form inside <dialog method="dialog"> closes the dialog on submit, which is
 * why the work happens first and the close is left to the browser: closing by
 * hand as well means closing twice, and the second one throws.
 */
function onSubmit(selector: string, handler: (data: FormData) => Promise<void>): void {
  root?.querySelector<HTMLFormElement>(selector)?.addEventListener("submit", async (event) => {
    const form = event.currentTarget as HTMLFormElement;
    const data = new FormData(form);
    const dialog = form.closest("dialog");
    if (!dialog) event.preventDefault();
    try {
      await handler(data);
    } catch (err) {
      // The dialog has already closed itself by the time this runs, so the
      // message goes where the screen can show it.
      showFormError(err);
    }
  });
}

/**
 * What this account may do here, asked of its memberships rather than of a flag.
 *
 * The screens used to check `admin`, which is stricter than the rules: an owner
 * may create repositories in their organisation, rename it and store its token,
 * and the panel offered none of it. A screen that hides what the server would
 * allow is as wrong as one that offers what it would refuse — the difference is
 * only which of the two you find out about.
 */
function ownsOrganization(slug: string): boolean {
  if (me?.admin === true) return true;
  return (me?.memberships ?? []).some(
    (m) => m.role === "owner" && m.scope === "org" && m.organization === slug,
  );
}

/** Ending an organisation is the server's, not its owner's. */
function isAdministrator(): boolean {
  return me?.admin === true;
}

/**
 * A screen only the administrator is offered: signed in, and an administrator,
 * or sent back to the overview.
 *
 * The server refuses the data on these screens to anybody else, so nothing
 * leaks without this. What it stops is the screen drawing itself anyway from
 * a refusal: a collaborator who typed /panel/about saw a version of "—" and
 * "Could not reach the release server", which was false — it was reached, and
 * said no. The menu does not offer these screens; the address should not
 * either.
 */
async function requireAdministrator(): Promise<boolean> {
  if (!(await requirePanel())) return false;
  if (isAdministrator()) return true;
  go("/panel");
  return false;
}

/** The same, for what belongs to whoever owns one organisation. */
async function requireOwner(orgSlug: string): Promise<boolean> {
  if (!(await requirePanel())) return false;
  if (ownsOrganization(orgSlug)) return true;
  go(`/panel/${encodeURIComponent(orgSlug)}`);
  return false;
}

/** Signed in, or sent to the door. Every panel screen starts here. */
async function requirePanel(): Promise<boolean> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) {
    go("/sign-in");
    return false;
  }
  return true;
}

async function showProfile(): Promise<void> {
  if (!(await requirePanel())) return;
  await showInShell(
    views.profileScreen(
      { username: me?.username ?? "", avatar: me?.avatar, role: views.accountRole(me, text) },
      text,
    ),
    "/panel/account",
  );

  // The picture, which is read in the browser and sent as a data URL. A file
  // upload would need a store, a route to serve it from and a second thing to
  // back up; one small image belongs in the row beside the name.
  const avatarFile = root?.querySelector<HTMLInputElement>("#avatar-file");
  root?.querySelector<HTMLButtonElement>("[data-pick-avatar]")?.addEventListener("click", () => avatarFile?.click());
  avatarFile?.addEventListener("change", async () => {
    const picture = await readAsDataURL(avatarFile.files?.[0]);
    if (picture === null) return;
    try {
      await api.post("/api/me/avatar", { avatar: picture });
      me = null;
      await render();
    } catch (err) {
      showFormError(err);
    }
  });
  root?.querySelector<HTMLButtonElement>("[data-clear-avatar]")?.addEventListener("click", async () => {
    await api.post("/api/me/avatar", { avatar: "" }).catch(() => undefined);
    me = null;
    await render();
  });

  root?.querySelector<HTMLFormElement>("form.profile-name")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    try {
      await api.post("/api/me/profile", { username: String(data.get("username") ?? "") });
      await render();
    } catch (err) {
      showFormError(err);
    }
  });

  root?.querySelector<HTMLFormElement>("form.profile-password")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    const next = String(data.get("new") ?? "");
    if (next !== String(data.get("again") ?? "")) {
      showFormError(new ApiError(400, "error.password_mismatch"));
      return;
    }
    try {
      await api.post("/api/me/password", { current: String(data.get("current") ?? ""), new: next });
      await render();
    } catch (err) {
      showFormError(err);
    }
  });
}

async function showAppearance(): Promise<void> {
  if (!(await requirePanel())) return;
  await showInShell(
    views.appearanceScreen(
      availableLocales.map((l) => ({ code: l.code, name: `${l.flag} ${l.name}` })),
      i18n.current,
      currentTheme(),
      text,
    ),
    "/panel/appearance",
  );

  root?.querySelectorAll<HTMLButtonElement>("[data-theme-choice]").forEach((button) =>
    button.addEventListener("click", () => {
      applyTheme((button.dataset.themeChoice ?? "system") as Theme);
      void render();
    }),
  );
  root?.querySelector<HTMLSelectElement>("#appearance-language")?.addEventListener("change", (e) => {
    const code = (e.target as HTMLSelectElement).value;
    void i18n.setLanguage(code).then(async () => {
      // On the account as well as in this browser, so it is there on the next
      // machine. Failing quietly: the language has already changed on screen,
      // and a toast about a preference nobody asked to save would be noise.
      if (me) await api.post("/api/me/language", { language: code }).catch(() => undefined);
      await render();
    });
  });
}

async function showPeople(): Promise<void> {
  if (!(await requireAdministrator())) return;
  const body = await api
    .get<{ people: Parameters<typeof views.peopleScreen>[0] }>("/api/people")
    .catch(() => null);
  if (!body) {
    await showInShell(views.emptyState(i18n.t("error.forbidden"), icons.warning(28)), "/panel/people");
    return;
  }

  // Every place a role can be granted in, named the way the rest of the API
  // names them. Fetched once for the screen rather than per person.
  const { organizations } = await api
    .get<{ organizations: OrganizationSummary[] }>("/api/orgs")
    .catch(() => ({ organizations: [] as OrganizationSummary[] }));
  const places: { organization: string; repository?: string; label: string }[] = [];
  for (const org of organizations ?? []) {
    places.push({ organization: org.slug, label: org.name });
    const repos = await api
      .get<{ repositories: RepositorySummary[] }>(`/api/orgs/${encodeURIComponent(org.slug)}/repos`)
      .catch(() => ({ repositories: [] as RepositorySummary[] }));
    for (const repo of repos.repositories ?? []) {
      places.push({ organization: org.slug, repository: repo.slug, label: `${org.name} / ${repo.name}` });
    }
  }

  await showInShell(
    views.peopleScreen(body.people ?? [], text) +
      views.modal(
        {
          id: "person-modal",
          title: text.t("people.add"),
          lead: text.t("people.addHelp"),
          body:
            views.field({ id: "person-username", name: "username", label: text.t("auth.username"), required: true, example: "ana" }, text) +
            views.field({ id: "person-password", name: "password", label: text.t("auth.password"), type: "password", required: true, hint: text.t("auth.passwordHelp") }, text) +
            // Chosen here rather than left to whatever their browser asks for:
            // otherwise somebody's first sign-in lands in a language they did
            // not pick, and the setting to change it is in that language.
            views.choiceField({
              id: "person-language",
              name: "language",
              label: text.t("people.language"),
              hint: text.t("people.languageHelp"),
              options: availableLocales.map((l) => [l.code, `${l.flag} ${l.name}`] as [string, string]),
              selected: "en",
            }),
          submit: text.t("auth.create"),
          formClass: "person-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "person-roles",
          title: text.t("people.roles"),
          lead: text.t("people.rolesHelp"),
          body: "",
          submit: text.t("people.grant"),
          formClass: "person-roles-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "person-edit",
          title: text.t("people.edit"),
          lead: text.t("people.editHelp"),
          body:
            views.field({ id: "edit-username", name: "username", label: text.t("auth.username") }, text) +
            views.field({ id: "edit-password", name: "password", label: text.t("people.newPassword"), type: "password", hint: text.t("people.newPasswordHelp") }, text),
          submit: text.t("action.save"),
          formClass: "person-edit-form",
        },
        text,
      ),
    "/panel/people",
  );

  onSubmit("form.person-form", async (data) => {
    await api.post("/api/people", {
      username: String(data.get("username") ?? ""),
      password: String(data.get("password") ?? ""),
      language: String(data.get("language") ?? ""),
    });
    toast(i18n.t("toast.personAdded"));
    await render();
  });

  let editing = 0;
  root?.querySelectorAll<HTMLButtonElement>("[data-edit-person]").forEach((button) =>
    button.addEventListener("click", () => {
      editing = Number(button.dataset.editPerson ?? 0);
      const name = root?.querySelector<HTMLInputElement>("#edit-username");
      if (name) name.value = button.dataset.username ?? "";
      const pass = root?.querySelector<HTMLInputElement>("#edit-password");
      if (pass) pass.value = "";
      openModal("person-edit");
    }),
  );
  onSubmit("form.person-edit-form", async (data) => {
    const payload: Record<string, string> = {};
    const name = String(data.get("username") ?? "");
    const pass = String(data.get("password") ?? "");
    if (name) payload.username = name;
    if (pass) payload.password = pass;
    await api.patch(`/api/people/${editing}`, payload);
    await render();
  });

  // Roles, in a modal of their own. Cramming them into the edit form would put
  // "change this person's name" and "let this person into an organisation" one
  // above the other, which is two different weights of decision on one button.
  let granting = 0;
  root?.querySelectorAll<HTMLButtonElement>("[data-roles-person]").forEach((button) =>
    button.addEventListener("click", () => {
      granting = Number(button.dataset.rolesPerson ?? 0);
      const person = (body.people ?? []).find((p) => p.id === granting);
      const holder = root?.querySelector<HTMLDivElement>("#person-roles .modal-body");
      if (holder && person) holder.innerHTML = views.rolesModalBody(person.grants, places, text);
      wireRevoke();
      openModal("person-roles");
    }),
  );

  const wireRevoke = () => {
    root?.querySelectorAll<HTMLButtonElement>("[data-revoke]").forEach((button) =>
      button.addEventListener("click", async () => {
        try {
          await api.post(`/api/people/${granting}/grants`, {
            organization: button.dataset.organization ?? "",
            repository: button.dataset.repository ?? "",
            role: "",
          });
          await render();
        } catch (err) {
          showFormError(err);
        }
      }),
    );
  };

  onSubmit("form.person-roles-form", async (data) => {
    const [organization, repository] = String(data.get("where") ?? "").split("/");
    await api.post(`/api/people/${granting}/grants`, {
      organization: organization ?? "",
      repository: repository ?? "",
      role: String(data.get("role") ?? ""),
    });
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-person]").forEach((button) =>
    button.addEventListener("click", async () => {
      if (!(await askConfirm(i18n.t("people.delete"), i18n.t("people.confirmDelete", { name: button.dataset.name ?? "" })))) return;
      try {
        await api.del(`/api/people/${button.dataset.deletePerson}`);
        await render();
      } catch (err) {
        showFormError(err);
      }
    }),
  );
}

async function showBackups(): Promise<void> {
  if (!(await requireAdministrator())) return;
  await showInShell(views.backupScreen(text), "/panel/backups");

  const file = root?.querySelector<HTMLInputElement>("#restore-file");
  const note = root?.querySelector<HTMLParagraphElement>(".restore-note");
  root?.querySelector<HTMLButtonElement>("[data-pick-restore]")?.addEventListener("click", () => file?.click());

  file?.addEventListener("change", async () => {
    const chosen = file.files?.[0];
    if (!chosen) return;
    // Asked in words rather than with a second button: this replaces every
    // report anybody has written since the backup was taken, and there is no
    // undo that is not another restore.
    if (!(await askConfirm(i18n.t("backups.restore"), i18n.t("backups.confirmRestore", { file: chosen.name })))) {
      file.value = "";
      return;
    }

    const form = new FormData();
    form.set("backup", chosen);
    if (note) {
      note.textContent = i18n.t("backups.restoring");
      note.hidden = false;
    }
    try {
      const done = await api.upload<{ kept: string }>("/api/restore", form);
      // This one keeps its line as well as toasting: it ends with "restart the
      // server", which is an instruction, and an instruction that flies away
      // after three seconds is one somebody will ask for again.
      if (note) note.textContent = i18n.t("backups.restored", { kept: done.kept });
      toast(i18n.t("toast.restored"));
    } catch (err) {
      if (note) note.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
      report(err);
    }
    file.value = "";
  });
}

async function showAbout(): Promise<void> {
  if (!(await requireAdministrator())) return;
  // Asked of the server rather than assembled from what the page happens to
  // know: uptime, disk and memory are facts about the machine, and the browser
  // has none of them.
  const facts = await api.get<views.ServerFacts>("/api/server").catch(() => ({}) as views.ServerFacts);
  await showInShell(views.aboutScreen(facts, text), "/panel/about");

  const zone = root?.querySelector<HTMLElement>("[data-update-zone]");
  const check = root?.querySelector<HTMLButtonElement>("[data-check]");
  const beta = root?.querySelector<HTMLInputElement>("[data-beta]");

  // The update is asked after the page is drawn: the release server can take
  // seconds to answer, and nothing else here waits on it.
  const ask = async () => {
    if (!zone) return;
    zone.innerHTML = views.updateZone("checking", text);
    if (check) check.disabled = true;
    const status = await api.get<views.UpdateStatus>("/api/update").catch(() => null);
    // The page may have been left while the release server was thinking.
    if (!zone.isConnected) return;
    zone.innerHTML = views.updateZone(status, text);
    // The bar and the menu follow what was just found.
    if (status) noteUpdate(status);
    if (beta && status) {
      beta.checked = status.beta === true;
      beta.disabled = false;
    }
    if (check) {
      check.disabled = false;
      // Asking again changes nothing when the reason is a fact of this
      // installation — no keys, no network, a container — so the button is
      // only offered where a second look can say something new.
      const fixed = ["error.update_offline", "error.update_unverifiable"];
      check.hidden = status !== null && fixed.includes(status.reason ?? "");
    }
  };

  check?.addEventListener("click", () => void ask());

  beta?.addEventListener("change", async () => {
    try {
      await api.post("/api/update/channel", { beta: beta.checked });
    } catch (err) {
      beta.checked = !beta.checked;
      report(err);
      return;
    }
    // Which release is "latest" depends on the channel, so it is asked again.
    await ask();
  });

  // Delegated, because the zone is rewritten every time it is asked. The
  // button opens the update dialog, which does the rest.
  zone?.addEventListener("click", (event) => {
    if (!(event.target as HTMLElement).closest("[data-update]") || !updateFound) return;
    openUpdateDialog(updateFound);
  });

  await ask();
}

/** A page that could not be loaded. The screen has nothing else to show. */
function fail(err: unknown): void {
  const key = err instanceof ApiError ? err.key : "error.internal";
  show(views.failureScreen(i18n.t(key), text));
}

/**
 * An action that did not work, said without taking the page away.
 *
 * This used to be `fail`, which replaces the whole screen: a status change that
 * hit a network error left somebody looking at one grey sentence where their
 * report had been, with everything they were reading gone.
 */
function report(err: unknown): void {
  toastError(i18n.t(err instanceof ApiError ? err.key : "error.internal"));
}

// --- the views -------------------------------------------------------------

async function render(): Promise<void> {
  restoreTheme();
  await i18n.ready;
  await loadInstance();

  const path = location.pathname.replace(/\/+$/, "") || "/";
  // The editor is a mode of the front page: staying there keeps it, going
  // anywhere else saves and leaves it.
  if (editor) {
    if (path === "/") return;
    await stopEditing(false);
  }
  // Where the review screen was opened from, which is where leaving it goes:
  // the bar's icon, the close button and Escape all take you back rather than
  // to a fixed page.
  if (path === "/review" && lastRendered !== "" && !lastRendered.startsWith("/review") &&
      !lastRendered.startsWith("/panel/reports")) {
    reviewCameFrom = lastRendered;
  }
  lastRendered = location.pathname + location.search;
  renderNav();

  const parts = path.split("/").filter(Boolean);

  try {
    if (path === "/") return await showOrganizations();
    if (path === "/sign-in") return await showSignIn();
    if (path === "/setup") {
      // English, whatever the browser asks for. The first screen of the
      // installation is where a language is chosen rather than inferred, and
      // it is the language of this account and no one else's — everybody who
      // arrives later reads English until they choose otherwise.
      await startInEnglish();
      return showSetupStep(1);
    }
    if (path === "/setup/account") return showSetupStep(2);
    if (path === "/setup/instance") return showSetupStep(3);
    if (path === "/setup/done") return showSetupDone();
    if (path === "/panel") return await showPanel();
    if (path === "/review") return await showReview();
    // Where the queue used to live. Kept as a door rather than a 404: it is in
    // people's history and in links they sent each other, and the archive it
    // used to hold is a mode of the screen that replaced it.
    // An address saved before the reports screen existed. A filter in it
    // means the archive — the queue is never filtered — so it opens there.
    if (path === "/panel/reports") {
      const old = new URLSearchParams(location.search);
      if (old.toString() !== "" && !old.has("mode")) old.set("mode", "all");
      return go(`/review${old.toString() ? `?${old.toString()}` : ""}`);
    }
    // Where updates used to live. They are part of About now; the address is kept as a door because it is in people's
    // bookmarks and in the documentation of older releases.
    if (path === "/panel/updates") return go("/panel/about");
    if (path === "/panel/account") return await showProfile();
    if (path === "/panel/appearance") return await showAppearance();
    if (path === "/panel/people") return await showPeople();
    if (path === "/panel/organizations") return await showOrganizationsPanel();
    if (path === "/panel/personalization") return await showPersonalization();
    if (path === "/panel/notifications") return await showNotifications();
    if (path === "/panel/backups") return await showBackups();
    if (path === "/panel/about") return await showAbout();
    // Before the generic /panel/{org}: "new" is a screen, not an organisation
    // anybody would be able to open.
    if (path === "/panel/new") return await showOrganizationWizard(1);
    if (path === "/panel/new/connection") return await showOrganizationWizard(2);
    if (parts[0] === "panel" && parts[2] === "new" && parts[1]) {
      return await showRepositoryWizard(parts[1], 1);
    }
    if (parts[0] === "panel" && parts[2] === "accepts" && parts[1]) {
      return await showRepositoryWizard(parts[1], 2);
    }
    if (parts[0] === "panel" && parts[1] && parts.length === 2) {
      return await showPanelOrganization(parts[1]);
    }
    // Under "repos" rather than straight after the organisation: /panel/{org}/new
    // is the wizard, and a repository called "new" would otherwise never open.
    if (parts[0] === "panel" && parts[2] === "repos" && parts[1] && parts[3] && parts.length === 4) {
      return await showPanelRepository(parts[1], parts[3]);
    }
    if (parts[0] === "i" && parts[1]) return await showItem(parts[1]);
    if (parts.length === 1 && parts[0]) return await showRepositories(parts[0]);
    if (parts.length === 2 && parts[0] && parts[1]) return await showBoard(parts[0], parts[1]);
    if (parts.length === 3 && parts[2] === "new" && parts[0] && parts[1]) {
      return await showReportForm(parts[0], parts[1]);
    }
    // Where one board's queue used to live. Review filtered to that board is
    // the queue now; the address stays a door, because it is in people's
    // history and in links they sent each other.
    if (parts.length === 3 && parts[2] === "queue" && parts[0] && parts[1]) {
      return go(
        "/review?" +
          new URLSearchParams({ mode: "all", moderation: "pending", board: `${parts[0]}/${parts[1]}` }).toString(),
      );
    }
    if (parts.length === 3 && parts[2] === "roadmap" && parts[0] && parts[1]) {
      return await showRoadmap(parts[0], parts[1]);
    }
    show(views.failureScreen(i18n.t("error.not_found"), text));
  } catch (err) {
    fail(err);
  }
}

async function showOrganizations(): Promise<void> {
  // A brand new installation opens on the wizard, not on a landing that hides
  // it behind "sign in". Whoever reaches an unclaimed instance first is the
  // person who is going to set it up, and asking them to find the door is how
  // the first minute goes wrong.
  const status = await api.get<Status>("/api/status").catch(() => null);
  if (status?.needs_setup) return go("/setup");

  // Who is looking decides whether the page can be arranged.
  if (!me) me = await api.get<Me>("/api/me").catch(() => null);

  await fetchLanding();
  drawLanding();
}

async function showRepositories(orgSlug: string): Promise<void> {
  const body = await api.get<{
    organization: OrganizationSummary;
    repositories: RepositorySummary[];
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  show(views.repositoryList(body.organization, body.repositories ?? [], text));
}

/** Reports per page of a public board: enough to read, few enough to load at once. */
const BOARD_PAGE = 20;

async function showBoard(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);

  // Everything the list is narrowed by lives in the address: it survives a
  // reload and the back button, it can be sent to somebody, and it belongs to
  // this board — a filter kept in a variable followed the visitor to the next
  // board they opened. Open is the default, and is left out of the address.
  const read = (): views.BoardFilters => {
    const q = new URLSearchParams(location.search);
    return {
      state: q.get("state") === "closed" ? "closed" : "open",
      kind: q.get("kind") === "bug" || q.get("kind") === "idea" ? (q.get("kind") ?? "") : "",
      sort: q.get("sort") === "votes" ? "votes" : "new",
      query: q.get("q") ?? "",
    };
  };
  const write = (f: views.BoardFilters) => {
    const q = new URLSearchParams();
    if (f.state === "closed") q.set("state", "closed");
    if (f.kind) q.set("kind", f.kind);
    if (f.sort === "votes") q.set("sort", "votes");
    if (f.query.trim()) q.set("q", f.query);
    history.replaceState({}, "", location.pathname + (q.toString() ? `?${q.toString()}` : ""));
  };

  let filters = read();
  let shown: ItemSummary[] = [];
  let totals: views.BoardTotals = { total: 0, open: 0, closed: 0 };

  const load = async (offset: number) => {
    const query = new URLSearchParams({ state: filters.state, limit: String(BOARD_PAGE), offset: String(offset) });
    if (filters.kind) query.set("kind", filters.kind);
    if (filters.sort === "votes") query.set("sort", "votes");
    if (filters.query) query.set("q", filters.query);
    const body = await api.get<{ items: ItemSummary[]; total?: number; counts?: { open?: number; closed?: number } }>(
      `${base}/items?${query}`,
    );
    totals = { total: body.total ?? 0, open: body.counts?.open ?? 0, closed: body.counts?.closed ?? 0 };
    return body.items ?? [];
  };

  const draw = async () => {
    shown = await load(0);
    show(views.board(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, shown, filters, totals, text));
    wire();
  };

  const change = (next: Partial<views.BoardFilters>) => {
    filters = { ...filters, ...next };
    write(filters);
    return draw();
  };

  // The next page, added under the rows already read: nothing is redrawn, so
  // the place somebody was reading and the scroll stay where they were.
  const more = async () => {
    const page = await load(shown.length).catch((err) => {
      report(err);
      return null;
    });
    if (!page) return;
    shown = [...shown, ...page];
    root?.querySelector("[data-items]")?.insertAdjacentHTML("beforeend", views.boardRows(page, text));
    const foot = root?.querySelector("[data-board-foot]");
    if (foot) foot.outerHTML = views.boardFoot(shown.length, totals.total, text);
    root?.querySelector<HTMLButtonElement>("[data-more]")?.addEventListener("click", () => void more());
  };

  const wire = () => {
    root?.querySelectorAll<HTMLButtonElement>("[data-state]").forEach((b) =>
      b.addEventListener("click", () => void change({ state: b.dataset.state === "closed" ? "closed" : "open" })),
    );
    root?.querySelectorAll<HTMLButtonElement>("[data-kind]").forEach((b) =>
      b.addEventListener("click", () => void change({ kind: b.dataset.kind ?? "" })),
    );
    root?.querySelectorAll<HTMLButtonElement>(".sort").forEach((b) =>
      b.addEventListener("click", () => void change({ sort: b.dataset.sort ?? "new" })),
    );
    root?.querySelector<HTMLButtonElement>("[data-more]")?.addEventListener("click", () => void more());
    const search = root?.querySelector<HTMLInputElement>(".search");
    search?.addEventListener("input", debounce(() => {
      void change({ query: search.value }).then(() => {
        const box = root?.querySelector<HTMLInputElement>(".search");
        box?.focus();
        box?.setSelectionRange(box.value.length, box.value.length);
      });
    }, 250));
  };

  await draw();
}

async function showReportForm(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);

  let kind = repo.accepts_bugs ? "bug" : "idea";

  // Fetching the form is what separates somebody who opened the page from a
  // script that found the endpoint. The token it returns is bound to this
  // board, this moment and this part of the internet, and is spent once.
  const ticket = await api
    .get<{ token: string; accepting: boolean }>(`${base}/form`)
    .catch(() => ({ token: "", accepting: true }));

  if (!ticket.accepting) {
    show(`<p class="notice">${escapeHTML(i18n.t("error.board_closed"))}</p>`);
    return;
  }

  show(
    views.reportForm(
      repo.organization ?? { slug: orgSlug, name: orgSlug },
      repo,
      kind,
      text,
      repo.accepts_images ? PICTURES_ON_A_REPORT : 0,
    ),
  );

  const form = root?.querySelector<HTMLFormElement>("form.report");
  const similar = root?.querySelector<HTMLDivElement>(".similar");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");
  const tray = wireTray(form ?? null, { remove: i18n.t("pictures.remove") }, (key) => {
    if (!errorLine) return;
    errorLine.textContent = i18n.t(key);
    errorLine.hidden = false;
  });

  root?.querySelectorAll<HTMLButtonElement>(".choice-card").forEach((button) =>
    button.addEventListener("click", () => {
      kind = button.dataset.choice ?? kind;
      root?.querySelectorAll(".choice-card").forEach((b) => {
        b.classList.toggle("on", b === button);
        b.setAttribute("aria-pressed", b === button ? "true" : "false");
      });
      // The questions that belong to this kind, shown and hidden rather than
      // drawn and thrown away: rebuilding the block would lose whatever had
      // already been typed into it.
      root?.querySelectorAll<HTMLElement>(".kind-fields").forEach((block) => {
        block.hidden = block.dataset.for !== kind;
      });
    }),
  );

  const title = form?.querySelector<HTMLInputElement>('[name="title"]');
  title?.addEventListener(
    "input",
    debounce(async () => {
      const value = title.value.trim();
      if (value.length < 4 || !similar) {
        if (similar) similar.hidden = true;
        return;
      }
      const found = await api
        .get<{ similar: Array<{ public_id: string; title: string }> }>(
          `${base}/similar?title=${encodeURIComponent(value)}`,
        )
        .catch(() => ({ similar: [] }));
      similar.innerHTML = views.similarItems(found.similar ?? [], text);
      similar.hidden = (found.similar ?? []).length === 0;
    }, 300),
  );

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (errorLine) errorLine.hidden = true;

    const data = new FormData(form);
    try {
      const sent = await api.post<Submitted>(`${base}/items`, {
        kind,
        title: String(data.get("title") ?? ""),
        body: String(data.get("body") ?? ""),
        email: String(data.get("email") ?? ""),
        website: String(data.get("website") ?? ""),
        // The answers to the questions this kind was asked. Sent whatever the
        // kind is: the server keeps the ones that belong to it and drops the
        // rest, so somebody who typed steps and then changed their mind and
        // called it an idea does not send them.
        fields: {
          version: String(data.get("version") ?? ""),
          steps: String(data.get("steps") ?? ""),
        },
        token: ticket.token,
      });
      // The pictures go after the report, hung on it by its follow link. A
      // refused one costs only itself, and the page says which reason.
      const chosen = tray?.pictures() ?? [];
      const refused =
        sent.public_id && chosen.length > 0
          ? await sendPictures(sent.public_id, chosen, { token: sent.follow_token })
          : "";
      // The address to keep, and the path to walk. The first is absolute
      // because it is going into somebody's notes; the second is what this
      // page's own button follows, and the router only takes a path.
      const path = `/i/${sent.public_id}?k=${sent.follow_token}`;
      const link = `${location.origin}${path}`;
      show(
        views.submitted(
          {
            link,
            path,
            org: orgSlug,
            repo: repoSlug,
            board: repo.name,
            problem: refused ? i18n.t("pictures.someFailed", { reason: i18n.t(refused) }) : "",
          },
          text,
        ),
      );
      root?.querySelector<HTMLButtonElement>("[data-copy]")?.addEventListener("click", async (e) => {
        await navigator.clipboard?.writeText(link).catch(() => undefined);
        (e.currentTarget as HTMLButtonElement).textContent = i18n.t("sent.copied");
      });
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

async function showRoadmap(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);
  const board = await api.get<{ columns: Array<{ column: string; items: ItemSummary[] }> }>(
    `${base}/roadmap`,
  );
  show(views.roadmap(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, board.columns ?? [], text));
}

async function showItem(publicID: string): Promise<void> {
  const token = new URLSearchParams(location.search).get("k");
  const path = `/api/items/${encodeURIComponent(publicID)}${token ? `?k=${encodeURIComponent(token)}` : ""}`;
  const item = await api.get<ItemDetail>(path);

  // Whether this person works on this report is the server's to say, and it
  // says it in the report itself: a page that decides for itself who may act
  // is a second implementation of the rules.
  const allowed = item.team ?? null;

  // The address is the public one — this is the page that travels — but for
  // the team it is where the work is done, so they get the panel around it and
  // the report as they work on it. Marked under the organisations, which is
  // where the board it belongs to is found in the panel.
  const org = item.repository?.organization?.slug ?? "";
  if (allowed) {
    const board = item.repository ? `${org}/${item.repository.slug}` : "";
    const others = (await mergeCandidates(board)).filter((r) => r.public_id !== publicID);
    await showInShell(
      views.itemTeamView(item, allowed, others, text, repliesTakePictures(item) ? PICTURES_ON_A_MESSAGE : 0),
      org ? `/panel/${org}` : "/panel",
    );
    wireVote(publicID);
    wireTeamItem(publicID, item);
    return;
  }

  // The reporter sees their own pictures before they are public, and may take
  // one back; their follow-ups may carry pictures too.
  const view: views.PictureView = item.mine ? { remove: "reporter", token: token ?? undefined } : {};
  const followUps = item.mine && repliesTakePictures(item) ? PICTURES_ON_A_MESSAGE : 0;
  const staff = me !== null && (me.admin === true || (me.memberships ?? []).length > 0);
  if (staff) await showInShell(views.itemView(item, text, view, followUps), org ? `/panel/${org}` : "/panel");
  else show(views.itemView(item, text, view, followUps));
  wireVote(publicID);

  root?.querySelectorAll<HTMLButtonElement>("[data-remove-picture]").forEach((button) =>
    button.addEventListener("click", async () => {
      if (!(await askConfirm(i18n.t("pictures.remove"), i18n.t("pictures.confirmRemove")))) return;
      try {
        await removePicture(publicID, button.dataset.removePicture ?? "", token ?? undefined);
        toast(i18n.t("pictures.removed"));
        await showItem(publicID);
      } catch (err) {
        report(err);
      }
    }),
  );

  // The reporter's way back to their own report. The address is this page's,
  // token and all, and it is filled in here rather than in the template
  // because the template is a pure function and does not know where it is.
  const follow = root?.querySelector<HTMLInputElement>("[data-follow-link]");
  if (follow) {
    follow.value = location.href;
    root?.querySelector<HTMLButtonElement>(".yours [data-copy]")?.addEventListener("click", async (event) => {
      await navigator.clipboard?.writeText(follow.value).catch(() => undefined);
      const span = (event.currentTarget as HTMLElement).querySelector("span");
      if (span) span.textContent = i18n.t("sent.copied");
    });
  }

  const commentForm = root?.querySelector<HTMLFormElement>("form.comment-form");
  const commentSent = commentForm?.querySelector<HTMLParagraphElement>(".comment-sent");
  const commentTray = wireTray(commentForm ?? null, { remove: i18n.t("pictures.remove") }, (key) => {
    if (!commentSent) return;
    commentSent.textContent = i18n.t(key);
    commentSent.hidden = false;
  });
  commentForm?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(commentForm);
    // Said as it was, not as "sent": a page opened before the team answered
    // would otherwise promise a review that is never going to happen.
    let said: string;
    try {
      const posted = await api.post<{ id: number }>(`/api/items/${encodeURIComponent(publicID)}/comments`, {
        body: String(data.get("body") ?? ""),
        token: token ?? "",
      });
      said = i18n.t("item.commentWaiting");
      const chosen = commentTray?.pictures() ?? [];
      if (chosen.length > 0) {
        const refused = await sendPictures(publicID, chosen, { comment: posted.id, token: token ?? undefined });
        if (refused) said += " " + i18n.t("pictures.someFailed", { reason: i18n.t(refused) });
      }
      commentForm.reset();
      commentTray?.clear();
    } catch (err) {
      said = i18n.t(err instanceof ApiError ? err.key : "error.internal");
    }
    if (commentSent) {
      commentSent.textContent = said;
      commentSent.hidden = false;
    }
  });
}

/** Whether a message on this report may carry pictures: the board takes them and
 *  the report is still open. */
function repliesTakePictures(item: ItemDetail): boolean {
  return item.repository?.accepts_images === true && item.commentable !== false;
}

function wireVote(publicID: string, scope: ParentNode | null = root): void {
  scope?.querySelector<HTMLButtonElement>("[data-vote]")?.addEventListener("click", async (event) => {
    const button = event.currentTarget as HTMLButtonElement;
    const voted = button.classList.contains("voted");
    const result = await api[voted ? "del" : "post"]<{ votes: number; voted: boolean }>(
      `/api/items/${encodeURIComponent(publicID)}/votes`,
    );
    // Only the button is redrawn. Replacing the page would scroll the thread
    // back to the top under whoever was reading it.
    button.classList.toggle("voted", result.voted);
    button.setAttribute("aria-pressed", result.voted ? "true" : "false");
    const label = button.querySelector(".label");
    if (label) label.textContent = result.voted ? i18n.t("item.voted") : i18n.t("item.vote");
    const count = button.querySelector(".count");
    if (count) count.textContent = i18n.number(result.votes);
  });
}

/**
 * The team's report: each card's one job.
 *
 * Every change re-renders the page rather than patching it: the title and the
 * status appear in more than one place on this screen, and updating the one
 * that was easy to reach is how the two start disagreeing.
 */
/**
 * What a report's team view does, wherever it is drawn.
 *
 * On its own page (/i/…) that is the whole page; on the reports screen it is
 * the pane beside the list. The pane passes its own scope and says what
 * happens afterwards — the row in the list changes, or goes — instead of the
 * whole screen being redrawn under the person working down it.
 */
interface TeamItemHost {
  /** Where the view is drawn. The page, when not given. */
  scope?: ParentNode | null;
  /** After a change: redraw what shows it. The whole page, when not given. */
  changed?: () => Promise<void>;
  /** After the report is deleted. Its repository's page, when not given. */
  deleted?: () => void;
  /** A moderation decision, when the host takes those itself. */
  moderate?: (state: string) => Promise<void>;
}

function wireTeamItem(publicID: string, item: ItemDetail, host: TeamItemHost = {}): void {
  const scope = host.scope ?? root;
  const base = `/api/items/${encodeURIComponent(publicID)}`;
  const act = async (call: () => Promise<unknown>, done?: string) => {
    try {
      await call();
      // A new title, a merge: what the merge lists say has changed.
      boardReports.clear();
      if (done) toast(i18n.t(done));
      await (host.changed ?? render)();
    } catch (err) {
      report(err);
    }
  };

  scope?.querySelectorAll<HTMLButtonElement>("[data-moderate]").forEach((button) =>
    button.addEventListener("click", () =>
      host.moderate
        ? host.moderate(button.dataset.moderate ?? "")
        : act(() => api.post(`${base}/moderation`, { state: button.dataset.moderate })),
    ),
  );

  scope?.querySelectorAll<HTMLButtonElement>("[data-delete-comment]").forEach((button) =>
    button.addEventListener("click", async () => {
      if (!(await askConfirm(i18n.t("item.deleteComment"), i18n.t("item.confirmDeleteComment")))) return;
      void act(() => api.del(`/api/comments/${button.dataset.deleteComment}`), "item.commentDeleted");
    }),
  );

  // Rewriting a message where it stands, as the report's own words are: the
  // pencil swaps the text for a form, Cancel swaps it back untouched.
  scope?.querySelectorAll<HTMLButtonElement>("[data-edit-comment]").forEach((button) =>
    button.addEventListener("click", () => {
      const comment = button.closest<HTMLElement>(".comment");
      const words = comment?.querySelector<HTMLElement>(".what");
      if (!comment || !words || comment.querySelector("form.comment-edit")) return;
      const holder = document.createElement("div");
      holder.innerHTML = views.commentEditForm(words.textContent ?? "", text);
      const form = holder.firstElementChild as HTMLFormElement;
      words.hidden = true;
      button.hidden = true;
      words.after(form);
      const area = form.querySelector<HTMLTextAreaElement>("textarea");
      area?.focus();
      form.querySelector("[data-comment-cancel]")?.addEventListener("click", () => {
        form.remove();
        words.hidden = false;
        button.hidden = false;
      });
      form.addEventListener("submit", (event) => {
        event.preventDefault();
        void act(
          () => api.patch(`/api/comments/${button.dataset.editComment}`, { body: area?.value ?? "" }),
          "panel.saved",
        );
      });
    }),
  );

  // Deleting, for whoever runs the organisation. Afterwards there is no
  // report to come back to, so it goes to the board it was on.
  scope?.querySelector<HTMLButtonElement>("[data-delete-item]")?.addEventListener("click", async () => {
    if (!(await askConfirm(i18n.t("item.delete"), i18n.t("item.confirmDelete", { title: item.title })))) return;
    try {
      await api.del(base);
      boardReports.clear();
      toast(i18n.t("item.deleted"));
      if (host.deleted) return host.deleted();
      const org = item.repository?.organization?.slug;
      go(org && item.repository ? `/panel/${org}/repos/${item.repository.slug}` : "/panel");
    } catch (err) {
      report(err);
    }
  });

  scope?.querySelector<HTMLSelectElement>("#status-select")?.addEventListener("change", (e) =>
    act(() => api.post(`${base}/status`, { status: (e.target as HTMLSelectElement).value }), "toast.statusChanged"),
  );

  // Rewriting, where the words stand: the pencil swaps them for a form, and
  // Cancel swaps them back without asking the server anything.
  const words = scope?.querySelector<HTMLElement>(".detail-words");
  const pencil = scope?.querySelector<HTMLButtonElement>("[data-rewrite]");
  pencil?.addEventListener("click", () => {
    if (!words || words.querySelector("form.rewrite")) return;
    const kept = words.innerHTML;
    pencil.hidden = true;
    words.innerHTML = views.rewriteForm({ title: item.title ?? "", body: item.body ?? "" }, text);
    words.querySelector<HTMLInputElement>("input[name=title]")?.focus();
    words.querySelector("[data-rewrite-cancel]")?.addEventListener("click", () => {
      words.innerHTML = kept;
      pencil.hidden = false;
    });
    const form = words.querySelector<HTMLFormElement>("form.rewrite");
    form?.addEventListener("submit", (e) => {
      e.preventDefault();
      const data = new FormData(form);
      void act(
        () => api.patch(base, { title: String(data.get("title") ?? ""), body: String(data.get("body") ?? "") }),
        "panel.saved",
      );
    });
  });

  // Creating an issue answers "queued", not "created": the request does not
  // call the forge — forges rate-limit and forges go down — so saying it was
  // created would be reporting something that has not happened yet.
  scope?.querySelector<HTMLButtonElement>("[data-create-issue]")?.addEventListener("click", async () => {
    try {
      await api.post(`${base}/issue`);
      toast(i18n.t("panel.issueQueued"));
    } catch (err) {
      report(err);
    }
  });
  const attach = scope?.querySelector<HTMLFormElement>("form.attach");
  attach?.addEventListener("submit", (event) => {
    event.preventDefault();
    const value = String(new FormData(attach).get("issue") ?? "");
    void act(() => api.post(`${base}/attach`, { issue: value }), "toast.issueAttached");
  });

  const reply = scope?.querySelector<HTMLFormElement>("form.team-reply");
  const replyTray = wireTray(reply ?? null, { remove: i18n.t("pictures.remove") }, (key) => toastError(i18n.t(key)));
  reply?.addEventListener("submit", (event) => {
    event.preventDefault();
    const body = String(new FormData(reply).get("body") ?? "");
    void act(async () => {
      const posted = await api.post<{ id: number }>(`${base}/reply`, { body });
      // The reply's pictures hang on it once it exists. One refused is said;
      // the reply itself is already in.
      const chosen = replyTray?.pictures() ?? [];
      const refused = chosen.length > 0 ? await sendPictures(publicID, chosen, { comment: posted.id }) : "";
      if (refused) toastError(i18n.t("pictures.someFailed", { reason: i18n.t(refused) }));
    }, "toast.replied");
  });

  scope?.querySelectorAll<HTMLButtonElement>("[data-remove-picture]").forEach((button) =>
    button.addEventListener("click", async () => {
      if (!(await askConfirm(i18n.t("pictures.remove"), i18n.t("pictures.confirmRemove")))) return;
      void act(() => removePicture(publicID, button.dataset.removePicture ?? ""), "pictures.removed");
    }),
  );

  // Typing narrows the list of reports to the titles that contain it. Hidden
  // options are what the house dropdown leaves out, so the picker shows the
  // same narrowing; the chosen one moves to the first that still matches.
  const search = scope?.querySelector<HTMLInputElement>("[data-merge-search]");
  const into = scope?.querySelector<HTMLSelectElement>("#merge-into");
  search?.addEventListener("input", () => {
    if (!into) return;
    const q = search.value.trim().toLocaleLowerCase();
    let first: HTMLOptionElement | null = null;
    for (const option of into.options) {
      option.hidden = q !== "" && !(option.textContent ?? "").toLocaleLowerCase().includes(q);
      if (!option.hidden && !first) first = option;
    }
    if (into.selectedOptions[0]?.hidden && first) {
      into.value = first.value;
      into.dispatchEvent(new Event("change", { bubbles: true }));
    }
    // Nothing matches: nothing to merge into, and a button that looks ready
    // and does nothing is worse than one that says it cannot.
    const submit = scope?.querySelector<HTMLButtonElement>("form.merge [type=submit]");
    if (submit) submit.disabled = first === null;
  });
  const merge = scope?.querySelector<HTMLFormElement>("form.merge");
  merge?.addEventListener("submit", (event) => {
    event.preventDefault();
    const target = String(new FormData(merge).get("into") ?? "");
    if (!target || into?.selectedOptions[0]?.hidden) return;
    // The merged report now redirects, so the page it lands on is the survivor.
    void act(() => api.post(`${base}/merge`, { into: target }), "toast.merged");
  });
}

/** Storing the forge token, and showing where the forge should deliver. */
function wireCredentials(orgSlug: string): void {
  const form = root?.querySelector<HTMLFormElement>("form.credentials");
  if (!form) return;

  const errorLine = form.querySelector<HTMLParagraphElement>(".form-error");

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      await api.post(`/api/orgs/${encodeURIComponent(orgSlug)}/credentials`, {
        token: String(data.get("token") ?? ""),
        webhook_secret: String(data.get("webhook_secret") ?? ""),
      });
      form.reset();
      // The address has a card of its own, and re-rendering fills it in. It
      // used to be printed at the foot of this form as well, which meant the
      // same value in two places on one screen.
      await render();
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

/** Choosing where a board's team is told something arrived. */
function wireChannel(
  where: { save: (data: Record<string, unknown>) => Promise<unknown>; test: (data: Record<string, unknown>) => Promise<unknown> },
  saved?: () => Promise<void>,
): void {
  const form = root?.querySelector<HTMLFormElement>("form.channel");
  if (!form) return;

  const kind = form.querySelector<HTMLSelectElement>("#chan-kind");
  const fields = form.querySelector<HTMLDivElement>(".channel-fields");
  const mail = form.querySelector<HTMLDivElement>(".channel-mail");
  const serverNote = form.querySelector<HTMLParagraphElement>("[data-channel-server]");
  const errorLine = form.querySelector<HTMLParagraphElement>(".form-error");
  const result = form.querySelector<HTMLParagraphElement>(".channel-result");
  const testButton = form.querySelector<HTMLButtonElement>("[data-channel-test]");

  // Only the fields the chosen channel needs. Asking for a URL for an email
  // channel is a form arguing with the person filling it in; and the server's
  // channel has nothing to fill in here at all.
  const sync = () => {
    const value = kind?.value ?? "";
    if (fields) fields.hidden = value === "" || value === "smtp" || value === "server";
    if (mail) mail.hidden = value !== "smtp";
    if (serverNote) serverNote.hidden = value !== "server";
    if (testButton) testButton.hidden = value === "";
  };
  kind?.addEventListener("change", sync);
  sync();

  const read = (): Record<string, unknown> => {
    const data = new FormData(form);
    return {
      kind: String(data.get("kind") ?? ""),
      url: String(data.get("url") ?? ""),
      token: String(data.get("token") ?? ""),
      to: String(data.get("to") ?? ""),
      clear_token: data.get("clear_token") !== null,
    };
  };
  // The failure stays beside the form as well as toasting: it is about what was
  // typed, and the fix is in the fields somebody is looking at.
  const fail = (err: unknown) => {
    if (result) result.hidden = true;
    if (errorLine) {
      errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
      errorLine.hidden = false;
    }
    report(err);
  };
  const clear = () => {
    if (errorLine) errorLine.hidden = true;
    if (result) result.hidden = true;
  };

  testButton?.addEventListener("click", async () => {
    clear();
    testButton.disabled = true;
    try {
      await where.test(read());
      if (result) {
        result.textContent = i18n.t("panel.channelTestSent");
        result.hidden = false;
      }
    } catch (err) {
      fail(err);
    } finally {
      testButton.disabled = false;
    }
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clear();
    try {
      await where.save(read());
      toast(i18n.t("panel.saved"));
      await saved?.();
    } catch (err) {
      fail(err);
    }
  });
}

/**
 * What the wizard is carrying between steps.
 *
 * In memory on purpose: a half-finished organisation is not something to leave
 * in a database, and a reload landing on step three of a form whose first two
 * steps no longer exist is worse than starting again.
 */
const draft: { forge: string; orgSlug: string; repoSlug: string; picture: string; logo: string; orgIcon: string } = {
  forge: "none",
  orgSlug: "",
  repoSlug: "",
  // Chosen before the account exists, so held here and sent the moment it does.
  picture: "",
  logo: "",
  orgIcon: "",
};

/** The error line under a wizard card, which every step has one of. */
function showFormError(err: unknown): void {
  const line = root?.querySelector<HTMLParagraphElement>(".form-error");
  if (!line) return;
  line.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
  line.hidden = false;
}

/**
 * The forge fields appear only when there is a forge, and their examples change
 * with it: an owner on GitHub and an owner on a self-hosted Forgejo look
 * nothing alike, and a single generic example teaches neither.
 */
function wireForgeChoice(rerender: (forge: string) => void): void {
  const select = root?.querySelector<HTMLSelectElement>("#org-forge");
  select?.addEventListener("change", () => {
    draft.forge = select.value;
    rerender(select.value);
  });
}

function showSetupStep(step: 1 | 2 | 3): void {
  show(
    views.setupWizard(step, text, {
      languages: availableLocales.map((l) => ({ code: l.code, name: `${l.flag} ${l.name}` })),
      language: i18n.current,
      theme: currentTheme(),
      picture: draft.picture,
      logo: draft.logo,
    }),
  );

  if (step === 1) {
    // Applied as they are chosen, not on submit: a language you cannot read is
    // a poor way to ask whether you can read it.
    root?.querySelectorAll<HTMLButtonElement>("[data-theme-choice]").forEach((button) =>
      button.addEventListener("click", () => {
        applyTheme((button.dataset.themeChoice ?? "system") as Theme);
        void render();
      }),
    );
    root?.querySelector<HTMLSelectElement>("#setup-language")?.addEventListener("change", (e) => {
      void i18n.setLanguage((e.target as HTMLSelectElement).value).then(() => void render());
    });
    root?.querySelector<HTMLFormElement>("form.setup-preferences")?.addEventListener("submit", (event) => {
      event.preventDefault();
      go("/setup/account");
    });
    return;
  }

  if (step === 2) {
    const avatarFile = root?.querySelector<HTMLInputElement>("#avatar-file");
    root?.querySelector<HTMLButtonElement>("[data-pick-avatar]")?.addEventListener("click", () => avatarFile?.click());
    avatarFile?.addEventListener("change", async () => {
      const picked = await readAsDataURL(avatarFile.files?.[0]);
      if (picked === null) return;
      // Kept until the account exists: there is nobody to attach it to yet.
      draft.picture = picked;
      showSetupStep(2);
    });

    root?.querySelector<HTMLFormElement>("form.setup-account")?.addEventListener(
      "submit",
      async (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget as HTMLFormElement);
        const password = String(data.get("password") ?? "");
        if (password !== String(data.get("password_again") ?? "")) {
          // Caught here rather than at the server, which never sees the second
          // box: this is the one mistake that would lock somebody out of an
          // instance with no reset to fall back on.
          showFormError(new ApiError(400, "error.password_mismatch"));
          return;
        }
        try {
          await api.post("/api/setup", { username: String(data.get("username") ?? ""), password });
          if (draft.picture) await api.post("/api/me/avatar", { avatar: draft.picture }).catch(() => undefined);
          me = await api.get<Me>("/api/me");
          go("/setup/instance");
        } catch (err) {
          showFormError(err);
        }
      },
    );
    return;
  }

  const logoFile = root?.querySelector<HTMLInputElement>("#logo-file");
  root?.querySelector<HTMLButtonElement>("[data-pick-logo]")?.addEventListener("click", () => logoFile?.click());
  logoFile?.addEventListener("change", async () => {
    const picked = await readAsDataURL(logoFile.files?.[0]);
    if (picked === null) return;
    draft.logo = picked;
    showSetupStep(3);
  });

  root?.querySelector<HTMLFormElement>("form.setup-instance")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      try {
        await api.post("/api/instance", {
          name: String(data.get("name") ?? ""),
          logo: draft.logo,
          accent: "",
        });
        // Read again, the way every other screen that writes this does. It is
        // held for the life of the page, so without this the bar and the front
        // page went on saying LastHelp for the whole session after somebody had
        // just named their installation — a setting that did not take.
        instanceLoaded = false;
        go("/setup/done");
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

function showSetupDone(): void {
  show(views.setupDone(text, instanceLogo));
}

async function showOrganizationWizard(step: 1 | 2): Promise<void> {
  if (!(await requireAdministrator())) return;
  await showInShell(views.organizationWizard(step, text, draft.forge, draft.orgIcon), "/panel/new");
  // Kept in the draft: changing the forge draws this step again.
  wireIconPicker((icon) => {
    draft.orgIcon = icon;
  });

  if (step === 1) {
    wireForgeChoice((forge) => {
      draft.forge = forge;
      showOrganizationWizard(1);
    });
  }

  root?.querySelector<HTMLFormElement>("form.setup-organization")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      draft.forge = String(data.get("forge") ?? "none");
      draft.orgSlug = String(data.get("slug") ?? "");
      try {
        await api.post("/api/organizations", {
          name: String(data.get("name") ?? ""),
          slug: draft.orgSlug,
          forge: draft.forge,
          base_url: String(data.get("base_url") ?? ""),
          forge_owner: String(data.get("forge_owner") ?? ""),
          icon: draft.orgIcon,
        });
        draft.orgIcon = "";
        go(draft.forge === "none" ? `/panel/${draft.orgSlug}` : "/panel/new/connection");
      } catch (err) {
        showFormError(err);
      }
    },
  );

  root?.querySelector<HTMLFormElement>("form.setup-token")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      const token = String(data.get("token") ?? "");
      try {
        if (token) {
          await api.post(`/api/orgs/${encodeURIComponent(draft.orgSlug)}/credentials`, { token });
        }
        go(`/panel/${draft.orgSlug}`);
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

async function showRepositoryWizard(orgSlug: string, step: 1 | 2): Promise<void> {
  if (!(await requireOwner(orgSlug))) return;
  const body = await api.get<{
    organization: OrganizationSummary & { forge?: string };
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  draft.orgSlug = orgSlug;
  await showInShell(views.repositoryWizard(body.organization, step, text), `/panel/${orgSlug}`);

  root?.querySelector<HTMLFormElement>("form.setup-repository-identity")?.addEventListener(
    "submit",
    (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      draft.repoSlug = String(data.get("slug") ?? "");
      pendingRepository = {
        name: String(data.get("name") ?? ""),
        slug: draft.repoSlug,
        forge_repo: String(data.get("forge_repo") ?? ""),
      };
      go(`/panel/${orgSlug}/accepts`);
    },
  );

  root?.querySelector<HTMLFormElement>("form.setup-repository-accepts")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      try {
        await api.post(`/api/orgs/${encodeURIComponent(orgSlug)}/repositories`, {
          ...pendingRepository,
          accepts_bugs: data.get("accepts_bugs") !== null,
          accepts_ideas: data.get("accepts_ideas") !== null,
          accepts_images: data.get("accepts_images") !== null,
        });
        go(`/panel/${orgSlug}`);
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

let pendingRepository: { name: string; slug: string; forge_repo: string } = {
  name: "",
  slug: "",
  forge_repo: "",
};

async function showSignIn(): Promise<void> {
  const status = await api.get<Status>("/api/status");
  // A fresh instance is claimed through the wizard, which starts by asking
  // which language to say the rest of it in.
  if (status.needs_setup) return go("/setup");
  show(views.signInForm(text, { name: instanceName, logo: instanceLogo }));

  const form = root?.querySelector<HTMLFormElement>("form.auth");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    // `username`, which is what the field is called. It read `email` — the name
    // from before the rename — so every sign-in sent an empty username and was
    // told one was needed, on a form where it was plainly filled in. The two
    // sides of a rename, and only one of them moved.
    const payload = {
      username: String(data.get("username") ?? ""),
      password: String(data.get("password") ?? ""),
    };
    try {
      // Not /api/setup: a fresh instance was sent to the wizard above, so that
      // branch could never be taken. A branch nothing can reach is a branch
      // nobody tests.
      await api.post("/api/session", payload);
      me = await api.get<Me>("/api/me");
      go("/panel");
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

async function showPanel(): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");
  // Who is signed in changed, so the nav has to be redrawn: it was rendered
  // before this ran and still says "sign in".
  renderNav();

  const data = await api
    .get<Parameters<typeof views.overviewScreen>[0]>("/api/overview")
    .catch(() => null);

  if (!data) {
    // No role anywhere: nothing to summarise, and nobody to show it to but
    // the administrator who could grant one.
    await showInShell(
      `<h1 class="visually-hidden">${escapeHTML(i18n.t("shell.overview"))}</h1>` +
        views.emptyState(i18n.t("overview.noRoles"), icons.person(28)),
      "/panel",
    );
    return;
  }
  await showInShell(views.overviewScreen(data, text), "/panel");
  // What a day of the chart was, in the reader's own language and split the way
  // the card beside it splits everything: the day, what arrived, and what kind
  // it was. The numbers come from the point that was drawn; the words and the
  // formats come from here, which is where the catalogue is.
  attachChartHover(root, (point) => {
    const day = Number(point.day ?? 0);
    const total = Number(point.count ?? 0);
    const kinds: Array<[string, number]> = [
      ["idea", Number(point.idea ?? 0)],
      ["bug", Number(point.bug ?? 0)],
    ];
    const rows = kinds
      .filter(([, n]) => n > 0)
      .map(
        ([kind, n]) => `<span class="tip-row">
          <span class="swatch swatch-${kind}"></span>${escapeHTML(i18n.t(`kind.${kind}`))}
          <span class="count">${escapeHTML(i18n.number(n))}</span>
        </span>`,
      )
      .join("");
    return `<span class="tip-day">${escapeHTML(day > 0 ? i18n.date(day) : "")}</span>
      <span class="tip-total">${escapeHTML(i18n.t("overview.reports"))}
        <span class="count">${escapeHTML(i18n.number(total))}</span></span>
      ${rows}`;
  });
}

/**
 * The panel's own listing of reports.
 *
 * The filters live in the address rather than in a variable: a view somebody
 * has narrowed down is then something they can bookmark, reload and send to
 * whoever else looks after that board. Every control writes the address and
 * the screen redraws from it, so there is one source of truth for what is on
 * screen instead of two that drift.
 */
/**
 * Reviewing what is waiting.
 *
 * A place, not a layer: it has its own address, so a filtered view can be sent
 * to somebody and a reload lands back on it. An overlay could do neither.
 *
 * Everything on screen is read from the address — the mode, what is chosen and
 * every narrowing of the archive — so there is one source of truth for what is
 * shown instead of a variable and an address that drift apart.
 */
const REVIEW_PAGE = 50;

/** Whether the search box had the cursor when the screen was last redrawn. */
let reviewSearching = false;

function leaveReview(): void {
  go(reviewCameFrom);
}

async function showReview(): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const query = new URLSearchParams(location.search);
  // The door in the bar names no mode: it lands on what is waiting when
  // something is, and on everything when nothing is — an empty queue is not
  // where somebody opening "Reports" wants to be. The tabs always name theirs.
  const waiting = (me.pending?.reports ?? 0) + (me.pending?.messages ?? 0);
  const asked = query.get("mode");
  const mode = asked === "all" ? "all" : asked === "pending" ? "pending" : waiting > 0 ? "pending" : "all";
  const filters: views.ReportFilters = {
    moderation: "",
    kind: "",
    status: "",
    board: "",
    sort: "",
    q: "",
  };
  if (mode === "all") {
    for (const key of Object.keys(filters) as (keyof views.ReportFilters)[]) {
      filters[key] = query.get(key) ?? "";
    }
  }

  const ask = (offset: number) => {
    const params = new URLSearchParams(
      mode === "pending" ? { moderation: "pending" } : { ...filters },
    );
    for (const [key, value] of [...params.entries()]) if (value === "") params.delete(key);
    params.set("limit", String(REVIEW_PAGE));
    params.set("offset", String(offset));
    return api.get<views.ReportsData>(`/api/reports?${params.toString()}`);
  };

  const data = await ask(0).catch(() => null);
  if (!data) {
    show(views.failureScreen(i18n.t("error.forbidden"), text));
    return;
  }

  // Messages only in the pending mode, and that is not the old screen's rule
  // in disguise: there they vanished when an unrelated report filter moved,
  // which made a whole queue look like it answered a question it did not.
  // Here the two modes ARE the question — what is waiting, or everything ever
  // — and a message that has been dealt with is not waiting.
  if (mode === "pending") {
    const waiting = await api
      .get<{ comments: views.WaitingComment[] }>("/api/panel/comments")
      .catch(() => ({ comments: [] as views.WaitingComment[] }));
    data.comments = waiting.comments ?? [];
  }

  const rows = data.items ?? [];
  const view: views.ReviewView = {
    mode,
    chosen:
      query.get("at") ??
      rows[0]?.public_id ??
      (data.comments?.[0] ? `c:${data.comments[0].id}` : ""),
    filters,
    more: rows.length === REVIEW_PAGE,
    now: Math.floor(Date.now() / 1000),
    waiting,
  };

  show(views.reviewScreen(data, view, text));
  renderNav();
  await wireReview(data, view, ask);
}

/**
 * The two panes, the keyboard, and what a decision does.
 *
 * A decision removes the row and moves to the next one rather than redrawing
 * the screen: the list is what somebody is working down, and rebuilding it
 * puts them back at the top after every single item.
 */
async function wireReview(
  data: views.ReportsData,
  view: views.ReviewView,
  ask: (offset: number) => Promise<views.ReportsData>,
): Promise<void> {
  const screen = root?.querySelector<HTMLElement>(".review");
  const list = root?.querySelector<HTMLElement>("[data-list]");
  const pane = root?.querySelector<HTMLElement>("[data-detail]");
  const rowOf = (id: string) => data.items.find((r) => r.public_id === id) ?? null;
  // A message is selected by "c:<id>", which is not a report's public id: the
  // list holds both kinds and the pane has to draw whichever was chosen.
  const commentOf = (id: string) =>
    id.startsWith("c:")
      ? ((data.comments ?? []).find((c) => String(c.id) === id.slice(2)) ?? null)
      : null;
  // What the pane needs for a report: the whole of it, what this account may
  // do to it, and — once per board — the reports it could be merged into.
  // Kept once asked for, and dropped when the report changes: stepping back to
  // a row already read asks the server nothing, and two reads of the same row
  // at once — arriving, then a click on it — share the one request.
  const teams = new Map<string, ReturnType<typeof fetchTeam>>();
  const loadTeam = (id: string) => {
    let kept = teams.get(id);
    if (!kept) {
      kept = fetchTeam(id);
      teams.set(id, kept);
      kept.catch(() => teams.delete(id));
    }
    return kept;
  };
  async function fetchTeam(id: string) {
    // One request per report: what this account may do to it comes inside.
    const item = await api.get<ItemDetail>(`/api/items/${encodeURIComponent(id)}`);
    const allowed = item.team ?? { current: item.status, statuses: [] };
    const board = item.repository ? `${item.repository.organization?.slug ?? ""}/${item.repository.slug}` : "";
    return { item, allowed, others: (await mergeCandidates(board)).filter((o) => o.public_id !== id) };
  }

  // On a phone there is no room for the detail beside the list, so a chosen
  // row opens it as a drawer from the trailing edge. Only a tap opens it: arriving on the
  // screen, or a decision moving to the next row, keeps the list in view.
  const narrow = () => window.matchMedia("(max-width: 640px)").matches;
  const reading = (on: boolean) => screen?.classList.toggle("reading", on);

  // What the pane is showing. Choosing the same row again leaves it alone: the
  // pane may be holding a half-written edit or reply, and drawing it again
  // would throw that away.
  let shown = "";
  const draw = async (id: string, settle = false, force = false) => {
    if (!pane) return;
    if (id === shown && !force) return;
    const note = commentOf(id);
    if (note) {
      pane.innerHTML = views.reviewDetail(null, [], text, note);
      shown = id;
      return;
    }
    const row = rowOf(id);
    if (!row) {
      pane.innerHTML = views.reviewDetail(null, [], text);
      shown = "";
      return;
    }
    // Walking with the keys, once the selection has settled: a held arrow
    // passes a row every few milliseconds, and reading each one would reach
    // the server's per-address ceiling. A row only passed over is never asked
    // for. Arriving, clicking or deciding reads at once.
    if (settle && !teams.has(id)) {
      await new Promise((r) => setTimeout(r, SETTLE_MS));
      if (view.chosen !== id) return;
    }
    // The report as the team works on it — edit, delete, status, issue, the
    // conversation — not a read-only summary with its door somewhere else.
    const team = await loadTeam(id).catch((err) => {
      report(err);
      return null;
    });
    // Another row may have been chosen while this one loaded, or another read
    // of this one drew it first.
    if (view.chosen !== id || !team || (id === shown && !force)) return;
    pane.innerHTML = `<div class="rev-team">${views.itemTeamView(
      team.item,
      team.allowed,
      team.others,
      text,
      repliesTakePictures(team.item) ? PICTURES_ON_A_MESSAGE : 0,
    )}</div>`;
    shown = id;
    wireVote(id, pane);
    const changed = async () => {
      teams.delete(id);
      // The row wears what changed — its title, its status — and the pane
      // is drawn again from the server, where the list stays put. One read
      // serves both.
      const fresh = await loadTeam(id).catch(() => null);
      const at = data.items.find((r) => r.public_id === id);
      if (fresh && at) {
        at.title = fresh.item.title;
        at.status = fresh.item.status;
        if (fresh.item.moderation) at.moderation = fresh.item.moderation;
        redrawList();
      }
      await draw(id, false, true);
    };
    wireTeamItem(id, team.item, {
      scope: pane,
      changed,
      deleted: () => drop(id),
      // What is waiting goes through the queue's own decision, which moves on
      // to the next row. Anything else — approving a report rejected last
      // week — is a change like any other, and stays where it is.
      moderate: async (state) => {
        if (rowOf(id)?.moderation === "pending") return decide(state);
        try {
          await api.post(`/api/items/${encodeURIComponent(id)}/moderation`, { state });
          await changed();
        } catch (err) {
          report(err);
        }
      },
    });
  };

  // The list, drawn again where it stands: a row changed, and the scroll and
  // the selection stay where the person left them.
  const redrawList = () => {
    if (!list) return;
    const top = list.scrollTop;
    list.innerHTML = views.reviewList(data, view, text);
    list.scrollTop = top;
  };

  const select = (id: string, settle = false) => {
    view.chosen = id;
    for (const el of root?.querySelectorAll<HTMLElement>("[data-pick]") ?? []) {
      el.classList.toggle("on", el.dataset.pick === id);
    }
    root?.querySelector<HTMLElement>(`[data-pick="${CSS.escape(id)}"]`)?.scrollIntoView({ block: "nearest" });
    // The address follows the selection, so what somebody is looking at is
    // what they can send.
    const next = new URLSearchParams(location.search);
    next.set("at", id);
    history.replaceState({}, "", `/review?${next.toString()}`);
    void draw(id, settle);
  };

  // Delegated to the list, which survives a further page being drawn into it.
  list?.addEventListener("click", (event) => {
    const target = event.target as HTMLElement;
    const row = target.closest<HTMLElement>("[data-pick]");
    if (row) {
      select(row.dataset.pick ?? "");
      if (narrow()) reading(true);
      return;
    }
    if (target.closest("[data-more]")) void more();
    if (target.closest("[data-unfilter]")) go("/review?mode=all");
  });
  for (const el of root?.querySelectorAll<HTMLElement>("[data-mode]") ?? []) {
    el.addEventListener("click", () => go(`/review?mode=${el.dataset.mode}`));
  }
  root?.querySelector<HTMLElement>("[data-leave]")?.addEventListener("click", leaveReview);
  for (const el of root?.querySelectorAll<HTMLElement>("[data-shut]") ?? []) {
    el.addEventListener("click", () => reading(false));
  }

  // The archive's narrowings write the address and the screen redraws from
  // it. The chosen row is dropped: it may not be in what comes back.
  const narrowTo = (change: Partial<views.ReportFilters>) => {
    const next = new URLSearchParams({ mode: "all" });
    for (const [key, value] of Object.entries({ ...view.filters, ...change })) {
      if (value) next.set(key, String(value));
    }
    go(`/review?${next.toString()}`);
  };
  for (const el of root?.querySelectorAll<HTMLSelectElement>("[data-f]") ?? []) {
    el.addEventListener("change", () => narrowTo({ [el.dataset.f ?? ""]: el.value }));
  }
  const search = root?.querySelector<HTMLInputElement>("[data-q]");
  if (search) {
    search.addEventListener(
      "input",
      debounce(() => {
        reviewSearching = true;
        narrowTo({ q: search.value });
      }, 350),
    );
    // Typing is redrawn from the address like every other narrowing, so the
    // box is handed back the cursor where it was — otherwise every pause in
    // typing would throw the person out of the field they are typing in.
    //
    // Read and cleared here, once, rather than cleared on blur: replacing the
    // screen removes the old box, removing a focused element fires blur, and
    // that blur arrived before this line and wiped the flag it was about to
    // read.
    if (reviewSearching) {
      search.focus();
      search.setSelectionRange(search.value.length, search.value.length);
    }
  }
  reviewSearching = false;

  // A further page, appended rather than navigated to: the rows already read
  // stay where they are, and so does the scroll.
  let loaded = data.items.length;
  const more = async () => {
    const page = await ask(loaded).catch((err) => {
      report(err);
      return null;
    });
    if (!page || !list) return;
    data.items = [...data.items, ...(page.items ?? [])];
    loaded = data.items.length;
    view.more = (page.items ?? []).length === REVIEW_PAGE;
    const top = list.scrollTop;
    list.innerHTML = views.reviewList(data, view, text);
    list.scrollTop = top;
  };

  if (view.chosen) void draw(view.chosen);

  const order = () => [...(root?.querySelectorAll<HTMLElement>("[data-pick]") ?? [])];
  const step = (by: number) => {
    const all = order();
    const at = all.findIndex((el) => el.dataset.pick === view.chosen);
    const next = all[Math.min(all.length - 1, Math.max(0, at + by))];
    if (next) select(next.dataset.pick ?? "", true);
  };

  const decide = async (state: string) => {
    const id = view.chosen;
    if (!id) return;
    const all = order();
    const at = all.findIndex((el) => el.dataset.pick === id);
    // Only what is waiting can be decided: in the archive a key press on a
    // report published last March would undo somebody's work by accident.
    const row = rowOf(id);
    if (!id.startsWith("c:") && row?.moderation !== "pending") return;
    try {
      const comment = id.startsWith("c:");
      await api.post(
        comment
          ? `/api/comments/${encodeURIComponent(id.slice(2))}/moderation`
          : `/api/items/${encodeURIComponent(id)}/moderation`,
        { state: comment && state === "spam" ? "rejected" : state },
      );
    } catch (err) {
      report(err);
      return;
    }
    if (view.mode === "all" && view.filters.moderation !== "pending") {
      // In the archive the row stays, wearing its new state: a row that
      // vanished would read as a report that was deleted. Unless the list is
      // filtered to what is waiting — a repository's queue is exactly that —
      // where a decided row no longer answers the filter, and staying would
      // leave it looking undecided.
      if (row) row.moderation = state;
      all[at]?.classList.toggle("out", state === "rejected" || state === "spam");
      teams.delete(id);
      void draw(id, false, true);
      if (me) me = await api.get<Me>("/api/me").catch(() => me);
      renderNav();
      return;
    }
    drop(id);
  };

  // Gone from the list and from the data behind it, so stepping and counting
  // agree with what is on the screen; the next row is chosen. What a decision
  // does in the queue, and what deleting does anywhere.
  function drop(id: string): void {
    const all = order();
    const at = all.findIndex((el) => el.dataset.pick === id);
    const after = all[at + 1] ?? all[at - 1];
    all[at]?.remove();
    data.items = data.items.filter((r) => r.public_id !== id);
    if (id.startsWith("c:")) {
      data.comments = (data.comments ?? []).filter((c) => String(c.id) !== id.slice(2));
    }
    if (after) select(after.dataset.pick ?? "");
    else {
      if (pane) pane.innerHTML = views.reviewDetail(null, [], text);
      shown = "";
      reading(false);
    }
    void api.get<Me>("/api/me").then((fresh) => {
      me = fresh;
      renderNav();
    }).catch(() => undefined);
  }

  // Delegated, because the detail pane is rewritten on every selection and a
  // listener bound to its buttons would die with them.
  pane?.addEventListener("click", (event) => {
    const button = (event.target as HTMLElement).closest<HTMLButtonElement>("[data-act]");
    if (button) void decide(button.dataset.act ?? "");
  });

  reviewKeys?.();
  reviewKeys = keyboardFor({
    step,
    decide,
    // Escape closes the drawer first when one is open: on a phone it is the
    // thing in front, and leaving the whole screen from inside it would skip
    // a step the person can see.
    leave: () => (screen?.classList.contains("reading") ? reading(false) : leaveReview()),
    find: () => search?.focus(),
  });
}

/**
 * The reports a report can be merged into, per board, kept for a minute.
 *
 * Every report read in the pane needs its board's list, and the reports screen
 * is opened again and again: asked each time, it was a request per visit
 * against the per-address ceiling, for a list that changes when somebody
 * writes a report. At worst one that arrived this minute is not offered yet;
 * a change made here drops the list at once.
 */
const boardReports = new Map<string, { at: number; list: { public_id: string; title: string }[] }>();
const BOARD_REPORTS_MS = 60_000;

async function mergeCandidates(board: string): Promise<{ public_id: string; title: string }[]> {
  if (!board) return [];
  const kept = boardReports.get(board);
  if (kept && Date.now() - kept.at < BOARD_REPORTS_MS) return kept.list;
  const siblings = await api
    .get<views.ReportsData>(`/api/reports?${new URLSearchParams({ board, limit: "200" }).toString()}`)
    .catch(() => null);
  const list = (siblings?.items ?? []).map((r) => ({ public_id: r.public_id, title: r.title }));
  boardReports.set(board, { at: Date.now(), list });
  return list;
}

/** How long a selection has to stay before the pane asks the server for it. */
const SETTLE_MS = 150;

/** Set while the review screen is up, so its keys are removed when it is not. */
let reviewKeys: (() => void) | null = null;

function keyboardFor(on: {
  step: (by: number) => void;
  decide: (state: string) => Promise<void>;
  leave: () => void;
  find: () => void;
}): () => void {
  const handler = (event: KeyboardEvent) => {
    // Only on this screen. The listener is removed when the next one is drawn,
    // but a key pressed between the address changing and that draw would
    // otherwise decide something on a page nobody is looking at.
    if (location.pathname !== "/review") return;
    // Never while somebody is typing: a queue with single-letter shortcuts and
    // a search box is a queue that approves things while you look for one.
    const target = event.target as HTMLElement | null;
    if (target && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)) {
      if (event.key === "Escape") target.blur();
      return;
    }
    if (target?.isContentEditable) return;
    // Nor from a dropdown: its trigger is a button, and "p" pressed on the
    // filter somebody just used would publish whatever is chosen behind it.
    if (target?.closest(".sel")) {
      if (event.key === "Escape") target.blur();
      return;
    }
    if (event.metaKey || event.ctrlKey || event.altKey) return;

    switch (event.key) {
      case "ArrowDown":
      case "j":
        event.preventDefault();
        on.step(1);
        break;
      case "ArrowUp":
      case "k":
        event.preventDefault();
        on.step(-1);
        break;
      case "p":
      case "P":
        void on.decide("published");
        break;
      case "r":
      case "R":
        void on.decide("rejected");
        break;
      case "s":
      case "S":
        void on.decide("spam");
        break;
      case "/":
        event.preventDefault();
        on.find();
        break;
      case "Escape":
        on.leave();
        break;
    }
  };
  window.addEventListener("keydown", handler);
  return () => window.removeEventListener("keydown", handler);
}

/** The organisations, as cards, with the way to add one. */
async function showOrganizationsPanel(): Promise<void> {
  if (!(await requirePanel())) return;
  // The panel's list, not the public one: the organisations this person may
  // open, with what they are there and what is waiting in them.
  const list = await api.get<{ organizations: views.OrganizationRow[]; can_add: boolean; can_delete: boolean }>(
    "/api/panel/organizations",
  );

  await showInShell(
    views.organizationsScreen(list.organizations ?? [], { canAdd: list.can_add, canDelete: list.can_delete }, text) +
      // Each dialog only for whoever can open it: a form nobody may send is
      // one the server would refuse.
      (!list.can_add
        ? ""
        : views.modal(
        {
          id: "org-modal",
          title: text.t("panel.newOrganization"),
          lead: text.t("setup.orgLead"),
          body: views.iconField(text, "", "", "org-new-icon") + views.organizationFields(text, draft.forge),
          submit: text.t("auth.create"),
          formClass: "org-form",
        },
        text,
      )) +
      (!(list.organizations ?? []).some((o) => o.can_manage)
        ? ""
        : views.modal(
        {
          id: "org-rename",
          title: text.t("organizations.rename"),
          body: views.field({ id: "org-new-name", name: "name", label: text.t("panel.name"), required: true }, text),
          submit: text.t("action.save"),
          formClass: "org-rename-form",
        },
        text,
      )),
    "/panel/organizations",
  );


  onSubmit("form.org-form", async (data) => {
    await api.post("/api/organizations", {
      name: String(data.get("name") ?? ""),
      slug: String(data.get("slug") ?? ""),
      forge: String(data.get("forge") ?? "none"),
      base_url: String(data.get("base_url") ?? ""),
      forge_owner: String(data.get("forge_owner") ?? ""),
      icon: String(data.get("icon") ?? ""),
    });
    await render();
  });
  wireIconPicker();

  let renaming = "";
  root?.querySelectorAll<HTMLButtonElement>("[data-edit-org]").forEach((button) =>
    button.addEventListener("click", () => {
      renaming = button.dataset.editOrg ?? "";
      const input = root?.querySelector<HTMLInputElement>("#org-new-name");
      if (input) input.value = button.dataset.name ?? "";
      openModal("org-rename");
    }),
  );
  onSubmit("form.org-rename-form", async (data) => {
    await api.patch(`/api/orgs/${encodeURIComponent(renaming)}`, { name: String(data.get("name") ?? "") });
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-org]").forEach((button) =>
    button.addEventListener("click", async () => {
      const slug = button.dataset.deleteOrg ?? "";
      if (!(await askConfirm(i18n.t("org.delete"), i18n.t("organizations.confirmDelete", { name: button.dataset.name ?? slug })))) return;
      await api.del(`/api/orgs/${encodeURIComponent(slug)}`).catch(showFormError);
      await render();
    }),
  );

  // The forge fields follow the choice inside the modal too.
  const select = root?.querySelector<HTMLSelectElement>("#org-forge");
  const fields = root?.querySelector<HTMLDivElement>(".forge-fields");
  const sync = () => {
    if (fields) fields.hidden = (select?.value ?? "none") === "none";
  };
  select?.addEventListener("change", sync);
  sync();
}

/** The channel every repository may choose, set once by whoever runs the server. */
async function showNotifications(): Promise<void> {
  if (!(await requireAdministrator())) return;
  const channel = (await api
    .get<{ kind: string; channel_url: string; channel_to: string; channel_has_token: boolean }>("/api/notifications")
    .catch(() => null)) ?? { kind: "", channel_url: "", channel_to: "", channel_has_token: false };

  await showInShell(views.notificationsScreen(channel, text), "/panel/notifications");
  wireChannel(
    {
      save: (data) => api.put("/api/notifications", data),
      test: (data) => api.post("/api/notifications/test", data),
    },
    showNotifications,
  );
}

/** The name, the logo and the accent of this installation. */
async function showPersonalization(): Promise<void> {
  if (!(await requireAdministrator())) return;
  const instance = (await api
    .get<{ name: string; logo: string; accent: string }>("/api/instance")
    .catch(() => null)) ?? { name: "", logo: "", accent: "" };

  await showInShell(views.personalizationScreen(instance, text), "/panel/personalization");

  let logo = instance.logo;
  const logoFile = root?.querySelector<HTMLInputElement>("#logo-file");
  root?.querySelector<HTMLButtonElement>("[data-pick-logo]")?.addEventListener("click", () => logoFile?.click());
  logoFile?.addEventListener("change", async () => {
    const picked = await readAsDataURL(logoFile.files?.[0]);
    if (picked === null) return;
    logo = picked;
    await api.post("/api/instance", { ...instance, logo }).catch(showFormError);
    instanceLoaded = false;
    await render();
  });
  root?.querySelector<HTMLButtonElement>("[data-clear-logo]")?.addEventListener("click", async () => {
    await api.post("/api/instance", { ...instance, logo: "" }).catch(showFormError);
    instanceLoaded = false;
    await render();
  });
  // The accent swatches. Choosing one writes it into the form's hidden field,
  // which the Save button sends; the eighth takes any colour and wears it,
  // through a rule because this page's policy drops style attributes.
  const accentField = root?.querySelector<HTMLInputElement>("#accent");
  const swatches = [...(root?.querySelectorAll<HTMLElement>(".color-grid .accent-swatch") ?? [])];
  const choose = (chosen: HTMLElement | null, value: string) => {
    if (accentField) accentField.value = value;
    for (const s of swatches) {
      const on = s === chosen;
      s.classList.toggle("on", on);
      s.setAttribute("aria-checked", String(on));
    }
  };
  const paintCustom = (hex: string) =>
    setRule("accent-custom", /^#[0-9a-f]{6}$/i.test(hex) ? `.acc-custom.on{background:${hex}}` : undefined);
  root?.querySelectorAll<HTMLButtonElement>("[data-accent]").forEach((b) =>
    b.addEventListener("click", () => choose(b, b.dataset.accent ?? "")),
  );
  const customInput = root?.querySelector<HTMLInputElement>("[data-accent-custom]");
  if (customInput?.closest(".accent-swatch")?.classList.contains("on")) paintCustom(customInput.value);
  customInput?.addEventListener("input", () => {
    choose(customInput.closest<HTMLElement>(".accent-swatch"), customInput.value.toLowerCase());
    paintCustom(customInput.value);
  });

  onSubmit("form.instance-settings", async (data) => {
    await api.post("/api/instance", {
      name: String(data.get("name") ?? ""),
      logo,
      accent: String(data.get("accent") ?? ""),
    });
    instanceLoaded = false;
    await render();
  });
}

async function showPanelOrganization(orgSlug: string): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const base = `/api/orgs/${encodeURIComponent(orgSlug)}`;
  const data = await api.get<views.OrganizationPanel>(`${base}/panel`).catch(() => null);
  // Nobody with no board here learns anything from the address: the list of
  // organisations says which ones they may open.
  if (!data) return go("/panel/organizations");
  const team = data.can_manage
    ? await api
        .get<{ members: views.TeamMember[]; accounts: { username: string }[] }>(`${base}/team`)
        .catch(() => null)
    : null;
  const forge = data.organization.forge ?? "none";

  await showInShell(
    views.organizationScreen(data, team, me.username ?? "", location.origin, text) +
      // Only for whoever may add one: a dialog nobody can open is still a
      // form on the page, one the server would refuse.
      (!data.can_manage
        ? ""
        : views.modal(
        {
          id: "repo-modal",
          title: text.t("panel.newRepository"),
          lead: text.t("repo.lead"),
          body: views.iconField(text, "") + views.repositoryFields(text, forge, forge !== "none") + views.acceptsFields(text),
          submit: text.t("auth.create"),
          formClass: "repo-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "member-modal",
          title: text.t("people.roles"),
          lead: text.t("org.editLead"),
          body: views.teamRolesFields(data, text),
          submit: text.t("action.save"),
          formClass: "member-form",
        },
        text,
      )),
    `/panel/${orgSlug}`,
  );

  wireCredentials(orgSlug);

  // The one value on this page that has to end up in another application.
  root?.querySelector<HTMLButtonElement>("[data-copy-webhook]")?.addEventListener("click", async (event) => {
    const address = root?.querySelector<HTMLElement>("[data-webhook]")?.textContent ?? "";
    try {
      await navigator.clipboard.writeText(address);
      const button = event.currentTarget as HTMLButtonElement;
      const label = button.querySelector("span");
      if (label) label.textContent = i18n.t("action.copied");
    } catch {
      // No clipboard permission, or an insecure origin. The address is on the
      // screen and selectable, which is the fallback that always worked.
    }
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-open]").forEach((button) =>
    button.addEventListener("click", () => go(button.dataset.open ?? "/")),
  );

  onSubmit("form.repo-form", async (formData) => {
    await api.post(`${base}/repositories`, {
      name: String(formData.get("name") ?? ""),
      slug: String(formData.get("slug") ?? ""),
      forge_repo: String(formData.get("forge_repo") ?? ""),
      accepts_bugs: formData.get("accepts_bugs") !== null,
      accepts_ideas: formData.get("accepts_ideas") !== null,
      accepts_images: formData.get("accepts_images") !== null,
      icon: String(formData.get("icon") ?? ""),
    });
    await render();
  });
  wireIconPicker();

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-repo]").forEach((button) =>
    button.addEventListener("click", async () => {
      const slug = button.dataset.deleteRepo ?? "";
      if (!(await askConfirm(i18n.t("repo.delete"), i18n.t("organizations.confirmDeleteRepo", { name: button.dataset.name ?? slug })))) return;
      try {
        await api.del(`${base}/repos/${encodeURIComponent(slug)}`);
        await render();
      } catch (err) {
        showFormError(err);
      }
    }),
  );

  // The whole team is managed here: added with a role, changed with the pencil,
  // taken off with the bin. A role is one of two things — "owner" of the
  // organisation, or "repo:<slug>" for a collaborator on one repository — and
  // each is given or taken through the same endpoint.
  const setRole = (username: string, role: string, on: boolean) =>
    role === "owner"
      ? api.post(`${base}/team`, { username, repository: "", role: on ? "owner" : "" })
      : api.post(`${base}/team`, { username, repository: role.slice("repo:".length), role: on ? "collaborator" : "" });
  const rolesOf = (button: HTMLElement) => [
    ...(button.dataset.owner === "true" ? ["owner"] : []),
    ...(button.dataset.repos ?? "").split(" ").filter(Boolean).map((slug) => `repo:${slug}`),
  ];

  onSubmit("form.member-add", async (formData) => {
    await setRole(String(formData.get("username") ?? ""), String(formData.get("role") ?? ""), true);
    await render();
  });

  let member = "";
  let memberHad: string[] = [];
  root?.querySelectorAll<HTMLButtonElement>("[data-edit-member]").forEach((button) =>
    button.addEventListener("click", () => {
      member = button.dataset.editMember ?? "";
      memberHad = rolesOf(button);
      root?.querySelectorAll<HTMLInputElement>('form.member-form input[name="roles"]').forEach((box) => {
        box.checked = memberHad.includes(box.value);
      });
      openModal("member-modal");
    }),
  );
  onSubmit("form.member-form", async (formData) => {
    const want = formData.getAll("roles").map(String);
    // Given before taken: an owner turned into a collaborator is never, for a
    // moment, nobody on the team.
    for (const role of want.filter((r) => !memberHad.includes(r))) await setRole(member, role, true);
    for (const role of memberHad.filter((r) => !want.includes(r))) await setRole(member, role, false);
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-remove-member]").forEach((button) =>
    button.addEventListener("click", async () => {
      const username = button.dataset.removeMember ?? "";
      if (!(await askConfirm(i18n.t("org.removeMember"), i18n.t("org.confirmRemove", { name: username, organization: data.organization.name })))) return;
      try {
        for (const role of rolesOf(button)) await setRole(username, role, false);
        await render();
      } catch (err) {
        report(err);
      }
    }),
  );

  onSubmit("form.org-settings", async (formData) => {
    await api.patch(base, { name: String(formData.get("name") ?? ""), icon: String(formData.get("icon") ?? "") });
    toast(i18n.t("panel.saved"));
    await render();
  });

  root?.querySelector<HTMLButtonElement>("[data-delete-org]")?.addEventListener("click", async () => {
    if (!(await askConfirm(i18n.t("org.delete"), i18n.t("organizations.confirmDelete", { name: data.organization.name })))) return;
    try {
      await api.del(base);
      go("/panel/organizations");
    } catch (err) {
      report(err);
    }
  });
}

async function showPanelRepository(orgSlug: string, repoSlug: string): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<views.RepositoryPanel>(`${base}/panel`).catch(() => null);
  // Nobody who may not triage it learns anything from the address: the
  // organisation's page says what they may open.
  if (!repo) return go(`/panel/${encodeURIComponent(orgSlug)}`);
  const team = repo.can_manage
    ? await api
        .get<{ members: views.TeamMember[]; accounts: { username: string }[] }>(
          `/api/orgs/${encodeURIComponent(orgSlug)}/team`,
        )
        .catch(() => null)
    : null;

  await showInShell(views.repositoryScreen(repo, team, text), `/panel/${orgSlug}`);

  onSubmit("form.repo-settings", async (data) => {
    await api.patch(base, {
      name: String(data.get("name") ?? ""),
      forge_repo: String(data.get("forge_repo") ?? repo.forge_repo),
      accepts_bugs: data.get("accepts_bugs") !== null,
      accepts_ideas: data.get("accepts_ideas") !== null,
      accepts_images: data.get("accepts_images") !== null,
      icon: String(data.get("icon") ?? ""),
    });
    toast(i18n.t("panel.saved"));
    await render();
  });
  wireIconPicker();

  const channelURL = `${base}/channel`;
  wireChannel(
    {
      save: (data) => api.post(channelURL, data),
      test: (data) => api.post(`${channelURL}/test`, data),
    },
    render,
  );

  const teamURL = `/api/orgs/${encodeURIComponent(orgSlug)}/team`;
  onSubmit("form.team-add", async (data) => {
    await api.post(teamURL, {
      username: String(data.get("username") ?? ""),
      repository: repoSlug,
      role: "collaborator",
    });
    await render();
  });
  root?.querySelectorAll<HTMLButtonElement>("[data-revoke]").forEach((button) =>
    button.addEventListener("click", async () => {
      if (!(await askConfirm(i18n.t("repo.revoke"), i18n.t("repo.confirmRevoke", { name: button.dataset.name ?? "" })))) return;
      try {
        await api.post(teamURL, { username: button.dataset.revoke ?? "", repository: repoSlug, role: "" });
        await render();
      } catch (err) {
        report(err);
      }
    }),
  );

  root?.querySelector<HTMLButtonElement>("[data-delete-repo]")?.addEventListener("click", async () => {
    if (!(await askConfirm(i18n.t("repo.delete"), i18n.t("organizations.confirmDeleteRepo", { name: repo.name })))) return;
    try {
      await api.del(base);
      go(`/panel/${encodeURIComponent(orgSlug)}`);
    } catch (err) {
      report(err);
    }
  });
}

// --- odds and ends ---------------------------------------------------------

function debounce<T extends (...args: never[]) => void>(fn: T, ms: number): T {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return ((...args: never[]) => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  }) as T;
}

/**
 * The language of the account that is signed in, if it has one.
 *
 * The browser's preference decides for somebody with no account, which is
 * every visitor of a public board. For somebody signed in it is a setting of
 * theirs, so it follows them to the next machine instead of living in one
 * browser's storage.
 */
async function applyAccountLanguage(): Promise<void> {
  const chosen = me?.language;
  if (chosen && chosen !== i18n.current) await i18n.setLanguage(chosen);
}

/** The first run reads English until the administrator says otherwise. */
async function startInEnglish(): Promise<void> {
  if (i18n.chosen) return;
  await i18n.setLanguage("en");
}

void (async () => {
  await i18n.ready;
  me = await api.get<Me>("/api/me").catch(() => null);
  await applyAccountLanguage();
  i18n.subscribe(() => renderNav());
  // Once, for every dialog the product will ever draw.
  wireDialogs();
  wirePanelToggle();
  // And for every select: the house dropdown over each one, wherever it is
  // drawn, now or later.
  watchSelects();
  await render();
})();
