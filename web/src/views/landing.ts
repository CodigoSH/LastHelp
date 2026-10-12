/**
 * The public front page: a stack of bands, each divided into columns, each
 * column a list of modules as tall as what they say.
 *
 * Pure functions of the page and what the server sent with it, so they can be
 * tested without a browser. The editor wraps the same markup; it does not draw
 * a second version of the page, because two drawings of one page are two pages
 * that drift apart.
 */
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";
import { brandMark, emptyState, markOf, type Text } from "./templates";

export type LandingKind =
  | "heading"
  | "text"
  | "search"
  | "boards"
  | "reports"
  | "roadmap"
  | "statistics"
  | "links"
  | "image";

export type LandingColumns = "1" | "1-1" | "1-1-1" | "2-1" | "1-2" | "1-1-1-1";

/** The six layouts, in the order the editor offers them. */
export const LANDING_LAYOUTS: LandingColumns[] = ["1", "1-1", "1-1-1", "2-1", "1-2", "1-1-1-1"];

/** How many columns a layout has. */
export function columnCount(c: LandingColumns): number {
  return c.split("-").length;
}

export interface LandingLink {
  label: string;
  to: "form" | "board" | "roadmap" | "url";
  scope?: string;
  url?: string;
  button?: boolean;
}

export interface LandingModule {
  id: string;
  kind: LandingKind;
  name?: string;
  hide_name?: boolean;
  style?: "" | "soft" | "plain";
  body?: string;
  tone?: "" | "note" | "notice";
  title?: string;
  subtitle?: string;
  hide_logo?: boolean;
  logo_size?: "" | "small" | "large";
  align?: "" | "start";
  with_search?: boolean;
  scope?: string;
  rows?: number;
  show?: "" | "most_voted" | "recently_done" | "latest" | "planned";
  display?: "" | "list";
  links?: LandingLink[];
  image?: string;
  alt?: string;
  caption?: string;
}

export interface LandingBand {
  id: string;
  columns: LandingColumns;
  cells: LandingModule[][];
  name?: string;
  hide_name?: boolean;
  background?: "" | "soft" | "accent";
  width?: "" | "full";
  spacing?: "" | "compact" | "airy";
}

export interface LandingSettings {
  title?: string;
  description?: string;
  share_image?: string;
  footer?: LandingLink[];
  footer_text?: string;
  footer_hide_brand?: boolean;
  footer_hide_credit?: boolean;
}

/** The id the editor chooses the footer by. No band or module can have it: ids have no spaces. */
export const FOOTER_ID = " footer";

export interface LandingPage {
  bands: LandingBand[];
  settings: LandingSettings;
}

export interface LandingBoard {
  org: string;
  org_name: string;
  slug: string;
  name: string;
  accepts_bugs: boolean;
  accepts_ideas: boolean;
  /** Published and still open, per kind: what the board's own open tab shows. */
  total_bugs?: number;
  total_ideas?: number;
  icon?: string;
}

export interface LandingReport {
  public_id: string;
  title: string;
  kind: string;
  status?: string;
  votes: number;
  board: string;
  closed_at?: number;
}

export interface LandingRoadmap {
  planned: LandingReport[];
  in_progress: LandingReport[];
  done: LandingReport[];
}

export interface LandingCounts {
  received: number;
  open: number;
  done: number;
}

/** What each data module was filled with, by module id. */
export type LandingContent = Record<string, LandingBoard[] | LandingReport[] | LandingRoadmap | LandingCounts | null>;

/** Everything a page is drawn with besides the page itself. */
export interface LandingContext {
  instance: { name?: string; logo?: string };
  content: LandingContent;
  /** Whether the editor is drawing it: empty modules then show what fills them. */
  editing?: boolean;
  /** The band or module the editor has chosen, by id. */
  selected?: string;
}

/**
 * The words of the default page, by id.
 *
 * A page stored with twenty languages inside would be unreadable, and one stored
 * with English inside would be every visitor's language decided once. So the
 * default page carries ids and this looks them up in the visitor's catalogue —
 * until the operator writes something of their own, which is then what is
 * stored and shown.
 */
function defaultBandName(id: string, text: Text): string {
  if (id === "where") return text.t("landing.bandWhere");
  if (id === "how") return text.t("landing.bandHow");
  return "";
}

