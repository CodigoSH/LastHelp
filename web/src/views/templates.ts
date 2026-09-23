/**
 * The markup, as pure functions.
 *
 * They take their text as a parameter rather than reaching for the i18n
 * singleton, which is what lets them be tested without a browser: importing the
 * singleton drags in fetch and localStorage and the whole file stops counting
 * as testable.
 */
import type {
  ItemDetail,
  ItemSummary,
  OrganizationSummary,
  ReportField,
  RepositorySummary,
} from "../api";
import { escapeAttr, escapeHTML } from "../escape";
import { areaChart, donut } from "./charts";
import { icons } from "./icons";
import { actionButton, iconButton } from "./modal";

export { modal, actionButton, iconButton } from "./modal";
export { field, choiceField, areaField } from "./wizard";
import { areaField, choiceField, exampleFor, field, wizard } from "./wizard";

/** Everything a template needs to say something. */
export interface Text {
  t(key: string, params?: Record<string, string | number>): string;
  date(value: number): string;
  number(value: number): string;
  /** A size, at a scale a person reads, with the unit named in their language. */
  bytes(value: number): string;
  /** How long ago, in the largest unit that still says something. */
  since(seconds: number): string;
  /** How old, as short as the language writes it — for a column of rows. */
  age(seconds: number): string;
}

/**
 * The first screen anybody sees, signed in or not.
 *
 * It used to be one grey sentence — "nothing has been set up here yet" — on an
 * otherwise blank page. Nothing said what this is, and an operator who had just
 * installed it read that, correctly, as an unfinished product. A page that
 * admits it is empty before it says what it is has the order backwards.
 *
 * What it says now is the name of the installation, under its own mark. This
 * page is the front door of somebody's project — the first thing a stranger
 * sees of it — and the software running it is not what they came to read. The
 * name of the product lives on the About screen, where an operator looks for
 * it. An administrator looking at an empty one is offered the way out; a
 * visitor is told plainly, without being sent to a form that would refuse them.
 */
export interface LandingModule {
  id: string;
  kind:
    | "hero"
    | "organisations"
    | "text"
    | "boards"
    | "most_voted"
    | "recently_done"
    | "call_to_action"
    | "recent_activity"
    | "roadmap"
    | "statistics"
    | "section";
  /** One-based, the way CSS grid lines are. */
  x: number;
  y: number;
  w: number;
  h: number;
  body?: string;
  /** Which board a data module draws from, as "org/board". Empty for all. */
  scope?: string;
  /** What this module calls itself. Empty means the name of what it draws. */
  name?: string;
  /** How many things a module that lists them shows. Zero is as many as it has. */
  rows?: number;
  /** The colour of the module's own border, and how thick. */
  border_color?: string;
  border_width?: number;
  /** No border at all, which is not the same as not having chosen a width. */
  bare?: boolean;
  /** Nothing behind it, so the page shows through. */
  clear?: boolean;
  /** Where this module's name sits. Empty means the top. */
  label?: "top" | "bottom" | "pill" | "off";
  /**
   * The id of the section holding this module, empty for one on the page.
   *
   * A child's x, y, w and h are in ITS SECTION's grid, not the page's: a child
   * at 1,1 is the top-left cell of its section, wherever that section is.
   */
  parent?: string;
}

/** A board as the front page shows it, without going through its organisation. */
export interface LandingBoard {
  org: string;
  org_name: string;
  slug: string;
  name: string;
  accepts_bugs: boolean;
  accepts_ideas: boolean;
}

/** One report in a most-voted or recently-done module. */
export interface LandingReport {
  public_id: string;
  title: string;
  kind: string;
  status: string;
  votes: number;
  board: string;
  /** Only on something that has finished. */
  closed_at?: number;
}

/** The three roadmap columns, each holding a few reports. */
export interface LandingRoadmap {
  planned: LandingReport[];
  in_progress: LandingReport[];
  done: LandingReport[];
}

/** How much came in and where it got to, or null when nothing has. */
export interface LandingCounts {
  received: number;
  open: number;
  done: number;
}

/** Where a call-to-action button goes, or null when there is nowhere to go. */
export interface LandingDestination {
  org: string;
  slug: string;
  name: string;
  href: string;
}

/**
 * What the data modules of a page have to show, keyed by module id.
 *
 * By id and not by kind, because two modules of the same kind on one page is
 * the point: the most wanted ideas for one board beside those for another.
 */
export type LandingContent = Record<
  string,
  | LandingBoard[]
  | LandingReport[]
  | LandingDestination
  | LandingRoadmap
  | LandingCounts
  | null
  | undefined
>;

/** Twelve columns, which is the dashboard's grid and now this page's. */
export const LANDING_COLUMNS = 12;

/**
 * The public front page, composed.
 *
 * It used to be a fixed arrangement — mark, tagline, three sentences, the list
 * of organisations — which is the shape of a screen that asks for one thing,
 * on the one screen that has to show a visitor where to go. Now the operator
 * decides what is on it and in what order, and the default is exactly what was
 * there before so that an installation which never opens the editor sees no
 * change.
 *
 * A module this version does not know is skipped here rather than drawn as a
 * hole. The server refuses to store one, so the only way to arrive at this is
 * an older binary reading a newer layout — which is somebody downgrading, and
 * being shown the rest of their page is better than being shown none of it.
 */
export function landing(
  orgs: OrganizationSummary[],
  isAdmin: boolean,
  text: Text,
  instance?: { name?: string; logo?: string },
  modules?: LandingModule[],
  editing = false,
  content: LandingContent = {},
  allBoards: LandingBoard[] = [],
): string {
  const point = (icon: string, words: string) =>
    `<li><span class="point-icon">${icon}</span><span>${escapeHTML(words)}</span></li>`;


  /**
   * What the block that opens the page says until somebody writes it.
   *
   * It used to be all this module could say: a fixed mark, a fixed name and a
   * sentence nobody could change. It is a starting point now — the first thing
   * typed into the block replaces the lot — so an installation that never
   * touches its front page still opens saying who it is.
   *
   * Built as markup rather than as a component because that is what the block
   * holds: an operator who edits it is editing exactly this, and can take the
   * mark out or put their own words around it.
   */
  const heroWords = () => `<p class="hero-mark-line">${brandMark(instance?.logo, 44)}</p>
    <h1>${escapeHTML(instance?.name || text.t("app.name"))}</h1>
    <p class="lead">${escapeHTML(text.t("app.tagline"))}</p>`;

  /**
   * What a module calls itself, in one place.
   *
   * Read twice — once to draw the header inside the box and once to draw the
   * pill on its border — and a module whose two names drifted apart would be
   * the kind of fault nobody looks for.
   */
  const heading = (m: LandingModule): { icon: string; title: string; count?: string } | null => {
    const named = byKind(m);
    if (!named) return null;
    // What the operator called it wins over the name of what it draws. Three
    // boards modules pointed at three boards all saying "Repositories" tells
    // nobody which is which.
    return { ...named, title: (m.name ?? "").trim() || named.title };
  };

  const byKind = (m: LandingModule): { icon: string; title: string; count?: string } | null => {
    const rows = content[m.id];
    const many = Array.isArray(rows) ? rows.length : 0;
    switch (m.kind) {
      case "organisations":
        return { icon: icons.organization(16), title: text.t("shell.organizations"), count: text.number(orgs.length) };
      case "boards":
        return { icon: icons.repository(16), title: text.t("portal.boards"), count: text.number(many) };
      case "most_voted":
        return { icon: icons.idea(16), title: text.t("landing.moduleVoted"), count: text.number(many) };
      case "recently_done":
        return { icon: icons.queue(16), title: text.t("landing.moduleDone"), count: text.number(many) };
      case "recent_activity":
        return { icon: icons.queue(16), title: text.t("landing.moduleActivity"), count: text.number(many) };
      case "roadmap":
        return { icon: icons.open(16), title: text.t("landing.moduleRoadmap") };
      case "statistics":
        return { icon: icons.idea(16), title: text.t("landing.moduleStats") };
      default:
        return null;
    }
  };

  /**
   * One thing worth saying, with a mark beside it.
   *
   * It drew three fixed sentences — the right three for us, and nobody else's
   * to change. Now it is one point in the operator's own words, and wanting
   * three means putting three on the page: side by side, stacked, or in a block
   * of four. Deciding that is the reason this page is a grid at all.
   */
  // A point with nothing written falls back to one of the three things worth
  // saying about this product, in the order they appear on the page. That is
  // what keeps an installation that never composes anything showing the same
  // three sentences it always did, in its own language — the domain cannot
  // carry translated text, and putting English in the default layout would ship
  // one language to twenty.
  const spare = [
    { icon: icons.person(18), words: text.t("landing.pointNoAccount") },
    { icon: icons.idea(18), words: text.t("landing.pointVotes") },
    { icon: icons.open(18), words: text.t("landing.pointForge") },
  ];
  const pointsSoFar: string[] = [];

  // Which of the three a block gets is its position among the blocks that have
  // asked, which is the order they sit on the page.
  const introWords = (id: string) => {
    if (!pointsSoFar.includes(id)) pointsSoFar.push(id);
    const fallback = spare[Math.min(pointsSoFar.indexOf(id), spare.length - 1)]!;
    return `<ul class="points">${point(fallback.icon, fallback.words)}</ul>`;
  };

  const organisations = (head: string, at: string) =>
    orgs.length === 0
      ? emptyState(text.t(isAdmin ? "landing.emptyOperator" : "portal.empty"), icons.organization(28))
      : `<section class="landing-scroll ${labelClass(at)}">
          ${head}
          ${organizationList(orgs, text)}
        </section>`;

  /**
   * The boards themselves, skipping the organisation that holds them.
   *
   * The card is the one the organisation page already uses, copied rather than
   * reinvented, with the organisation's name added underneath: on a front page
   * that lists boards from everywhere, "web" on its own does not say whose.
   */
  const boards = (rows: LandingBoard[], head: string, at: string) =>
    rows.length === 0
      ? emptyState(text.t("landing.noBoards"), icons.repository(28))
      : `<section class="landing-scroll ${labelClass(at)}">
          ${head}
          <ul class="cards">${rows
            .map(
              (b) => `<li><a class="card" href="/${escapeAttr(b.org)}/${escapeAttr(b.slug)}">
                <span class="card-mark" aria-hidden="true">${escapeHTML(initials(b.name))}</span>
                <span class="card-body">
                  <span class="card-title">${escapeHTML(b.name)}</span>
                  <span class="card-sub">${escapeHTML(b.org_name)}</span>
                  <span class="chips">
                    ${b.accepts_bugs ? `<span class="chip">${icons.bug(12)}<span>${escapeHTML(text.t("kind.bug"))}</span></span>` : ""}
                    ${b.accepts_ideas ? `<span class="chip">${icons.idea(12)}<span>${escapeHTML(text.t("kind.idea"))}</span></span>` : ""}
                  </span>
                </span>
                <span class="card-go" aria-hidden="true">${icons.chevron(18)}</span>
              </a></li>`,
            )
            .join("")}</ul>
        </section>`;

  /**
   * One board, as the module itself.
   *
   * A module that shows one thing is that thing: a list of one row inside a box
   * with a heading saying "boards" is three pieces of furniture around a single
   * card. This is the shape that makes eight boards in a section read as eight
   * boards rather than as eight lists.
   */
  const oneBoard = (rows: LandingBoard[]) => {
    const b = rows[0];
    if (!b) return emptyState(text.t("landing.noBoards"), icons.repository(28));
    return `<a class="landing-one" href="/${escapeAttr(b.org)}/${escapeAttr(b.slug)}">
      <span class="card-mark" aria-hidden="true">${escapeHTML(initials(b.name))}</span>
      <span class="card-body">
        <span class="card-title">${escapeHTML(b.name)}</span>
        <span class="card-sub">${escapeHTML(b.org_name)}</span>
        <span class="chips">
          ${b.accepts_bugs ? `<span class="chip">${icons.bug(12)}<span>${escapeHTML(text.t("kind.bug"))}</span></span>` : ""}
          ${b.accepts_ideas ? `<span class="chip">${icons.idea(12)}<span>${escapeHTML(text.t("kind.idea"))}</span></span>` : ""}
        </span>
      </span>
      <span class="card-go" aria-hidden="true">${icons.chevron(18)}</span>
    </a>`;
  };

  /** One organisation, the same way. */
  const oneOrg = (scope: string) => {
    const o = orgs.find((each) => each.slug === scope);
    if (!o) return emptyState(text.t("portal.empty"), icons.organization(28));
    return `<a class="landing-one" href="/${escapeAttr(o.slug)}">
      <span class="card-mark" aria-hidden="true">${escapeHTML(initials(o.name))}</span>
      <span class="card-body">
        <span class="card-title">${escapeHTML(o.name)}</span>
        <span class="card-sub">/${escapeHTML(o.slug)}</span>
      </span>
      <span class="card-go" aria-hidden="true">${icons.chevron(18)}</span>
    </a>`;
  };

  /**
   * The ideas people asked for most.
   *
   * The page has promised this since it existed — "ideas collect votes, which
   * shows what is most requested" — and had nowhere that showed one. The row is
   * the dashboard's, votes and all, so a visitor and the team read the same
   * thing in the same shape.
   */
  const voted = (rows: LandingReport[], head: string, at: string) =>
    rows.length === 0
      ? emptyState(text.t("landing.noVotes"), icons.idea(28))
      : `<section class="landing-scroll ${labelClass(at)}">
          ${head}
          <ul class="briefs">${rows
            .map(
              (v) => `<li><a class="brief" href="/i/${escapeAttr(v.public_id)}">
                <span class="score small"><span class="count">${escapeHTML(text.number(v.votes))}</span></span>
                <span class="brief-title">${escapeHTML(v.title)}</span>
                <span class="card-sub">${escapeHTML(v.board)}</span>
              </a></li>`,
            )
            .join("")}</ul>
        </section>`;

  /**
   * What was put right lately.
   *
   * The strongest thing this page can say to somebody weighing up whether to
   * type anything: reports here get read and acted on. So each row carries when
   * it finished — without a date this is just a list of things that are done,
   * and "done in 2019" and "done last week" say opposite things.
   */
  const finished = (rows: LandingReport[], head: string, at: string) =>
    rows.length === 0
      ? emptyState(text.t("landing.noneDone"), icons.queue(28))
      : `<section class="landing-scroll ${labelClass(at)}">
          ${head}
          <ul class="briefs">${rows
            .map(
              (r) => `<li><a class="brief" href="/i/${escapeAttr(r.public_id)}">
                ${kindMark(r.kind)}
                <span class="brief-title">${escapeHTML(r.title)}</span>
                ${r.closed_at ? `<span class="when">${escapeHTML(text.date(r.closed_at))}</span>` : ""}
              </a></li>`,
            )
            .join("")}</ul>
        </section>`;

  /** What has arrived lately, newest first. The board's own row, unchanged. */
  const lately = (rows: LandingReport[], head: string, at: string) =>
    rows.length === 0
      ? emptyState(text.t("landing.noneYet"), icons.queue(28))
      : `<section class="landing-scroll ${labelClass(at)}">
          ${head}
          <ul class="briefs">${rows
            .map(
              (r) => `<li><a class="brief" href="/i/${escapeAttr(r.public_id)}">
                ${kindMark(r.kind)}
                <span class="brief-title">${escapeHTML(r.title)}</span>
                <span class="card-sub">${escapeHTML(r.board)}</span>
              </a></li>`,
            )
            .join("")}</ul>
        </section>`;

  /**
   * What is coming, being worked on, and done.
   *
   * The same three columns the board's own roadmap uses, stacked rather than
   * side by side: a module is a box on a page, and three columns inside one are
   * three columns of two words each.
   */
  const coming = (columns: LandingRoadmap | null, head: string, at: string) => {
    const names: Array<[keyof LandingRoadmap, string]> = [
      ["planned", text.t("roadmap.planned")],
      ["in_progress", text.t("roadmap.in_progress")],
      ["done", text.t("roadmap.done")],
    ];
    const any = names.some(([key]) => (columns?.[key]?.length ?? 0) > 0);
    if (!columns || !any) return emptyState(text.t("landing.noneComing"), icons.queue(28));

    return `<section class="landing-scroll ${labelClass(at)}">
      ${head}
      <div class="landing-columns">${names
        .map(([key, title]) => {
          const rows = columns[key] ?? [];
          if (rows.length === 0) return "";
          return `<div class="landing-column">
            <p class="column-name">${escapeHTML(title)}</p>
            <ul class="briefs">${rows
              .map(
                (r) => `<li><a class="brief" href="/i/${escapeAttr(r.public_id)}">
                  ${kindMark(r.kind)}
                  <span class="brief-title">${escapeHTML(r.title)}</span>
                </a></li>`,
              )
              .join("")}</ul>
          </div>`;
        })
        .join("")}</div>
    </section>`;
  };

  /**
   * How much came in and where it got to.
   *
   * What is avoided is the three zeroes, not the module. A fresh installation
   * showing "0 received, 0 open, 0 done" tells the first person who ever looks
   * that nothing happens here, which is untrue — so the server sends nothing
   * and this says there is nothing to count yet, the same way every other
   * module says it has nothing to show.
   *
   * It does not vanish. A module that disappears leaves a hole where the
   * operator put something, and reappears in edit mode, which reads as a page
   * that failed rather than as a page with nothing in it yet.
   */
  const counted = (counts: LandingCounts | null, head: string, at: string) => {
    if (!counts) return emptyState(text.t("landing.noneCounted"), icons.idea(28));
    const figure = (value: number, label: string) =>
      `<div class="landing-figure">
        <span class="figure-number">${escapeHTML(text.number(value))}</span>
        <span class="figure-label">${escapeHTML(label)}</span>
      </div>`;
    return `<section class="landing-scroll ${labelClass(at)}">
      ${head}
      <div class="landing-figures">
        ${figure(counts.received, text.t("landing.countReceived"))}
        ${figure(counts.open, text.t("landing.countOpen"))}
        ${figure(counts.done, text.t("landing.countDone"))}
      </div>
    </section>`;
  };

  /**
   * The one thing this page exists to make happen.
   *
   * Until now the only route to a report form was the list: pick an
   * organisation, pick a board, find the button. An operator with one project
   * can put that button on the front page instead.
   *
   * Nothing is drawn when there is nowhere to send anybody. A button to a form
   * that does not exist is worse than no button, and in edit mode the module is
   * still there to open and point somewhere.
   */
  const action = (to: LandingDestination | null, label: string) =>
    !to
      ? ""
      : `<section class="section landing-action">
          <a class="button primary with-icon" href="${escapeAttr(to.href)}">
            ${icons.add()}<span>${escapeHTML(label.trim() || text.t("board.report"))}</span>
          </a>
          <span class="card-sub">${escapeHTML(to.name)}</span>
        </section>`;

  /**
   * A section: a box with its own grid, and the modules the operator put in it.
   *
   * As many columns and rows as the section is wide and tall, so a child at 3,2
   * of a six-by-two section lands in that section's third column and second
   * row. The dashboard this comes from does exactly this, and it is what lets a
   * section be dragged elsewhere without touching anything inside it.
   */
  const group = (m: LandingModule, inside: LandingModule[]) =>
    `<div class="landing-section" data-section="${escapeAttr(m.id)}">${
      inside.length === 0 && !editing
        ? ""
        : inside.map((child) => cell(child)).join("")
    }</div>`;

  // The operator's own words. Escaped and shown as written, line breaks and
  // all: this module takes text, not markup, and the domain stores it that way
  // for the same reason.
  /**
   * A block the operator writes, where they see it.
   *
   * The markup is printed rather than escaped, which is the one place on this
   * page that happens — and it is safe for a reason that has nothing to do with
   * trusting the browser: the server runs the markup policy over this field on
   * the way in AND on the way out, so what arrives here has already been
   * through it twice. See internal/core/landing/clean.go.
   *
   * An empty block falls back rather than showing a blank box. The fallbacks
   * live here, in the catalogue, because they have to be in the visitor's
   * language and the stored layout cannot carry twenty of them.
   */
  const words = (m: LandingModule, fallback: string) => {
    const written = (m.body ?? "").trim();
    const inner = written !== "" ? written : fallback;
    // Written while the page is being arranged, and only then. It carried a
    // latch that opened it on the ordinary page as well; that is one way in
    // too many for a page every visitor sees, and arranging already opens
    // every block on it.
    return `<div class="words">
      <div class="words-box"${
        editing ? ` contenteditable="true" data-words-body="${escapeAttr(m.id)}"` : ""
      } data-words-empty="${written === "" ? "1" : "0"}">${inner}</div>
    </div>`;
  };

  const shown =
    modules && modules.length > 0
      ? modules
      : ([
          { id: "hero", kind: "hero", x: 1, y: 1, w: 12, h: 4 },
          { id: "intro", kind: "intro", x: 1, y: 5, w: 12, h: 2 },
          { id: "organisations", kind: "organisations", x: 1, y: 7, w: 12, h: 3 },
        ] as LandingModule[]);

  // In edit mode a module that draws nothing still has to be there: an empty
  // text module is exactly the one somebody needs to open and write in, and a
  // module that vanishes because it has no content yet cannot be edited back.
  // A section holds modules, so the page draws the sections and each section
  // draws what is inside it. The children are not in this list: they are placed
  // in their section's own grid, in their section's own coordinates.
  const childrenOf = (id: string) => shown.filter((m) => m.parent === id);

  /**
   * One module as a cell of whichever grid it belongs to.
   *
   * Named rather than written inline in the loop because a section calls it for
   * its own children: a child is drawn by exactly the same recipe as a module
   * on the page, which is what makes dragging one out of a section produce the
   * same thing it was before it went in.
   */
  const cell = (m: LandingModule): string => {
      // Where this module keeps its name. Empty is the top, which is what every
      // module drew before there was anywhere else to put it.
      const at = m.label ?? "top";
      const name = heading(m);
      const head = name ? moduleHead(name.icon, name.title, name.count, at) : "";

      const inner =
        m.kind === "hero"
          ? words(m, heroWords())
          : m.kind === "organisations"
              ? showsOne(m)
                ? oneOrg(m.scope ?? "")
                : organisations(head, at)
              : m.kind === "text"
                ? words(
                    m,
                    // The three the default page ships with keep their
                    // sentences; anything the operator added themselves starts
                    // empty, with a line saying so while they are arranging.
                    /^point-[123]$/.test(m.id)
                      ? introWords(m.id)
                      : editing
                        ? `<p class="words-prompt">${escapeHTML(text.t("landing.writeHere"))}</p>`
                        : "",
                  )
                : m.kind === "boards"
                  ? showsOne(m)
                    ? oneBoard((content[m.id] as LandingBoard[]) ?? [])
                    : boards((content[m.id] as LandingBoard[]) ?? [], head, at)
                  : m.kind === "most_voted"
                    ? voted((content[m.id] as LandingReport[]) ?? [], head, at)
                    : m.kind === "recently_done"
                      ? finished((content[m.id] as LandingReport[]) ?? [], head, at)
                      : m.kind === "call_to_action"
                        ? action((content[m.id] as LandingDestination) ?? null, m.body ?? "")
                        : m.kind === "recent_activity"
                          ? lately((content[m.id] as LandingReport[]) ?? [], head, at)
                          : m.kind === "roadmap"
                            ? coming((content[m.id] as LandingRoadmap) ?? null, head, at)
                            : m.kind === "statistics"
                              ? counted((content[m.id] as LandingCounts) ?? null, head, at)
                              : m.kind === "section"
                                ? group(m, childrenOf(m.id))
                                : "";

      // A section is not a box with a name at the top: its name is always the
      // pill on its border, or nothing at all, which is what it is in the
      // dashboard this comes from and is the whole of what makes it readable as
      // a container rather than as one more module.
      if (m.kind === "section") {
        const title = (m.body ?? "").trim();
        const tools = editing ? kebab(m.id, text) : "";
        const handle = editing ? resizeHandle(m.id) : "";
        return `<div class="landing-cell landing-group${title ? " has-pill" : ""}"
                     data-module="${escapeAttr(m.id)}"
                     ${editing ? 'draggable="true"' : ""}>${
                       title ? modulePill(title) : ""
                     }${tools}${inner}${handle}</div>`;
      }

      // The pill lives outside the scrolling box, as a child of the cell: it
      // hangs over the border, and a box that scrolls clips whatever hangs over
      // its edge. The cell leaves room for it with padding.
      const pill = at === "pill" && name && inner !== "" ? modulePill(name.title, name.count) : "";
      if (inner === "" && !editing) return "";

      // One button, not two. The dashboard in this house puts a module's
      // actions behind a single kebab with a tooltip, and two loose icons
      // floating over content is both a different look and one more thing
      // overlapping whatever the module is showing.
      const tools = editing ? kebab(m.id, text) : "";

      // Placed by the same custom properties the dashboard uses, so the
      // stylesheet does the arithmetic and nothing here computes pixels.
      const handle = editing ? resizeHandle(m.id) : "";
      // No style attribute. This product serves `style-src 'self'` without
      // unsafe-inline — it is the one place the house is stricter than the
      // dashboard this grid comes from — so the browser would drop it and every
      // module would land in cell one. The coordinates go in through the
      // CSSOM instead, which the policy does not govern, which is the same way
      // this product already does an accent colour and a tooltip position.
      return `<div class="landing-cell${pill ? " has-pill" : ""}" data-module="${escapeAttr(m.id)}"
                   ${editing ? 'draggable="true"' : ""}>${pill}${tools}${
                     inner || `<p class="hint">${escapeHTML(text.t("landing.moduleText"))}</p>`
                   }${handle}</div>`;
  };

  const drawn = shown
    .filter((m) => !m.parent)
    .map(cell)
    .join("");

  // The bar that turns editing on, and the controls that only exist while it
  // is. Modelled on the dashboard this house already has: you arrange the page
  // on the page, not in a settings screen somewhere else.
  return `<div class="page">
    <div class="landing-grid${editing ? " editing" : ""}">
      ${drawn}
      ${editing ? `<div class="landing-ghost" hidden></div>` : ""}
    </div>
    ${editing ? wordsBar(text) : ""}
    ${editing ? modulePicker(text) + moduleModal(text, allBoards, orgs) + confirmModal(text) : ""}
  </div>`;
}

