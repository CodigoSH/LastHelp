/**
 * The application shell: routing, fetching, and putting the templates on screen.
 *
 * Rendering is deliberately per-view rather than one big redraw. Reassigning the
 * whole container on every interaction destroys and recreates every child — the
 * entrance animations replay, a focused field loses focus, and a list that was
 * scrolled jumps back to the top. This house has measured that three times.
 */
import { api, ApiError } from "./api";
import type {
  ItemDetail,
  ItemSummary,
  Me,
  OrganizationSummary,
  ReportField,
  RepositorySummary,
  Status,
  Submitted,
} from "./api";
import { i18n } from "./services/i18n";
import { availableLocales } from "./locales/metadata";
import { applyTheme, currentTheme, restoreTheme, type Theme } from "./services/theme";
import { escapeAttr, escapeHTML } from "./escape";
import { toast, toastError } from "./services/toast";
import { shrink, TooLarge } from "./services/image";
import { wireWords } from "./services/words";
import { NoWebP, putImage } from "./services/landingImage";
import { attachChartHover } from "./services/chartHover";
import { setRule } from "./services/dynamicStyles";
import { watchSelects } from "./services/select";
import { attachPasswordToggles } from "./services/password";
import * as views from "./views/templates";
import { icons } from "./views/icons";