const POINTS: Record<string, { icon: () => string; title: string; body: string }> = {
  "point-1": { icon: () => icons.person(18), title: "landing.point1Title", body: "landing.pointNoAccount" },
  "point-2": { icon: () => icons.idea(18), title: "landing.point2Title", body: "landing.pointVotes" },
  "point-3": { icon: () => icons.open(18), title: "landing.point3Title", body: "landing.pointStatus" },
};

/** The default words a text module falls back to, or "" for one with none. */
export function defaultWords(id: string, text: Text): string {
  const point = POINTS[id];
  if (!point) return "";
  return `<h3>${escapeHTML(text.t(point.title))}</h3><p>${escapeHTML(text.t(point.body))}</p>`;
}

/** What a named module is called, by kind — and by list, for reports. */
export function kindName(m: LandingModule, text: Text): string {
  switch (m.kind) {
    case "boards":
      return text.t("landing.bandWhere");
    case "reports":
      switch (m.show) {
        case "recently_done":
          return text.t("landing.moduleDone");
        case "latest":
          return text.t("landing.moduleActivity");
        case "planned":
          return text.t("landing.showPlanned");
        default:
          return text.t("landing.moduleVoted");
      }
    case "roadmap":
      return text.t("landing.moduleRoadmap");
    case "statistics":
      return text.t("landing.moduleStats");
    case "links":
      return text.t("landing.moduleLinks");
    default:
      return "";
  }
}

function kindIcon(m: LandingModule): string {
  switch (m.kind) {
    case "boards":
      return icons.repository(16);
    case "reports":
      return m.show === "recently_done" ? icons.check(16) : m.show === "latest" ? icons.queue(16) : icons.idea(16);
    case "roadmap":
      return icons.open(16);
    case "statistics":
      return icons.overview(16);
    default:
      return icons.link(16);
  }
}

const NAMED = new Set<LandingKind>(["boards", "reports", "roadmap", "statistics", "links"]);

function kindMark(kind: string): string {
  return `<span class="kind-mark kind-${escapeAttr(kind)}" aria-hidden="true">${
    kind === "bug" ? icons.bug(16) : icons.idea(16)
  }</span>`;
}

// --- the page ---------------------------------------------------------------

/** The whole page: its bands, then its footer. */
export function landingPage(page: LandingPage, ctx: LandingContext, text: Text): string {
  if (!ctx.editing) {
    return `<div class="lp">
      ${page.bands.map((b) => band(b, ctx, text)).join("")}
      ${footer(page.settings, ctx, text)}
    </div>`;
  }
  // While editing, a way to add a section sits before every band and after
  // the last one: a new section goes exactly where it was asked for.
  return `<div class="lp lp-editing">
    ${insertBand(0, text)}
    ${page.bands.map((b, i) => band(b, ctx, text) + insertBand(i + 1, text)).join("")}
    ${footer(page.settings, ctx, text)}
  </div>`;
}

/**
 * One band: a full-width stretch with its own background, and the columns
 * inside it at the page's measure — or at the window's, when it is full width.
 */
export function band(b: LandingBand, ctx: LandingContext, text: Text): string {
  const name = b.hide_name ? "" : (b.name ?? "").trim() || defaultBandName(b.id, text);
  const classes = [
    "lp-band",
    b.background ? `bg-${b.background}` : "",
    b.width === "full" ? "full" : "",
    `space-${b.spacing || "normal"}`,
  ]
    .filter(Boolean)
    .join(" ");
  // Every module in a band is as tall as the tallest in its row of columns:
  // two lists side by side end on one line rather than one short and one long.
  const selected = ctx.editing && ctx.selected === b.id;
  return `<section class="${classes}${selected ? " is-selected" : ""}" data-band="${escapeAttr(b.id)}"${
    ctx.editing ? ` tabindex="-1"` : ""
  }>
    ${ctx.editing ? bandTools(text) : ""}
    <div class="lp-inner">
      ${name ? `<h2 class="lp-pill"><span>${escapeHTML(name)}</span></h2>` : ""}
      <div class="lp-cols cols-${escapeAttr(b.columns)}">
        ${b.cells
          .map(
            (cell, i) => `<div class="lp-col" data-column="${i}">
              ${cell.map((m) => module(m, ctx, text)).join("")}
              ${ctx.editing ? addModule(b.id, i, cell.length === 0, text) : ""}
            </div>`,
          )
          .join("")}
      </div>
    </div>
  </section>`;
}