/**
 * One modal for every module, told which one it is when it opens.
 *
 * The same shape the dashboard in this house uses: not a form per kind, but one
 * dialog whose body is filled for the module being edited. A module that grows
 * a setting later adds a field here rather than a screen somewhere.
 */
/**
 * Choosing what to add, as a grid of cards.
 *
 * The dashboard's own picker: every kind laid out with its mark, its name and a
 * line saying what it draws, and you pick one. It was a dropdown and an Add
 * button sitting on the page — two controls that told you nothing about what
 * you were about to get, and that lived in the middle of the thing every
 * visitor sees.
 *
 * A group is not in here. It goes in the menu beside this, because it is not a
 * module that draws something: it is the box the others go into.
 */
/**
 * The bar that follows the cursor while a block of words is being written.
 *
 * One bar, moved into whichever block has the cursor, rather than one per
 * module: a page of eight modules would otherwise rebuild eight of these on
 * every redraw, and the dashboard this is taken from has one too.
 *
 * Alignment and colour are not the browser's own commands. This page is served
 * with style-src 'self', so an inline style is dropped without a word and those
 * two buttons would appear to work and change nothing — they write classes out
 * of the set the server will accept instead. See services/words.ts.
 */
function wordsBar(text: Text): string {
  const tool = (does: string, label: string, icon: string, wide = false) =>
    `<button type="button" class="words-tool${wide ? " wide" : ""}" data-do="${escapeAttr(does)}"
      title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}">${icon}</button>`;

  const ink = (name: string, label: string) =>
    `<button type="button" class="words-ink ${name}" data-do="ink:${escapeAttr(name)}"
      title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}"><span></span></button>`;

  return `<div class="words-bar" hidden>
    <div class="words-group">
      ${tool("undo", text.t("words.undo"), icons.undo(15))}
      ${tool("redo", text.t("words.redo"), icons.redo(15))}
    </div>
    <div class="words-group">
      ${tool("formatBlock:h1", text.t("words.h1"), "H1", true)}
      ${tool("formatBlock:h2", text.t("words.h2"), "H2", true)}
      ${tool("formatBlock:h3", text.t("words.h3"), "H3", true)}
      ${tool("formatBlock:p", text.t("words.paragraph"), "P", true)}
    </div>
    <div class="words-group">
      ${tool("bold", text.t("words.bold"), icons.bold(15))}
      ${tool("italic", text.t("words.italic"), icons.italic(15))}
      ${tool("underline", text.t("words.underline"), icons.underline(15))}
      ${tool("strikeThrough", text.t("words.strike"), icons.strike(15))}
    </div>
    <div class="words-group">
      ${tool("align:align-start", text.t("words.alignStart"), icons.alignStart(15))}
      ${tool("align:align-center", text.t("words.alignCenter"), icons.alignCenter(15))}
      ${tool("align:align-end", text.t("words.alignEnd"), icons.alignEnd(15))}
      ${tool("align:align-justify", text.t("words.alignJustify"), icons.alignJustify(15))}
    </div>
    <div class="words-group">
      ${tool("insertUnorderedList", text.t("words.bullets"), icons.bullets(15))}
      ${tool("insertOrderedList", text.t("words.numbers"), icons.numbers(15))}
      ${tool("link:", text.t("words.link"), icons.link(15))}
      ${tool("image:", text.t("words.image"), icons.image(15))}
    </div>
    <div class="words-group">
      ${ink("ink-plain", text.t("words.inkPlain"))}
      ${ink("ink-dim", text.t("words.inkDim"))}
      ${ink("ink-accent", text.t("words.inkAccent"))}
      ${ink("ink-danger", text.t("words.inkDanger"))}
      ${ink("ink-success", text.t("words.inkSuccess"))}
      ${ink("ink-warning", text.t("words.inkWarning"))}
      ${ink("ink-info", text.t("words.inkInfo"))}
    </div>
  </div>`;
}

function modulePicker(text: Text): string {
  const kinds: Array<[string, string, string, string]> = [
    ["hero", icons.overview(32), text.t("landing.moduleHero"), text.t("landing.aboutHero")],
    ["text", icons.edit(32), text.t("landing.moduleText"), text.t("landing.aboutText")],
    ["call_to_action", icons.add(32), text.t("landing.moduleAction"), text.t("landing.aboutAction")],
    ["organisations", icons.organization(32), text.t("shell.organizations"), text.t("landing.aboutOrgs")],
    ["boards", icons.repository(32), text.t("portal.boards"), text.t("landing.aboutBoards")],
    ["most_voted", icons.idea(32), text.t("landing.moduleVoted"), text.t("landing.aboutVoted")],
    ["recently_done", icons.check(32), text.t("landing.moduleDone"), text.t("landing.aboutDone")],
    ["recent_activity", icons.queue(32), text.t("landing.moduleActivity"), text.t("landing.aboutActivity")],
    ["roadmap", icons.open(32), text.t("landing.moduleRoadmap"), text.t("landing.aboutRoadmap")],
    ["statistics", icons.monitor(32), text.t("landing.moduleStats"), text.t("landing.aboutStats")],
  ];

  return `<dialog class="modal chooser" id="module-picker">
    <div class="picker-head">
      <div class="title-group">
        <h2>${escapeHTML(text.t("landing.newModule"))}</h2>
        <p>${escapeHTML(text.t("landing.newModuleLead"))}</p>
      </div>
      <button type="button" class="picker-close" data-close
        aria-label="${escapeAttr(text.t("action.close"))}">${icons.close(20)}</button>
    </div>
    <div class="picker-grid">${kinds
      .map(
        ([kind, icon, name, about]) => `<button type="button" class="picker-card" data-pick="${escapeAttr(kind)}">
          <span class="picker-icon" aria-hidden="true">${icon}</span>
          <span class="picker-name">${escapeHTML(name)}</span>
          <span class="picker-about">${escapeHTML(about)}</span>
        </button>`,
      )
      .join("")}</div>
  </dialog>`;
}

/**
 * A module's settings, in the shape the dashboard uses for its own.
 *
 * The tabs ARE the header — General and Customization, with the close beside
 * them — and the only action at the foot is Save. That is not decoration: it is
 * the same dialog somebody has already used on the other product, and the point
 * of all of this is that they do not have to learn it twice.
 *
 * General is what the module shows: what it is called, where it draws from, and
 * its words. Customization is how it looks: where its name sits. A field a kind
 * has no use for is not shown to it — a dialog offering a control that does
 * nothing is a dialog that lies about what it does.
 */
/**
 * The dashboard's ten. Copied rather than chosen again, so a border set over
 * there and a border set here are the same colour and not two that nearly
 * match.
 */
const BORDER_COLOURS = [
  "#3b82f6",
  "#10b981",
  "#f59e0b",
  "#ef4444",
  "#8b5cf6",
  "#ec4899",
  "#64748b",
  "#f8fafc",
  "#1e293b",
  "#333333",
];

/**
 * Where a name sits, as the four little diagrams the dashboard draws.
 *
 * Its own marks, verbatim: a box with a line near the top, near the bottom, one
 * along the border, and a box with a dot for none. A word for each would be a
 * second vocabulary for something somebody has already learned by looking.
 */
function labelMark(at: string): string {
  const box = `<rect x="4" y="4" width="16" height="16" rx="2" stroke-opacity="0.5" />`;
  const inner =
    at === "top"
      ? `${box}<path d="M8 8 h8" /><path d="M12 13 v2" />`
      : at === "pill"
        ? `${box}<path d="M7 4 h10" stroke-width="4" />`
        : at === "off"
          ? `${box}<circle cx="12" cy="12" r="1.5" fill="currentColor" />`
          : `${box}<path d="M8 16 h8" /><path d="M12 9 v2" />`;
  return `<svg viewBox="0 0 24 24" width="20" height="20" fill="none"
    stroke="currentColor" stroke-width="2" aria-hidden="true">${inner}</svg>`;
}

/** How a module's own box looks: its border's colour and how thick it is. */
function appearanceControls(text: Text): string {
  return `<div class="field-group">
      <label>${escapeHTML(text.t("landing.borderColour"))}</label>
      <div class="colour-grid">
        <button type="button" class="colour-swatch none" data-colour=""
          title="${escapeAttr(text.t("landing.borderDefault"))}"
          aria-label="${escapeAttr(text.t("landing.borderDefault"))}"></button>
        ${BORDER_COLOURS.map(
          // No style attribute: this product serves style-src 'self', and the
          // ten colours are a known set, so they are rules in the stylesheet
          // keyed by the same attribute the button already carries.
          (colour) => `<button type="button" class="colour-swatch" data-colour="${escapeAttr(colour)}"
            aria-label="${escapeAttr(colour)}"></button>`,
        ).join("")}
      </div>
      <input type="hidden" name="borderColour" />
    </div>

    <div class="field-group">
      <label>${escapeHTML(text.t("landing.borderWidth"))}</label>
      <div class="segmented">
        ${[
          [0, text.t("landing.noBorder")],
          [1, text.t("landing.thin")],
          [2, text.t("landing.regular")],
          [3, text.t("landing.thick")],
        ]
          .map(
            ([width, name]) => `<button type="button" class="segment" data-width="${width}">
              <span class="segment-rule"></span>
              <span>${escapeHTML(String(name))}</span>
            </button>`,
          )
          .join("")}
      </div>
      <input type="hidden" name="borderWidth" />
      <label class="switch-row">
        <input type="checkbox" name="clear" />
        <span>${escapeHTML(text.t("landing.clear"))}</span>
      </label>
      <p class="hint">${escapeHTML(text.t("landing.clearHelp"))}</p>
    </div>`;
}

/**
 * Asking before removing, which the dashboard does and this did not.
 *
 * Their shape: a title, a line saying what will happen, and one button in the
 * danger variant. No cancel — the cross is how you back out, which is the same
 * rule the settings dialog follows.
 */
/**
 * Asking before removing something.
 *
 * A different composition from the settings dialog, and it was wearing that
 * one's: a padded box, a title over a rule, the sentence, and one danger button
 * at the end of the line. Not the full-width outlined action — that belongs to
 * the dialog where the action is "save what I just typed", and this is the
 * dialog where the action is "yes, lose it".
 */
function confirmModal(text: Text): string {
  return `<dialog class="modal confirm" id="confirm-remove">
    <div class="confirm-head">
      <h2 class="confirm-title" data-confirm-title></h2>
      <button type="button" class="modal-close" data-close
        aria-label="${escapeAttr(text.t("action.close"))}">${icons.close(20)}</button>
    </div>
    <p class="modal-message" data-confirm-message></p>
    <div class="confirm-actions">
      <button type="button" class="button danger" data-confirm>${escapeHTML(text.t("action.remove"))}</button>
    </div>
  </dialog>`;
}