const text: views.Text = {
  t: (key, params) => i18n.t(key, params),
  date: (value) => i18n.date(value),
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

// --- the shell -------------------------------------------------------------

function renderNav(): void {
  if (!nav) return;

  // No language picker here. A visitor who has not chosen one is already
  // reading their browser's language, and somebody who wants a different one
  // sets it once — in the wizard on the first run, or under Appearance — rather
  // than being offered a list of twenty on every page of the product.
  /**
   * Arranging the front page, from the bar above it.
   *
   * One control that turns into the other, which is how the dashboard in this
   * house does it: a pencil while the page is just a page, a tick in the accent
   * colour while it is being arranged. It used to be a button inside the page
   * itself, which put a control for the operator in the middle of the thing
   * every visitor sees.
   *
   * Only on the front page, and only for somebody who can change it.
   */
  const composing = me?.admin === true && location.pathname === "/" && landingEditing;
  const compose =
    me?.admin === true && location.pathname === "/"
      ? composing
        ? // While arranging: a plus that opens a short menu, and the tick that
          // finishes. The dashboard's shape exactly — adding is two decisions,
          // a module or a group, and only the first needs a picker.
          `<div class="add-wrap">
             <button class="icon-button" data-add-menu
                     title="${escapeAttr(i18n.t("landing.add"))}"
                     aria-label="${escapeAttr(i18n.t("landing.add"))}"
                     aria-haspopup="menu" aria-expanded="false">${icons.add(16)}</button>
             <div class="add-menu" role="menu" hidden>
               <button type="button" class="add-menu-item" role="menuitem" data-open-modal="module-picker">
                 ${icons.module(14)}<span>${escapeHTML(i18n.t("landing.addModule"))}</span></button>
               <button type="button" class="add-menu-item" role="menuitem" data-add-section>
                 ${icons.section(14)}<span>${escapeHTML(i18n.t("landing.addSection"))}</span></button>
             </div>
           </div>
           <button class="icon-button composing" data-edit-done
                   title="${escapeAttr(i18n.t("action.done"))}"
                   aria-label="${escapeAttr(i18n.t("action.done"))}">${icons.check(16)}</button>`
        : `<button class="icon-button" data-edit-page
                   title="${escapeAttr(i18n.t("landing.compose"))}"
                   aria-label="${escapeAttr(i18n.t("landing.compose"))}">${icons.edit(16)}</button>`
      : "";

  // The panel is somewhere to go, so it is a button. Leaving is NOT here: the
  // panel's own sidebar carries it, and a red control in the bar of every
  // public page is the one thing on that bar nobody came for.
  // The panel's own control, which opens it and closes it.
  //
  // The dashboard's drawer toggle: a box whose side column fills while the
  // thing is open, so the icon says which state you are in rather than what
  // clicking will do. Disabled while the front page is being arranged — one
  // mode at a time, and a control that does something else mid-gesture is a
  // control somebody hits by accident.
  const inPanel = document.body.classList.contains("admin-view");
  const inReview = location.pathname === "/review";
  const panel = me
    ? `<button class="icon-button panel-toggle${composing || inReview ? " cannot" : ""}" data-panel-toggle
               aria-pressed="${inPanel}"
               title="${escapeAttr(i18n.t("nav.panel"))}"
               aria-label="${escapeAttr(i18n.t("nav.panel"))}">
         <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
              aria-hidden="true">
           <rect x="3" y="3" width="18" height="18" rx="2" stroke-width="1.5" />
           <rect class="panel-indicator${inPanel ? " on" : ""}" x="15" y="3" width="6" height="18" />
           <path d="M15 3V21" stroke-width="1.5" />
         </svg>
       </button>`
    : "";

  /**
   * What is waiting, and the way into it.
   *
   * Shown to whoever may triage something — a collaborator on a repository or
   * the owner of an organisation — and NOT to an administrator as such: the
   * person who moderates daily is usually the first and not the second, and
   * hanging this off `admin` would hide it from exactly them.
   *
   * Never hidden when the queue is empty. It disappears in the dashboard in
   * this house when the FEATURE is off, which is a decision of installation; a
   * control that comes and goes with the work is one nobody learns is there,
   * and it takes away the only way to see that you are finished.
   *
   * The count is the server's, already narrowed to the boards this account may
   * open — the page has no way to know which those are, and a number it worked
   * out itself would be a second opinion on a permission.
   */
  const canTriage = (me?.memberships?.length ?? 0) > 0;
  const waiting = (me?.pending?.reports ?? 0) + (me?.pending?.messages ?? 0);
  const review =
    me && canTriage
      ? // While the screen is open this is its way out, the way the status
        // icon closes Lastboard's status page: it goes back where you were.
        `<a class="review-pill${inReview ? " on" : ""}${waiting > 0 ? " has" : ""}"
             href="${escapeAttr(inReview ? reviewCameFrom : "/review")}" data-review
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

  nav.querySelector<HTMLButtonElement>("[data-signout]")?.addEventListener("click", async () => {
    await api.del("/api/session").catch(() => undefined);
    me = null;
    go("/");
  });
}

/**
 * The front page while it is being arranged.
 *
 * Kept in module scope because editing is a mode the page is in, not a thing
 * one render knows about: a module saved from the modal has to redraw the page
 * it is on, and the page has to come back in the same mode.
 */
let landingModules: views.LandingModule[] = [];
let landingContent: views.LandingContent = {};
let landingBoards: views.LandingBoard[] = [];
let landingEditing = false;

/**
 * Where each module sits, as a rule rather than an attribute.
 *
 * The dashboard writes these four numbers into a style attribute. Here that
 * would be dropped on the floor: the content policy this product serves has no
 * unsafe-inline, so every module would fall back to cell one and stack. Same
 * numbers, same custom properties, delivered the way the policy allows.
 */
/**
 * The cells are square, which is what lets one divisor serve both axes when
 * something is dragged: a row is as tall as a column is wide, so the same
 * arithmetic reads a sideways drag and a downwards one. The stylesheet cannot
 * know the width, so the row height is synced from here — through the sheet
 * rather than the element, because this product's content policy drops style
 * attributes. It runs on every draw and on resize, not only while arranging:
 * a visitor who never edits anything still has to see square cells.
 */
function squareLandingCells(): void {
  const grid = root?.querySelector<HTMLElement>(".landing-grid");
  if (!grid) return;
  const width = (grid.getBoundingClientRect().width - (views.LANDING_COLUMNS - 1) * 16) /
    views.LANDING_COLUMNS;
  if (width > 0) setRule("landing-rows", `.landing-grid{--row-height:${width}px}`);
}

window.addEventListener("resize", squareLandingCells);

function placeLandingModules(modules: views.LandingModule[]): void {
  // A border somebody chose, through the sheet like everything else here: this
  // product's content policy drops a style attribute.
  setRule(
    "landing-borders",
    modules
      .filter((m) => m.border_color || m.border_width || m.bare || m.clear)
      .map(
        (m) =>
          `.landing-cell[data-module="${cssEscape(m.id)}"] > :not(.kebab-btn):not(.kebab-menu):not(.resize-handle):not(.section-legend){` +
          `${m.border_color ? `border-color:${m.border_color};` : ""}` +
          `${m.bare ? "border-width:0;" : m.border_width ? `border-width:${m.border_width}px;` : ""}` +
          // Nothing behind it and nothing under it: a surface that keeps its
          // shadow while losing its fill reads as a pane of glass rather than
          // as part of the page.
          `${m.clear ? "background:transparent;box-shadow:none;" : ""}}` +
          // The box a block of words lives in is one level further in, so it
          // has to be told as well or the fill stays where it always was.
          `${
            m.clear
              ? `.landing-cell[data-module="${cssEscape(m.id)}"] .words-box{background:transparent;box-shadow:none;${m.bare ? "border-width:0;" : ""}}`
              : ""
          }`,
      )
      .join("\n"),
  );
  setRule(
    "landing-modules",
    modules
      .map(
        (m) =>
          `.landing-cell[data-module="${cssEscape(m.id)}"]{--x:${m.x};--y:${m.y};--w:${m.w};--h:${m.h}}`,
      )
      .join("\n"),
  );
}

/** An id is ours, but it reaches a selector, so it is escaped like one. */
function cssEscape(value: string): string {
  return value.replace(/["\\]/g, "\\$&");
}

/**
 * What this installation decided the page is made of, and what goes in it.
 *
 * Failing softly: an unreachable layout leaves the page as the default rather
 * than as an error, because this is the one screen a stranger sees first.
 */
async function fetchLanding(): Promise<void> {
  type Composed = {
    modules: views.LandingModule[];
    content?: views.LandingContent;
    boards?: views.LandingBoard[];
  };
  const composed = await api
    .get<Composed>("/api/landing")
    .catch((): Composed => ({ modules: [] }));

  landingModules = composed.modules ?? [];
  landingContent = composed.content ?? {};
  landingBoards = composed.boards ?? [];
}

function drawLanding(orgs: OrganizationSummary[]): void {
  placeLandingModules(landingModules);
  show(
    views.landing(
      orgs,
      me?.admin === true,
      text,
      { name: instanceName, logo: instanceLogo },
      landingModules,
      landingEditing,
      landingContent,
      landingBoards,
    ),
  );
  // After the markup is on the page, never before: the row height is measured
  // from the grid, and a grid that has not been written yet measures nothing.
  // Placed before, this silently left the public page on the fallback height
  // and only the second draw ever looked right.
  squareLandingCells();
  // The bar above the page carries the control that turns arranging on and off,
  // so it has to be redrawn when that changes: the pencil becomes a tick.
  renderNav();
  wireLanding(orgs);
}

/**
 * Arranging the page, on the page.
 *
 * Every change is saved as it is made rather than collected behind a button.
 * The page under it is the thing being changed, so it is already showing what
 * would be saved, and a separate save turns that into a question about whether
 * what you are looking at is real.
 */
function wireLanding(orgs: OrganizationSummary[]): void {
  /**
   * Save, and read the page back when what it is made of changed.
   *
   * Moving and resizing needs no reread: the modules are the same ones and
   * their content has not moved with them. Adding one, removing one or pointing
   * one at a different board does — a module that has just appeared has nothing
   * to draw until the server says what goes in it.
   */
  /**
   * Writing a block of words, on the page, where it is read.
   *
   * Saved as it is typed like everything else here, but not on every keystroke:
   * a PUT per character would be a request per character. The wait is short
   * enough that leaving the page straight after typing still lands, and
   * finishing the arrangement saves whatever is outstanding anyway.
   */
  let penWaiting: ReturnType<typeof setTimeout> | undefined;
  wireWords({
    root: document,
    bar: document.querySelector<HTMLElement>(".words-bar"),
    image: {
      pick: pickImageFile,
      upload: putImage,
      // A browser with no WebP encoder gets told that; everything else goes
      // through the one place that turns a refusal into words.
      failed: (err) => (err instanceof NoWebP ? toastError(i18n.t("error.no_webp")) : report(err)),
    },
    onChange: (id, html) => {
      const target = landingModules.find((m) => m.id === id);
      if (!target) return;
      target.body = html;
      clearTimeout(penWaiting);
      penWaiting = setTimeout(() => void save(), 700);
    },
  });

  const save = async (refresh = false) => {
    try {
      clearTimeout(penWaiting);
      await api.put("/api/landing", {
        modules: landingModules.map((m) => ({
          id: m.id,
          kind: m.kind,
          x: m.x,
          y: m.y,
          w: m.w,
          h: m.h,
          ...(m.kind === "text" ||
          m.kind === "hero" ||
          m.kind === "call_to_action" ||
          m.kind === "section"
            ? { body: m.body ?? "" }
            : {}),
          ...(m.scope ? { scope: m.scope } : {}),
          ...(m.label ? { label: m.label } : {}),
          ...(m.name ? { name: m.name } : {}),
          ...(m.border_color ? { border_color: m.border_color } : {}),
          ...(m.border_width ? { border_width: m.border_width } : {}),
          ...(m.bare ? { bare: true } : {}),
          ...(m.clear ? { clear: true } : {}),
          ...(m.rows ? { rows: m.rows } : {}),
          ...(m.parent ? { parent: m.parent } : {}),
        })),
      });
      if (refresh) await fetchLanding();
    } catch (err) {
      report(err);
    }
  };

  // Looked up in the document, not in the page: the control lives in the bar
  // above it now, which is outside what this view draws.
  document.querySelector<HTMLButtonElement>("[data-edit-page]")?.addEventListener("click", () => {
    landingEditing = true;
    drawLanding(orgs);
  });
  document.querySelector<HTMLButtonElement>("[data-edit-done]")?.addEventListener("click", () => {
    landingEditing = false;
    drawLanding(orgs);
  });

  /**
   * Put a new module on the page.
   *
   * Called from the picker, which is a grid of cards saying what each one
   * draws, and from the menu beside it for a group. Both end here: a module is
   * a module whichever door it came through.
   */
  const add = async (kind: views.LandingModule["kind"]) => {
    // Under everything: the one place a new module is certainly not on top of
    // another one, whatever the page already looks like.
    const bottom = landingModules.reduce((low, m) => Math.max(low, m.y + m.h), 1);
    const added: views.LandingModule = {
      id: `${kind}-${Date.now().toString(36)}`,
      kind,
      x: 1,
      y: bottom,
      // Sized from what each one draws, not from the width of the page.
      //
      // Everything used to arrive twelve columns wide, so a page of modules was
      // a stack of bands and the only way to get anything else was to drag every
      // corner. Now each kind arrives at a size that suits it, and the corner is
      // for changing your mind — which is the way round the dashboard has it.
      ...sizeFor(kind),
      body: "",
    };
    landingModules = [...landingModules, added];
    await save(true);
    drawLanding(orgs);
    // Opened straight away, the way the dashboard does it: a module you just
    // added with nothing in it is a module you were about to configure.
    openModuleConfig(added.id, orgs);
  };

  root?.querySelectorAll<HTMLButtonElement>("[data-pick]").forEach((card) =>
    card.addEventListener("click", async () => {
      card.closest("dialog")?.close();
      await add((card.dataset.pick ?? "text") as views.LandingModule["kind"]);
    }),
  );

  // A group comes straight from the menu: there is only one of it, so there is
  // nothing to pick.
  document.querySelector<HTMLButtonElement>("[data-add-section]")?.addEventListener("click", async () => {
    closeMenus();
    await add("section");
  });

  // The plus, and its short menu.
  const adder = document.querySelector<HTMLButtonElement>("[data-add-menu]");
  adder?.addEventListener("click", (event) => {
    event.stopPropagation();
    const menu = adder.parentElement?.querySelector<HTMLElement>(".add-menu");
    const wasOpen = menu ? !menu.hidden : false;
    closeMenus();
    if (menu && !wasOpen) {
      menu.hidden = false;
      adder.setAttribute("aria-expanded", "true");
    }
  });

  // The menu opens one at a time, and anything else closes it: a second menu
  // left open behind the first is two sets of actions on screen for two
  // different modules.
  // Across the document, not just the page: the menu that adds things lives in
  // the bar above it, and a menu left open up there while another opens down
  // here is two sets of actions on screen for two different things.
  const closeMenus = () =>
    document.querySelectorAll<HTMLElement>(".kebab-menu").forEach((menu) => {
      menu.hidden = true;
      document
        .querySelectorAll<HTMLButtonElement>("[data-module-menu], [data-add-menu]")
        .forEach((button) => button.setAttribute("aria-expanded", "false"));
    });

  root?.querySelectorAll<HTMLButtonElement>("[data-module-menu]").forEach((button) =>
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      const id = button.dataset.moduleMenu ?? "";
      const menu = root?.querySelector<HTMLElement>(`.kebab-menu[data-menu-for="${id}"]`);
      const wasOpen = menu ? !menu.hidden : false;
      closeMenus();
      if (menu && !wasOpen) {
        menu.hidden = false;
        button.setAttribute("aria-expanded", "true");
      }
    }),
  );
  document.addEventListener("click", closeMenus, { once: true });

  root?.querySelectorAll<HTMLButtonElement>("[data-edit-module]").forEach((button) =>
    button.addEventListener("click", () => openModuleConfig(button.dataset.editModule ?? "", orgs)),
  );

  // Removing asks first, the way it does over there. A module somebody spent a
  // minute pointing at the right board should not go on one stray click.
  let pending = "";
  const confirmRemove = async () => {
    const gone = pending;
    pending = "";
      // What was inside a removed section comes out onto the page rather than
      // being removed with it: those are modules somebody composed, and a
      // container disappearing is not a decision to throw away its contents.
      // Left pointing at a section that is not there, the layout would be
      // refused outright and the whole page would fail to save.
    landingModules = landingModules.filter((m) => m.id !== gone);
    spillOnto(landingModules.filter((m) => m.parent === gone));
    await save(true);
    drawLanding(orgs);
  };

  root?.querySelectorAll<HTMLButtonElement>("[data-remove-module]").forEach((button) =>
    button.addEventListener("click", () => {
      pending = button.dataset.removeModule ?? "";
      const target = landingModules.find((m) => m.id === pending);
      const section = target?.kind === "section";
      const dialog = root?.querySelector<HTMLDialogElement>("#confirm-remove");
      const title = dialog?.querySelector<HTMLElement>("[data-confirm-title]");
      const message = dialog?.querySelector<HTMLElement>("[data-confirm-message]");
      if (title) title.textContent = i18n.t(section ? "landing.removeSection" : "landing.removeModule");
      // A section says what happens to what is inside it: those modules come
      // back onto the page rather than going with it, and somebody about to
      // click should know that before they do.
      if (message) {
        message.textContent = i18n.t(section ? "landing.removeSectionSays" : "landing.removeModuleSays");
      }
      closeMenus();
      openModal("confirm-remove");
    }),
  );

  root?.querySelector<HTMLButtonElement>("[data-confirm]")?.addEventListener("click", async () => {
    root?.querySelector<HTMLDialogElement>("#confirm-remove")?.close();
    await confirmRemove();
  });

  if (landingEditing) wireLandingGrid(orgs, () => save());

  // One dialog, filled for whichever module opened it.
  const dialog = root?.querySelector<HTMLFormElement>("form.module-config");
  dialog?.querySelectorAll<HTMLButtonElement>("[data-tab]").forEach((tab) =>
    tab.addEventListener("click", () => showTab(dialog, tab.dataset.tab ?? "general")),
  );

  // The three rows of buttons that stand in for a field: each writes its value
  // into the hidden input beside it, which is what the form submits.
  const rows: Array<[string, string, string]> = [
    ["[data-at]", "at", "labelAt"],
    ["[data-colour]", "colour", "borderColour"],
    ["[data-width]", "width", "borderWidth"],
  ];
  for (const [selector, key, field] of rows) {
    dialog?.querySelectorAll<HTMLButtonElement>(selector).forEach((button) =>
      button.addEventListener("click", () => {
        const value = button.dataset[key] ?? "";
        const input = dialog.querySelector<HTMLInputElement>(`[name=${field}]`);
        if (input) input.value = value;
        pickOne(dialog, selector, key, value);
        if (key === "at") {
          showMark(dialog, value);
          dialog.querySelector<HTMLElement>("[data-marks]")!.hidden = true;
          dialog.querySelector("[data-open-marks]")?.setAttribute("aria-expanded", "false");
        }
      }),
    );
  }

  // The button that opens that menu, and anything else that closes it.
  const marksButton = dialog?.querySelector<HTMLButtonElement>("[data-open-marks]");
  marksButton?.addEventListener("click", (event) => {
    event.stopPropagation();
    const menu = dialog?.querySelector<HTMLElement>("[data-marks]");
    if (!menu) return;
    menu.hidden = !menu.hidden;
    marksButton.setAttribute("aria-expanded", String(!menu.hidden));
  });
  dialog?.addEventListener("click", (event) => {
    if ((event.target as HTMLElement | null)?.closest("[data-open-marks], [data-marks]")) return;
    const menu = dialog.querySelector<HTMLElement>("[data-marks]");
    if (menu) menu.hidden = true;
  });
  dialog?.addEventListener("submit", async () => {
    const data = new FormData(dialog);
    const id = String(data.get("id") ?? "");
    const target = landingModules.find((m) => m.id === id);
    if (!target) return;
    // A block of words is written where it is seen, so the dialog does not ask
    // for it: reading an empty field here would wipe what somebody just typed.
    if (target.kind === "call_to_action") {
      target.body = String(data.get("label") ?? "").trim();
    }
    if (lists(target.kind)) target.rows = Number(data.get("rows") ?? 0) || 0;
    if (usesHeader(target.kind)) target.name = String(data.get("name") ?? "").trim();
    // A section's name is its body: it is the pill on its border and nothing
    // else, so there is no second name to keep.
    if (target.kind === "section") target.body = String(data.get("name") ?? "").trim();
    target.border_color = String(data.get("borderColour") ?? "");
    // Nought is a choice here and nowhere else: the four segments are none,
    // thin, regular and thick, and "none" is the one that has to be told apart
    // from nobody having touched the control. An untouched control leaves the
    // field empty; a chosen "none" leaves a nought in it.
    const chosenWidth = String(data.get("borderWidth") ?? "");
    target.bare = chosenWidth === "0";
    target.border_width = Number(chosenWidth) || 0;
    target.clear = data.get("clear") !== null;
    if (takesScope(target.kind)) target.scope = String(data.get("scope") ?? "");
    if (usesHeader(target.kind)) {
      target.label = String(data.get("labelAt") ?? "top") as views.LandingModule["label"];
    }
    // Reread: pointing a module at a different board changes what goes in it,
    // and the words of a text module are the only thing this dialog can change
    // that the page already knows.
    await save(takesScope(target.kind));
    drawLanding(orgs);
  });
}

/**
 * Dragging and resizing, ported from the dashboard rather than re-derived.
 *
 * The arithmetic is theirs: the cell under the TOP-LEFT corner of what is being
 * carried, not under the pointer, because dragging a wide module by its middle
 * and having it land where the pointer is puts it somewhere nobody aimed. The
 * snap is strict — floor to a cell, clamp inside the grid — and the ghost shows
 * where it would land before the mouse is let go.
 *
 * Green means it fits. Red means it overlaps something, and the drop is
 * refused: the grid has no opinion about two modules in one cell, so this is
 * the only thing between an operator and a page with a module hidden under
 * another one.
 */
function wireLandingGrid(orgs: OrganizationSummary[], save: () => Promise<void>): void {
  const grid = root?.querySelector<HTMLElement>(".landing-grid.editing");
  const ghost = root?.querySelector<HTMLElement>(".landing-ghost");
  if (!grid || !ghost) return;

  const GAP = 16;
  const cols = views.LANDING_COLUMNS;
  const cell = () => {
    const box = grid.getBoundingClientRect();
    return { box, width: (box.width - (cols - 1) * GAP) / cols };
  };

  /**
   * Does this fit, in whichever grid it is being dropped into?
   *
   * Two grids, two sets of bounds: the page is twelve columns, a section is as
   * many as it is wide. And a module is only measured against what shares its
   * grid — a child at 1,1 of a section and a module at 1,1 of the page are not
   * on top of each other, and asking whether they are is asking a question with
   * no meaning.
   */
  const fits = (
    id: string,
    x: number,
    y: number,
    w: number,
    h: number,
    parent = "",
  ): boolean => {
    if (x < 1 || y < 1) return false;
    if (parent === "") {
      if (x + w - 1 > cols) return false;
    } else {
      const holder = landingModules.find((m) => m.id === parent);
      if (!holder || holder.kind !== "section") return false;
      if (x + w - 1 > holder.w || y + h - 1 > holder.h) return false;
    }
    return !landingModules.some(
      (m) =>
        m.id !== id &&
        (m.parent ?? "") === parent &&
        x < m.x + m.w &&
        m.x < x + w &&
        y < m.y + m.h &&
        m.y < y + h,
    );
  };

  // Lastboard damps the ghost to about 60fps rather than answering every move
  // event: each update rewrites a rule in the sheet, and a pointer produces far
  // more events than a screen has frames.
  let damped: ReturnType<typeof setTimeout> | null = null;
  const damp = (run: () => void) => {
    if (damped) return;
    damped = setTimeout(() => {
      damped = null;
      run();
    }, 16);
  };

  const showGhost = (x: number, y: number, w: number, h: number, ok: boolean, parent = "") => {
    // The ghost is moved into whichever grid it is describing. Its rule places
    // it from --x and --y, and those mean the page's columns out here and the
    // section's columns in there, so the same element says the right thing in
    // both once it is in the right parent.
    const host = parent
      ? grid.querySelector<HTMLElement>(`.landing-section[data-section="${cssEscape(parent)}"]`)
      : grid;
    if (host && ghost.parentElement !== host) host.appendChild(ghost);
    grid.querySelectorAll(".landing-group").forEach((el) =>
      el.classList.toggle("drop-target", parent !== "" && el.getAttribute("data-module") === parent),
    );
    ghost.hidden = false;
    // Through the sheet, like the modules: setProperty on the element writes
    // the style attribute, which this product's content policy drops.
    // Under the grid in the selector, so it outscores the stylesheet's own
    // `.landing-ghost` rather than tying with it and depending on sheet order.
    setRule(
      "landing-ghost",
      `.landing-grid .landing-ghost{--x:${x};--y:${y};--w:${w};--h:${h}}`,
    );
    ghost.classList.toggle("invalid", !ok);
  };
  const hideGhost = () => {
    if (ghost.parentElement !== grid) grid.appendChild(ghost);
    grid.querySelectorAll(".landing-group.drop-target").forEach((el) =>
      el.classList.remove("drop-target"),
    );
    ghost.hidden = true;
    ghost.classList.remove("invalid");
    setRule("landing-ghost");
  };

  let carrying: views.LandingModule | null = null;
  let grabX = 0;
  let grabY = 0;

  grid.querySelectorAll<HTMLElement>(".landing-cell").forEach((cellEl) => {
    cellEl.addEventListener("dragstart", (event) => {
      // The innermost module wins. A child sits inside its section's own cell,
      // and both are draggable, so without this the event reaches the section
      // afterwards and the section decides it is the thing being carried —
      // picking up a module out of a group would pick up the group.
      event.stopPropagation();
      const id = cellEl.dataset.module ?? "";
      carrying = landingModules.find((m) => m.id === id) ?? null;
      if (!carrying) return;
      const box = cellEl.getBoundingClientRect();
      // Where inside the module it was picked up, so it lands under the hand
      // rather than jumping so its corner is at the pointer.
      grabX = (event as DragEvent).clientX - box.left;
      grabY = (event as DragEvent).clientY - box.top;
      cellEl.classList.add("carrying");
      (event as DragEvent).dataTransfer?.setData("text/plain", id);
    });
    cellEl.addEventListener("dragend", (event) => {
      event.stopPropagation();
      cellEl.classList.remove("carrying");
      carrying = null;
      hideGhost();
    });
  });

  /**
   * Which section, if any, the carried module's top-left corner is over.
   *
   * A section is never dropped into a section, so one is never looking for a
   * home. The corner rather than the pointer, for the same reason every other
   * measurement here uses it: what lands somewhere is the module, not the hand.
   */
  const sectionUnder = (cornerX: number, cornerY: number): HTMLElement | null => {
    if (!carrying || carrying.kind === "section") return null;
    for (const el of grid.querySelectorAll<HTMLElement>(".landing-section")) {
      const box = el.getBoundingClientRect();
      if (cornerX >= box.left && cornerX <= box.right && cornerY >= box.top && cornerY <= box.bottom) {
        return el;
      }
    }
    return null;
  };

  /**
   * The cell the carried module would land in, and the grid it would land in.
   *
   * Two grids. The page's cells are measured from the page's grid, a section's
   * from that section's own box — measured, not derived by subtracting the
   * section's position from a page coordinate. The dashboard this comes from
   * derives it, and that is only right while a section has no padding: its own
   * has twelve pixels of it, so its inner cells are narrower than the page's
   * and the two drift apart across the section.
   */
  const targetCell = (event: DragEvent) => {
    const cornerX = event.clientX - grabX;
    const cornerY = event.clientY - grabY;
    const w = carrying?.w ?? 1;
    const h = carrying?.h ?? 1;

    const host = sectionUnder(cornerX, cornerY);
    if (host) {
      const parent = host.dataset.section ?? "";
      const holder = landingModules.find((m) => m.id === parent);
      const box = host.getBoundingClientRect();
      if (holder) {
        const colWidth = (box.width - (holder.w - 1) * GAP) / holder.w;
        const rowHeight = (box.height - (holder.h - 1) * GAP) / holder.h;
        const x = Math.max(1, Math.min(holder.w - w + 1,
          Math.floor((cornerX - box.left) / (colWidth + GAP)) + 1));
        const y = Math.max(1, Math.min(holder.h - h + 1,
          Math.floor((cornerY - box.top) / (rowHeight + GAP)) + 1));
        return { x, y, parent };
      }
    }

    const { box, width } = cell();
    const x = Math.max(1, Math.min(cols - w + 1,
      Math.floor((cornerX - box.left) / (width + GAP)) + 1));
    const y = Math.max(1, Math.floor((cornerY - box.top) / (width + GAP)) + 1);
    return { x, y, parent: "" };
  };

  grid.addEventListener("dragover", (event) => {
    if (!carrying) return;
    event.preventDefault();
    const moved = event as DragEvent;
    damp(() => {
      if (!carrying) return;
      const { x, y, parent } = targetCell(moved);
      showGhost(x, y, carrying.w, carrying.h,
        fits(carrying.id, x, y, carrying.w, carrying.h, parent), parent);
    });
  });

  grid.addEventListener("drop", async (event) => {
    if (!carrying) return;
    event.preventDefault();
    const { x, y, parent } = targetCell(event as DragEvent);
    const moving = carrying;
    hideGhost();

    const leaving = (moving.parent ?? "") !== "" && parent === "";
    if (!fits(moving.id, x, y, moving.w, moving.h, parent)) {
      // Refusing a drop that collides is right everywhere except here: a module
      // being taken OUT of a section has nowhere else to go, and on a page with
      // no free cell — which is most pages — refusing left it trapped inside
      // for good. Leaving always works; where it lands is below everything if
      // there was no room under the hand.
      if (!leaving) return;
      spillOnto([moving]);
      await save();
      await fetchLanding();
      drawLanding(orgs);
      return;
    }
    moving.x = x;
    moving.y = y;
    // Into a section, out of one, or from one to another. The coordinates are
    // already in the grid it is landing in, so the move is the same three
    // numbers either way.
    const changedGrid = (moving.parent ?? "") !== parent;
    if (parent === "") delete moving.parent;
    else moving.parent = parent;
    await save();
    if (changedGrid) await fetchLanding();
    drawLanding(orgs);
  });

  // Resizing, from the corner. The pointer arithmetic is in physical pixels and
  // the handle is drawn with borders, so this corner stays the bottom-right one
  // in every reading direction: turning only the triangle would leave the
  // cursor and the shape disagreeing.
  grid.querySelectorAll<HTMLElement>("[data-resize]").forEach((handle) => {
    handle.addEventListener("pointerdown", (event) => {
      const id = handle.dataset.resize ?? "";
      const target = landingModules.find((m) => m.id === id);
      if (!target) return;
      event.preventDefault();
      event.stopPropagation();

      const startX = event.clientX;
      const startY = event.clientY;
      const fromW = target.w;
      const fromH = target.h;
      let w = fromW;
      let h = fromH;
      // Lastboard draws the ghost on the way down, before anything has moved, so
      // taking the corner already says what is being resized.
      showGhost(target.x, target.y, fromW, fromH, true);

      const move = (moved: PointerEvent) => damp(() => {
        const { width } = cell();
        const parent = target.parent ?? "";
        const holder = parent ? landingModules.find((m) => m.id === parent) : null;
        const most = holder ? holder.w : cols;
        // One column everywhere, in a section and on the page alike.
        const least = 1;
        w = Math.max(least, Math.min(most - target.x + 1,
          fromW + Math.round((moved.clientX - startX) / (width + GAP))));
        h = Math.max(1, fromH + Math.round((moved.clientY - startY) / (width + GAP)));
        if (holder) h = Math.min(h, holder.h - target.y + 1);
        showGhost(target.x, target.y, w, h,
          fits(target.id, target.x, target.y, w, h, parent), parent);
      });
      const done = async () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", done);
        hideGhost();
        const parent = target.parent ?? "";
        if (!fits(target.id, target.x, target.y, w, h, parent)) return;
        target.w = w;
        target.h = h;
        // A section that shrank may no longer hold everything it did. What no
        // longer fits comes out onto the page rather than being drawn past the
        // edge of the box that is supposed to contain it.
        const ejected = target.kind === "section" ? evict(target) : 0;
        await save();
        if (ejected > 0) await fetchLanding();
        drawLanding(orgs);
      };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", done);
    });
  });
}

function openModuleConfig(id: string, orgs: OrganizationSummary[]): void {
  const target = landingModules.find((m) => m.id === id);
  const form = root?.querySelector<HTMLFormElement>("form.module-config");
  if (!target || !form) return;

  form.querySelector<HTMLInputElement>("[name=id]")!.value = target.id;

  // A block of words has no field here at all: it is written on the page, in
  // the place it will be read, with the bar that follows the cursor. A textarea
  // in a dialog was asking somebody to compose their front page through a
  // letterbox.

  // And only a module that draws something out of the database is asked which
  // board to draw it from, for the same reason.
  const where = form.querySelector<HTMLElement>("[data-for-scope]");
  const scope = form.querySelector<HTMLSelectElement>("[name=scope]");
  if (where && scope) {
    where.hidden = !takesScope(target.kind);
    // Which levels this module can use. A button goes to one report form, so
    // "everything" and "an organisation" are not answers it can act on; the
    // organisations module has no use for a board. Offering a level a module
    // cannot use is offering a choice that quietly does nothing.
    const allowed =
      target.kind === "call_to_action"
        ? ["board"]
        : target.kind === "organisations"
          ? ["all", "org"]
          : ["all", "org", "board"];
    scope.querySelectorAll<HTMLOptionElement>("option").forEach((option) => {
      option.hidden = !allowed.includes(option.dataset.level ?? "");
    });
    scope.value = target.scope ?? "";
    // A module whose stored level is no longer on offer falls back to the first
    // one that is, rather than showing a blank control.
    if (scope.selectedIndex < 0 || scope.options[scope.selectedIndex]?.hidden) {
      const first = [...scope.options].find((option) => !option.hidden);
      if (first) scope.value = first.value;
    }
  }

  // And only a module that draws a name is asked where to keep it — as the four
  // little diagrams, not four words.
  // Where the name goes, on the name: the dropdown lives inside the same box as
  // the name it places, which is where theirs is.
  const at = form.querySelector<HTMLInputElement>("[name=labelAt]");
  const placing = form.querySelector<HTMLElement>(".icon-dropdown");
  if (placing) placing.hidden = !usesHeader(target.kind) && target.kind !== "section";
  if (at) {
    const section = target.kind === "section";
    at.value = section ? "pill" : (target.label ?? "top");
    pickOne(form, "[data-at]", "at", at.value);
    showMark(form, at.value);
    // A section wears its name on its border or wears none, which is what it
    // does over there. The other two are shown and cannot be taken: a control
    // that quietly ignores half of itself is worse than one that says which
    // half it will not do.
    form.querySelectorAll<HTMLButtonElement>("[data-at]").forEach((mark) => {
      const allowed = !section || mark.dataset.at === "pill" || mark.dataset.at === "off";
      mark.disabled = !allowed;
      mark.classList.toggle("cannot", !allowed);
    });
  }

  // How its own box looks, which every module has.
  const colour = form.querySelector<HTMLInputElement>("[name=borderColour]");
  if (colour) {
    colour.value = target.border_color ?? "";
    pickOne(form, "[data-colour]", "colour", colour.value);
  }
  const width = form.querySelector<HTMLInputElement>("[name=borderWidth]");
  if (width) {
    width.value = target.bare ? "0" : target.border_width ? String(target.border_width) : "";
    pickOne(form, "[data-width]", "width", width.value);
  }
  const clear = form.querySelector<HTMLInputElement>("[name=clear]");
  if (clear) clear.checked = target.clear === true;

  // How many things a module that lists them shows.
  const counting = form.querySelector<HTMLElement>("[data-for-rows]");
  const rows = form.querySelector<HTMLInputElement>("[name=rows]");
  if (counting && rows) {
    counting.hidden = !lists(target.kind);
    rows.value = String(target.rows ?? 0);
    stepRows(counting, rows);
  }


  // The button's words, a section's name, and a point's sentence. All three are
  // one short line the operator writes, so they are one field.
  const naming = form.querySelector<HTMLElement>("[data-for-label]");
  const label = form.querySelector<HTMLInputElement>("[name=label]");
  if (naming && label) {
    const named = target.kind === "call_to_action";
    naming.hidden = !named;
    label.value = named ? (target.body ?? "") : "";
    const labelText = form.querySelector<HTMLElement>("[data-for-label] label");
    if (labelText) labelText.textContent = i18n.t("landing.label");
  }

  // What the operator calls it, for the modules that draw a name at all.
  const naming2 = form.querySelector<HTMLElement>("[data-for-name]");
  const name = form.querySelector<HTMLInputElement>("[name=name]");
  if (naming2 && name) {
    naming2.hidden = !usesHeader(target.kind);
    name.value = target.name ?? "";
    name.placeholder = defaultNameFor(target.kind);
  }

  // A section is named in the same field as everything else and placed with the
  // same marks. Over there a section's name IS the pill on its border, so the
  // only position it can take is that one — and saying so with the same control
  // greyed down to one choice is clearer than a different control.
  if (target.kind === "section" && naming2 && name) {
    naming2.hidden = false;
    name.value = target.body ?? "";
    name.placeholder = "";
  }

  // A tab with nothing in it says so rather than opening empty.
  const look = form.querySelector<HTMLElement>("[data-look-empty]");
  if (look) look.hidden = usesHeader(target.kind);

  // Always opened on the first tab: which one was left showing is a fact about
  // the last module somebody configured, not about this one.
  showTab(form, "general");
  void orgs;
  openModal("module-config");
}

/** The name of what a module draws, which is what it is called until it is not. */
function defaultNameFor(kind: views.LandingModule["kind"]): string {
  switch (kind) {
    case "organisations":
      return i18n.t("shell.organizations");
    case "boards":
      return i18n.t("portal.boards");
    case "most_voted":
      return i18n.t("landing.moduleVoted");
    case "recently_done":
      return i18n.t("landing.moduleDone");
    case "recent_activity":
      return i18n.t("landing.moduleActivity");
    case "roadmap":
      return i18n.t("landing.moduleRoadmap");
    case "statistics":
      return i18n.t("landing.moduleStats");
    default:
      return "";
  }
}

/**
 * Mark one of a row of buttons as the chosen one.
 *
 * Three rows work this way — the colour, the width, the place a name goes — and
 * three copies of the same four lines is three places for them to drift apart.
 */
function pickOne(form: HTMLFormElement, selector: string, key: string, value: string): void {
  form.querySelectorAll<HTMLButtonElement>(selector).forEach((button) => {
    button.classList.toggle("on", (button.dataset[key] ?? "") === value);
  });
}

/** Put the chosen diagram on the button that opens the menu of them. */
function showMark(form: HTMLFormElement, at: string): void {
  const chosen = form.querySelector<HTMLElement>(`[data-at="${at}"]`);
  const shown = form.querySelector<HTMLElement>("[data-current-mark]");
  if (chosen && shown) shown.innerHTML = chosen.innerHTML;
}

/** Show one tab of the settings dialog and mark it as the one showing. */
/**
 * Wires the minus and plus either side of a count, and keeps them honest: at
 * nought there is nothing to take away and at the ceiling nothing to add, and
 * a button that still looks pressable at the end of its range is a button that
 * lies. Typing into the field still works — the steps are a second way in, not
 * the only one.
 */
/**
 * Asks for a picture.
 *
 * The input is made, used and dropped rather than sitting in the markup: a file
 * input left on the page is a control a visitor can see the shadow of, and one
 * more thing for a redraw to lose the listener of.
 */
function pickImageFile(): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "image/*";
    input.addEventListener("change", () => resolve(input.files?.[0] ?? null), { once: true });
    // A picker closed with Escape fires nothing at all in some browsers, so
    // this also listens for the window coming back and settles at nothing.
    window.addEventListener(
      "focus",
      () => setTimeout(() => resolve(input.files?.[0] ?? null), 400),
      { once: true },
    );
    input.click();
  });
}

function stepRows(group: HTMLElement, field: HTMLInputElement): void {
  const low = Number(field.min || 0);
  const high = Number(field.max || 20);
  const steps = [...group.querySelectorAll<HTMLButtonElement>("[data-step]")];

  const settle = (): void => {
    const now = Number(field.value || 0);
    for (const step of steps) {
      const by = Number(step.dataset.step ?? 0);
      step.disabled = by < 0 ? now <= low : now >= high;
    }
  };

  for (const step of steps) {
    if (step.dataset.wired === "1") continue;
    step.dataset.wired = "1";
    step.addEventListener("click", () => {
      const by = Number(step.dataset.step ?? 0);
      field.value = String(Math.min(high, Math.max(low, Number(field.value || 0) + by)));
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }
  if (field.dataset.wired !== "1") {
    field.dataset.wired = "1";
    field.addEventListener("input", settle);
  }
  settle();
}

function showTab(form: HTMLFormElement, which: string): void {
  form.querySelectorAll<HTMLElement>("[data-panel]").forEach((panel) => {
    panel.hidden = panel.dataset.panel !== which;
  });
  form.querySelectorAll<HTMLButtonElement>("[data-tab]").forEach((tab) => {
    const on = tab.dataset.tab === which;
    tab.classList.toggle("on", on);
    tab.setAttribute("aria-selected", String(on));
  });
}

/**
 * What a shrunken section can no longer hold comes out onto the page.
 *
 * Which children, decided by WHERE they are and not by how many there are: the
 * dashboard this comes from compares the number of children against the number
 * of cells, so a section with two children shrunk under the second one ejects
 * nothing and draws that child outside the box meant to contain it. A child
 * that no longer fits is one whose far edge is past the section's new one.
 *
 * Each one goes under everything on the page, one below the next, which is the
 * one place a module is certainly not on top of something else.
 */
function evict(section: views.LandingModule): number {
  const out = landingModules.filter(
    (m) => m.parent === section.id && (m.x + m.w - 1 > section.w || m.y + m.h - 1 > section.h),
  );
  spillOnto(out);
  return out.length;
}

/**
 * Put these modules on the page, one under the other, below everything.
 *
 * Under everything is the one place a module is certainly not on top of another
 * one. The width is widened to the page's minimum: a module one column wide is
 * fine inside a section and is not a module out here.
 */
function spillOnto(modules: views.LandingModule[]): void {
  let row = landingModules.reduce((low, m) => (m.parent ? low : Math.max(low, m.y + m.h)), 1);
  for (const child of modules) {
    delete child.parent;
    child.x = 1;
    child.y = row;
    child.w = Math.max(2, Math.min(views.LANDING_COLUMNS, child.w));
    row += child.h;
  }
}

/** The kinds that draw a name of their own, and so can be told where to put it. */
function usesHeader(kind: views.LandingModule["kind"]): boolean {
  return (
    kind === "organisations" ||
    kind === "boards" ||
    kind === "most_voted" ||
    kind === "recently_done" ||
    kind === "recent_activity" ||
    kind === "roadmap" ||
    kind === "statistics"
  );
}

/**
 * How big a module arrives.
 *
 * A first size, not a rule: every one of these can be dragged to anything from
 * a single cell upwards. What it is for is that a module lands looking like the
 * thing it is — a button is a button, a list is a column, a banner is a banner —
 * instead of as one more band across the page.
 */
function sizeFor(kind: views.LandingModule["kind"]): { w: number; h: number } {
  // Three by two, all of them. It was a size per kind, worked out from what
  // each one draws, which sounds right and is not: what a module should be is
  // the operator's decision and the corner is how they make it. One arrival
  // size means a page of new modules is a row of equal tiles to arrange rather
  // than a stack of things that already decided for themselves.
  //
  // A section is the exception, and only just: at three by two it holds one
  // module, which is a container nothing fits into.
  void kind;
  return kind === "section" ? { w: 6, h: 3 } : { w: 3, h: 2 };
}

/** The kinds that draw a list, and so can be told how long it is. */
function lists(kind: views.LandingModule["kind"]): boolean {
  return (
    kind === "organisations" ||
    kind === "boards" ||
    kind === "most_voted" ||
    kind === "recently_done" ||
    kind === "recent_activity" ||
    kind === "roadmap"
  );
}

/**
 * The kinds that can be pointed at one place.
 *
 * Wider than usesData: where a module's content comes from and whether it can
 * be narrowed are two different questions. The organisations module draws the
 * list the page already carries, and is still something you can aim at one.
 */
function takesScope(kind: views.LandingModule["kind"]): boolean {
  return kind === "organisations" || usesData(kind);
}

/** The kinds that draw out of the database, and so have a board to be told. */
function usesData(kind: views.LandingModule["kind"]): boolean {
  return (
    kind === "boards" ||
    kind === "most_voted" ||
    kind === "recently_done" ||
    kind === "call_to_action" ||
    kind === "recent_activity" ||
    kind === "roadmap" ||
    kind === "statistics"
  );
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
        isAdmin: me?.admin === true,
        mayAddOrganization: me?.admin === true,
        instance: instanceName || i18n.t("app.name"),
        logo: instanceLogo,
        avatar: me?.avatar,
        waiting: me?.pending,
        content,
      },
      text,
    );
  }
  // Arriving at the panel, it comes in from the side the reading direction ends
  // on — the dashboard's own drawer does exactly this. Only on arrival: moving
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
async function readAsDataURL(file: File | undefined): Promise<string | null> {
  if (!file) return null;
  try {
    // Scaled and re-encoded rather than measured and refused. Every photograph
    // a phone takes is over the limit, and telling the person setting up a
    // server to go and find a smaller picture is not a feature.
    return await shrink(file);
  } catch (err) {
    // A file this browser cannot decode — an HEIC from an iPhone, say — is not
    // a file that is too large, and saying so sends somebody off to shrink an
    // image that was never the problem.
    showFormError(new ApiError(400, err instanceof TooLarge ? "error.image_too_big" : "error.image_unreadable"));
    return null;
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
      { username: me?.username ?? "", name: me?.name, avatar: me?.avatar, admin: me?.admin === true },
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
      await api.post("/api/me/profile", { display_name: String(data.get("display_name") ?? "") });
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
      availableLocales.map((l) => ({ code: l.code, name: l.name })),
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
  if (!(await requirePanel())) return;
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
            views.field({ id: "person-name", name: "display_name", label: text.t("profile.name"), example: "Ana" }, text) +
            views.field({ id: "person-password", name: "password", label: text.t("auth.password"), type: "password", required: true, hint: text.t("auth.passwordHelp") }, text) +
            // Chosen here rather than left to whatever their browser asks for:
            // otherwise somebody's first sign-in lands in a language they did
            // not pick, and the setting to change it is in that language.
            views.choiceField({
              id: "person-language",
              name: "language",
              label: text.t("people.language"),
              hint: text.t("people.languageHelp"),
              options: availableLocales.map((l) => [l.code, l.name] as [string, string]),
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
            views.field({ id: "edit-name", name: "display_name", label: text.t("profile.name") }, text) +
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
      display_name: String(data.get("display_name") ?? ""),
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
      const name = root?.querySelector<HTMLInputElement>("#edit-name");
      if (name) name.value = button.dataset.name ?? "";
      const pass = root?.querySelector<HTMLInputElement>("#edit-password");
      if (pass) pass.value = "";
      openModal("person-edit");
    }),
  );
  onSubmit("form.person-edit-form", async (data) => {
    const payload: Record<string, string> = {};
    const name = String(data.get("display_name") ?? "");
    const pass = String(data.get("password") ?? "");
    if (name) payload.display_name = name;
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
      if (!confirm(i18n.t("people.confirmDelete", { name: button.dataset.name ?? "" }))) return;
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
  if (!(await requirePanel())) return;
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
    if (!confirm(i18n.t("backups.confirmRestore", { file: chosen.name }))) {
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
  if (!(await requirePanel())) return;
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
    if (beta && status) {
      beta.checked = status.beta === true;
      beta.disabled = false;
    }
    if (check) {
      check.disabled = false;
      // Asking again changes nothing when the reason is a fact of this
      // installation — no keys, no network, a container — so the button is
      // only offered where a second look can say something new.
      const fixed = ["error.update_offline", "error.update_unverifiable", "error.update_container"];
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

  // Delegated, because the zone is rewritten every time it is asked.
  zone?.addEventListener("click", async (event) => {
    const button = (event.target as HTMLElement).closest<HTMLButtonElement>("[data-update]");
    if (!button) return;
    const note = zone.querySelector<HTMLParagraphElement>(".update-note");
    button.disabled = true;
    if (note) {
      note.textContent = i18n.t("update.installing");
      note.hidden = false;
    }
    try {
      await api.post("/api/update");
      // The server restarts a moment after answering, so waiting and
      // reloading is what brings the page back on the new version.
      setTimeout(() => location.reload(), 4000);
    } catch (err) {
      button.disabled = false;
      if (note) note.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
    }
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
  // Where the review screen was opened from, which is where leaving it goes:
  // the bar's icon, the close button and Escape all take you back rather than
  // to a fixed page, the way closing Lastboard's status page leaves you on the
  // board you were on.
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
    if (path === "/panel/reports") return go("/review" + location.search);
    // Where updates used to live. They are part of About now, as in
    // Lastboard; the address is kept as a door because it is in people's
    // bookmarks and in the documentation of older releases.
    if (path === "/panel/updates") return go("/panel/about");
    if (path === "/panel/account") return await showProfile();
    if (path === "/panel/appearance") return await showAppearance();
    if (path === "/panel/people") return await showPeople();
    if (path === "/panel/organizations") return await showOrganizationsPanel();
    if (path === "/panel/personalization") return await showPersonalization();
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
    if (parts[0] === "i" && parts[1]) return await showItem(parts[1]);
    if (parts.length === 1 && parts[0]) return await showRepositories(parts[0]);
    if (parts.length === 2 && parts[0] && parts[1]) return await showBoard(parts[0], parts[1]);
    if (parts.length === 3 && parts[2] === "new" && parts[0] && parts[1]) {
      return await showReportForm(parts[0], parts[1]);
    }
    if (parts.length === 3 && parts[2] === "queue" && parts[0] && parts[1]) {
      return await showQueue(parts[0], parts[1]);
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

  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");
  // Who is looking decides what an empty instance says. An administrator gets
  // the way out of it; a visitor gets told plainly, rather than being sent to a
  // form that would refuse them.
  if (!me) me = await api.get<Me>("/api/me").catch(() => null);

  await fetchLanding();
  drawLanding(organizations ?? []);
}

async function showRepositories(orgSlug: string): Promise<void> {
  const body = await api.get<{
    organization: OrganizationSummary;
    repositories: RepositorySummary[];
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  show(views.repositoryList(body.organization, body.repositories ?? [], text));
}

const filters = { kind: "", sort: "new", query: "" };

async function showBoard(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);

  const load = async (): Promise<ItemSummary[]> => {
    const query = new URLSearchParams();
    if (filters.kind) query.set("kind", filters.kind);
    if (filters.sort === "votes") query.set("sort", "votes");
    if (filters.query) query.set("q", filters.query);
    const body = await api.get<{ items: ItemSummary[] }>(`${base}/items?${query}`);
    return body.items ?? [];
  };

  const draw = async () => {
    show(views.board(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, await load(), filters, text));
    wire();
  };

  const wire = () => {
    root?.querySelectorAll<HTMLButtonElement>(".tab").forEach((b) =>
      b.addEventListener("click", () => {
        filters.kind = b.dataset.kind ?? "";
        void draw();
      }),
    );
    root?.querySelectorAll<HTMLButtonElement>(".sort").forEach((b) =>
      b.addEventListener("click", () => {
        filters.sort = b.dataset.sort ?? "new";
        void draw();
      }),
    );
    const search = root?.querySelector<HTMLInputElement>(".search");
    search?.addEventListener("input", debounce(() => {
      filters.query = search.value;
      void draw().then(() => root?.querySelector<HTMLInputElement>(".search")?.focus());
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

  show(views.reportForm(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, kind, text));

  const form = root?.querySelector<HTMLFormElement>("form.report");
  const similar = root?.querySelector<HTMLDivElement>(".similar");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

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

  // Somebody with a role keeps their panel around the report.
  //
  // The address is the public one — this is the page that travels, and a
  // stranger opening it must not be shown an application — but for the team it
  // is where the work is done, and arriving at it from the listing used to drop
  // them out of the panel entirely: no navigation, no way back to what they
  // were working through except the browser's own button.
  const staff = me !== null && (me.admin === true || (me.memberships ?? []).length > 0);
  if (staff) {
    // Under Reports in the sidebar, because that is the screen this was
    // reached from and the one to go back to.
    await showInShell(views.itemView(item, text), "/panel/reports");
  } else {
    show(views.itemView(item, text));
  }

  root?.querySelector<HTMLButtonElement>("[data-vote]")?.addEventListener("click", async (event) => {
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

  // The team's control, and only if the server says this person may use it.
  // Asking is the check; drawing the select is not. A page that decides for
  // itself who may act is a second implementation of the rules.
  if (me) {
    const allowed = await api
      .get<{ current: string; statuses: string[] }>(`/api/items/${encodeURIComponent(publicID)}/statuses`)
      .catch(() => null);
    if (allowed) {
      const tools = root?.querySelector<HTMLElement>(".team-tools");
      if (tools) tools.hidden = false;

      // What is still waiting is decided here rather than back in a list: the
      // reason somebody opens a waiting report is to read it before deciding,
      // and the decision was one screen away from the reading.
      if (item.moderation && item.moderation !== "published") {
        // Above the tools and outside them: it is the one decision on this
        // screen that the screen cannot undo.
        tools?.insertAdjacentHTML("beforebegin", views.moderationCard(text));
        root?.querySelectorAll<HTMLButtonElement>("[data-moderate]").forEach((button) =>
          button.addEventListener("click", async () => {
            try {
              await api.post(`/api/items/${encodeURIComponent(publicID)}/moderation`, {
                state: button.dataset.moderate,
              });
              await render();
            } catch (err) {
              report(err);
            }
          }),
        );
      }

      tools
        ?.querySelector(".tools")
        ?.insertAdjacentHTML("beforeend", views.statusControl(allowed.current, allowed.statuses, text));

      root?.querySelector<HTMLSelectElement>("#status-select")?.addEventListener("change", async (e) => {
        const select = e.target as HTMLSelectElement;
        await api
          .post(`/api/items/${encodeURIComponent(publicID)}/status`, { status: select.value })
          .then(() => toast(i18n.t("toast.statusChanged")))
          .catch((err) => report(err));
      });

      wireIssueControl(publicID, item.issue ?? null);
      wireTriage(publicID, { title: item.title ?? "", body: item.body ?? "" });
    }
  }

  const commentForm = root?.querySelector<HTMLFormElement>("form.comment-form");
  commentForm?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(commentForm);
    await api
      .post(`/api/items/${encodeURIComponent(publicID)}/comments`, {
        body: String(data.get("body") ?? ""),
        token: token ?? "",
      })
      .catch(() => undefined);
    commentForm.reset();
    const sent = commentForm.querySelector<HTMLParagraphElement>(".comment-sent");
    if (sent) sent.hidden = false;
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
function wireChannel(orgSlug: string): void {
  const form = root?.querySelector<HTMLFormElement>("form.channel");
  if (!form) return;

  const kind = form.querySelector<HTMLSelectElement>("#chan-kind");
  const fields = form.querySelector<HTMLDivElement>(".channel-fields");
  const mail = form.querySelector<HTMLDivElement>(".channel-mail");
  const errorLine = form.querySelector<HTMLParagraphElement>(".form-error");

  // Only the fields the chosen channel needs. Asking for a URL for an email
  // channel is a form arguing with the person filling it in.
  const sync = () => {
    const value = kind?.value ?? "";
    if (fields) fields.hidden = value === "" || value === "smtp";
    if (mail) mail.hidden = value !== "smtp";
  };
  kind?.addEventListener("change", sync);
  sync();

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const repo = String(data.get("repo") ?? "");
    try {
      await api.post(
        `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repo)}/channel`,
        {
          kind: String(data.get("kind") ?? ""),
          url: String(data.get("url") ?? ""),
          token: String(data.get("token") ?? ""),
          to: String(data.get("to") ?? ""),
        },
      );
      toast(i18n.t("panel.saved"));
    } catch (err) {
      // The failure stays beside the form as well as toasting: it is about
      // what was typed, and the fix is in the fields somebody is looking at.
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
      report(err);
    }
  });
}

/**
 * The bridge control, and the only place the panel talks to a forge.
 *
 * Creating an issue answers "queued", not "created": the request does not call
 * the forge — forges rate-limit and forges go down — so saying it was created
 * would be reporting something that has not happened yet.
 */
function wireIssueControl(publicID: string, linked: { number: number; url: string } | null): void {
  root
    ?.querySelector(".team-tools .tools")
    ?.insertAdjacentHTML("beforeend", views.issueControl(linked, text));

  root?.querySelector<HTMLButtonElement>("[data-create-issue]")?.addEventListener("click", async () => {
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/issue`);
      toast(i18n.t("panel.issueQueued"));
    } catch (err) {
      report(err);
    }
  });

  const attach = root?.querySelector<HTMLFormElement>("form.attach");
  attach?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const value = String(new FormData(attach).get("issue") ?? "");
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/attach`, { issue: value });
      toast(i18n.t("toast.issueAttached"));
      await render();
    } catch (err) {
      report(err);
    }
  });
}

/** Replying as the team, and merging a duplicate away. */
function wireTriage(publicID: string, current: { title: string; body: string }): void {
  root?.querySelector(".team-tools .tools")?.insertAdjacentHTML(
    "beforeend",
    views.triageControls(text, current),
  );

  // Rewriting the report. The page is re-rendered afterwards rather than the
  // heading patched in place: the title appears in more than one spot on this
  // screen, and updating the one that was easy to reach is how the two start
  // disagreeing.
  const rewrite = root?.querySelector<HTMLFormElement>("form.rewrite");
  rewrite?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(rewrite);
    try {
      await api.patch(`/api/items/${encodeURIComponent(publicID)}`, {
        title: String(data.get("title") ?? ""),
        body: String(data.get("body") ?? ""),
      });
      toast(i18n.t("panel.saved"));
      await render();
    } catch (err) {
      report(err);
    }
  });

  const reply = root?.querySelector<HTMLFormElement>("form.team-reply");
  reply?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(reply);
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/reply`, {
        body: String(data.get("body") ?? ""),
      });
      reply.reset();
      toast(i18n.t("toast.replied"));
      await render();
    } catch (err) {
      report(err);
    }
  });

  const merge = root?.querySelector<HTMLFormElement>("form.merge");
  merge?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(merge);
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/merge`, {
        into: String(data.get("into") ?? ""),
      });
      // The merged item now redirects, so the page it lands on is the survivor.
      toast(i18n.t("toast.merged"));
      await render();
    } catch (err) {
      report(err);
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
const draft: { forge: string; orgSlug: string; repoSlug: string; picture: string; logo: string } = {
  forge: "none",
  orgSlug: "",
  repoSlug: "",
  // Chosen before the account exists, so held here and sent the moment it does.
  picture: "",
  logo: "",
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
      languages: availableLocales.map((l) => ({ code: l.code, name: l.name })),
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
  show(views.setupDone(text));
}

async function showOrganizationWizard(step: 1 | 2): Promise<void> {
  await showInShell(views.organizationWizard(step, text, draft.forge), "/panel/new");

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
        });
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
    // Not an administrator. The numbers count the whole installation, which is
    // not theirs to see; the places they were given a role in are.
    await showInShell(views.memberHome(me?.memberships ?? [], text), "/panel");
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
 * to somebody and a reload lands back on it. Lastboard's equivalent is an
 * overlay because that product has no router; copying the overlay would be
 * importing a limitation rather than the design.
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
  const mode = query.get("mode") === "all" ? "all" : "pending";
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
  const fields = new Map<string, ReportField[]>();

  // On a phone there is no room for the detail beside the list, so a chosen
  // row opens it as a drawer from the trailing edge — Lastboard's answer on
  // the same page at the same width. Only a tap opens it: arriving on the
  // screen, or a decision moving to the next row, keeps the list in view.
  const narrow = () => window.matchMedia("(max-width: 640px)").matches;
  const reading = (on: boolean) => screen?.classList.toggle("reading", on);

  const draw = async (id: string) => {
    if (!pane) return;
    const note = commentOf(id);
    if (note) {
      pane.innerHTML = views.reviewDetail(null, [], text, note);
      return;
    }
    const row = rowOf(id);
    if (row && !fields.has(id)) {
      const full = await api
        .get<{ fields?: ReportField[] }>(`/api/items/${encodeURIComponent(id)}`)
        .catch(() => ({ fields: [] as ReportField[] }));
      fields.set(id, full.fields ?? []);
    }
    pane.innerHTML = views.reviewDetail(row, fields.get(id) ?? [], text);
  };

  const select = (id: string) => {
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
    void draw(id);
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
    if (next) select(next.dataset.pick ?? "");
  };

  const decide = async (state: string) => {
    const id = view.chosen;
    if (!id) return;
    const all = order();
    const at = all.findIndex((el) => el.dataset.pick === id);
    const after = all[at + 1] ?? all[at - 1];
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
    if (view.mode === "all") {
      // In the archive the row stays, wearing its new state: a row that
      // vanished would read as a report that was deleted.
      if (row) row.moderation = state;
      all[at]?.classList.toggle("out", state === "rejected" || state === "spam");
      void draw(id);
      if (me) me = await api.get<Me>("/api/me").catch(() => me);
      renderNav();
      return;
    }
    // Gone from the list and from the data behind it, so stepping and counting
    // agree with what is on the screen.
    all[at]?.remove();
    data.items = data.items.filter((r) => r.public_id !== id);
    if (id.startsWith("c:")) {
      data.comments = (data.comments ?? []).filter((c) => String(c.id) !== id.slice(2));
    }
    if (after) select(after.dataset.pick ?? "");
    else {
      if (pane) pane.innerHTML = views.reviewDetail(null, [], text);
      reading(false);
    }
    if (me) me = await api.get<Me>("/api/me").catch(() => me);
    renderNav();
  };

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
  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");

  await showInShell(
    views.organizationsScreen(
      organizations ?? [],
      { canAdd: isAdministrator(), canManage: ownsOrganization, canDelete: isAdministrator() },
      text,
    ) +
      views.modal(
        {
          id: "org-modal",
          title: text.t("panel.newOrganization"),
          lead: text.t("setup.orgLead"),
          body: views.organizationFields(text, draft.forge),
          submit: text.t("auth.create"),
          formClass: "org-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "org-rename",
          title: text.t("organizations.rename"),
          body: views.field({ id: "org-new-name", name: "name", label: text.t("panel.name"), required: true }, text),
          submit: text.t("action.save"),
          formClass: "org-rename-form",
        },
        text,
      ),
    "/panel/organizations",
  );


  onSubmit("form.org-form", async (data) => {
    await api.post("/api/organizations", {
      name: String(data.get("name") ?? ""),
      slug: String(data.get("slug") ?? ""),
      forge: String(data.get("forge") ?? "none"),
      base_url: String(data.get("base_url") ?? ""),
      forge_owner: String(data.get("forge_owner") ?? ""),
    });
    await render();
  });

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
      if (!confirm(i18n.t("organizations.confirmDelete", { name: button.dataset.name ?? slug }))) return;
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

/** The name, the logo and the accent of this installation. */
async function showPersonalization(): Promise<void> {
  if (!(await requirePanel())) return;
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
  root?.querySelector<HTMLButtonElement>("[data-reset-accent]")?.addEventListener("click", async () => {
    await api.post("/api/instance", { ...instance, accent: "" }).catch(showFormError);
    await render();
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

  const body = await api.get<{
    organization: OrganizationSummary & { forge?: string; webhook_path?: string };
    repositories: RepositorySummary[];
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  const repos = body.repositories ?? [];
  const canManage = ownsOrganization(orgSlug);
  const forge = body.organization.forge ?? "none";

  await showInShell(
    views.organizationScreen(body.organization, repos, canManage, isAdministrator(), text) +
      // The token and the channel belong to whoever runs the organisation. A
      // collaborator triages without ever being able to answer "what is the
      // token", which is the whole reason the two roles exist.
      (canManage ? views.connectionForms(body.organization, repos, location.origin, text) : "") +
      views.modal(
        {
          id: "repo-modal",
          title: text.t("panel.newRepository"),
          lead: text.t("repo.lead"),
          body: views.repositoryFields(text, forge, forge !== "none") + views.acceptsFields(text),
          submit: text.t("auth.create"),
          formClass: "repo-form",
        },
        text,
      ),
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
  wireChannel(orgSlug);

  root?.querySelectorAll<HTMLButtonElement>("[data-open]").forEach((button) =>
    button.addEventListener("click", () => go(button.dataset.open ?? "/")),
  );

  onSubmit("form.repo-form", async (data) => {
    await api.post(`/api/orgs/${encodeURIComponent(orgSlug)}/repositories`, {
      name: String(data.get("name") ?? ""),
      slug: String(data.get("slug") ?? ""),
      forge_repo: String(data.get("forge_repo") ?? ""),
      accepts_bugs: data.get("accepts_bugs") !== null,
      accepts_ideas: data.get("accepts_ideas") !== null,
    });
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-repo]").forEach((button) =>
    button.addEventListener("click", async () => {
      const slug = button.dataset.deleteRepo ?? "";
      if (!confirm(i18n.t("organizations.confirmDeleteRepo", { name: button.dataset.name ?? slug }))) return;
      try {
        await api.del(`/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(slug)}`);
        await render();
      } catch (err) {
        showFormError(err);
      }
    }),
  );
}

/**
 * The queue is the panel's front page for one board: what is waiting, with two
 * buttons.
 *
 * Acting on an entry removes that entry and leaves the rest alone. Redrawing
 * the whole list after every decision would throw away the scroll position of
 * somebody working down a long queue.
 */
/**
 * Approve or hide a message, wherever the row is being shown.
 *
 * Bound by both the queue of one board and the queue that spans them. The row
 * is removed rather than the list redrawn: somebody working down a queue keeps
 * their place, and a redraw after every decision throws it away.
 */
function bindCommentDecisions(): void {
  root?.querySelectorAll<HTMLButtonElement>("[data-comment-act]").forEach((button) =>
    button.addEventListener("click", async () => {
      const entry = button.closest<HTMLLIElement>("li[data-comment]");
      const id = entry?.dataset.comment;
      if (!id) return;
      try {
        await api.post(`/api/comments/${encodeURIComponent(id)}/moderation`, {
          state: button.dataset.commentAct,
        });
        entry?.remove();
      } catch (err) {
        report(err);
      }
    }),
  );
}

async function showQueue(orgSlug: string, repoSlug: string): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const body = await api.get<{ items: Array<ItemSummary & { body: string; fields?: ReportField[] }> }>(
    `${base}/queue`,
  );
  const items = body.items ?? [];
  const bodies = new Map(items.map((i) => [i.public_id, i.body]));
  const fields = new Map(items.map((i) => [i.public_id, i.fields ?? []]));

  // The comments waiting on this board. Failing softly rather than taking the
  // screen down with it: the reports are what somebody came here for.
  const waiting = await api
    .get<{ comments: views.WaitingComment[] }>(`${base}/comments`)
    .catch(() => ({ comments: [] as views.WaitingComment[] }));

  const repo = await api.get<RepositorySummary & { organization?: OrganizationSummary }>(base);
  await showInShell(
    views.queueScreen(
      repo.organization ?? { slug: orgSlug, name: orgSlug },
      repo,
      items,
      bodies,
      fields,
      waiting.comments ?? [],
      text,
    ),
    `/${orgSlug}/${repoSlug}/queue`,
  );

  bindCommentDecisions();

  root?.querySelectorAll<HTMLButtonElement>("[data-act]").forEach((button) =>
    button.addEventListener("click", async () => {
      const entry = button.closest<HTMLLIElement>("li[data-id]");
      const id = entry?.dataset.id;
      if (!id) return;
      try {
        await api.post(`/api/items/${encodeURIComponent(id)}/moderation`, {
          state: button.dataset.act,
        });
        entry?.remove();
        if (!root?.querySelector("li[data-id]")) {
          const list = root?.querySelector(".queue");
          list?.replaceWith(
            Object.assign(document.createElement("p"), {
              className: "empty",
              textContent: i18n.t("panel.empty"),
            }),
          );
        }
      } catch (err) {
        report(err);
      }
    }),
  );
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