/**
 * One module, in its frame.
 *
 * A module with nothing to show says so and keeps its place: one that vanished
 * would read as a page that failed to load, and would leave the operator
 * nothing to click to fix it.
 */
export function module(m: LandingModule, ctx: LandingContext, text: Text): string {
  const inner = moduleBody(m, ctx, text);
  const named = NAMED.has(m.kind) && !m.hide_name;
  const head = named
    ? `<header class="lp-head">
        <span class="lp-head-icon" aria-hidden="true">${kindIcon(m)}</span>
        <h3>${escapeHTML((m.name ?? "").trim() || kindName(m, text))}</h3>
      </header>`
    : "";
  const style = m.style || "card";
  if (!ctx.editing) {
    return `<article class="lp-mod mod-${escapeAttr(m.kind)} style-${escapeAttr(style)}" data-module="${escapeAttr(m.id)}">
      ${head}${inner}
    </article>`;
  }
  const selected = ctx.selected === m.id;
  return `<article class="lp-mod mod-${escapeAttr(m.kind)} style-${escapeAttr(style)}${selected ? " is-selected" : ""}"
      data-module="${escapeAttr(m.id)}" tabindex="0" aria-label="${escapeAttr(moduleLabel(m, text))}">
    ${moduleTools(m, text)}
    ${head}${inner}
  </article>`;
}

function moduleBody(m: LandingModule, ctx: LandingContext, text: Text): string {
  const data = ctx.content[m.id];
  switch (m.kind) {
    case "heading":
      return heading(m, ctx, text);
    case "text":
      return words(m, ctx, text);
    case "search":
      return search(text);
    case "boards":
      return boards(m, (data as LandingBoard[] | undefined) ?? [], text);
    case "reports":
      return reports(m, (data as LandingReport[] | undefined) ?? [], text);
    case "roadmap":
      return roadmap((data as LandingRoadmap | undefined) ?? null, text);
    case "statistics":
      return statistics((data as LandingCounts | undefined) ?? null, text);
    case "links":
      return links(m.links ?? [], ctx, text);
    case "image":
      return picture(m, ctx, text);
  }
}

// --- editing ----------------------------------------------------------------

/** What a module is called in the editor, whatever it shows. */
export function moduleLabel(m: LandingModule, text: Text): string {
  return (m.name ?? "").trim() || kindName(m, text) || text.t(KIND_NAMES[m.kind]);
}

/** Every kind's name in the editor, for the picker and the panel. */
export const KIND_NAMES: Record<LandingKind, string> = {
  heading: "landing.kindHeading",
  text: "landing.kindText",
  search: "landing.kindSearch",
  boards: "landing.kindBoards",
  reports: "landing.kindReports",
  roadmap: "landing.kindRoadmap",
  statistics: "landing.kindStatistics",
  links: "landing.kindLinks",
  image: "landing.kindImage",
};

function tool(act: string, label: string, icon: string, attrs = ""): string {
  return `<button type="button" class="lpe-tool" data-act="${escapeAttr(act)}" ${attrs}
    title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}">${icon}</button>`;
}

/**
 * A band's controls, on its top edge while it is pointed at or chosen. Moving
 * is by button rather than by dragging: a band is the size of the window, and
 * dragging one past another means scrolling while holding it.
 */
function bandTools(text: Text): string {
  return `<div class="lpe-tools lpe-band-tools">
    <button type="button" class="lpe-tag" data-act="select">${icons.section(13)}<span>${escapeHTML(
      text.t("landing.section"),
    )}</span></button>
    ${tool("up", text.t("landing.moveUp"), icons.chevron(14), 'data-turn="up"')}
    ${tool("down", text.t("landing.moveDown"), icons.chevron(14), 'data-turn="down"')}
    ${tool("copy", text.t("landing.duplicate"), icons.copy(14))}
    ${tool("remove", text.t("action.remove"), icons.remove(14), 'data-danger="1"')}
  </div>`;
}

/**
 * A module's controls. The grip is the only thing that drags: the rest of the
 * module is where its words are selected and typed, and a whole module that
 * drags turns every attempt to select a sentence into a move.
 */