function moduleModal(
  text: Text,
  boards: LandingBoard[] = [],
  orgs: OrganizationSummary[] = [],
): string {
  const options = [
    `<option value="" data-level="all">${escapeHTML(text.t("landing.scopeAll"))}</option>`,
    ...orgs.map(
      (o) => `<option value="${escapeAttr(o.slug)}" data-level="org">${escapeHTML(o.name)}</option>`,
    ),
    ...boards.map(
      (b) =>
        `<option value="${escapeAttr(b.org + "/" + b.slug)}" data-level="board">${escapeHTML(
          b.org_name + " / " + b.name,
        )}</option>`,
    ),
  ].join("");

  return `<dialog class="modal" id="module-config">
    <form class="module-config" method="dialog">
      <div class="modal-head tabbed">
        <div class="modal-tabs" role="tablist">
          <button type="button" class="tab-btn on" role="tab" data-tab="general"
            aria-selected="true">${escapeHTML(text.t("landing.tabGeneral"))}</button>
          <button type="button" class="tab-btn" role="tab" data-tab="look"
            aria-selected="false">${escapeHTML(text.t("landing.tabLook"))}</button>
        </div>
        <button type="button" class="modal-close" data-close
          aria-label="${escapeAttr(text.t("action.close"))}">${icons.close()}</button>
      </div>

      <div class="modal-body">
        <input type="hidden" name="id" />

        <div class="tab-panel" data-panel="general">
          <div class="field-group" data-for-name hidden>
            <label for="module-name">${escapeHTML(text.t("landing.name"))}</label>
            <div class="input-group">
              <div class="icon-dropdown" data-for-label-at>
                <button type="button" class="icon-dropdown-btn" data-open-marks
                  title="${escapeAttr(text.t("landing.labelAt"))}"
                  aria-label="${escapeAttr(text.t("landing.labelAt"))}"
                  aria-haspopup="menu" aria-expanded="false">
                  <span data-current-mark>${labelMark("top")}</span>
                  <svg class="caret" viewBox="0 0 24 24" width="10" height="10"
                       fill="currentColor" aria-hidden="true"><path d="M7 10l5 5 5-5z" /></svg>
                </button>
                <input type="hidden" name="labelAt" />
                <div class="dropdown-menu" data-marks role="menu" hidden>
                  ${(
                    [
                      ["top", text.t("landing.labelTop")],
                      ["bottom", text.t("landing.labelBottom")],
                      ["pill", text.t("landing.labelPill")],
                      ["off", text.t("landing.labelOff")],
                    ] as Array<[string, string]>
                  )
                    .map(
                      ([at, name]) => `<button type="button" class="dropdown-item" role="menuitem"
                        data-at="${escapeAttr(at)}" title="${escapeAttr(name)}"
                        aria-label="${escapeAttr(name)}">${labelMark(at)}</button>`,
                    )
                    .join("")}
                </div>
              </div>
              <input id="module-name" name="name" type="text" maxlength="80" />
            </div>
            <p class="hint">${escapeHTML(text.t("landing.nameHelp"))}</p>
          </div>
          <div class="field-group" data-for-scope hidden>
            <label for="module-scope">${escapeHTML(text.t("landing.scope"))}</label>
            <select id="module-scope" name="scope">${options}</select>
          </div>
          
          <div class="field-group" data-for-rows hidden>
            <label for="module-rows">${escapeHTML(text.t("landing.rows"))}</label>
            <div class="stepper">
              <button type="button" class="stepper-step" data-step="-1"
                aria-label="${escapeAttr(text.t("landing.rowsFewer"))}">${icons.minus(16)}</button>
              <input id="module-rows" name="rows" type="number" min="0" max="20" step="1"
                inputmode="numeric" />
              <button type="button" class="stepper-step" data-step="1"
                aria-label="${escapeAttr(text.t("landing.rowsMore"))}">${icons.add(16)}</button>
            </div>
            <p class="hint">${escapeHTML(text.t("landing.rowsHelp"))}</p>
          </div>
          
          <div class="field-group" data-for-label hidden>
            <label for="module-label">${escapeHTML(text.t("landing.label"))}</label>
            <input id="module-label" name="label" type="text" maxlength="160"
                   placeholder="${escapeAttr(text.t("board.report"))}" />
          </div>
        </div>

        <div class="tab-panel" data-panel="look" hidden>
          ${appearanceControls(text)}
        </div>

        <p class="form-error" role="alert" hidden></p>
      </div>

      <div class="modal-foot">
        <button type="submit" class="save-changes">${escapeHTML(text.t("landing.saveChanges"))}</button>
      </div>
    </form>
  </dialog>`;
}

/**
 * A page that could not be shown, as a page.
 *
 * It was the bare grey panel used for an empty list, dropped into the middle of
 * an otherwise blank screen with no title and nowhere to go — which reads as a
 * page that failed to load rather than as an answer. What somebody needs here
 * is what happened and one way onward.
 */
export function failureScreen(message: string, text: Text): string {
  return `<div class="page narrow failure">
    <span class="hero-mark">${icons.warning(28)}</span>
    <h1>${escapeHTML(message)}</h1>
    <a class="button primary" href="/">${escapeHTML(text.t("nav.home"))}</a>
  </div>`;
}

/**
 * Nothing here, said as a place rather than as a missing thing.
 *
 * One function, because there are seven of these across the product and they
 * used to be seven grey sentences of their own invention. A mark, the sentence,
 * and — where there is one — the thing to do about it.
 */
export function emptyState(words: string, icon = "", action = ""): string {
  return `<div class="empty">
    ${icon}
    <span>${escapeHTML(words)}</span>
    ${action}
  </div>`;
}

/** The small title of a part of a page, with what it holds counted beside it. */
export function sectionHead(title: string, count?: string): string {
  return `<h2 class="section-legend">
    <span>${escapeHTML(title)}</span>
    ${count !== undefined ? `<span class="count-chip">${escapeHTML(count)}</span>` : ""}
  </h2>`;
}

/**
 * How a module in the front page's grid names itself.
 *
 * Deliberately not sectionHead. That draws the pill mounted on the border, and
 * in the dashboard this grid comes from that pill belongs to one thing only: a
 * section, which is a container other items are dragged into. A module wearing
 * it reads as a container with its contents missing — and to somebody who knows
 * the dashboard, as a container they should be able to drop a module into.
 *
 * A module is that dashboard's widget, not its section, so it names itself the
 * way a widget does: its icon and its name inside the box, at the top.
 */
function moduleHead(icon: string, title: string, count?: string, at: string = "top"): string {
  // The pill is drawn outside this box, as a child of the cell: it sits on the
  // border, and this box scrolls, so anything hanging over its edge would be
  // clipped by the very thing that keeps a long list inside its module.
  if (at === "pill" || at === "off") return "";
  return `<header class="module-header">
    <span class="module-icon" aria-hidden="true">${icon}</span>
    <h2>${escapeHTML(title)}</h2>
    ${count !== undefined ? `<span class="count-chip">${escapeHTML(count)}</span>` : ""}
  </header>`;
}

/**
 * One button, not two. The dashboard in this house puts a module's actions
 * behind a single kebab with a tooltip, and two loose icons floating over
 * content is both a different look and one more thing overlapping whatever the
 * module is showing.
 *
 * Written once because a section wears it too: two copies of this would be two
 * menus that drift apart, and the one on the rarer element is the one that
 * would be left behind.
 */
function kebab(id: string, text: Text): string {
  return `<button type="button" class="kebab-btn" data-module-menu="${escapeAttr(id)}"
       title="${escapeAttr(text.t("action.edit"))}"
       aria-label="${escapeAttr(text.t("action.edit"))}"
       aria-haspopup="menu" aria-expanded="false">${icons.dots()}</button>
     <div class="kebab-menu" data-menu-for="${escapeAttr(id)}" role="menu" hidden>
       <button type="button" class="kebab-item" role="menuitem" data-edit-module="${escapeAttr(id)}">
         ${icons.edit(15)}<span>${escapeHTML(text.t("action.edit"))}</span></button>
       <button type="button" class="kebab-item menu-delete" role="menuitem" data-remove-module="${escapeAttr(id)}">
         ${icons.remove(15)}<span>${escapeHTML(text.t("action.remove"))}</span></button>
     </div>`;
}

/** The corner a module is resized from. */
function resizeHandle(id: string): string {
  return `<span class="resize-handle" data-resize="${escapeAttr(id)}" aria-hidden="true"></span>`;
}

/**
 * Was this module pointed at a single thing?
 *
 * What it was ASKED for, not what came back. An installation that happens to
 * have one organisation should not find its list quietly turned into a tile,
 * and then turned back the day it has two.
 */
function showsOne(m: LandingModule): boolean {
  if (!m.scope) return false;
  const deeper = m.scope.includes("/");
  return m.kind === "organisations" ? !deeper : deeper;
}

/** The class that puts a module's header where the operator asked for it. */
function labelClass(at: string): string {
  return at === "bottom" ? "label-bottom" : "";
}

/** The name on the border, for a module whose operator asked for it there. */
function modulePill(title: string, count?: string): string {
  return `<h2 class="section-legend">
    <span>${escapeHTML(title)}</span>
    ${count !== undefined ? `<span class="count-chip">${escapeHTML(count)}</span>` : ""}
  </h2>`;
}

/** The mark in front of a name: two letters of the thing itself. */
export function initials(name: string): string {
  return (name || "?")
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => [...part][0]?.toUpperCase() ?? "")
    .join("");
}

export function organizationList(orgs: OrganizationSummary[], text: Text): string {
  if (orgs.length === 0) return emptyState(text.t("portal.empty"), icons.organization(28));

  return `<ul class="cards">${orgs
    .map(
      (o) => `<li><a class="card" href="/${escapeAttr(o.slug)}">
        <span class="card-mark" aria-hidden="true">${escapeHTML(initials(o.name))}</span>
        <span class="card-body">
          <span class="card-title">${escapeHTML(o.name)}</span>
          <span class="card-sub">/${escapeHTML(o.slug)}</span>
        </span>
        <span class="card-go" aria-hidden="true">${icons.chevron(18)}</span>
      </a></li>`,
    )
    .join("")}</ul>`;
}

/**
 * One organisation, and the boards inside it.
 *
 * What a board takes is said as two small words on the card rather than as a
 * sentence under the name: it is what somebody is choosing between, and a line
 * of prose repeated on every card is a line nobody reads twice.
 */
export function repositoryList(
  org: OrganizationSummary,
  repos: RepositorySummary[],
  text: Text,
): string {
  const header = `<header class="page-head">
    <div class="head-text">
      <p class="crumb"><a href="/">${escapeHTML(text.t("nav.home"))}</a></p>
      <h1>${escapeHTML(org.name)}</h1>
    </div>
  </header>`;

  if (repos.length === 0) {
    return `<div class="page">${header}
      ${emptyState(text.t("portal.empty"), icons.repository(28))}
    </div>`;
  }

  return `<div class="page">${header}
    <section class="section">
      ${sectionHead(text.t("portal.boards"), text.number(repos.length))}
      <ul class="cards">${repos
        .map(
          (r) => `<li><a class="card" href="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}">
            <span class="card-mark" aria-hidden="true">${escapeHTML(initials(r.name))}</span>
            <span class="card-body">
              <span class="card-title">${escapeHTML(r.name)}</span>
              <span class="chips">
                ${r.accepts_bugs ? `<span class="chip">${icons.bug(12)}<span>${escapeHTML(text.t("kind.bug"))}</span></span>` : ""}
                ${r.accepts_ideas ? `<span class="chip">${icons.idea(12)}<span>${escapeHTML(text.t("kind.idea"))}</span></span>` : ""}
              </span>
            </span>
            <span class="card-go" aria-hidden="true">${icons.chevron(18)}</span>
          </a></li>`,
        )
        .join("")}</ul>
    </section>
  </div>`;
}

/**
 * The panel's front page, and the first minute of every operator.
 *
 * It exists because a fresh instance used to say "nothing is set up yet" and
 * offer no way to set anything up: the only path was curl. A screen that names
 * a problem it cannot help with reads as broken rather than as empty.
 */
/**
 * The panel's front page for somebody who is not an administrator.
 *
 * Their world is the places they were given a role in, so that is what it
 * shows: a card each, going straight to the queue, which is the screen they
 * actually came for. It used to be the administrator's page with the numbers
 * missing and an "add an organisation" button that would have been refused —
 * an item you can see and cannot use is worse than one that was never there.
 */