function moduleTools(m: LandingModule, text: Text): string {
  return `<div class="lpe-tools lpe-mod-tools">
    <span class="lpe-grip" draggable="true" data-grip="${escapeAttr(m.id)}"
          title="${escapeAttr(text.t("landing.drag"))}" aria-hidden="true">${icons.grip(14)}</span>
    ${tool("copy", text.t("landing.duplicate"), icons.copy(14))}
    ${tool("remove", text.t("action.remove"), icons.remove(14), 'data-danger="1"')}
  </div>`;
}

function addModule(bandID: string, column: number, empty: boolean, text: Text): string {
  return `<button type="button" class="lpe-add${empty ? " empty" : ""}"
      data-add-module="${escapeAttr(bandID)}" data-column="${column}">
    ${icons.add(14)}<span>${escapeHTML(text.t("landing.addModule"))}</span>
  </button>`;
}

function insertBand(at: number, text: Text): string {
  return `<div class="lpe-insert">
    <button type="button" data-add-band="${at}">${icons.add(13)}<span>${escapeHTML(
      text.t("landing.addSection"),
    )}</span></button>
  </div>`;
}

// --- the modules ------------------------------------------------------------

/**
 * The top of the page: who this is, one line about it, and — on the default
 * page — the search that finds a report before somebody writes it again.
 *
 * An empty title and line are the installation's own name and tagline, so the
 * default page says who it belongs to without storing it twice.
 */
/**
 * The installation's mark, bare, as the About screen draws it: no tile around
 * it. Three sizes, chosen in the module; empty is About's own.
 */
const LOGO_PX = { small: 56, "": 80, large: 120 } as const;
function logo(src: string | undefined, size: "" | "small" | "large"): string {
  const px = LOGO_PX[size] ?? LOGO_PX[""];
  const url = src && src.trim() !== "" ? src : "/logo.svg";
  return `<img class="lp-logo${size ? ` logo-${size}` : ""}" src="${escapeAttr(url)}" alt="" width="${px}" height="${px}" />`;
}

function heading(m: LandingModule, ctx: LandingContext, text: Text): string {
  const title = (m.title ?? "").trim() || ctx.instance.name || text.t("app.name");
  const subtitle = (m.subtitle ?? "").trim() || text.t("app.tagline");
  // Written where it is read while editing, like the words of a text module.
  const editable = (field: "title" | "subtitle") =>
    ctx.editing ? ` contenteditable="true" spellcheck="true" data-field="${field}" data-for="${escapeAttr(m.id)}"` : "";
  return `<div class="lp-heading${m.align === "start" ? " align-start" : ""}">
    ${m.hide_logo ? "" : logo(ctx.instance.logo, m.logo_size ?? "")}
    <h1${editable("title")}>${escapeHTML(title)}</h1>
    <p class="lp-subtitle"${editable("subtitle")}>${escapeHTML(subtitle)}</p>
    ${m.with_search ? search(text) : ""}
  </div>`;
}

/**
 * The search a visitor meets before the form. Results appear as they type; the
 * last line is always the way to write a new report, so finding nothing is a
 * step forward rather than a dead end.
 */
function search(text: Text): string {
  return `<form class="lp-search" role="search" data-landing-search>
    <span class="lp-search-icon" aria-hidden="true">${icons.search(18)}</span>
    <input type="search" name="q" autocomplete="off"
           placeholder="${escapeAttr(text.t("landing.searchPlaceholder"))}"
           aria-label="${escapeAttr(text.t("landing.searchPlaceholder"))}" />
    <div class="lp-results" role="listbox" hidden></div>
  </form>`;
}

/**
 * The operator's words, written where they are seen.
 *
 * The markup is printed rather than escaped, which is the one place on this
 * page that happens — and it is safe because the server runs its markup policy
 * over this field on the way in and on the way out. See landing/clean.go.
 */
function words(m: LandingModule, ctx: LandingContext, text: Text): string {
  const written = (m.body ?? "").trim();
  const fallback = defaultWords(m.id, text);
  const inner =
    written !== ""
      ? written
      : fallback || (ctx.editing ? `<p class="words-prompt">${escapeHTML(text.t("landing.writeHere"))}</p>` : "");
  // A point of the default page keeps its mark outside the words, so writing
  // over the words does not take it away: the markup policy keeps no pictures
  // drawn in SVG, and the mark belongs to the module, not to what it says.
  const point = POINTS[m.id];
  return `<div class="words${m.tone ? ` tone-${escapeAttr(m.tone)}` : ""}${point ? " point" : ""}">
    ${point ? `<span class="point-icon" aria-hidden="true">${point.icon()}</span>` : ""}
    <div class="words-box"${ctx.editing ? ` contenteditable="true" data-words-body="${escapeAttr(m.id)}"` : ""}
         data-words-empty="${written === "" ? "1" : "0"}">${inner}</div>
  </div>`;
}

/**
 * Where to report: the boards, as cards or as a list, grouped under their
 * organisation when there is more than one.
 */
function boards(m: LandingModule, rows: LandingBoard[], text: Text): string {
  if (rows.length === 0) return emptyState(text.t("landing.noBoards"), icons.repository(28));
  const orgs = [...new Set(rows.map((r) => r.org))];
  const draw = (list: LandingBoard[]) =>
    m.display === "list"
      ? `<ul class="lp-rows">${list
          .map(
            (b) => `<li><a class="brief" href="/${escapeAttr(b.org)}/${escapeAttr(b.slug)}">
              ${markOf(b.name, b.icon, "lp-board-mark")}
              <span class="brief-title">${escapeHTML(b.name)}</span>
              ${accepts(b, text)}
            </a></li>`,
          )
          .join("")}</ul>`
      : `<ul class="lp-cards">${list
          .map(
            (b) => `<li><a class="lp-card" href="/${escapeAttr(b.org)}/${escapeAttr(b.slug)}">
              ${markOf(b.name, b.icon, "lp-board-mark")}
              <span class="lp-card-body">
                <span class="lp-card-title">${escapeHTML(b.name)}</span>
                ${accepts(b, text)}
              </span>
              <span class="lp-card-go" aria-hidden="true">${icons.chevron(16)}</span>
            </a></li>`,
          )
          .join("")}</ul>`;
  if (orgs.length < 2) return draw(rows);
  return orgs
    .map((slug) => {
      const inOrg = rows.filter((r) => r.org === slug);
      return `<div class="lp-org">
        <p class="lp-org-name">${escapeHTML(inOrg[0]?.org_name ?? slug)}</p>
        ${draw(inOrg)}
      </div>`;
    })
    .join("");
}

/**
 * What a board takes, as the marks of its kinds, each with how many it has had,
 * answered or not: a count of what is open sits at zero on a team that keeps up,
 * and reads as nothing ever being done. A kind the board does not take has no
 * mark, so a missing one is never read as a zero.
 */
function accepts(b: LandingBoard, text: Text): string {
  const mark = (kind: "bug" | "idea", total: number) => {
    const name = text.t(`kind.${kind}`);
    const label = `${name} ${text.number(total)}`;
    return `<span class="kind kind-${kind}" role="img" title="${escapeAttr(label)}"
       aria-label="${escapeAttr(label)}">${kind === "bug" ? icons.bug(14) : icons.idea(14)}<span>${escapeHTML(text.number(total))}</span></span>`;
  };
  return `<span class="lp-accepts">${b.accepts_bugs ? mark("bug", b.total_bugs ?? 0) : ""}${b.accepts_ideas ? mark("idea", b.total_ideas ?? 0) : ""}</span>`;
}

/**
 * A list of reports. Which one decides the row: votes lead the most voted, the
 * date closes what was resolved — "done last week" and "done in 2019" say
 * opposite things — and the rest name their board.
 */
function reports(m: LandingModule, rows: LandingReport[], text: Text): string {
  if (rows.length === 0) {
    const empty =
      m.show === "recently_done"
        ? "landing.noneDone"
        : m.show === "latest"
          ? "landing.noneYet"
          : m.show === "planned"
            ? "landing.noneComing"
            : "landing.noVotes";
    return emptyState(text.t(empty), icons.queue(28));
  }
  return `<ul class="lp-rows">${rows
    .map((r) => {
      const lead =
        !m.show || m.show === "most_voted"
          ? `<span class="score small"><span class="count">${escapeHTML(text.number(r.votes))}</span></span>`
          : kindMark(r.kind);
      const tail =
        m.show === "recently_done" && r.closed_at
          ? `<span class="when">${escapeHTML(text.date(r.closed_at))}</span>`
          : `<span class="card-sub">${escapeHTML(r.board)}</span>`;
      return `<li><a class="brief" href="/i/${escapeAttr(r.public_id)}">
        ${lead}<span class="brief-title">${escapeHTML(r.title)}</span>${tail}
      </a></li>`;
    })
    .join("")}</ul>`;
}