export function memberHome(
  memberships: { scope: string; role: string; organization?: string; repository?: string; name?: string }[],
  text: Text,
): string {
  if (memberships.length === 0) {
    return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.overview"))}</h1>
      ${emptyState(text.t("overview.noRoles"), icons.person(28))}`;
  }

  const cards = memberships
    .map((m) => {
      const href = m.repository
        ? `/${m.organization}/${m.repository}/queue`
        : `/panel/${m.organization}`;
      return `<li class="bento-card">
        <a class="card-link" href="${escapeAttr(href)}">
          <div class="card-head">${m.repository ? icons.repository() : icons.organization()}<h3>${escapeHTML(m.name ?? "")}</h3></div>
          <span class="card-sub"><span class="chip">${escapeHTML(text.t(`role.${m.role}`))}</span></span>
        </a>
      </li>`;
    })
    .join("");

  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.overview"))}</h1>
    <p class="lead">${escapeHTML(text.t("overview.memberLead"))}</p>
    <ul class="bento">${cards}</ul>`;
}


/** The forge choices, in the order somebody is likely to recognise them. */
export function forgeOptions(text: Text): [string, string][] {
  return [
    ["none", text.t("panel.forgeNone")],
    ["github", "GitHub"],
    ["gitlab", "GitLab"],
    ["gitea", "Gitea"],
    ["forgejo", "Forgejo"],
  ];
}


/** One organisation in the panel: its boards, and a form to add another. */
export function panelOrganization(
  org: OrganizationSummary,
  repos: RepositorySummary[],
  text: Text,
): string {
  const list =
    repos.length === 0
      ? `${emptyState(text.t("portal.empty"), icons.repository(28))}`
      : `<ul class="cards">${repos
          .map(
            (r) => `<li><a class="card" href="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}">
              <span class="card-title">${escapeHTML(r.name)}</span>
              <span class="card-sub">/${escapeHTML(org.slug)}/${escapeHTML(r.slug)}</span>
            </a></li>`,
          )
          .join("")}</ul>`;

  const exports =
    repos.length === 0
      ? ""
      : `<p class="exports">${repos
          .map(
            (r) =>
              `<a href="/api/orgs/${escapeAttr(org.slug)}/repos/${escapeAttr(r.slug)}/export"
                  download>${escapeHTML(text.t("panel.export", { repository: r.name }))}</a>`,
          )
          .join(" · ")}</p>`;

  return `
    <p class="crumb"><a href="/panel">${escapeHTML(text.t("nav.panel"))}</a></p>
    <h1>${escapeHTML(org.name)}</h1>
    ${list}
    ${exports}

    <p class="head-actions">
      <a class="button primary" href="/panel/${escapeAttr(org.slug)}/new">${escapeHTML(text.t("panel.newRepository"))}</a>
    </p>`;
}

/**
 * The forge connection, and the channel the team reads.
 *
 * Both were API-only for a while, which meant the panel offered a forge in the
 * organisation form and then abandoned whoever chose one: no way to enter a
 * token, no way to find the webhook address. The same failure as an empty panel
 * that says nothing is configured and offers nothing to configure.
 */
export function connectionForms(
  org: OrganizationSummary & { forge?: string; webhook_path?: string },
  repos: RepositorySummary[],
  /** Where this instance answers, so the address can be shown whole. Passed in
   *  rather than read from `location`: these templates are pure so they can be
   *  tested without a browser, and one reach for a global ends that. */
  origin: string,
  text: Text,
): string {
  const forgeCard =
    !org.forge || org.forge === "none"
      ? ""
      : `<li class="bento-card">
          <div class="card-head">${icons.repository()}<h3>${escapeHTML(text.t("panel.connection"))}</h3></div>
          <span class="card-sub">${escapeHTML(text.t("panel.tokenHelp"))}</span>
          <form class="credentials">
            ${field({ id: "cred-token", name: "token", label: text.t("panel.token"), type: "password", required: true }, text)}
            ${field({ id: "cred-secret", name: "webhook_secret", label: text.t("panel.webhookSecret"), type: "password", required: true, hint: text.t("panel.secretHelp") }, text)}
            <p class="form-error" role="alert" hidden></p>
            <div class="wizard-actions"><span class="spacer"></span>
              <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
          </form>
        </li>`;

  const kinds = [
    ["", text.t("panel.channelNone")],
    ["ntfy", "ntfy"],
    ["gotify", "Gotify"],
    ["webhook", "Webhook"],
    ["smtp", "Email"],
  ];

  const channelCard =
    repos.length === 0
      ? ""
      : `<li class="bento-card">
          <div class="card-head">${icons.warning()}<h3>${escapeHTML(text.t("panel.channel"))}</h3></div>
          <span class="card-sub">${escapeHTML(text.t("panel.channelHelp"))}</span>
          <form class="channel">
            ${choiceField({
              id: "chan-repo",
              name: "repo",
              label: text.t("portal.boards"),
              options: repos.map((r) => [r.slug, r.name] as [string, string]),
            })}
            ${choiceField({
              id: "chan-kind",
              name: "kind",
              label: text.t("panel.channel"),
              options: kinds.map(([v, l]) => [v ?? "", l ?? ""] as [string, string]),
            })}
            <div class="channel-fields" hidden>
              ${field({ id: "chan-url", name: "url", label: text.t("panel.address2"), example: "https://ntfy.sh/acme-feedback" }, text)}
              ${field({ id: "chan-token", name: "token", label: text.t("panel.token"), type: "password" }, text)}
            </div>
            <div class="channel-mail" hidden>
              ${field({ id: "chan-to", name: "to", label: text.t("panel.mailTo"), type: "email", example: "team@example.com" }, text)}
            </div>
            <p class="form-error" role="alert" hidden></p>
            <div class="wizard-actions"><span class="spacer"></span>
              <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
          </form>
        </li>`;

  if (forgeCard === "" && channelCard === "") return "";

  // The webhook address, which the operator has to copy into their forge. It
  // belongs beside the connection rather than at the bottom of a form, and it
  // is the one value on this page somebody needs on a clipboard.
  const address = org.webhook_path
    ? `<li class="bento-card">
        <div class="card-head">${icons.open()}<h3>${escapeHTML(text.t("panel.webhookPath"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("panel.webhookPathHelp"))}</span>
        <code class="fact-value" data-webhook>${escapeHTML(origin + org.webhook_path)}</code>
        ${actionButton("copy", text.t("action.copy"), "data-copy-webhook")}
      </li>`
    : "";

  // Cards like every other in the panel, one under another; each names itself,
  // so no heading over them.
  return `<ul class="settings settings-cards">${forgeCard}${address}${channelCard}</ul>`;
}

/** The team's triage controls on one item: rewrite, reply, and merge a duplicate away. */
export function triageControls(text: Text, current: { title: string; body: string }): string {
  return `
    <section class="triage">
      <h2>${escapeHTML(text.t("panel.rewrite"))}</h2>
      <p class="hint">${escapeHTML(text.t("panel.rewriteHelp"))}</p>
      <form class="rewrite">
        <input name="title" value="${escapeAttr(current.title)}" maxlength="200" required
               aria-label="${escapeAttr(text.t("form.title"))}" />
        <textarea name="body" rows="5" aria-label="${escapeAttr(text.t("form.body"))}">${escapeHTML(current.body)}</textarea>
        <button class="button" type="submit">${escapeHTML(text.t("panel.save"))}</button>
      </form>

      <h2>${escapeHTML(text.t("panel.reply"))}</h2>
      <form class="team-reply">
        <textarea name="body" rows="3" placeholder="${escapeAttr(text.t("item.commentPlaceholder"))}"></textarea>
        <button class="button primary" type="submit">${escapeHTML(text.t("form.send"))}</button>
      </form>

      <h2>${escapeHTML(text.t("panel.merge"))}</h2>
      <p class="hint">${escapeHTML(text.t("panel.mergeHelp"))}</p>
      <form class="merge">
        <input name="into" placeholder="${escapeAttr(text.t("panel.mergePlaceholder"))}" required />
        <button class="button" type="submit">${escapeHTML(text.t("panel.merge"))}</button>
      </form>
    </section>`;
}

export function board(
  org: OrganizationSummary,
  repo: RepositorySummary,
  items: ItemSummary[],
  filters: { kind: string; sort: string; query: string },
  text: Text,
): string {
  const tab = (value: string, label: string) =>
    `<button class="tab${filters.kind === value ? " on" : ""}" data-kind="${escapeAttr(value)}">${escapeHTML(label)}</button>`;

  const tabs = [
    tab("", text.t("board.all")),
    repo.accepts_bugs ? tab("bug", text.t("kind.bug")) : "",
    repo.accepts_ideas ? tab("idea", text.t("kind.idea")) : "",
  ].join("");

  const sorts = `
    <button class="sort${filters.sort !== "votes" ? " on" : ""}" data-sort="new">${escapeHTML(text.t("board.sortNew"))}</button>
    <button class="sort${filters.sort === "votes" ? " on" : ""}" data-sort="votes">${escapeHTML(text.t("board.sortVotes"))}</button>`;

  const report = `/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/new`;

  // An empty board is not a failure, and it is the state every board starts in.
  // It carries the one thing to do about it rather than a grey sentence.
  const list =
    items.length === 0
      ? emptyState(
          text.t("board.empty"),
          icons.queue(28),
          `<a class="button primary with-icon" href="${report}">${icons.add()}<span>${escapeHTML(text.t("board.report"))}</span></a>`,
        )
      : // The list lives inside a section of the board, and the house draws a
        // section as a bordered box whose name rides on its border. The legend
        // is the filter you are looking through and how many it found, which is
        // the one sentence the list cannot say about itself.
        `<section class="section">
          ${sectionHead(
            filters.kind === "bug"
              ? text.t("kind.bug")
              : filters.kind === "idea"
                ? text.t("kind.idea")
                : text.t("board.all"),
            text.number(items.length),
          )}
          <ul class="items">${items.map((i) => itemRow(i, text)).join("")}</ul>
        </section>`;

  return `<div class="page">
    <header class="page-head board-head">
      <div class="head-text">
        <p class="crumb"><a href="/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a></p>
        <h1>${escapeHTML(repo.name)}</h1>
        <span class="chips">
          ${repo.accepts_bugs ? `<span class="chip">${icons.bug(12)}<span>${escapeHTML(text.t("kind.bug"))}</span></span>` : ""}
          ${repo.accepts_ideas ? `<span class="chip">${icons.idea(12)}<span>${escapeHTML(text.t("kind.idea"))}</span></span>` : ""}
        </span>
      </div>
      <div class="head-actions">
        <a class="icon-button" href="/feed/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/atom.xml"
           title="${escapeAttr(text.t("nav.feed"))}" aria-label="${escapeAttr(text.t("nav.feed"))}">${icons.feed(16)}</a>
        <a class="button" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/roadmap">${escapeHTML(text.t("nav.roadmap"))}</a>
        <a class="button primary with-icon" href="${report}">${icons.add()}<span>${escapeHTML(text.t("board.report"))}</span></a>
      </div>
    </header>
    <div class="controls">
      <div class="tabs">${tabs}</div>
      <div class="sorts">${sorts}</div>
      <label class="search-wrap">
        ${icons.search(16)}
        <input class="search" type="search" value="${escapeAttr(filters.query)}"
               placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(text.t("board.search"))}" />
      </label>
    </div>
    ${list}
  </div>`;
}

/**
 * One report in the list.
 *
 * The score leads the row. On an ideas board it is what the list is sorted by
 * and what the eye runs down the column looking for, and it used to be four
 * grey words at the far end of the line, past the state, where nothing lined
 * up with anything. The word stays under the number: a bare 3 beside a title is
 * a number nobody can name, which is why every counter in this house is written
 * as a label and a value.
 *
 * A bug gets the same tile with its mark in it rather than nothing, so that
 * every title on a mixed board starts on the same vertical line. Two hundred
 * people agreeing that something crashes does not make it crash harder, so a
 * bug is not given a count.
 */
function itemRow(item: ItemSummary, text: Text): string {
  const lead =
    item.kind === "idea"
      ? `<span class="score${item.voted ? " voted" : ""}">
           <span class="count">${escapeHTML(text.number(item.votes))}</span>
           <span class="label">${escapeHTML(text.t("item.votesLabel"))}</span>
         </span>`
      : `<span class="score mark kind-bug" aria-hidden="true">${icons.bug(20)}</span>`;

  return `<li><a class="item" href="/i/${escapeAttr(item.public_id)}">
    ${lead}
    <span class="item-main">
      <span class="item-title">${escapeHTML(item.title)}</span>
      <span class="item-meta">
        <span class="kind kind-${escapeAttr(item.kind)}">
          ${item.kind === "bug" ? icons.bug(13) : icons.idea(13)}
          <span class="kind-name">${escapeHTML(text.t(`kind.${item.kind}`))}</span>
        </span>
        <span class="when">${escapeHTML(text.date(item.created_at))}</span>
      </span>
    </span>
    <span class="status status-${escapeAttr(item.status)}">${escapeHTML(text.t(`status.${item.status}`))}</span>
  </a></li>`;
}

/**
 * The roadmap: three columns, and all three are drawn even when empty.
 *
 * Drawing only the columns that have something in them leaves the reader
 * working out which two of the three they are looking at, and makes an empty
 * board look broken rather than quiet.
 */
export function roadmap(
  org: OrganizationSummary,
  repo: RepositorySummary,
  columns: Array<{ column: string; items: ItemSummary[] }>,
  text: Text,
): string {
  return `<div class="page">
    <header class="page-head">
      <div class="head-text">
        <p class="crumb"><a href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}">${escapeHTML(repo.name)}</a></p>
        <h1>${escapeHTML(text.t("nav.roadmap"))}</h1>
        <p class="lead">${escapeHTML(text.t("roadmap.lead"))}</p>
      </div>
    </header>
    <div class="roadmap">
      ${columns
        .map(
          (c) => `<section class="column column-${escapeAttr(c.column)}">
            <header class="column-head">
              <span class="dot" aria-hidden="true"></span>
              <h2>${escapeHTML(text.t(`roadmap.${c.column}`))}</h2>
              <span class="count-chip">${escapeHTML(text.number(c.items.length))}</span>
            </header>
            ${
              c.items.length === 0
                ? `<p class="empty column-empty">${escapeHTML(text.t("overview.nothingYet"))}</p>`
                : `<ul>${c.items
                    .map(
                      (i) => `<li><a href="/i/${escapeAttr(i.public_id)}">
                        <span class="item-title">${escapeHTML(i.title)}</span>
                        <span class="item-meta">
                          <span class="kind kind-${escapeAttr(i.kind)}">
                            ${i.kind === "bug" ? icons.bug(13) : icons.idea(13)}
                            <span class="kind-name">${escapeHTML(text.t(`kind.${i.kind}`))}</span>
                          </span>
                          ${
                            i.kind === "idea"
                              ? `<span class="when">${escapeHTML(text.t("item.votes", { count: text.number(i.votes) }))}</span>`
                              : ""
                          }
                        </span>
                      </a></li>`,
                    )
                    .join("")}</ul>`
            }
          </section>`,
        )
        .join("")}
    </div>
  </div>`;
}

/**
 * The team's bridge to the forge: open an issue, or attach one that exists.
 *
 * Both, because a project with four thousand open issues does not want a four
 * thousand and first — it wants to say "this is already #4213". Offering only
 * the first is what makes this unusable on day one for anybody who has been
 * going for a while.
 */
export function issueControl(
  linked: { number: number; url: string } | null,
  text: Text,
): string {
  if (linked) {
    return `<p class="issue-link">${escapeHTML(text.t("item.issue"))}
      <a href="${escapeAttr(linked.url)}" target="_blank" rel="noopener">#${escapeHTML(String(linked.number))}</a></p>`;
  }

  return `<div class="issue-control">
    <button class="button" data-create-issue>${escapeHTML(text.t("panel.createIssue"))}</button>
    <form class="attach">
      <input name="issue" placeholder="${escapeAttr(text.t("panel.attachPlaceholder"))}"
             aria-label="${escapeAttr(text.t("panel.attachIssue"))}" />
      <button class="button" type="submit">${escapeHTML(text.t("panel.attachIssue"))}</button>
    </form>
  </div>`;
}

/**
 * The decision, on the report itself and in a card of its own.
 *
 * It existed only in the queue, so somebody who opened a waiting report to read
 * it properly — which is the whole point of opening it — then had to go back to
 * a list to act on what they had just read.
 *
 * Its own card rather than three more buttons at the top of the team's tools:
 * this is the one thing on the screen that is not reversible from the same
 * screen, and stacked in with the status select and the issue form it read as
 * one heap of controls. The card's title is the state it is in, which is why
 * the notice above is drawn for the reporter only — the same sentence twice on
 * one page is the sentence nobody reads.
 */
export function moderationCard(text: Text): string {
  return `<section class="bento-card decide">
    <div class="card-head">${icons.queue()}<h3>${escapeHTML(text.t("item.waiting"))}</h3></div>
    <div class="actions">
      <button class="button primary with-icon" data-moderate="published">${icons.check()}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
      <button class="button with-icon" data-moderate="rejected">${icons.close()}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
      <button class="button danger with-icon" data-moderate="spam">${icons.warning()}<span>${escapeHTML(text.t("panel.spam"))}</span></button>
    </div>
  </section>`;
}

/** The team's status control, drawn only for somebody who may use it. */
export function statusControl(
  current: string,
  statuses: string[],
  text: Text,
): string {
  return `<div class="status-control">
    <label for="status-select">${escapeHTML(text.t("panel.status"))}</label>
    <select id="status-select">
      ${statuses
        .map(
          (s) =>
            `<option value="${escapeAttr(s)}"${s === current ? " selected" : ""}>${escapeHTML(text.t(`status.${s}`))}</option>`,
        )
        .join("")}
    </select>
  </div>`;
}

/**
 * The form somebody reports through.
 *
 * It is the narrowest point of the whole product: everything downstream — the
 * queue, the board, the forge issue — is fed by what a stranger is willing to
 * type here once. So it says where it is going and what will happen next, and
 * it asks a bug the two questions that decide whether the report is information
 * or homework: which version, and how to see it.
 *
 * Those two existed everywhere except here. The table stored them, the export
 * carried them, the forge issue printed them under a comment explaining that a
 * bug with no version and no steps is work rather than information — and the
 * form never asked, so every one of them was empty in every report ever made.
 */
export function reportForm(
  org: OrganizationSummary,
  repo: RepositorySummary,
  kind: string,
  text: Text,
): string {
  // A card each, not two words in a row: the choice decides which questions
  // come next, and it is the first thing anybody touches on this page.
  const choice = (value: string, label: string, help: string, icon: string) =>
    `<button type="button" class="choice-card${kind === value ? " on" : ""}" data-choice="${escapeAttr(value)}"
       aria-pressed="${kind === value ? "true" : "false"}">
      ${icon}
      <span class="choice-title">${escapeHTML(label)}</span>
      <span class="choice-sub">${escapeHTML(help)}</span>
    </button>`;

  const choices = [
    repo.accepts_bugs ? choice("bug", text.t("form.kindBug"), text.t("form.kindBugHelp"), icons.bug(20)) : "",
    repo.accepts_ideas ? choice("idea", text.t("form.kindIdea"), text.t("form.kindIdeaHelp"), icons.idea(20)) : "",
  ].join("");

  // Only a bug is asked these, and the block is drawn once and hidden rather
  // than rebuilt on every choice: rebuilding it would throw away whatever had
  // already been typed in it.
  const bugFields = repo.accepts_bugs
    ? `<div class="kind-fields" data-for="bug"${kind === "bug" ? "" : " hidden"}>
        ${field(
          {
            id: "report-version",
            name: "version",
            label: text.t("form.version"),
            hint: text.t("form.versionHelp"),
            example: "1.4.2",
          },
          text,
        )}
        ${areaField({
          id: "report-steps",
          name: "steps",
          label: text.t("form.steps"),
          hint: text.t("form.stepsHelp"),
          example: text.t("form.stepsPlaceholder"),
          rows: 4,
        })}
      </div>`
    : "";

  return `<div class="page narrow">
    <header class="page-head">
      <div class="head-text">
        <p class="crumb"><a href="/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a>
          <span aria-hidden="true">${icons.chevron(12)}</span>
          <a href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}">${escapeHTML(repo.name)}</a></p>
        <h1>${escapeHTML(text.t("board.report"))}</h1>
        <p class="lead">${escapeHTML(text.t("form.lead"))}</p>
      </div>
    </header>

    <form class="report">
      <div class="kind-choices">${choices}</div>

      <!--
        The honeypot. Hidden from people by CSS and from assistive technology by
        aria-hidden and tabindex, so nobody who can see or hear the page is ever
        offered it; something filling in every input it finds will fill it in.
      -->
      <div class="trap" aria-hidden="true">
        <label for="report-website">Website</label>
        <input id="report-website" name="website" type="text" tabindex="-1" autocomplete="off" />
      </div>

      <div class="wizard-card">
        ${field(
          {
            id: "report-title",
            name: "title",
            label: text.t("form.title"),
            required: true,
            example: text.t("form.titlePlaceholder"),
          },
          undefined,
        )}

        <div class="similar" hidden></div>

        ${areaField({
          id: "report-body",
          name: "body",
          label: text.t("form.body"),
          example: text.t("form.bodyPlaceholder"),
          rows: 6,
        })}

        ${bugFields}

        ${
          // Only when there is a relay to notify with. Asking for an address the
          // server cannot use is collecting personal data for nothing.
          repo.can_notify
            ? field(
                {
                  id: "report-email",
                  name: "email",
                  label: text.t("form.email"),
                  type: "email",
                  hint: text.t("form.emailHelp"),
                  autocomplete: "email",
                },
                undefined,
              )
            : ""
        }
      </div>

      <p class="form-error" role="alert" hidden></p>
      <div class="wizard-actions">
        <span class="spacer"></span>
        <button class="button primary" type="submit">${escapeHTML(text.t("form.send"))}</button>
      </div>
    </form>
  </div>`;
}

export function similarItems(items: Array<{ public_id: string; title: string }>, text: Text): string {
  if (items.length === 0) return "";
  return `<p class="similar-head">${escapeHTML(text.t("form.similar"))}</p>
    <ul>${items
      .map((i) => `<li><a href="/i/${escapeAttr(i.public_id)}">${escapeHTML(i.title)}</a></li>`)
      .join("")}</ul>
    <p class="hint">${escapeHTML(text.t("form.similarHelp"))}</p>`;
}

/**
 * Sent, and the link back to it.
 *
 * The link is the whole page: it is the only way the person who just wrote a
 * report — with no account, by design — ever reaches it again. So it sits in a
 * card of its own with the copy button beside it, rather than as the fourth
 * paragraph of a page of prose.
 */
export function submitted(
  sent: { link: string; path: string; org: string; repo: string; board: string },
  text: Text,
): string {
  return `<div class="page narrow">
    <header class="page-head">
      <div class="head-text">
        <p class="crumb">${icons.check(14)}<span>${escapeHTML(text.t("sent.title"))}</span></p>
        <h1>${escapeHTML(text.t("sent.waiting"))}</h1>
      </div>
    </header>
    <section class="bento-card">
      <div class="card-head">${icons.copy()}<h3>${escapeHTML(text.t("sent.copy"))}</h3></div>
      <span class="card-sub">${escapeHTML(text.t("sent.keepLink"))}</span>
      <div class="follow-link">
        <input readonly value="${escapeAttr(sent.link)}" aria-label="${escapeAttr(text.t("sent.copy"))}" />
        <button class="button" data-copy>${escapeHTML(text.t("sent.copy"))}</button>
      </div>
    </section>
    <!--
      The same row the wizard and the form use: the way back on the end the eye
      starts from, the way on at the other one, and the width of the page
      between them. Two buttons side by side at the start of the line give the
      same weight to going back and to going on, and read as one control that
      was cut in half.
    -->
    <div class="wizard-actions">
      <a class="button with-icon back" href="/${escapeAttr(sent.org)}/${escapeAttr(sent.repo)}">
        ${icons.chevron(14)}<span>${escapeHTML(sent.board)}</span>
      </a>
      <span class="spacer"></span>
      <a class="button primary" href="${escapeAttr(sent.path)}">${escapeHTML(text.t("sent.open"))}</a>
    </div>
  </div>`;
}

/**
 * The label a question is shown under, in the reader's language.
 *
 * The answer carries a label of its own and it is stored in English, because it
 * travels into the forge issue, which the team reads — not necessarily in the
 * language the reporter's browser happened to be in. Drawing that stored label
 * here put two English words in the middle of an otherwise Arabic page.
 *
 * So the questions the form asks now are translated, and anything else falls
 * back to what was stored: a question reworded or retired next month still
 * reads the way it was sent, which is the reason the label is kept at all.
 */
// The noun, not the question. The form asks "Which version"; a report shows
// "Version" over the answer, which is what a definition list is for.
const questionKeys: Record<string, string> = { version: "field.version", steps: "field.steps" };

function questionLabel(field: ReportField, text: Text): string {
  const key = questionKeys[field.key];
  return key ? text.t(key) : field.label;
}

/** The answers to the form, drawn where the report is. */
function answers(fields: ReportField[], text: Text): string {
  if (fields.length === 0) return "";
  // The heading sits outside the list. A <dl> may only hold dt/dd pairs, or
  // divs wrapping them — a loose div of text in there is markup that renders
  // and does not parse.
  return `<section class="answers">
    <p class="answers-head">${escapeHTML(text.t("item.answers"))}</p>
    <dl>${fields
      .map(
        (f) => `<div>
          <dt>${escapeHTML(questionLabel(f, text))}</dt>
          <dd>${escapeHTML(f.value)}</dd>
        </div>`,
      )
      .join("")}</dl>
  </section>`;
}

/**
 * One report, in public.
 *
 * This is the page that travels. Every other screen is reached by walking the
 * product; this one arrives in somebody's chat as a bare link, and whoever
 * opens it has never seen the rest. So it says three things the old one did
 * not: which board it belongs to, and a way back to it; what state it is in,
 * in the colour the board uses rather than as one more grey word; and, for a
 * reporter following their own submission, whether it is on the board yet.
 *
 * The vote is a block rather than a button in a row. On an ideas board it is
 * the whole point of the page, and it was a grey rectangle the same weight as
 * "Send".
 */
export function itemView(item: ItemDetail, text: Text): string {
  const board = item.repository;
  const org = board?.organization;
  const crumb = board
    ? `<p class="crumb">${
        org
          ? `<a href="/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a> <span aria-hidden="true">${icons.chevron(12)}</span> `
          : ""
      }<a href="/${escapeAttr(org?.slug ?? "")}/${escapeAttr(board.slug)}">${escapeHTML(board.name)}</a></p>`
    : "";

  const merged = item.merged_into
    ? `<p class="notice">${icons.info()}<a href="/i/${escapeAttr(item.merged_into)}">${escapeHTML(text.t("item.merged"))}</a></p>`
    : "";

  // Waiting to be read, said to the person who wrote it. Somebody on the team
  // is told the same thing by the card carrying the decision, and saying it
  // twice on one page is how a screen stops being read at all. It used to be
  // drawn for anybody holding a follow link, which meant a published report
  // told its own author, forever, that it was still waiting.
  const waiting =
    item.mine && item.moderation && item.moderation !== "published"
      ? `<p class="notice">${icons.queue()}<span>${escapeHTML(text.t("item.waiting"))}</span></p>`
      : "";

  // The way back to their own report, for the person who has it. The link they
  // were given at submission time is this page's address plus the token, and
  // this is the one place they can find it again without a browser history.
  const yours = item.mine
    ? `<section class="bento-card yours">
        <div class="card-head">${icons.person()}<h3>${escapeHTML(text.t("item.yours"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("item.yoursHelp"))}</span>
        <div class="follow-link">
          <input readonly data-follow-link value="" aria-label="${escapeAttr(text.t("sent.copy"))}" />
          ${actionButton("copy", text.t("sent.copy"), "data-copy")}
        </div>
      </section>`
    : "";

  const vote =
    item.kind === "idea"
      ? `<button class="vote-block${item.voted ? " voted" : ""}" data-vote
           aria-pressed="${item.voted ? "true" : "false"}">
           <span class="count">${escapeHTML(text.number(item.votes))}</span>
           <span class="label">${escapeHTML(item.voted ? text.t("item.voted") : text.t("item.vote"))}</span>
         </button>`
      : "";

  // The forge issue, when the team has bridged this one. It is what a reporter
  // came to find out — whether anybody is actually working on it — and it was
  // already in the payload, drawn only for signed-in staff.
  const issue = item.issue
    ? `<a class="tracked" href="${escapeAttr(item.issue.url)}" target="_blank" rel="noopener">
         ${icons.open(14)}<span>${escapeHTML(text.t("item.trackedAs", { number: text.number(item.issue.number) }))}</span>
       </a>`
    : "";

  // Blank lines are how people write a report: what happened, then what they
  // expected, then their version. Rendered as one run-on paragraph, a careful
  // report reads like a worse one than it is.
  const body = item.body
    .split(/\n{2,}/)
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => `<p>${escapeHTML(part)}</p>`)
    .join("");

  const thread =
    item.comments.length === 0
      ? `${emptyState(text.t("item.threadEmpty"), icons.people(28))}`
      : `<ul class="comments">${item.comments
          .map((c) => {
            const who = c.name || text.t(`comment.${c.author}`);
            return `<li class="comment comment-${escapeAttr(c.author)}">
              <p class="who">
                ${avatar(who)}
                <span class="who-name">${escapeHTML(who)}</span>
                ${c.author === "team" ? `<span class="who-badge">${escapeHTML(text.t("comment.team"))}</span>` : ""}
                <span class="when">${escapeHTML(text.date(c.created_at))}</span>
              </p>
              <p class="what">${escapeHTML(c.body)}</p>
            </li>`;
          })
          .join("")}</ul>`;

  return `<div class="page">
    ${crumb}${merged}${waiting}
    <article class="detail">
      <div class="detail-main">
        <p class="meta">
          <span class="kind kind-${escapeAttr(item.kind)}">
            ${item.kind === "bug" ? icons.bug(14) : icons.idea(14)}
            <span>${escapeHTML(text.t(`kind.${item.kind}`))}</span>
          </span>
          ${stateChip(item, text)}
          <span class="when">${escapeHTML(text.date(item.created_at))}</span>
          ${issue}
        </p>
        <h1>${escapeHTML(item.title)}</h1>
        <div class="body">${body}</div>
        ${answers(item.fields ?? [], text)}
      </div>
      ${vote}
    </article>

    <!--
      The team's tools have a container of their own, and it is not inside the
      report. They used to be appended to the article, which made them children
      of its grid: the status select and the attach-an-issue form became a
      second column, and the report itself was squeezed into a strip a few
      words wide. Nothing in the markup looked wrong — it was two correct
      pieces meeting badly, and it only showed up in a screenshot.
    -->
    <section class="bento-card team-tools" hidden>
      <div class="card-head">${icons.people()}<h3>${escapeHTML(text.t("panel.teamTools"))}</h3></div>
      <span class="card-sub">${escapeHTML(text.t("panel.teamToolsHelp"))}</span>
      <div class="tools"></div>
    </section>

    ${yours}

    <section class="thread">
      <h2>${escapeHTML(text.t("item.comments"))}
        <span class="count-chip">${escapeHTML(text.number(item.comments.length))}</span></h2>
      ${thread}
      <form class="comment-form bento-card">
        <label for="comment-body">${escapeHTML(text.t("item.addComment"))}</label>
        <textarea id="comment-body" name="body" rows="3"
                  placeholder="${escapeAttr(text.t("item.commentPlaceholder"))}"></textarea>
        <p class="hint">${escapeHTML(text.t("item.commentHint"))}</p>
        <div class="comment-actions">
          <button class="button primary" type="submit">${escapeHTML(text.t("form.send"))}</button>
          <p class="comment-sent" role="status" hidden>${escapeHTML(text.t("item.commentWaiting"))}</p>
        </div>
      </form>
    </section>
  </div>`;
}

/**
 * Signing in, once the instance has been claimed.
 *
 * Claiming goes through the wizard, so this no longer takes a flag for it: a
 * parameter that nothing passes is a branch nobody tests. It wears the same card
 * as the wizard steps, because a login that looks like a different product is
 * how a page reads as bolted on.
 */
/**
 * Signing in: the first screen of the day for everybody on the team.
 *
 * It was a heading reading "Sign in" over two boxes and a button reading "Sign
 * in", under a bar with a link reading "Sign in" — the same word three times
 * and nothing else on the page. What it says now is where you are: the mark and
 * the name of the installation, which is the thing somebody is signing in to.
 *
 * The card is the only thing on a plain field, centred, with the action full
 * width under it. The eye on the password comes from the service that puts one
 * on every password field in the product.
 */
export function signInForm(text: Text, instance?: { name?: string; logo?: string }): string {
  // One card holding all of it — mark, where you are, the fields, the way in —
  // which is what the setup does and what the reference's own entrance does.
  // The mark and the name used to float above the card with the button hanging
  // below it: three groups on two consecutive screens answering the same
  // question two different ways.
  return `<section class="signin">
    <form class="auth">
      <div class="wizard-card">
        <img class="wizard-mark brand-logo mark-56" src="${escapeAttr(
          instance?.logo && instance.logo.trim() !== "" ? instance.logo : "/logo.svg",
        )}" alt="" width="56" height="56" />
        <header class="signin-head">
          <h1>${escapeHTML(instance?.name || text.t("app.name"))}</h1>
        </header>
        <div class="wizard-fields">
          ${field({ id: "auth-username", name: "username", label: text.t("auth.username"), required: true, autocomplete: "username" }, text)}
          ${field({ id: "auth-password", name: "password", label: text.t("auth.password"), type: "password", required: true, autocomplete: "current-password" }, text)}
        </div>
        <p class="form-error" role="alert" hidden></p>
        <div class="wizard-actions">
          <button class="button primary signin-go" type="submit">${escapeHTML(text.t("nav.signIn"))}</button>
        </div>
      </div>
    </form>
  </section>`;
}

/**
 * The triage queue for one repository.
 *
 * It used to say "Waiting" and nothing else — not which repository, not how
 * many, and with a breadcrumb that pointed at the public page without saying
 * so. Somebody who triages three repositories had no way to tell which one they
 * were looking at.
 *
 * The three actions carry icons and keep their weights apart: publishing is the
 * one that happens most and is the primary, spam is destructive and reads as
 * it, rejecting sits between them.
 */
export interface WaitingComment {
  id: number;
  author: string;
  body: string;
  created_at: number;
  item: { public_id: string; title: string };
  /**
   * Which board it came from. Absent on the queue of one board, where the
   * answer is the page itself; present on the one that spans them, where a
   * decision without it would be taken about a message from nowhere.
   */
  board?: { organization?: string; repository?: string; name?: string };
}

/**
 * The messages waiting, wherever they are being shown.
 *
 * One function for both screens rather than two that drift: the queue of one
 * board and the queue that spans boards differ in a chip, and the day the
 * buttons change they have to change in one place. The row carries
 * data-comment, which is what the handlers bind to.
 */
export function waitingComments(comments: WaitingComment[], text: Text): string {
  if (comments.length === 0) return "";
  return `<h2 class="queue-head">${escapeHTML(text.t("panel.commentsWaiting", { count: comments.length }))}</h2>
    <ul class="queue">${comments
      .map(
        (c) => `<li class="bento-card" data-comment="${escapeAttr(String(c.id))}">
          <p class="meta">
            <span class="who-badge">${escapeHTML(text.t(`comment.${c.author}`))}</span>
            ${
              c.board?.name
                ? `<span class="chip">${escapeHTML(c.board.name)}</span>`
                : ""
            }
            <span class="when">${escapeHTML(text.date(c.created_at))}</span>
          </p>
          <p class="body">${escapeHTML(c.body)}</p>
          <a class="on" href="/i/${escapeAttr(c.item.public_id)}"
             title="${escapeAttr(text.t("panel.commentOn"))}">${icons.open(14)}<span>${escapeHTML(c.item.title)}</span></a>
          <div class="actions">
            <button class="button primary with-icon" data-comment-act="published">${icons.check()}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
            <button class="button ghost with-icon" data-comment-act="rejected">${icons.close()}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
          </div>
        </li>`,
      )
      .join("")}</ul>`;
}

export function queueScreen(
  org: OrganizationSummary,
  repo: RepositorySummary,
  items: ItemSummary[],
  bodies: Map<string, string>,
  fields: Map<string, ReportField[]>,
  comments: WaitingComment[],
  text: Text,
): string {
  const header = `<p class="crumb"><a href="/panel/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a></p>
    <div class="screen-head">
      <h1>${escapeHTML(repo.name)}</h1>
      <a class="button ghost with-icon" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}">
        ${icons.open()}<span>${escapeHTML(text.t("shell.publicBoard"))}</span></a>
    </div>
    ${
      // Said once. With nothing waiting the empty state below says so, and a
      // lead saying it too is the same sentence twice on a page with one line
      // of text on it.
      items.length === 0 && comments.length === 0
        ? ""
        : `<p class="lead">${escapeHTML(text.t("queue.lead", { count: items.length }))}</p>`
    }`;

  // The comments waiting, which is a second queue on the same board and not a
  // second screen: somebody triaging works through what has arrived, and what
  // has arrived is both kinds.
  const commentQueue = waitingComments(comments, text);

  if (items.length === 0) {
    return `${header}${commentQueue}
      ${
        comments.length === 0
          ? emptyState(
              text.t("queue.empty"),
              icons.check(28),
              `<a class="button" href="/panel/reports?board=${escapeAttr(`${org.slug}/${repo.slug}`)}">${escapeHTML(text.t("shell.reports"))}</a>`,
            )
          : ""
      }`;
  }

  return `${header}
    <ul class="queue">${items
      .map(
        (i) => `<li class="bento-card" data-id="${escapeAttr(i.public_id)}">
          <p class="meta">
            <span class="kind kind-${escapeAttr(i.kind)}">${escapeHTML(text.t(`kind.${i.kind}`))}</span>
            <span class="when">${escapeHTML(text.date(i.created_at))}</span>
          </p>
          <h3>${escapeHTML(i.title)}</h3>
          <p class="body">${escapeHTML(bodies.get(i.public_id) ?? "")}</p>
          ${answers(fields.get(i.public_id) ?? [], text)}
          <div class="actions">
            <button class="button primary with-icon" data-act="published">${icons.check()}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
            <button class="button ghost with-icon" data-act="rejected">${icons.close()}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
            <button class="button danger with-icon" data-act="spam">${icons.warning()}<span>${escapeHTML(text.t("panel.spam"))}</span></button>
          </div>
        </li>`,
      )
      .join("")}</ul>
    ${commentQueue}`;
}

/* ------------------------------------------------------------------ wizards
 *
 * Every one of these used to be a form dumped on a page with no order, no
 * examples and no sense of how far along you were. The operator's words: he did
 * not know what went in each box, because the boxes asked for formats and
 * showed none.
 *
 * The field helpers live in wizard.ts and carry a real example each.
 */

/**
 * The picture, as one control: the circle is the button.
 *
 * Lastboard's, and deliberately the same — a label, a separate upload button
 * and a sentence explaining that a picture is optional is three pieces of
 * furniture for something somebody either clicks or ignores. The badge on the
 * corner is what says it can be changed, and it says it in two words.
 *
 * Shared on purpose across the two products: somebody who has set up one
 * should recognise this.
 */
export interface PickerSpec {
  /** Where the click and the clear are wired: "avatar" or "logo". */
  what: "avatar" | "logo";
  /** The current image as a data URL, or empty. */
  current?: string;
  /** The initials to draw when there is none. */
  initials?: string;
  label: string;
  /** Drawn at the size the setup uses, which is bigger than a row's. */
  big?: boolean;
}

/**
 * Choosing an image, everywhere it is chosen.
 *
 * The circle is the button, a badge on the corner says it can be changed, and a
 * small cross clears it — the cross only when there is something to clear,
 * because a control that does nothing is a control somebody presses once and
 * distrusts afterwards.
 *
 * One function for all four places: the account and the installation in the
 * wizard, the profile and the personalisation screen. Three of them used to be
 * a label, an upload button and a remove button in a row; the setup had this;
 * and the difference was invisible until somebody used both.
 */
export function imagePicker(spec: PickerSpec, text: Text): string {
  const id = spec.what === "avatar" ? "avatar-file" : "logo-file";
  const preview =
    spec.current && spec.what === "logo"
      ? `<img class="logo-preview" src="${escapeAttr(spec.current)}" alt="" />`
      : avatar(spec.initials ?? "?", spec.current ?? "", spec.big);

  return `<div class="picker">
    <button type="button" class="avatar-pick" data-pick-${spec.what}
      aria-label="${escapeAttr(spec.label)}">
      ${preview}
      <span class="avatar-pick-badge">${escapeHTML(text.t("action.change"))}</span>
    </button>
    ${
      spec.current
        ? `<button type="button" class="picker-clear" data-clear-${spec.what}
             title="${escapeAttr(text.t("action.remove"))}"
             aria-label="${escapeAttr(text.t("action.remove"))}">${icons.close(12)}</button>`
        : ""
    }
    <!-- A sibling of the button, not a child: a button may not contain
         interactive content, and an input inside it takes the click the button
         forwards, which bubbles back to the button, forever. -->
    <input type="file" accept="image/*" id="${id}" hidden />
  </div>`;
}

function pictureField(text: Text, picture = ""): string {
  return imagePicker(
    { what: "avatar", current: picture, label: text.t("profile.picture"), big: true },
    text,
  );
}

function accountFields(text: Text, picture = ""): string {
  return (
    // The picture first, because it is the one optional thing here and putting
    // it after two required fields makes it look like a third.
    `<div class="field picture-field">${pictureField(text, picture)}</div>` +
    field({
      id: "username",
      name: "username",
      label: text.t("auth.username"),
      required: true,
      autocomplete: "username",
    }, text) +
    field({
      id: "password",
      name: "password",
      label: text.t("auth.password"),
      type: "password",
      required: true,
      autocomplete: "new-password",
      hint: text.t("auth.passwordHelp"),
    }, text) +
    // Confirm password, spelled the way every application spells it. A second
    // box is what stops the first install being locked out by a typo nobody
    // can see, and there is no reset to fall back on here.
    field({
      id: "password-again",
      name: "password_again",
      label: text.t("auth.passwordAgain"),
      type: "password",
      required: true,
      autocomplete: "new-password",
    }, text)
  );
}

/** The first thing the wizard asks, and the one thing every later screen uses. */
function preferenceFields(
  text: Text,
  languages: { code: string; name: string }[],
  language: string,
  theme: string,
): string {
  return (
    choiceField({
      id: "setup-language",
      name: "language",
      label: text.t("shell.language"),
      options: languages.map((l) => [l.code, l.name] as [string, string]),
      selected: language,
    }) +
    `<div class="field">
      <label>${escapeHTML(text.t("shell.theme"))}</label>
      ${themeChoice(theme, text)}
    </div>`
  );
}

export function organizationFields(text: Text, forge: string): string {
  const example = exampleFor(forge === "none" ? "forgejo" : forge);
  return (
    field({
      id: "org-name",
      name: "name",
      label: text.t("panel.name"),
      hint: text.t("panel.nameHelp"),
      example: "Acme",
      required: true,
    }, text) +
    field({
      id: "org-slug",
      name: "slug",
      label: text.t("panel.address"),
      hint: text.t("panel.addressHelp"),
      example: "acme",
      required: true,
      pattern: "[a-z0-9]+(-[a-z0-9]+)*",
    }, text) +
    choiceField({
      id: "org-forge",
      name: "forge",
      label: text.t("panel.forge"),
      options: forgeOptions(text),
      selected: forge,
      hint: text.t("panel.forgeHelp"),
    }) +
    `<div class="forge-fields"${forge === "none" ? " hidden" : ""}>
      ${field({
        id: "org-url",
        name: "base_url",
        label: text.t("panel.forgeURL"),
        hint: text.t("panel.forgeURLHelp"),
        example: example.url,
      }, text)}
      ${field({
        id: "org-owner",
        name: "forge_owner",
        label: text.t("panel.forgeOwner"),
        hint: text.t("panel.forgeOwnerHelp"),
        example: example.owner,
      }, text)}
    </div>`
  );
}

export function repositoryFields(text: Text, forge: string, hasForge: boolean): string {
  const example = exampleFor(forge);
  return (
    field({
      id: "repo-name",
      name: "name",
      label: text.t("panel.name"),
      hint: text.t("panel.nameHelp"),
      example: example.repo,
      required: true,
    }, text) +
    field({
      id: "repo-slug",
      name: "slug",
      label: text.t("panel.address"),
      hint: text.t("panel.addressHelp"),
      example: example.repo.toLowerCase(),
      required: true,
      pattern: "[a-z0-9]+(-[a-z0-9]+)*",
    }, text) +
    (hasForge
      ? field({
          id: "repo-forge",
          name: "forge_repo",
          label: text.t("panel.forgeRepo"),
          hint: text.t("panel.forgeRepoHelp"),
          example: example.repo,
        }, text)
      : "")
  );
}

/** What a board takes. Both on by default: a board that accepts nothing is
 *  refused by the server, and defaulting to off makes that the common mistake. */
export function acceptsFields(text: Text): string {
  // Two checkboxes that say what they turn on, not two pills reading "Bug" and
  // "Idea". This is a settings form: the control people know here is a
  // checkbox with a sentence beside it, and the words are what somebody
  // reporting will be offered rather than the internal name of a kind.
  const box = (name: string, label: string) =>
    `<label class="check">
      <input type="checkbox" name="${escapeAttr(name)}" checked />
      <span>${escapeHTML(label)}</span>
    </label>`;

  return `<div class="field">
    <label>${escapeHTML(text.t("repo.accepts"))}</label>
    <div class="checks">
      ${box("accepts_bugs", text.t("repo.acceptsBugs"))}
      ${box("accepts_ideas", text.t("repo.acceptsIdeas"))}
    </div>
    <p class="hint">${escapeHTML(text.t("repo.acceptsHelp"))}</p>
  </div>`;
}

/**
 * The first run, and it configures the installation — not the work.
 *
 * Language and look first, because everything after them is read in whatever
 * they choose. Then the administrator: there is no sign-up afterwards and no
 * password reset, so the account this step makes is the only way back in. Then
 * what this installation calls itself.
 *
 * Organisations and repositories deliberately are not here. They are the daily
 * work rather than the installation, they are created and deleted for years
 * after this screen is gone, and putting the first one in a wizard implies the
 * shape of everything that follows has to be decided before you have seen the
 * product.
 */
export function setupWizard(
  step: number,
  text: Text,
  prefs: {
    languages: { code: string; name: string }[];
    language: string;
    theme: string;
    picture?: string;
    logo?: string;
  } = {
    languages: [],
    language: "en",
    theme: "system",
  },
): string {
  const names = [
    text.t("setup.stepLanguage"),
    text.t("setup.stepAccount"),
    text.t("setup.stepInstance"),
  ];

  if (step === 1) {
    return wizard(
      {
        step: 1,
        names,
        mark: prefs.logo || "/logo.svg",
        // No supporting sentence. The title says what the step is, and a
        // settings screen that introduces itself is a screen explaining that
        // it exists.
        title: text.t("setup.languageTitle"),
        body: preferenceFields(text, prefs.languages, prefs.language, prefs.theme),
        submit: text.t("wizard.continue"),
        formClass: "setup-preferences",
      },
      text,
    );
  }

  if (step === 2) {
    return wizard(
      {
        step: 2,
        names,
        mark: prefs.logo || "/logo.svg",
        title: text.t("auth.setupTitle"),
          body: accountFields(text, prefs.picture ?? ""),
        submit: text.t("wizard.continue"),
        back: { href: "/setup", label: text.t("wizard.back") },
        formClass: "setup-account",
      },
      text,
    );
  }

  return wizard(
    {
      step: 3,
      names,
      mark: prefs.logo || "/logo.svg",
      // The same yardstick as the other two steps: the title says what the
      // step is, the hint says the one thing that would otherwise be found out
      // by getting it wrong, and there is nothing else.
      title: text.t("setup.instanceTitle"),
      body:
        // The logo first, and as one control, exactly like the picture on the
        // step before. Two identical jobs on two consecutive screens should
        // not be two different arrangements.
        `<div class="field picture-field">${imagePicker(
          { what: "logo", current: prefs.logo, initials: "L", label: text.t("personalization.logo"), big: true },
          text,
        )}</div>` +
        field(
          {
            id: "instance-name",
            name: "name",
            label: text.t("setup.instanceName"),
            hint: text.t("setup.instanceNameHelp"),
          },
          text,
        ),
      submit: text.t("wizard.finish"),
      back: { href: "/setup/account", label: text.t("wizard.back") },
      formClass: "setup-instance",
    },
    text,
  );
}

/** Adding an organisation on its own, once the instance is running. */
export function organizationWizard(step: number, text: Text, forge = "none"): string {
  const names = [text.t("org.stepIdentity"), text.t("org.stepForge")];

  if (step === 1) {
    return wizard(
      {
        step: 1,
        names,
        title: text.t("panel.newOrganization"),
        lead: text.t("setup.orgLead"),
        body: organizationFields(text, forge),
        submit: text.t("wizard.continue"),
        back: { href: "/panel", label: text.t("nav.back") },
        formClass: "setup-organization",
      },
      text,
    );
  }

  const example = exampleFor(forge);
  return wizard(
    {
      step: 2,
      names,
      title: text.t("panel.connection"),
      lead: forge === "none" ? text.t("org.noForgeLead") : text.t("org.forgeLead"),
      body:
        forge === "none"
          ? ""
          : field({
              id: "org-token",
              name: "token",
              label: text.t("panel.token"),
              hint: text.t("panel.tokenHelp") + " " + text.t("org.tokenWhere", { where: example.where, scope: example.scope }),
              type: "password",
              example: "glpat-… / ghp_… / " + example.scope,
            }, text),
      submit: text.t("wizard.finish"),
      back: { href: "/panel", label: text.t("nav.back") },
      formClass: "setup-token",
    },
    text,
  );
}

/** Adding a board to an organisation that already exists. */
export function repositoryWizard(
  org: OrganizationSummary & { forge?: string },
  step: number,
  text: Text,
): string {
  const names = [text.t("org.stepIdentity"), text.t("repo.stepAccepts")];
  const forge = org.forge ?? "none";

  if (step === 1) {
    return wizard(
      {
        step: 1,
        names,
        title: text.t("panel.newRepository"),
        lead: text.t("repo.lead"),
        body: repositoryFields(text, forge, forge !== "none"),
        submit: text.t("wizard.continue"),
        back: { href: `/panel/${escapeAttr(org.slug)}`, label: text.t("nav.back") },
        formClass: "setup-repository-identity",
      },
      text,
    );
  }

  return wizard(
    {
      step: 2,
      names,
      title: text.t("repo.stepAccepts"),
      lead: text.t("repo.acceptsLead"),
      body: acceptsFields(text),
      submit: text.t("wizard.finish"),
      back: { href: `/panel/${escapeAttr(org.slug)}`, label: text.t("nav.back") },
      formClass: "setup-repository-accepts",
    },
    text,
  );
}

/**
 * The end of the first run.
 *
 * It hands over rather than congratulating: the installation is configured and
 * the next thing is an organisation, which is work rather than setup. A last
 * screen that only says "done" leaves somebody looking for the next door.
 */
export function setupDone(text: Text): string {
  return `<section class="wizard">
    <header class="wizard-head">
      <h1>${escapeHTML(text.t("setup.doneTitle"))}</h1>
      <p class="lead">${escapeHTML(text.t("setup.doneLead"))}</p>
    </header>
    <div class="wizard-card">
      <p class="hint">${escapeHTML(text.t("setup.doneNext"))}</p>
    </div>
    <div class="wizard-actions">
      <span class="spacer"></span>
      <a class="button primary" href="/panel/new">${escapeHTML(text.t("panel.newOrganization"))}</a>
    </div>
  </section>`;
}

/* -------------------------------------------------------------- admin shell */

export interface SideOrganization {
  slug: string;
  name: string;
  repos?: { slug: string; name: string }[];
}

export interface ShellSpec {
  /** The path of the page being shown, so exactly one item is current. */
  path: string;
  organizations: SideOrganization[];
  /** Shown at the foot, so it is obvious which account is acting. */
  who: string;
  isAdmin: boolean;
  /** True for anybody who may create an organisation. */
  mayAddOrganization: boolean;
  avatar?: string;
  content: string;
  /** What is waiting, already narrowed by the server to what this account may open. */
  waiting?: { reports: number; messages: number };
}

/**
 * The administration, grouped.
 *
 * One flat list answers "where do I click" and nothing else. Grouped, it also
 * answers "what kind of thing is this" — the daily work, how it looks, your own
 * account, and the server — which is the difference between a menu and an
 * inventory.
 *
 * What each account sees is what it may do. The groups are built from the
 * permissions rather than rendered and then hidden, because an item you can see
 * and cannot use is worse than one that was never there.
 */
export function adminShell(spec: ShellSpec, text: Text): string {
  // A count, or nothing. Zero is nothing: a badge that says 0 is a badge that
  // teaches people to stop looking at badges.
  const badge = (n: number): string =>
    n > 0
      ? `<span class="side-count" aria-label="${escapeAttr(text.t("shell.waitingCount", { count: String(n) }))}">${escapeHTML(String(n))}</span>`
      : "";

  const item = (href: string, label: string, icon: string, extra = "", count = 0): string => {
    const current = spec.path === href || (href !== "/panel" && spec.path.startsWith(href + "/"));
    return `<a class="side-item ${extra}" href="${escapeAttr(href)}"${current ? ' aria-current="page"' : ""}>${icon}<span>${escapeHTML(label)}</span>${badge(count)}</a>`;
  };

  const group = (label: string, items: string): string =>
    items.trim() === "" ? "" : `<p class="side-label">${escapeHTML(label)}</p>${items}`;

  return `<div class="admin">
    <aside class="side">
      <!-- No installation mark here: the bar above carries it. What heads the
           menu is Lastboard's drawer header — who is acting: the picture, the
           name and the role. -->
      <div class="side-head">
        ${avatar(spec.who, spec.avatar)}
        <span class="who">${escapeHTML(spec.who)}</span>
        ${spec.isAdmin ? `<span class="side-role">${escapeHTML(text.t("people.administrator"))}</span>` : ""}
      </div>
      <nav aria-label="${escapeAttr(text.t("nav.panel"))}">
        ${group(
          text.t("shell.groupWork"),
          item("/panel", text.t("shell.overview"), icons.overview()) +
            // Reports are not here any more. What is waiting is a job rather
            // than a place, so it lives on its own full-width screen reached
            // from the bar above — and the count that used to sit here added
            // reports and messages into one number the tabs below it then
            // contradicted.
            item("/panel/organizations", text.t("shell.organizations"), icons.organization()),
        )}
        ${group(
          text.t("shell.groupAccount"),
          item("/panel/account", text.t("shell.profile"), icons.person()) +
            item("/panel/appearance", text.t("shell.appearance"), icons.palette()),
        )}
        ${group(
          text.t("shell.groupSystem"),
          spec.isAdmin
            ? item("/panel/people", text.t("shell.people"), icons.people()) +
                item("/panel/personalization", text.t("shell.personalization"), icons.brush()) +
                item("/panel/backups", text.t("shell.backups"), icons.backup()) +
                // Updates live inside About, as in Lastboard: what is running
                // and whether it is the one to run are one question.
                item("/panel/about", text.t("shell.about"), icons.info())
            : "",
        )}
      </nav>
      <!--
        The way out to what everybody else sees. There was none anywhere in the
        panel: somebody administering this had no link to the pages they were
        administering, and the only route was typing the address.
      -->
      <a class="side-item side-public" href="/">
        ${icons.open()}<span>${escapeHTML(text.t("shell.publicSite"))}</span>
      </a>
      <div class="side-foot">
        <button class="link with-icon" data-signout>${icons.signOut(16)}<span>${escapeHTML(text.t("nav.signOut"))}</span></button>
      </div>
    </aside>
    <main class="admin-main">${spec.content}</main>
  </div>`;
}

/**
 * The mark this installation wears.
 *
 * The operator's logo once they set one, and the product's until then — rather
 * than nothing, which is what every screen showed before: a name in text and an
 * empty space where a mark belongs. The default lives in web/public and is
 * served like any other asset, so it costs one cached request and no bytes in
 * the bundle.
 */
export function brandMark(logo: string | undefined, size = 24): string {
  const src = logo && logo.trim() !== "" ? logo : "/logo.svg";
  // The size is a class, not a style attribute: there are four of them in the
  // whole product, and the policy this page is served under does not allow an
  // inline style. A width attribute would not do — it does not beat a
  // stylesheet, which is what put the size in a variable in the first place.
  return `<img class="brand-logo mark-${size}" src="${escapeAttr(src)}" alt=""
    width="${size}" height="${size}" />`;
}

/** Initials when there is no picture, which is most of the time. */
export function avatar(name: string, src?: string, big = false): string {
  return `<span class="avatar${big ? " big" : ""}" aria-hidden="true">${
    src ? `<img src="${escapeAttr(src)}" alt="" />` : escapeHTML(initials(name))
  }</span>`;
}

/**
 * Every card the overview shows, in the order it shows them.
 *
 * One order, the same for everybody who opens this installation. It used to be
 * arrangeable per account, and two people looking at the same server were then
 * looking at two different screens: nothing could be pointed at by where it is,
 * and a card somebody had hidden made half a conversation about it impossible.
 *
 * The counting is the server's, not the browser's: a portal that has been
 * useful for two years opens as fast as one installed this morning.
 */
export const overviewCards = [
  "reports",
  "waiting",
  "published",
  "votes",
  "organizations",
  "people",
  "activity",
  "kinds",
  "statuses",
  "top",
  "recent",
  "boards",
  "moderation",
] as const;

export type OverviewCard = (typeof overviewCards)[number];

/** One report as a line in a list: enough to recognise it and to go to it. */
export interface Brief {
  public_id: string;
  title: string;
  kind: string;
  status: string;
  /** Absent means published: only the panel's lists carry anything else. */
  moderation?: string;
  votes: number;
  created_at: number;
}

export interface OverviewData {
  organizations: number;
  repositories: number;
  people: number;
  pending: number;
  published: number;
  votes: number;
  kinds: Record<string, number>;
  statuses: Record<string, number>;
  moderation: Record<string, number>;
  last_report: number;
  series: { day: number; reports: number; bugs: number; ideas: number }[];
  top: Brief[];
  recent: Brief[];
  boards: {
    org: string;
    repo: string;
    name: string;
    organization: string;
    published: number;
    pending: number;
    votes: number;
  }[];
}

/**
 * The state of a report as one chip: what it is waiting for, or what it is.
 *
 * A report that has not been published is not open — nobody outside the team
 * can see it — and showing its status there said "Open" beside a row that was
 * sitting in the queue. Until it is published the state is the moderation
 * state; after that it is the status, which is the thing the roadmap and the
 * board are built from.
 */
export function stateChip(
  of: { status: string; moderation?: string },
  text: Text,
): string {
  const waiting = of.moderation !== undefined && of.moderation !== "published";
  const name = waiting ? String(of.moderation) : of.status;
  const label = text.t(waiting ? `moderation.${name}` : `status.${name}`);
  return `<span class="status status-${escapeAttr(name)}">${escapeHTML(label)}</span>`;
}

/** The mark of a kind, in the tile the rest of the product uses for it. */
function kindMark(kind: string, size = 16): string {
  return `<span class="kind-mark kind-${escapeAttr(kind)}" aria-hidden="true">${
    kind === "bug" ? icons.bug(size) : icons.idea(size)
  }</span>`;
}

/** One report as a row of a list on the dashboard. */
function briefRow(b: Brief, text: Text, showVotes = false): string {
  return `<li><a class="brief" href="/i/${escapeAttr(b.public_id)}">
    ${
      showVotes
        ? `<span class="score small"><span class="count">${escapeHTML(text.number(b.votes))}</span></span>`
        : kindMark(b.kind)
    }
    <span class="brief-title">${escapeHTML(b.title)}</span>
    ${stateChip(b, text)}
  </a></li>`;
}

/**
 * The front page of the panel, the same one for everybody on the team.
 *
 * It was a wall of tiles each holding one number, and a number on its own
 * answers "how many" and nothing else: not whether it is getting busier, not
 * what people are actually asking for, not which board the work is in, and not
 * where to go next. Every tile is a way into the listing it counts, and beside
 * them are the three things the numbers cannot say — the last thirty days, the
 * most wanted ideas, and the newest reports.
 *
 * There is no arranging it. A dashboard that every account rearranges is one
 * nobody can talk about: "the third card" and "the one at the top" stop meaning
 * anything, and a card somebody hid is a screen the person beside them cannot
 * see. One order, decided here.
 */
export function overviewScreen(data: OverviewData, text: Text): string {
  const number = (value: number) => escapeHTML(text.number(value));

  const stat = (id: string, value: number, label: string, icon: string, href?: string) => {
    const inner = `<span class="value">${number(value)}</span>
      <span class="label">${icon}${escapeHTML(label)}</span>`;
    return card(
      id,
      href
        ? `<a class="stat" href="${escapeAttr(href)}">${inner}<span class="stat-go" aria-hidden="true">${icons.chevron(14)}</span></a>`
        : `<div class="stat">${inner}</div>`,
      "narrow",
    );
  };

  const bars = (id: string, title: string, counts: Record<string, number>, label: (key: string) => string) => {
    const entries = Object.entries(counts).filter(([, n]) => n > 0);
    const body =
      entries.length === 0
        ? `<p class="hint">${escapeHTML(text.t("overview.nothingYet"))}</p>`
        : `<div class="bars">${entries
            .sort((a, b) => b[1] - a[1])
            .map(([key, n]) => {
              const most = Math.max(...entries.map(([, count]) => count));
              return `<div class="bar-row hue-${escapeAttr(key)}">
                <span>${escapeHTML(label(key))}</span>
                <span class="bar-track"><span class="bar-fill" data-fill="${Math.round((n / most) * 20)}"></span></span>
                <span class="count">${number(n)}</span>
              </div>`;
            })
            .join("")}</div>`;
    return card(id, `<h3>${escapeHTML(title)}</h3>${body}`, "wide");
  };

  function card(id: string, body: string, size: string): string {
    return `<li class="dash-card ${size}" data-card="${escapeAttr(id)}">${body}</li>`;
  }

  const total = Object.values(data.kinds).reduce((a, b) => a + b, 0);
  const series = data.series ?? [];
  const arrived = series.reduce((sum, day) => sum + day.reports, 0);

  // The chart: thirty days, ends included, with the first and the last day
  // named under it. An empty installation gets a flat line rather than an empty
  // box — the shape of "nothing yet" is still the shape of the chart.
  const activity = card(
    "activity",
    `<h3>${escapeHTML(text.t("overview.activity"))}
       <span class="count-chip">${number(arrived)}</span></h3>
     <div class="chart-wrap">
       ${areaChart(
         series.map((day) => day.reports),
         {
           width: 640,
           height: 120,
           from: series.length > 0 ? text.date(series[0]!.day) : "",
           to: series.length > 0 ? text.date(series[series.length - 1]!.day) : "",
           meta: series.map((day) => ({
             day: day.day,
             bug: day.bugs ?? 0,
             idea: day.ideas ?? 0,
           })),
         },
       )}
     </div>`,
    "full",
  );

  const kinds = card(
    "kinds",
    `<h3>${escapeHTML(text.t("overview.byKind"))}</h3>
     <div class="donut-row">
       ${donut(
         [
           { value: data.kinds.idea ?? 0, className: "arc-idea" },
           { value: data.kinds.bug ?? 0, className: "arc-bug" },
         ],
         `<span class="donut-value">${number(total)}</span>`,
       )}
       <ul class="legend">
         <li><span class="swatch swatch-idea"></span>${escapeHTML(text.t("kind.idea"))}
           <span class="count">${number(data.kinds.idea ?? 0)}</span></li>
         <li><span class="swatch swatch-bug"></span>${escapeHTML(text.t("kind.bug"))}
           <span class="count">${number(data.kinds.bug ?? 0)}</span></li>
       </ul>
     </div>`,
    "wide",
  );

  const list = (id: string, title: string, rows: Brief[], votes: boolean) =>
    card(
      id,
      `<h3>${escapeHTML(title)}</h3>
       ${
         rows.length === 0
           ? `<p class="hint">${escapeHTML(text.t("overview.nothingYet"))}</p>`
           : `<ul class="briefs">${rows.map((b) => briefRow(b, text, votes)).join("")}</ul>`
       }`,
      "wide",
    );

  // The boards, busiest first, each a way into its own share of the work. What
  // is waiting is the number somebody acts on, so it wears the colour of a
  // thing that is waiting rather than being the third grey number in a row.
  const boards = card(
    "boards",
    `<h3>${escapeHTML(text.t("portal.boards"))}
       <span class="count-chip">${number(data.repositories)}</span></h3>
     ${
       (data.boards ?? []).length === 0
         ? `<p class="hint">${escapeHTML(text.t("overview.nothingYet"))}</p>`
         : `<ul class="board-rows">${data.boards
             .map(
               (b) => `<li><a href="/panel/reports?board=${escapeAttr(`${b.org}/${b.repo}`)}">
                 <span class="board-name">
                   <span class="card-title">${escapeHTML(b.name)}</span>
                   <span class="card-sub">${escapeHTML(b.organization)}</span>
                 </span>
                 <span class="board-counts">
                   ${
                     b.pending > 0
                       ? `<span class="status status-planned">${escapeHTML(text.t("panel.queue"))}
                            <span class="count">${number(b.pending)}</span></span>`
                       : ""
                   }
                   <span class="chip">${icons.check(12)}<span>${number(b.published)}</span></span>
                   <span class="chip">${icons.idea(12)}<span>${number(b.votes)}</span></span>
                 </span>
               </a></li>`,
             )
             .join("")}</ul>`
     }`,
    "wide",
  );

  const all: Record<OverviewCard, string> = {
    reports: stat("reports", total, text.t("overview.reports"), icons.repository(), "/panel/reports"),
    waiting: stat("waiting", data.pending, text.t("panel.queue"), icons.queue(), "/panel/reports?moderation=pending"),
    published: stat("published", data.published, text.t("overview.published"), icons.check(), "/panel/reports?moderation=published"),
    votes: stat("votes", data.votes, text.t("overview.votes"), icons.idea(), "/panel/reports?kind=idea&sort=votes"),
    organizations: stat("organizations", data.organizations, text.t("shell.organizations"), icons.organization(), "/panel/organizations"),
    people: stat("people", data.people, text.t("shell.people"), icons.people(), "/panel/people"),
    activity,
    kinds,
    statuses: bars("statuses", text.t("overview.byStatus"), data.statuses, (k) => text.t(`status.${k}`)),
    moderation: bars("moderation", text.t("overview.byModeration"), data.moderation, (k) => text.t(`moderation.${k}`)),
    top: list("top", text.t("board.sortVotes"), data.top ?? [], true),
    recent: list("recent", text.t("overview.recent"), data.recent ?? [], false),
    boards,
  };

  return `<div class="screen-head">
      <h1 class="visually-hidden">${escapeHTML(text.t("shell.overview"))}</h1>
    </div>
    ${total === 0 ? `<p class="lead">${escapeHTML(text.t("overview.emptyLead"))}</p>` : ""}

    <ul class="dashboard">
      ${overviewCards.map((name) => all[name]).join("")}
    </ul>`;
}

/**
 * Everything this account may triage, wherever it lives.
 *
 * The panel had a queue per board and a public page per board, and nothing
 * else: somebody looking after four repositories had no screen that answered
 * "what is waiting for me", and no way to find a report that had already been
 * published — which is most of them, and the ones people write in asking about.
 *
 * What it shows is decided by the server from the roles this account holds, so
 * a collaborator with one repository sees one repository here. The filters are
 * in the address, which is what makes a filtered view something you can send to
 * somebody else.
 */
export interface ReportRow extends Brief {
  moderation: string;
  /** Already sent by /api/reports; the row needs it to be decidable at a
      glance, which is the whole point of the review screen. */
  body?: string;
  board?: { org: string; repo: string; name: string; organization: string };
}

export interface ReportsData {
  items: ReportRow[];
  /**
   * The messages waiting across every board this account may triage. Here
   * rather than on a screen of their own: somebody working through what has
   * arrived does not care which of the two kinds it is, and a second screen is
   * a second place to remember to look.
   */
  comments?: WaitingComment[];
  boards: { org: string; repo: string; name: string; organization: string }[];
  counts: {
    moderation: Record<string, number>;
    status: Record<string, number>;
    kind: Record<string, number>;
  };
}

export interface ReportFilters {
  moderation: string;
  kind: string;
  status: string;
  board: string;
  sort: string;
  q: string;
}

/** Every status either kind can be in, in the order the work goes through. */
const everyStatus = [
  "new",
  "confirmed",
  "planned",
  "in_progress",
  "in_forge",
  "done",
  "closed",
  "declined",
  "not_reproducible",
];

/** What the review screen is showing, all of it read from the address. */
export interface ReviewView {
  mode: "pending" | "all";
  chosen: string;
  /** Only the archive narrows; what is waiting is never filtered. */
  filters: ReportFilters;
  /** Whether a further page may exist past what is drawn. */
  more: boolean;
  /** Seconds since the epoch, given rather than read so the rows can be tested. */
  now: number;
}

/**
 * Reviewing what is waiting: one screen, two panes, one job.
 *
 * It replaces a list that did three jobs at once — decide what is pending,
 * search the archive, and moderate messages — with the controls of the second
 * stacked on top of the first. Measured on that screen with six messages
 * waiting: the reports visible without scrolling were **none**, on a screen
 * titled Reports.
 *
 * The shape is the house's, not an invention and not a mail client: it is
 * Lastboard's MonitorPage — a list of 545px capped at 52% with its border, the
 * detail beside it, starting below the top bar because the bar stays (the icon
 * that opens this is also the way out). The rows are the menu's recipe, which
 * is what this product already uses for a chosen row.
 *
 * Two rules the old screen broke and this one keeps:
 *
 *   - **One counter.** What the bar says is what is in here. The badge used to
 *     add reports and messages together while the tabs below counted only
 *     reports, so the same screen showed 19, 16 and 13.
 *   - **A filter narrows, it never reveals.** The messages used to vanish the
 *     moment any report filter was touched. That is why the filters live only
 *     in the archive: what is waiting is a queue to empty, and a queue you can
 *     narrow is a queue where something waits unseen.
 */
export function reviewScreen(data: ReportsData, view: ReviewView, text: Text): string {
  const total = (data.items ?? []).length + (data.comments ?? []).length;
  return `<div class="review">
    <div class="rev-head">
      <span class="rev-where">${icons.queue(15)}<span>${escapeHTML(text.t("review.title"))}</span></span>
      <div class="rev-modes">
        <button class="tab${view.mode === "pending" ? " on" : ""}" data-mode="pending">
          ${escapeHTML(text.t("review.pending"))}
          ${
            // The waiting count stays on its tab while the archive is open:
            // it is the number the bar shows, and the two never disagree.
            view.mode === "pending"
              ? `<span class="count-chip">${escapeHTML(text.number(total))}</span>`
              : ""
          }</button>
        <button class="tab${view.mode === "all" ? " on" : ""}" data-mode="all">${escapeHTML(text.t("board.all"))}</button>
      </div>
      <span class="rev-spacer"></span>
      <button type="button" class="icon-button" data-leave
        title="${escapeAttr(text.t("action.close"))}"
        aria-label="${escapeAttr(text.t("action.close"))}">${icons.close(18)}</button>
    </div>
    <div class="rev-body">
      <aside class="rev-side">
        ${view.mode === "all" ? reviewFilters(data, view.filters, text) : ""}
        <div class="rev-list" data-list>${reviewList(data, view, text)}</div>
      </aside>
      <div class="rev-veil" data-shut></div>
      <section class="rev-detail">
        <button type="button" class="rev-shut" data-shut
          title="${escapeAttr(text.t("action.close"))}"
          aria-label="${escapeAttr(text.t("action.close"))}">${icons.close(18)}</button>
        <div data-detail></div>
      </section>
    </div>
    <div class="rev-foot">
      <span><kbd>↑</kbd><kbd>↓</kbd> ${escapeHTML(text.t("review.keyMove"))}</span>
      <span><kbd>P</kbd> ${escapeHTML(text.t("panel.approve"))}</span>
      <span><kbd>R</kbd> ${escapeHTML(text.t("panel.reject"))}</span>
      <span><kbd>S</kbd> ${escapeHTML(text.t("panel.spam"))}</span>
      <span><kbd>/</kbd> ${escapeHTML(text.t("board.search"))}</span>
      <span><kbd>Esc</kbd> ${escapeHTML(text.t("review.keyLeave"))}</span>
    </div>
  </div>`;
}

/**
 * The archive's controls: a search box and four narrowings.
 *
 * The search box is Lastboard's, from the head of that page's list. The four
 * selects are what the screen this replaced offered as tabs and selects on a
 * row of their own; in a column of 545px they have to be compact, and a select
 * carries its own label as the first option, so nothing is spent on captions.
 * An option that would find nothing is not offered.
 */
function reviewFilters(data: ReportsData, filters: ReportFilters, text: Text): string {
  const counts = data.counts ?? { moderation: {}, status: {}, kind: {} };
  const option = (value: string, label: string, selected: boolean) =>
    `<option value="${escapeAttr(value)}"${selected ? " selected" : ""}>${escapeHTML(label)}</option>`;
  const choices = (
    name: keyof ReportFilters,
    any: string,
    values: { value: string; label: string; count?: number }[],
  ) => {
    const chosen = filters[name];
    return `<select data-f="${escapeAttr(name)}" aria-label="${escapeAttr(any)}"
        class="${chosen ? "set" : ""}">
      ${option("", any, chosen === "")}
      ${values
        .filter((v) => v.count === undefined || v.count > 0 || v.value === chosen)
        .map((v) => option(v.value, v.label, v.value === chosen))
        .join("")}
    </select>`;
  };

  const kind = choices("kind", text.t("review.anyKind"), [
    { value: "bug", label: text.t("kind.bug"), count: counts.kind.bug ?? 0 },
    { value: "idea", label: text.t("kind.idea"), count: counts.kind.idea ?? 0 },
  ]);
  const moderation = choices(
    "moderation",
    text.t("review.anyModeration"),
    ["pending", "published", "rejected", "spam"].map((m) => ({
      value: m,
      label: text.t(`moderation.${m}`),
      count: counts.moderation[m] ?? 0,
    })),
  );
  const status = choices(
    "status",
    text.t("review.anyStatus"),
    everyStatus.map((s) => ({ value: s, label: text.t(`status.${s}`), count: counts.status[s] ?? 0 })),
  );
  const board =
    data.boards.length < 2
      ? ""
      : choices(
          "board",
          text.t("review.anyBoard"),
          data.boards.map((b) => ({ value: `${b.org}/${b.repo}`, label: `${b.organization} / ${b.name}` })),
        );

  return `<div class="rev-filter">
    <label class="rev-search">
      ${icons.search(14)}
      <input type="search" data-q value="${escapeAttr(filters.q)}"
        placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(text.t("board.search"))}" />
    </label>
    <div class="rev-narrow">${kind}${moderation}${status}${board}</div>
  </div>`;
}

/**
 * The rows, on their own so a further page can be drawn without the screen.
 */
export function reviewList(data: ReportsData, view: ReviewView, text: Text): string {
  const rows = data.items ?? [];
  const waiting = data.comments ?? [];
  const chosen = view.chosen;

  // Grouped by board, because moderating is already context-switching and
  // jumping between projects on every row makes it worse.
  const boards = new Map<string, { name: string; rows: ReportRow[] }>();
  for (const r of rows) {
    const key = r.board ? `${r.board.organization} / ${r.board.name}` : text.t("portal.boards");
    if (!boards.has(key)) boards.set(key, { name: key, rows: [] });
    boards.get(key)!.rows.push(r);
  }

  // Messages hang under the report they are about: six comments on one thing
  // are one context, not six unrelated rows.
  const under = new Map<string, WaitingComment[]>();
  for (const c of waiting) {
    const key = c.item?.public_id ?? "";
    if (!under.has(key)) under.set(key, []);
    under.get(key)!.push(c);
  }

  const preview = (body: string) =>
    `<p class="rev-p">${escapeHTML(body.replace(/\s+/g, " ").trim().slice(0, 180))}</p>`;

  // How long it has been waiting, short, with the date in the title for
  // whoever needs the day. A queue is read for which has waited longest, and
  // "23 Sept 2026" makes you do the subtraction in a column a third as wide.
  const when = (at: number) =>
    `<span class="rev-w" title="${escapeAttr(text.date(at))}">${escapeHTML(
      text.age(Math.max(0, view.now - at)),
    )}</span>`;

  const commentRow = (c: WaitingComment, onItsOwn: boolean) =>
    `<button type="button" class="rev-row${onItsOwn ? "" : " child"}${
      `c:${c.id}` === chosen ? " on" : ""
    }" data-pick="c:${escapeAttr(String(c.id))}" data-what="comment">
      <span class="rev-dot msg"></span>
      <span class="rev-main">
        <span class="rev-t">${escapeHTML(
          onItsOwn && c.item?.title ? c.item.title : text.t("review.message"),
        )}</span>
        ${preview(c.body ?? "")}
      </span>
      ${when(c.created_at)}
    </button>`;

  const reportRow = (r: ReportRow) => {
    const kids = under.get(r.public_id) ?? [];
    return `<button type="button" class="rev-row${r.public_id === chosen ? " on" : ""}${
      // In the archive, what was turned away is dimmed rather than hidden:
      // it is what somebody searches the archive for, and it should still
      // read as not part of the product's public face.
      r.moderation === "rejected" || r.moderation === "spam" ? " out" : ""
    }"
        data-pick="${escapeAttr(r.public_id)}" data-what="report">
        <span class="rev-dot ${escapeAttr(r.kind)}"></span>
        <span class="rev-main">
          <span class="rev-t">${escapeHTML(r.title)}</span>
          ${preview(r.body ?? "")}
        </span>
        ${when(r.created_at)}
      </button>
      ${kids.map((c) => commentRow(c, false)).join("")}`;
  };

  // A message whose report is not in this list — because the report was dealt
  // with weeks ago and only the message is waiting — still has to be drawn.
  // Counting something the list does not show is the fault this screen exists
  // to fix: the old one said 19 in the bar, 16 on one tab and 13 on the next.
  const shown = new Set(rows.map((r) => r.public_id));
  const orphans = waiting.filter((c) => !shown.has(c.item?.public_id ?? ""));

  const list =
    [...boards.values()]
      .map(
        (b) => `<p class="rev-group">${escapeHTML(b.name)}
        <span class="count-chip">${escapeHTML(text.number(b.rows.length))}</span></p>
      ${b.rows.map(reportRow).join("")}`,
      )
      .join("") +
    (orphans.length === 0
      ? ""
      : `<p class="rev-group">${escapeHTML(text.t("review.messages"))}
          <span class="count-chip">${escapeHTML(text.number(orphans.length))}</span></p>
        ${orphans.map((c) => commentRow(c, true)).join("")}`);

  if (rows.length + waiting.length === 0) {
    const narrowed = view.mode === "all" && Object.values(view.filters).some((v) => v !== "");
    return narrowed
      ? `<div class="rev-empty">${emptyState(text.t("review.nothingMatches"), icons.search(28))}
           <p class="rev-unfilter"><button type="button" class="button small" data-unfilter>${escapeHTML(
             text.t("review.clearFilters"),
           )}</button></p></div>`
      : `<div class="rev-empty">${emptyState(text.t("review.clear"), icons.check(28))}</div>`;
  }

  return (
    list +
    (view.more
      ? `<p class="rev-more"><button type="button" class="button small" data-more>${escapeHTML(
          text.t("review.more"),
        )}</button></p>`
      : "")
  );
}

/**
 * What the right-hand pane shows for whatever is selected.
 *
 * A report or a message: the list is deliberately heterogeneous — for whoever
 * is moderating there is one job, and which of the two kinds a row is is a
 * detail of the row. So this takes either, and both end in the same three
 * decisions taken with the same keys.
 */
export function reviewDetail(
  row: ReportRow | null,
  fields: ReportField[],
  text: Text,
  comment?: WaitingComment | null,
): string {
  if (comment) {
    return `<div class="rev-meta">
        <span class="chip">${icons.comment(12)}<span>${escapeHTML(text.t("review.message"))}</span></span>
        ${comment.board?.name ? `<span class="chip">${icons.repository(12)}<span>${escapeHTML(comment.board.name)}</span></span>` : ""}
        <span class="rev-when">${escapeHTML(text.date(comment.created_at))} · ${escapeHTML(comment.author)}</span>
      </div>
      <h1 class="rev-title">${escapeHTML(comment.body)}</h1>
      ${
        // The report it is about, above nothing else: a message read without
        // knowing what it answers is a decision taken blind. On the old screen
        // this sat underneath, so you read the comment first and found out
        // afterwards.
        comment.item?.public_id
          ? `<p class="rev-onwhat">${escapeHTML(text.t("review.onReport"))}
               <a href="/i/${escapeAttr(comment.item.public_id)}">${escapeHTML(comment.item.title)}</a></p>`
          : ""
      }
      <div class="rev-actions">
        <button class="button primary with-icon" data-act="published">${icons.check(14)}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
        <button class="button with-icon" data-act="rejected">${icons.close(14)}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
      </div>`;
  }
  if (!row) return `<p class="rev-nothing">${escapeHTML(text.t("review.pickOne"))}</p>`;
  return `<div class="rev-meta">
      <span class="kind kind-${escapeAttr(row.kind)}">
        ${row.kind === "bug" ? icons.bug(13) : icons.idea(13)}
        <span class="kind-name">${escapeHTML(text.t(`kind.${row.kind}`))}</span>
      </span>
      ${row.board ? `<span class="chip">${icons.repository(12)}<span>${escapeHTML(row.board.name)}</span></span>` : ""}
      <span class="rev-when">${escapeHTML(text.date(row.created_at))}</span>
    </div>
    <h1 class="rev-title">${escapeHTML(row.title)}</h1>
    <div class="rev-full">${escapeHTML(row.body ?? "").replace(/\n/g, "<br />")}</div>
    ${answers(fields, text)}
    ${
      row.moderation === "pending"
        ? `<div class="rev-actions">
             <button class="button primary with-icon" data-act="published">${icons.check(14)}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
             <button class="button with-icon" data-act="rejected">${icons.close(14)}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
             <button class="button danger with-icon" data-act="spam">${icons.warning(14)}<span>${escapeHTML(text.t("panel.spam"))}</span></button>
           </div>`
        : stateChip(row, text)
    }`;
}

/**
 * A card of Lastboard's settings panel.
 *
 * Its recipe, which every panel screen now wears: no icon, the card's name in
 * small quiet capitals, what it is for underneath, then its contents. An
 * action that belongs to the whole card — adding a user, adding a repository
 * — sits at the end of the title's line, as "+ Add user" does there.
 */
function settingsCard(title: string, body: string, opts: { lead?: string; action?: string; tag?: string } = {}): string {
  return `<section class="bento-card settings-card">
    <div class="card-top"><h3>${escapeHTML(title)}</h3>${opts.action ?? ""}</div>
    ${opts.lead ? `<p class="card-sub">${escapeHTML(opts.lead)}</p>` : ""}
    ${body}
  </section>`;
}

/**
 * One thing in a list, as Lastboard lists its users: a row with what it is on
 * one side and what can be done to it on the other.
 */
function settingsItem(spec: {
  lead: string;
  name: string;
  sub?: string;
  tags?: string;
  href?: string;
  actions?: string;
  attrs?: string;
}): string {
  const main = `${spec.lead}<span class="si-text"><span class="si-name">${escapeHTML(spec.name)}</span>${
    spec.sub ? `<span class="si-sub">${escapeHTML(spec.sub)}</span>` : ""
  }</span>${spec.tags ? `<span class="si-tags">${spec.tags}</span>` : ""}`;
  return `<li class="settings-item" ${spec.attrs ?? ""}>
    ${spec.href ? `<a class="si-main" href="${escapeAttr(spec.href)}">${main}</a>` : `<div class="si-main">${main}</div>`}
    ${spec.actions ? `<div class="si-actions">${spec.actions}</div>` : ""}
  </li>`;
}

export function profileScreen(
  me: { username: string; name?: string; avatar?: string; admin: boolean },
  text: Text,
): string {
  // Lastboard's "My profile" and "System password", card for card: the
  // picture, the name and the role; the name you appear under, with its save
  // button on the same line; and the password, closed by one button aligned
  // with the fields rather than stretched across the card.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.profile"))}</h1>
    <div class="settings">
      ${settingsCard(
        text.t("shell.profile"),
        `<div class="profile-head">
          ${imagePicker(
            { what: "avatar", current: me.avatar, initials: me.name || me.username, label: text.t("profile.picture"), big: true },
            text,
          )}
          <div class="profile-who">
            <span class="profile-name">${escapeHTML(me.name || me.username)}</span>
            ${me.admin ? `<span class="profile-role">${escapeHTML(text.t("people.administrator"))}</span>` : ""}
          </div>
        </div>
        <form class="profile-name">
          <div class="field">
            <label for="display-name">${escapeHTML(text.t("profile.name"))}</label>
            <div class="input-row">
              <input type="text" id="display-name" name="display_name" value="${escapeAttr(me.name ?? "")}" placeholder="Alex" />
              <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button>
            </div>
            <p class="hint">${escapeHTML(text.t("profile.nameHelp"))}</p>
          </div>
          <p class="form-error" role="alert" hidden></p>
        </form>`,
      )}
      ${settingsCard(
        text.t("profile.password"),
        `<form class="profile-password">
          ${field({ id: "current-password", name: "current", label: text.t("profile.current"), type: "password", required: true, autocomplete: "current-password" }, text)}
          ${field({ id: "new-password", name: "new", label: text.t("profile.new"), type: "password", required: true, autocomplete: "new-password" }, text)}
          ${field({ id: "new-password-again", name: "again", label: text.t("auth.passwordAgain"), type: "password", required: true, autocomplete: "new-password" }, text)}
          <p class="form-error" role="alert" hidden></p>
          <div class="settings-actions">
            <button class="button primary" type="submit">${escapeHTML(text.t("profile.change"))}</button></div>
        </form>`,
        { lead: text.t("profile.passwordHelp") },
      )}
    </div>`;
}