/**
 * The roadmap, in the three columns a board's own roadmap uses. Side by side
 * when the module is wide enough and stacked when it is not — decided by the
 * module's own width, so it reads the same in a full band and in a narrow
 * column.
 */
function roadmap(columns: LandingRoadmap | null, text: Text): string {
  const names: Array<[keyof LandingRoadmap, string]> = [
    ["planned", text.t("roadmap.planned")],
    ["in_progress", text.t("roadmap.in_progress")],
    ["done", text.t("roadmap.done")],
  ];
  if (!columns || !names.some(([key]) => (columns[key]?.length ?? 0) > 0)) {
    return emptyState(text.t("landing.noneComing"), icons.open(28));
  }
  return `<div class="lp-roadmap">${names
    .map(
      ([key, title]) => `<div class="lp-roadmap-col">
        <p class="column-name">${escapeHTML(title)}</p>
        ${
          (columns[key] ?? []).length === 0
            ? `<p class="hint">—</p>`
            : `<ul class="lp-rows">${(columns[key] ?? [])
                .map(
                  (r) => `<li><a class="brief" href="/i/${escapeAttr(r.public_id)}">
                    ${kindMark(r.kind)}<span class="brief-title">${escapeHTML(r.title)}</span>
                  </a></li>`,
                )
                .join("")}</ul>`
        }
      </div>`,
    )
    .join("")}</div>`;
}

/**
 * How much came in and where it got to. Never three zeroes: with nothing
 * published the server sends nothing and this says there is nothing to count.
 */
function statistics(counts: LandingCounts | null, text: Text): string {
  if (!counts) return emptyState(text.t("landing.noneCounted"), icons.overview(28));
  const figure = (value: number, label: string) =>
    `<div class="lp-stat">
      <span class="lp-stat-number">${escapeHTML(text.number(value))}</span>
      <span class="lp-stat-label">${escapeHTML(label)}</span>
    </div>`;
  return `<div class="lp-stats">
    ${figure(counts.received, text.t("landing.countReceived"))}
    ${figure(counts.open, text.t("landing.countOpen"))}
    ${figure(counts.done, text.t("landing.countDone"))}
  </div>`;
}

/** Where a link goes, as an address this page can put in an href. */
export function linkHref(l: LandingLink): string {
  const [org, board] = (l.scope ?? "").split("/");
  switch (l.to) {
    case "form":
      return `/${org}/${board}/new`;
    case "board":
      return `/${org}/${board}`;
    case "roadmap":
      return `/${org}/${board}/roadmap`;
    default:
      return l.url ?? "#";
  }
}

function linkTag(l: LandingLink): string {
  const outside = l.to === "url";
  const attrs = outside ? ` target="_blank" rel="noopener noreferrer"` : "";
  return l.button
    ? `<a class="button primary" href="${escapeAttr(linkHref(l))}"${attrs}>${escapeHTML(l.label)}</a>`
    : `<a class="lp-link" href="${escapeAttr(linkHref(l))}"${attrs}>${escapeHTML(l.label)}${
        outside ? `<span class="lp-out" aria-hidden="true">${icons.open(13)}</span>` : ""
      }</a>`;
}

/** A short list of places, each drawn as a button or as a link. */
function links(list: LandingLink[], ctx: LandingContext, text: Text): string {
  if (list.length === 0) {
    return ctx.editing ? emptyState(text.t("landing.noLinks"), icons.link(28)) : "";
  }
  const buttons = list.filter((l) => l.button);
  const plain = list.filter((l) => !l.button);
  return `${buttons.length ? `<div class="lp-buttons">${buttons.map(linkTag).join("")}</div>` : ""}
    ${plain.length ? `<ul class="lp-links">${plain.map((l) => `<li>${linkTag(l)}</li>`).join("")}</ul>` : ""}`;
}