/** Language and look, which belong to the reader rather than to the server. */
export function appearanceScreen(
  languages: { code: string; name: string }[],
  language: string,
  theme: string,
  text: Text,
): string {
  // Lastboard's "Look & feel": one card, a label over each control, and the
  // theme as its segmented control rather than three large tiles.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.appearance"))}</h1>
    <div class="settings">
      ${settingsCard(
        text.t("shell.appearance"),
        `${choiceField({
          id: "appearance-language",
          name: "language",
          label: text.t("shell.language"),
          options: languages.map((l) => [l.code, l.name] as [string, string]),
          selected: language,
        })}
        <div class="field">
          <label>${escapeHTML(text.t("shell.theme"))}</label>
          ${themeChoice(theme, text)}
        </div>`,
        { lead: text.t("appearance.lead") },
      )}
    </div>`;
}

function themeChoice(theme: string, text: Text): string {
  return `<div class="theme-choice" role="group" aria-label="${escapeAttr(text.t("shell.theme"))}">
    ${(["light", "dark", "system"] as const)
      .map(
        (value) =>
          `<button type="button" data-theme-choice="${value}" aria-pressed="${theme === value}">${
            value === "light" ? icons.sun(20) : value === "dark" ? icons.moon(20) : icons.monitor(20)
          }<span>${escapeHTML(
            text.t(
              value === "system"
                ? "shell.themeSystem"
                : value === "light"
                  ? "shell.themeLight"
                  : "shell.themeDark",
            ),
          )}</span></button>`,
      )
      .join("")}
  </div>`;
}

/** Everything about this installation that an operator chose rather than typed. */
export function personalizationScreen(
  instance: { name: string; logo: string; accent: string },
  text: Text,
): string {
  // Lastboard's "Logo & panel name": the mark and the name side by side, as one
  // thing — they are what the header shows together — and the accent below.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.personalization"))}</h1>
    <form class="instance-settings settings">
      ${settingsCard(
        text.t("shell.personalization"),
        `<div class="field">
          <label for="instance-name">${escapeHTML(text.t("setup.instanceName"))}</label>
          <div class="logo-row">
            ${imagePicker(
              { what: "logo", current: instance.logo, initials: instance.name || "L", label: text.t("personalization.logo") },
              text,
            )}
            <input type="text" id="instance-name" name="name" value="${escapeAttr(instance.name)}" placeholder="Acme" />
          </div>
          <p class="hint">${escapeHTML(text.t("setup.instanceNameHelp"))} ${escapeHTML(text.t("personalization.logoHelp"))}</p>
        </div>
        <div class="field">
          <label for="accent">${escapeHTML(text.t("personalization.accent"))}</label>
          <div class="accent-row">
            <input type="color" id="accent" name="accent" value="${escapeAttr(instance.accent || "#e8520e")}" />
            <button type="button" class="button ghost" data-reset-accent>${escapeHTML(text.t("personalization.accentReset"))}</button>
          </div>
          <p class="hint">${escapeHTML(text.t("personalization.accentHelp"))}</p>
        </div>
        <p class="form-error" role="alert" hidden></p>
        <div class="settings-actions">
          <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>`,
        { lead: text.t("personalization.lead") },
      )}
    </form>`;
}