/** One picture, with its description and an optional caption under it. */
function picture(m: LandingModule, ctx: LandingContext, text: Text): string {
  if (!m.image) {
    return ctx.editing ? emptyState(text.t("landing.noImage"), icons.image(28)) : "";
  }
  return `<figure class="lp-figure">
    <img src="${escapeAttr(m.image)}" alt="${escapeAttr(m.alt ?? "")}" loading="lazy" />
    ${m.caption ? `<figcaption>${escapeHTML(m.caption)}</figcaption>` : ""}
  </figure>`;
}

/** The foot of the page: whose it is, the operator's links, and what runs it. */
/**
 * The foot of the page: whose it is, a line of the operator's words, their
 * links, and what runs it — each of them the operator's to keep or drop.
 *
 * A foot with nothing left in it is not drawn for visitors. While editing it
 * always is, with its line written in place, because it is where the operator
 * clicks to get it back.
 */
/**
 * Where "Made with LastHelp" leads: the product's page on the house's site,
 * where somebody who liked this page can find what it runs on.
 */
export const PRODUCT_PAGE = "https://codigosh.com/projects/lasthelp/";

function footer(s: LandingSettings, ctx: LandingContext, text: Text): string {
  const links = s.footer ?? [];
  const words = (s.footer_text ?? "").trim();
  const brand = !s.footer_hide_brand;
  const credit = !s.footer_hide_credit;
  if (!ctx.editing && !brand && !words && !links.length && !credit) return "";
  const selected = ctx.editing && ctx.selected === FOOTER_ID;
  const line = ctx.editing
    ? `<span class="lp-footer-text" contenteditable="true" spellcheck="true" data-footer-text
         data-placeholder="${escapeAttr(text.t("landing.footerTextPlaceholder"))}">${escapeHTML(words)}</span>`
    : words
      ? `<span class="lp-footer-text">${escapeHTML(words)}</span>`
      : "";
  return `<footer class="lp-footer${selected ? " is-selected" : ""}"${ctx.editing ? ` data-footer tabindex="-1"` : ""}>
    ${
      ctx.editing
        ? `<div class="lpe-tools lpe-band-tools">
            <button type="button" class="lpe-tag" data-act="select-footer">${icons.section(13)}<span>${escapeHTML(
              text.t("landing.footer"),
            )}</span></button>
          </div>`
        : ""
    }
    <div class="lp-inner">
      ${brand ? `<span class="lp-footer-name">${brandMark(ctx.instance.logo, 24)}<span>${escapeHTML(ctx.instance.name || text.t("app.name"))}</span></span>` : ""}
      ${line}
      ${links.length ? `<nav class="lp-footer-links">${links.map(linkTag).join("")}</nav>` : ""}
      ${
        credit
          ? `<span class="lp-made">${text.markup("landing.madeWith", {
              product: `<a href="${escapeAttr(PRODUCT_PAGE)}" target="_blank" rel="noopener">LastHelp</a>`,
            })}</span>`
          : ""
      }
    </div>
  </footer>`;
}

/** One report the search found. */
export interface LandingSearchHit {
  public_id: string;
  title: string;
  kind: string;
  status: string;
  votes: number;
  board: string;
}

/**
 * What the search found, then the way to write a new report. With one board
 * that is its form; with several, a line for each, so nobody has to go and
 * find where to report after deciding nothing matched.
 */
export function searchResults(hits: LandingSearchHit[], boards: LandingBoard[], q: string, text: Text): string {
  const found = hits
    .map(
      (h) => `<a class="lp-hit" role="option" href="/i/${escapeAttr(h.public_id)}">
        ${kindMark(h.kind)}
        <span class="lp-hit-title">${escapeHTML(h.title)}</span>
        <span class="card-sub">${escapeHTML(h.board)}</span>
      </a>`,
    )
    .join("");
  const none = hits.length === 0 ? `<p class="lp-hit-none">${escapeHTML(text.t("landing.searchNone", { q }))}</p>` : "";
  const shown = boards.slice(0, 4);
  const create = shown
    .map(
      (b) => `<a class="lp-hit lp-hit-new" role="option" href="/${escapeAttr(b.org)}/${escapeAttr(b.slug)}/new">
        <span class="lp-hit-plus" aria-hidden="true">${icons.add(16)}</span>
        <span class="lp-hit-title">${escapeHTML(
          shown.length === 1 ? text.t("board.report") : text.t("landing.reportIn", { board: b.name }),
        )}</span>
      </a>`,
    )
    .join("");
  return `${found}${none}${create}`;
}