/** The organisations, as cards. */
export function organizationsScreen(
  orgs: OrganizationSummary[],
  may: { canAdd: boolean; canManage: (slug: string) => boolean; canDelete: boolean },
  text: Text,
): string {
  const rows = orgs
    .map((o) =>
      settingsItem({
        lead: `<span class="si-mark">${icons.organization(16)}</span>`,
        name: o.name,
        sub: `/${o.slug}`,
        href: `/panel/${o.slug}`,
        actions: may.canManage(o.slug)
          ? `${iconButton("edit", text.t("action.edit"), `data-edit-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`)}
             ${may.canDelete ? iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`) : ""}`
          : "",
      }),
    )
    .join("");

  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.organizations"))}</h1>
    <div class="settings">
      ${settingsCard(
        text.t("shell.organizations"),
        orgs.length === 0
          ? emptyState(text.t("panel.firstStep"), icons.organization(28))
          : `<ul class="settings-list">${rows}</ul>`,
        {
          lead: text.t("organizations.lead"),
          action: may.canAdd ? actionButton("add", text.t("panel.newOrganization"), 'data-add-org data-open-modal="org-modal"', "primary") : "",
        },
      )}
    </div>`;
}

/** One organisation: its repositories, and what can be done to it. */
export function organizationScreen(
  org: OrganizationSummary & { forge?: string },
  repos: RepositorySummary[],
  canManage: boolean,
  /** Deleting a repository ends every report in it, so it is the server's. */
  canDelete: boolean,
  text: Text,
): string {
  const rows = repos
    .map((r) =>
      settingsItem({
        lead: `<span class="si-mark">${icons.repository(16)}</span>`,
        name: r.name,
        sub: `/${org.slug}/${r.slug}`,
        tags: [
          r.accepts_bugs ? `<span class="chip">${escapeHTML(text.t("kind.bug"))}</span>` : "",
          r.accepts_ideas ? `<span class="chip">${escapeHTML(text.t("kind.idea"))}</span>` : "",
        ].join(""),
        href: `/${org.slug}/${r.slug}/queue`,
        actions: canManage
          ? `${iconButton("open", text.t("shell.publicBoard"), `data-open="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}"`)}
             ${iconButton("edit", text.t("action.edit"), `data-edit-repo="${escapeAttr(r.slug)}"`)}
             ${canDelete ? iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-repo="${escapeAttr(r.slug)}" data-name="${escapeAttr(r.name)}"`) : ""}`
          : "",
      }),
    )
    .join("");

  return `<p class="crumb"><a href="/panel/organizations">${escapeHTML(text.t("shell.organizations"))}</a></p>
    <h1 class="visually-hidden">${escapeHTML(org.name)}</h1>
    <div class="settings">
      ${settingsCard(
        org.name,
        `<p class="org-tags"><span class="mono-tag">/${escapeHTML(org.slug)}</span>
          ${org.forge && org.forge !== "none" ? `<span class="mono-tag accent">${escapeHTML(org.forge)}</span>` : ""}</p>
        ${repos.length === 0 ? emptyState(text.t("organizations.noRepositories"), icons.repository(28)) : `<ul class="settings-list repositories">${rows}</ul>`}`,
        {
          action: canManage ? actionButton("add", text.t("panel.newRepository"), 'data-add-repo data-open-modal="repo-modal"', "primary") : "",
        },
      )}
    </div>`;
}

/** Who may act here, what each of them is, and where. */
export function peopleScreen(
  people: {
    id: number;
    username: string;
    name: string;
    avatar?: string;
    admin: boolean;
    disabled: boolean;
    grants: { scope: string; role: string; where?: string; organization?: string; repository?: string }[];
  }[],
  text: Text,
): string {
  const rows = people
    .map((p) =>
      settingsItem({
        attrs: `data-person="${p.id}"`,
        lead: avatar(p.name || p.username, p.avatar),
        name: p.name || p.username,
        sub: p.username,
        tags: [
          p.admin ? `<span class="mono-tag accent">${escapeHTML(text.t("people.administrator"))}</span>` : "",
          p.disabled ? `<span class="mono-tag">${escapeHTML(text.t("people.disabled"))}</span>` : "",
          p.grants.length === 0 && !p.admin
            ? `<span class="mono-tag">${escapeHTML(text.t("people.noRoles"))}</span>`
            : p.grants
                .map((g) => `<span class="mono-tag">${escapeHTML(text.t(`role.${g.role}`))} · ${escapeHTML(g.where ?? "")}</span>`)
                .join(""),
        ].join(""),
        actions: `${iconButton("person", text.t("people.roles"), `data-roles-person="${p.id}" data-username="${escapeAttr(p.username)}"`)}
          ${iconButton("edit", text.t("action.edit"), `data-edit-person="${p.id}" data-name="${escapeAttr(p.name)}" data-username="${escapeAttr(p.username)}"`)}
          ${iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-person="${p.id}" data-name="${escapeAttr(p.username)}"`)}`,
      }),
    )
    .join("");

  // Lastboard's "Users": one card, the way to add somebody at the end of its
  // title, and a row per person with what can be done to them at the far end.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.people"))}</h1>
    <div class="settings">
      ${settingsCard(text.t("shell.people"), `<ul class="settings-list">${rows}</ul>`, {
        lead: text.t("people.lead"),
        action: actionButton("add", text.t("people.add"), 'data-open-modal="person-modal"', "primary"),
      })}
    </div>`;
}

/**
 * Backups: one file out, one file in.
 *
 * Restoring is deliberately the noisier half. Taking a copy is safe and can be
 * done at any time; putting one back replaces everything anybody has written
 * since, which is why it says so and asks again.
 */
export function backupScreen(text: Text): string {
  // Lastboard's "Full backup": one card, a row for taking one and a row for
  // putting one back, each with its button at the end of the line and a rule
  // between them. The warning is the restore row's own description, in red,
  // as theirs is.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.backups"))}</h1>
    <div class="settings">
      ${settingsCard(
        text.t("shell.backups"),
        `<div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("backups.take"))}</h4>
            <p>${escapeHTML(text.t("backups.help"))}</p>
          </div>
          <a class="button primary with-icon" href="/api/backup" download>${icons.download()}<span>${escapeHTML(text.t("backups.download"))}</span></a>
        </div>
        <div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("backups.restore"))}</h4>
            <p>${escapeHTML(text.t("backups.restoreHelp"))}</p>
            <p class="settings-danger">${escapeHTML(text.t("backups.restoreWarning"))}</p>
          </div>
          <div>
            <input type="file" accept=".db,application/octet-stream" id="restore-file" hidden />
            ${actionButton("upload", text.t("backups.chooseFile"), "data-pick-restore", "ghost")}
          </div>
        </div>
        <p class="restore-note" hidden></p>`,
        { lead: text.t("backups.lead") },
      )}
    </div>`;
}

/** What is running, where it came from, and whether there is more of it. */
export interface ServerFacts {
  version?: string;
  go?: string;
  os?: string;
  uptime_seconds?: number;
  started_at?: number;
  database_bytes?: number;
  disk_free_bytes?: number;
  disk_total_bytes?: number;
  memory_bytes?: number;
  schema?: number;
}

/**
 * About, for the person who has to keep this running: what it is, whether
 * there is something newer, and whether the machine under it is healthy.
 *
 * Updates used to be a page of their own beside this one, with the version on
 * both. They are the same question — "what am I running, and is it the one I
 * should be" — so, as in Lastboard, they are one screen: the product and its
 * version at the top with the update underneath, the release channel as a
 * setting on a row of its own, the server's health below, and the links out
 * at the foot.
 *
 * The update zone is drawn by `updateZone` and filled after the page: asking
 * the release server can take seconds, and the rest of this screen is known at
 * once.
 */
export function aboutScreen(facts: ServerFacts, text: Text): string {
  // A fact about the machine, as a card of the panel's recipe: its name as the
  // card's quiet title, the number under it at the size of a number.
  const row = (label: string, value: string, sub = "", extra = "") =>
    `<section class="bento-card about-stat">
      <div class="card-top"><h3>${escapeHTML(label)}</h3></div>
      <strong class="about-stat-value">${escapeHTML(value)}</strong>
      ${extra}
      ${sub ? `<p class="about-stat-sub">${escapeHTML(sub)}</p>` : ""}
    </section>`;

  const stats: string[] = [];
  if (facts.uptime_seconds !== undefined) {
    stats.push(
      row(
        text.t("about.uptime"),
        text.since(facts.uptime_seconds),
        facts.started_at !== undefined ? text.t("about.since", { date: text.date(facts.started_at) }) : "",
      ),
    );
  }
  if (facts.database_bytes !== undefined) {
    stats.push(
      // The size and nothing else: the schema number beside it answered a
      // question for whoever develops this, not for whoever runs it.
      row(text.t("about.database"), text.bytes(facts.database_bytes)),
    );
  }
  if (facts.disk_free_bytes !== undefined) {
    const total = facts.disk_total_bytes;
    // How full, as a bar under the row's words: a <meter>, so its length is an
    // attribute this page's policy allows. Amber past 75%, red past 90%.
    const meter =
      total !== undefined && total > 0
        ? // Machine numbers, never through the locale formatter: a decimal
          // comma in an attribute is a number the browser cannot read.
          `<meter class="about-meter" min="0" max="${String(Math.round(total))}"
             value="${String(Math.round(total - facts.disk_free_bytes))}"
             low="${String(Math.round(total * 0.75))}" high="${String(Math.round(total * 0.9))}" optimum="0"
             aria-label="${escapeAttr(text.t("about.disk"))}"></meter>`
        : "";
    stats.push(
      row(
        text.t("about.disk"),
        text.bytes(facts.disk_free_bytes),
        total !== undefined ? text.t("about.diskOf", { total: text.bytes(total) }) : "",
        meter,
      ),
    );
  }
  if (facts.memory_bytes !== undefined) {
    stats.push(row(text.t("about.memory"), text.bytes(facts.memory_bytes)));
  }

  const link = (href: string, icon: string, label: string) =>
    `<a href="${escapeAttr(href)}" target="_blank" rel="noopener noreferrer">${icon}<span>${escapeHTML(label)}</span></a>`;

  // Lastboard's About, card for card: the product as a statement in a card of
  // its own — mark, name, version, and the update under it; the beta switch
  // on a row in a card of its own; and the links out at the foot, over a
  // rule. The server's health, which theirs does not show, is one more card
  // of the same recipe with a row per fact.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.about"))}</h1>
    <div class="settings about">
      <section class="bento-card about-hero">
        <img class="about-logo" src="/logo.svg" alt="" width="72" height="72" />
        <h2 class="about-name">${escapeHTML(text.t("app.name"))}</h2>
        <p class="about-version">${escapeHTML(facts.version || "—")}</p>
        <div class="about-update" data-update-zone>${updateZone("checking", text)}</div>
        <button type="button" class="button ghost with-icon about-check" data-check hidden>
          ${icons.update(15)}<span>${escapeHTML(text.t("update.check"))}</span></button>
      </section>

      <section class="bento-card">
        <div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("update.beta"))}</h4>
            <p>${escapeHTML(text.t("update.channelHelp"))}</p>
          </div>
          <label class="toggle">
            <input type="checkbox" role="switch" data-beta disabled
              aria-label="${escapeAttr(text.t("update.beta"))}" />
            <span class="toggle-track" aria-hidden="true"></span>
          </label>
        </div>
      </section>

      ${stats.length === 0 ? "" : `<div class="about-stats">${stats.join("")}</div>`}

      <nav class="about-links" aria-label="${escapeAttr(text.t("shell.about"))}">
        ${link("https://git.codigosh.com/CodigoSH/LastHelp", icons.code(15), text.t("about.sourceCode"))}
        ${link("https://opencollective.com/codigosh", icons.heart(15), text.t("about.donate"))}
      </nav>
    </div>`;
}

/**
 * Release notes, drawn rather than shown as source.
 *
 * They are Keep-a-Changelog, generated from commit subjects: "### Added"
 * headings and "- " items and nothing else. Printed as they come, the person
 * deciding whether to install read markdown punctuation. This draws exactly
 * those two shapes — every piece of text still escaped — and anything else as
 * a plain line, so a note written by hand is never lost, only left plain.
 */
export function releaseNotes(markdown: string): string {
  const out: string[] = [];
  let items: string[] = [];
  const flush = () => {
    if (items.length > 0) out.push(`<ul>${items.map((i) => `<li>${escapeHTML(i)}</li>`).join("")}</ul>`);
    items = [];
  };
  for (const raw of markdown.split("\n")) {
    const line = raw.trim();
    if (line.startsWith("- ") || line.startsWith("* ")) {
      items.push(line.slice(2));
      continue;
    }
    flush();
    if (line === "") continue;
    const heading = /^#{1,6}\s+(.*)$/.exec(line);
    out.push(heading ? `<h4>${escapeHTML(heading[1] ?? "")}</h4>` : `<p>${escapeHTML(line)}</p>`);
  }
  flush();
  return out.join("");
}

/** What /api/update says. */
export interface UpdateStatus {
  current: string;
  latest?: string;
  available?: boolean;
  notes?: string;
  reason?: string;
  beta?: boolean;
}

/**
 * The part of About that says whether there is something newer.
 *
 * It says why it cannot check as plainly as it says there is something to
 * install. An empty answer where a reason belongs reads as "you are up to
 * date", which is the one thing it must never say by accident — so up to date
 * is green only when the release server was reached and said so.
 */
export function updateZone(status: UpdateStatus | "checking" | null, text: Text): string {
  if (status === "checking") {
    return `<p class="about-state checking" role="status"><span class="about-spin" aria-hidden="true"></span>
      <span>${escapeHTML(text.t("update.checking"))}</span></p>`;
  }
  if (status === null) {
    return `<p class="about-state warn" role="status">${icons.warning(16)}<span>${escapeHTML(text.t("error.update_unreachable"))}</span></p>`;
  }
  if (status.available) {
    // Lastboard's: a card within the card, lit from one corner, the badge that
    // pulses and the version under it, and one button — the notes are read
    // before pressing it, which is where reading them can still change a mind.
    return `<div class="about-new">
      <span class="about-new-glow" aria-hidden="true"></span>
      <div class="about-new-head">
        <span class="about-badge">${escapeHTML(text.t("update.badge"))}</span>
        <span class="about-new-version">${escapeHTML(status.latest ?? "")}</span>
      </div>
      ${status.notes ? `<div class="notes release-notes">${releaseNotes(status.notes)}</div>` : ""}
      <button class="button primary with-icon" data-update>${icons.download()}<span>${escapeHTML(text.t("update.install"))}</span></button>
      <p class="update-note" role="status" hidden></p>
    </div>`;
  }
  if (status.latest || status.reason === "error.update_none") {
    const said = status.reason ? text.t(status.reason) : text.t("update.upToDate");
    return `<p class="about-state ok" role="status">${icons.done(18)}<span>${escapeHTML(said)}</span></p>`;
  }
  return `<p class="about-state${status.reason === "error.update_unreachable" ? " warn" : ""}" role="status">${icons.info(16)}<span>${escapeHTML(
    text.t(status.reason ?? "error.update_unreachable"),
  )}</span></p>`;
}

/**
 * Where somebody may act, and as what.
 *
 * Roles are granted per place rather than globally: an organisation has owners,
 * a repository has collaborators, and somebody who helps with one repository
 * has no business in the one next door. The list is the current grants with a
 * way to take each one back, and one row to add another.
 */
export function rolesModalBody(
  grants: { role: string; where?: string; organization?: string; repository?: string }[],
  places: { organization: string; repository?: string; label: string }[],
  text: Text,
): string {
  const current =
    grants.length === 0
      ? `<p class="hint">${escapeHTML(text.t("people.noRoles"))}</p>`
      : `<ul class="grants">${grants
          .map(
            (g) => `<li>
              <span class="chip">${escapeHTML(text.t(`role.${g.role}`))}</span>
              <span>${escapeHTML(g.where ?? "")}</span>
              ${iconButton("remove", text.t("people.revoke"), `class="icon-button danger" data-revoke data-organization="${escapeAttr(g.organization ?? "")}" data-repository="${escapeAttr(g.repository ?? "")}"`)}
            </li>`,
          )
          .join("")}</ul>`;

  return `${current}
    <h3 class="grants-title">${escapeHTML(text.t("people.grantRole"))}</h3>
    ${choiceField({
      id: "grant-where",
      name: "where",
      label: text.t("people.where"),
      options: places.map((p) => [
        `${p.organization}${p.repository ? "/" + p.repository : ""}`,
        p.label,
      ]),
    })}
    ${choiceField({
      id: "grant-role",
      name: "role",
      label: text.t("people.role"),
      // Owner runs an organisation; collaborator triages one repository. The
      // pairing is not enforced here — authz decides what a role means — but
      // the order puts the likely one first for each kind of place.
      options: [
        ["collaborator", text.t("role.collaborator")],
        ["owner", text.t("role.owner")],
      ],
      hint: text.t("people.roleHelp"),
    })}`;
}
