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
  /** A sentence, escaped, with ready-made markup in its placeholders. */
  markup(key: string, markup: Record<string, string>): string;
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
/**
 * The report itself: what it is, what it says, and its votes. One function for
 * the page everybody reads and the team's, so the two cannot come to show the
 * same report differently. The team's carries a pencil in its meta line that
 * turns the words into a form where they stand.
 */
function reportArticle(item: ItemDetail, text: Text, pencil = ""): string {
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

  return `<article class="detail">
      <div class="detail-main">
        <p class="meta">
          <span class="kind kind-${escapeAttr(item.kind)}">
            ${item.kind === "bug" ? icons.bug(14) : icons.idea(14)}
            <span>${escapeHTML(text.t(`kind.${item.kind}`))}</span>
          </span>
          ${stateChip(item, text)}
          <span class="when">${escapeHTML(text.date(item.created_at))}</span>
          ${issue}
          ${pencil}
        </p>
        <div class="detail-words">
          <h1>${escapeHTML(item.title)}</h1>
          <div class="body">${body}</div>
        </div>
        ${answers(item.fields ?? [], text)}
      </div>
      ${vote}
    </article>`;
}

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
    ${reportArticle(item, text)}

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
        <img class="wizard-mark" src="${escapeAttr(
          instance?.logo && instance.logo.trim() !== "" ? instance.logo : "/logo.svg",
        )}" alt="" width="64" height="64" />
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
  // A logo nobody has chosen is LastHelp's own, the same one the sign-in and
  // the bar show, rather than initials that appear nowhere else.
  const preview =
    spec.what === "logo"
      ? `<img class="logo-preview" src="${escapeAttr(spec.current || "/logo.svg")}" alt="" />`
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
export function acceptsFields(text: Text, current?: { bugs: boolean; ideas: boolean }): string {
  // Two checkboxes that say what they turn on, not two pills reading "Bug" and
  // "Idea". This is a settings form: the control people know here is a
  // checkbox with a sentence beside it, and the words are what somebody
  // reporting will be offered rather than the internal name of a kind.
  const box = (name: string, label: string, on = true) =>
    `<label class="check">
      <input type="checkbox" name="${escapeAttr(name)}"${on ? " checked" : ""} />
      <span>${escapeHTML(label)}</span>
    </label>`;

  return `<div class="field">
    <label>${escapeHTML(text.t("repo.accepts"))}</label>
    <div class="checks">
      ${box("accepts_bugs", text.t("repo.acceptsBugs"), current?.bugs ?? true)}
      ${box("accepts_ideas", text.t("repo.acceptsIdeas"), current?.ideas ?? true)}
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
export function setupDone(text: Text, logo?: string): string {
  // One card, as every step before it: the mark, where you are, what comes
  // next, and the way there at full width. It was a heading, a card and a
  // button loose on the page with nothing between them.
  return `<section class="wizard">
    <div class="wizard-done">
      <div class="wizard-card">
        <img class="wizard-mark" src="${escapeAttr(logo && logo.trim() !== "" ? logo : "/logo.svg")}" alt="" width="64" height="64" />
        <header class="wizard-head">
          <h1 class="wizard-where">${escapeHTML(text.t("setup.doneTitle"))}</h1>
          <p class="lead">${escapeHTML(text.t("setup.doneLead"))}</p>
        </header>
        <p class="wizard-next">${escapeHTML(text.t("setup.doneNext"))}</p>
        <div class="wizard-actions">
          <a class="button primary" href="/panel/new">${escapeHTML(text.t("panel.newOrganization"))}</a>
        </div>
      </div>
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
  /** What the account is, written under the name; empty says nothing. */
  role: string;
  isAdmin: boolean;
  /** True for anybody who may create an organisation. */
  mayAddOrganization: boolean;
  avatar?: string;
  content: string;
  /** What is waiting, already narrowed by the server to what this account may open. */
  waiting?: { reports: number; messages: number };
  /** Something newer is released: Info in the menu says so, as in Lastboard. */
  updateAvailable?: boolean;
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

  const item = (href: string, label: string, icon: string, extra = "", count = 0, after = ""): string => {
    const current = spec.path === href || (href !== "/panel" && spec.path.startsWith(href + "/"));
    return `<a class="side-item ${extra}" href="${escapeAttr(href)}"${current ? ' aria-current="page"' : ""}>${icon}<span>${escapeHTML(label)}</span>${badge(count)}${after}</a>`;
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
        ${spec.role ? `<span class="side-role">${escapeHTML(spec.role)}</span>` : ""}
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
                item("/panel/about", text.t("shell.about"), icons.info(), "", 0, spec.updateAvailable ? sideUpdateBadge(text) : "")
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
  /** The numbers are the whole server's, which only its administrator sees;
   *  otherwise they are the reader's boards, and people is not theirs. */
  server?: boolean;
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

/**
 * What a repository takes, as the marks of its kinds rather than as words:
 * the same coloured mark Review puts beside a report of that kind, so the bug
 * on a repository's row is the bug on every report in it. Bare, not in a
 * tile: a tile beside the row's buttons reads as one more button. Named for
 * whoever cannot see the picture, since the picture is all there is.
 */
function acceptsMarks(repo: { accepts_bugs: boolean; accepts_ideas: boolean }, text: Text): string {
  const mark = (kind: string) =>
    `<span class="kind kind-${kind}" role="img" title="${escapeAttr(text.t(`kind.${kind}`))}"
       aria-label="${escapeAttr(text.t(`kind.${kind}`))}" data-accepts="${kind}">${
      kind === "bug" ? icons.bug(15) : icons.idea(15)
    }</span>`;
  return (repo.accepts_bugs ? mark("bug") : "") + (repo.accepts_ideas ? mark("idea") : "");
}

/**
 * What is waiting somewhere, as the review icon and its number — the mark the
 * bar already uses for the same thing — rather than a word and a number. Named
 * in full for whoever cannot see it.
 */
function waitingMark(count: number, text: Text): string {
  const label = text.t("shell.waitingCount", { count: text.number(count) });
  return `<span class="state-mark hold" role="img" data-pending="${count}"
    title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}">${icons.queue(14)}<b>${escapeHTML(text.number(count))}</b></span>`;
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
               (b) => `<li><a href="/review?mode=all&amp;board=${escapeAttr(`${b.org}/${b.repo}`)}">
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
    reports: stat("reports", total, text.t("overview.reports"), icons.repository(), "/review?mode=all"),
    waiting: stat("waiting", data.pending, text.t("panel.queue"), icons.queue(), "/review?mode=pending"),
    published: stat("published", data.published, text.t("overview.published"), icons.check(), "/review?mode=all&moderation=published"),
    votes: stat("votes", data.votes, text.t("overview.votes"), icons.idea(), "/review?mode=all&kind=idea&sort=votes"),
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

    <ul class="dashboard${data.server === false ? " team" : ""}">
      ${overviewCards
        .filter((name) => data.server !== false || name !== "people")
        .map((name) => all[name])
        .join("")}
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
  /** What waits, as the bar counts it: shown on its tab from either mode. */
  waiting?: number;
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
            // The waiting count stays on its tab from either mode. In the
            // queue it is what the list draws; from the archive it is the
            // server's, the number the bar shows — the two never disagree.
            `<span class="count-chip">${escapeHTML(text.number(view.mode === "pending" ? total : (view.waiting ?? 0)))}</span>`
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

/**
 * What an account is, in one word, for under its name: the administrator, or
 * the team. Roles are per organisation and per repository — one account can
 * own one organisation and collaborate on a repository of another — so a
 * single role written under the name would be true in one place and false in
 * the next. The word the team's replies already carry says what is true
 * everywhere.
 */
export function accountRole(me: { admin?: boolean; memberships?: Array<{ role: string }> } | null, text: Text): string {
  if (!me) return "";
  if (me.admin) return text.t("people.administrator");
  return (me.memberships ?? []).length > 0 ? text.t("comment.team") : "";
}

export function profileScreen(
  me: { username: string; avatar?: string; role: string },
  text: Text,
): string {
  // Lastboard's "My profile" and "System password", card for card: the
  // picture, the name and the role; the username — the one name an account
  // has, to sign in with and to sign replies — with its save button on the same
  // line; and the password, closed by one button aligned with the fields
  // rather than stretched across the card.
  return `<h1 class="visually-hidden">${escapeHTML(text.t("shell.profile"))}</h1>
    <div class="settings">
      ${settingsCard(
        text.t("shell.profile"),
        `<div class="profile-head">
          ${imagePicker(
            { what: "avatar", current: me.avatar, initials: me.username, label: text.t("profile.picture"), big: true },
            text,
          )}
          <div class="profile-who">
            <span class="profile-name">${escapeHTML(me.username)}</span>
            ${me.role ? `<span class="profile-role">${escapeHTML(me.role)}</span>` : ""}
          </div>
        </div>
        <form class="profile-name">
          <div class="field">
            <label for="profile-username">${escapeHTML(text.t("auth.username"))}</label>
            <div class="input-row">
              <input type="text" id="profile-username" name="username" value="${escapeAttr(me.username)}" required
                minlength="2" maxlength="60" autocomplete="username" autocapitalize="none" spellcheck="false" />
              <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button>
            </div>
            <p class="hint">${escapeHTML(text.t("profile.usernameHelp"))}</p>
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
        ${accentChoice(instance.accent, text)}
        <p class="form-error" role="alert" hidden></p>
        <div class="settings-actions">
          <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>`,
        { lead: text.t("personalization.lead") },
      )}
    </form>`;
}

/**
 * Lastboard's accent picker: seven swatches and an eighth for any colour, the
 * chosen one ringed in white with a tick.
 *
 * The house orange is one of the seven and choosing it stores nothing — the
 * default — so an installation that picked it follows the default rather than
 * freezing today's value. The colour of a swatch is its class, not a style:
 * this page's policy drops style attributes. The eighth takes whatever was
 * chosen through a rule, set from main.ts.
 */
export const ACCENTS: [string, string][] = [
  ["blue", "#228be6"],
  ["indigo", "#4c6ef5"],
  ["cyan", "#15aabf"],
  ["teal", "#12b886"],
  ["orange", "#e8520e"],
  ["red", "#fa5252"],
  ["grape", "#be4bdb"],
];

const TICK = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 16.17L4.83 12l-1.42 1.42L9 19 21 7l-1.42-1.42z"/></svg>`;

/** Their palette, filled with its wells cut out — copied, not approximated. */
const PALETTE = `<svg class="acc-palette" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3c-4.97 0-9 4.03-9 9s4.03 9 9 9c.83 0 1.5-.67 1.5-1.5 0-.39-.15-.74-.39-1.01-.23-.26-.38-.61-.38-.99 0-.83.67-1.5 1.5-1.5H16c2.76 0 5-2.24 5-5 0-4.42-4.03-8-9-8zm-5 9c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1zm3-3c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1zm5 0c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1zm3 3c-.55 0-1-.45-1-1s.45-1 1-1 1 .45 1 1-.45 1-1 1z"/></svg>`;

function accentChoice(current: string, text: Text): string {
  const value = (current || "").toLowerCase();
  const preset = ACCENTS.find(([, hex]) => hex === (value || "#e8520e"));
  const custom = !preset;
  return `<div class="field">
    <label>${escapeHTML(text.t("personalization.accent"))}</label>
    <input type="hidden" name="accent" id="accent" value="${escapeAttr(value)}" />
    <div class="color-grid" role="radiogroup" aria-label="${escapeAttr(text.t("personalization.accent"))}">
      ${ACCENTS.map(([name, hex]) => {
        const on = preset?.[0] === name;
        return `<button type="button" class="accent-swatch acc-${name}${on ? " on" : ""}" role="radio"
          aria-checked="${on}" aria-label="${escapeAttr(hex)}"
          data-accent="${name === "orange" ? "" : escapeAttr(hex)}">${TICK}</button>`;
      }).join("")}
      <label class="accent-swatch acc-custom${custom ? " on" : ""}" aria-label="${escapeAttr(text.t("personalization.accent"))}">
        ${PALETTE}
        <input type="color" data-accent-custom value="${escapeAttr(custom && value ? value : "#e8520e")}" />
      </label>
    </div>
    <p class="hint">${escapeHTML(text.t("personalization.accentHelp"))}</p>
  </div>`;
}

/** One organisation in the panel's list, as the server says it. */
export interface OrganizationRow {
  slug: string;
  name: string;
  forge: string;
  /** How many of its boards this reader works on. */
  repositories: number;
  /** What is waiting in those. */
  pending: number;
  /** What the reader is there: owner, collaborator or administrator. */
  role: string;
  /** Whether its forge has a token, for whoever connects it. */
  connected?: boolean;
  can_manage: boolean;
}

/**
 * The organisations the reader may open, each saying what they are there,
 * how many of its boards they work on, what is waiting in those, and whether
 * its forge is connected — so where the work is shows before opening anything.
 */
export function organizationsScreen(
  orgs: OrganizationRow[],
  may: { canAdd: boolean; canDelete: boolean },
  text: Text,
): string {
  const forgeName = (forge: string) => forgeOptions(text).find(([value]) => value === forge)?.[1] ?? forge;
  const roleName = (role: string) => (role === "administrator" ? text.t("people.administrator") : text.t(`role.${role}`));
  const rows = orgs
    .map((o) =>
      settingsItem({
        attrs: `data-org="${escapeAttr(o.slug)}"`,
        lead: `<span class="si-mark">${icons.organization(16)}</span>`,
        name: o.name,
        sub: `/${o.slug} · ${text.t("org.repoCount", { count: text.number(o.repositories) })}`,
        tags: [
          o.pending > 0
            ? waitingMark(o.pending, text)
            : "",
          o.forge && o.forge !== "none"
            ? `<span class="mono-tag" data-forge="${escapeAttr(o.forge)}">${escapeHTML(forgeName(o.forge))}</span>`
            : "",
          o.connected === false
            ? `<span class="state-mark hold" role="img" data-connected="false"
                 title="${escapeAttr(text.t("org.notConnected"))}" aria-label="${escapeAttr(text.t("org.notConnected"))}">${icons.warning(15)}</span>`
            : "",
          o.role
            ? `<span class="mono-tag${o.role === "owner" ? " accent" : ""}" data-role="${escapeAttr(o.role)}">${escapeHTML(roleName(o.role))}</span>`
            : "",
        ].join(""),
        href: `/panel/${o.slug}`,
        actions: o.can_manage
          ? `${iconButton("edit", text.t("organizations.rename"), `data-edit-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`)}
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
          : `<ul class="settings-list organizations">${rows}</ul>`,
        {
          lead: text.t("organizations.lead"),
          action: may.canAdd ? actionButton("add", text.t("panel.newOrganization"), 'data-add-org data-open-modal="org-modal"', "primary") : "",
        },
      )}
    </div>`;
}

/** Everything the panel's page for one organisation shows, as the server says it. */
export interface OrganizationPanel {
  organization: {
    slug: string;
    name: string;
    forge: string;
    base_url?: string;
    forge_owner?: string;
    webhook_path?: string;
  };
  /** The ones the reader may triage, each with what is waiting in it. */
  repositories: { slug: string; name: string; accepts_bugs: boolean; accepts_ideas: boolean; pending: number }[];
  /** Whether a token and a webhook secret are stored; never what they are. */
  connection?: { token: boolean; secret: boolean };
  can_manage: boolean;
  can_delete: boolean;
}

/**
 * One organisation, in sections: its repositories with what is waiting in
 * each, who works in it, its forge, its name, and ending it.
 *
 * It used to be one card of repositories with the forge's forms stacked under
 * it and nothing about the team at all, so an owner had to know that People
 * was where the organisation's own team lived. Each section is Lastboard's
 * settings card, the one recipe the panel uses for what it configures. A
 * collaborator sees the first two: where they work, and the way into it.
 */
export function organizationScreen(
  data: OrganizationPanel,
  team: { members: TeamMember[]; accounts: { username: string }[] } | null,
  /** The account reading, so it is not offered a way to remove itself. */
  self: string,
  /** Where this instance answers, so the webhook address can be shown whole.
   *  Passed in rather than read from `location`: these templates are pure. */
  origin: string,
  text: Text,
): string {
  const org = data.organization;
  const hasForge = !!org.forge && org.forge !== "none";

  const summary = settingsCard(
    org.name,
    `<p class="org-tags"><span class="mono-tag">/${escapeHTML(org.slug)}</span>
      ${hasForge ? `<span class="mono-tag accent">${escapeHTML(org.forge)}</span>` : ""}</p>`,
    { action: `<a class="button ghost" href="/${escapeAttr(org.slug)}">${escapeHTML(text.t("shell.publicBoard"))}</a>` },
  );

  const rows = data.repositories
    .map((r) =>
      settingsItem({
        attrs: `data-repo="${escapeAttr(r.slug)}"`,
        lead: `<span class="si-mark">${icons.repository(16)}</span>`,
        name: r.name,
        sub: `/${org.slug}/${r.slug}`,
        tags:
          (r.pending > 0
            ? waitingMark(r.pending, text)
            : "") + acceptsMarks(r, text),
        href: `/panel/${org.slug}/repos/${r.slug}`,
        actions: `${iconButton("open", text.t("shell.publicBoard"), `data-open="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}"`)}
          ${data.can_delete ? iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-repo="${escapeAttr(r.slug)}" data-name="${escapeAttr(r.name)}"`) : ""}`,
      }),
    )
    .join("");
  const repositories = settingsCard(
    text.t("portal.boards"),
    data.repositories.length === 0
      ? emptyState(text.t("organizations.noRepositories"), icons.repository(28))
      : `<ul class="settings-list repositories">${rows}</ul>`,
    {
      action: data.can_manage
        ? actionButton("add", text.t("panel.newRepository"), 'data-add-repo data-open-modal="repo-modal"', "primary")
        : "",
    },
  );

  if (!data.can_manage) {
    return `<p class="crumb"><a href="/panel/organizations">${escapeHTML(text.t("shell.organizations"))}</a></p>
      <h1 class="visually-hidden">${escapeHTML(org.name)}</h1>
      <div class="settings">${summary}${repositories}</div>`;
  }

  const forge = hasForge ? organizationForge(data, origin, text) : "";

  const settings = settingsCard(
    text.t("repo.settings"),
    `<form class="org-settings">
      ${field({ id: "os-name", name: "name", label: text.t("panel.name"), hint: text.t("panel.nameHelp"), required: true, value: org.name }, text)}
      <p class="form-error" role="alert" hidden></p>
      <div class="settings-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
    </form>`,
  );

  const danger = data.can_delete
    ? settingsCard(
        text.t("repo.danger"),
        `<div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("org.delete"))}</h4>
            <p class="settings-danger">${escapeHTML(text.t("org.deleteHelp"))}</p>
          </div>
          ${actionButton("remove", text.t("action.delete"), `data-delete-org="${escapeAttr(org.slug)}" data-name="${escapeAttr(org.name)}"`, "danger")}
        </div>`,
      )
    : "";

  return `<p class="crumb"><a href="/panel/organizations">${escapeHTML(text.t("shell.organizations"))}</a></p>
    <h1 class="visually-hidden">${escapeHTML(org.name)}</h1>
    <div class="settings">
      ${summary}
      ${repositories}
      ${team ? organizationTeam(data, team, self, text) : ""}
      ${forge}
      ${settings}
      ${danger}
    </div>`;
}

/**
 * Who works in an organisation, all of it managed here: each member with a
 * pencil to change what they are and a bin to take them off the team, and a
 * form to add somebody as an owner of the organisation or a collaborator on one
 * of its repositories.
 *
 * Collaborators used to be only listed here and changed on each repository's
 * page, and the form here only knew one role — so adding a collaborator meant
 * knowing that the organisation's own team page could not do it.
 */
function organizationTeam(
  data: OrganizationPanel,
  team: { members: TeamMember[]; accounts: { username: string }[] },
  self: string,
  text: Text,
): string {
  const isOwner = (m: TeamMember) => m.roles.some((r) => r.role === "owner" && r.repository === "");
  const owners = team.members.filter(isOwner);
  const collaborators = team.members.filter((m) => !isOwner(m));
  const reposOf = (m: TeamMember) => m.roles.filter((r) => r.role === "collaborator" && r.repository !== "");

  const row = (m: TeamMember) => {
    const owner = isOwner(m);
    return settingsItem({
      attrs: `data-member="${escapeAttr(m.username)}"`,
      lead: avatar(m.username, m.avatar),
      name: m.username,
      // Where a collaborator works is what says what they are here.
      sub: owner ? "" : reposOf(m).map((r) => r.where).join(", "),
      tags: owner
        ? `<span class="mono-tag accent" data-role="owner">${escapeHTML(text.t("role.owner"))}</span>`
        : `<span class="mono-tag" data-role="collaborator">${escapeHTML(text.t("role.collaborator"))}</span>`,
      // Not yourself: changing or taking away your own role from this page
      // would leave you on a page you may no longer open.
      actions:
        m.username === self
          ? ""
          : `${iconButton("edit", text.t("people.edit"), `data-edit-member="${escapeAttr(m.username)}" data-owner="${owner}" data-repos="${escapeAttr(reposOf(m).map((r) => r.repository).join(" "))}"`)}
            ${iconButton("remove", text.t("people.revoke"), `class="icon-button danger" data-remove-member="${escapeAttr(m.username)}" data-owner="${owner}" data-repos="${escapeAttr(reposOf(m).map((r) => r.repository).join(" "))}"`)}`,
    });
  };

  const taken = new Set(team.members.map((m) => m.username));
  const candidates = team.accounts.filter((a) => !taken.has(a.username));
  const add =
    candidates.length === 0
      ? ""
      : `<form class="member-add">
          <div class="member-add-fields">
            ${choiceField({
              id: "member-who",
              name: "username",
              label: text.t("auth.username"),
              options: candidates.map((a) => [a.username, a.username] as [string, string]),
            })}
            ${choiceField({
              id: "member-role",
              name: "role",
              label: text.t("people.role"),
              options: teamRoleOptions(data, text),
            })}
          </div>
          <div class="settings-actions">
            <button class="button primary" type="submit">${escapeHTML(text.t("org.addMember"))}</button></div>
        </form>`;

  return settingsCard(
    text.t("repo.team"),
    `<ul class="settings-list team">
      ${owners.map(row).join("")}
      ${collaborators.map(row).join("")}
    </ul>
    ${add}`,
    { lead: text.t("org.teamLead", { organization: data.organization.name }) },
  );
}

/**
 * The roles a member can have in one organisation, as the values the add form
 * sends: "owner" for the organisation, "repo:<slug>" for a collaborator on one
 * repository. One select rather than a role and a repository shown and hidden
 * by script: each option says the whole thing.
 */
function teamRoleOptions(data: OrganizationPanel, text: Text): [string, string][] {
  return [
    ["owner", text.t("org.ownerOf", { organization: data.organization.name })],
    ...data.repositories.map(
      (r) => [`repo:${r.slug}`, text.t("org.collaboratorOn", { repository: r.name })] as [string, string],
    ),
  ];
}

/**
 * What one member is in the organisation, as boxes to tick: owner of the
 * organisation, and collaborator on each repository. Filled in by the page
 * when the pencil opens it; an empty set is taking them off the team.
 */
export function teamRolesFields(data: OrganizationPanel, text: Text): string {
  const box = (value: string, label: string) =>
    `<label class="check">
      <input type="checkbox" name="roles" value="${escapeAttr(value)}" />
      <span>${escapeHTML(label)}</span>
    </label>`;
  return `<div class="field">
    <label>${escapeHTML(text.t("people.roles"))}</label>
    <div class="checks">
      ${teamRoleOptions(data, text).map(([value, label]) => box(value, label)).join("")}
    </div>
  </div>`;
}

/**
 * The forge, in one card: where it is and whether it is connected, the token
 * and the secret, and the address the forge delivers to.
 */
function organizationForge(data: OrganizationPanel, origin: string, text: Text): string {
  const org = data.organization;
  const connected = data.connection?.token === true;
  // The forge by the name it goes by, from the same list the wizard offers.
  const forgeName = forgeOptions(text).find(([value]) => value === org.forge)?.[1] ?? org.forge;
  const where = [forgeName, org.base_url, org.forge_owner].filter(Boolean).join(" · ");
  return settingsCard(
    text.t("panel.connection"),
    `<div class="settings-row">
      <div class="settings-info">
        <h4>${escapeHTML(where)}</h4>
        <p>${escapeHTML(text.t(connected ? "org.connectedHelp" : "org.notConnectedHelp"))}</p>
      </div>
      <span class="mono-tag${connected ? " accent" : ""}" data-connected="${connected}">${escapeHTML(text.t(connected ? "org.connected" : "org.notConnected"))}</span>
    </div>
    <form class="credentials">
      ${field({ id: "cred-token", name: "token", label: text.t("panel.token"), type: "password", required: true, hint: text.t("panel.tokenHelp") }, text)}
      ${field({ id: "cred-secret", name: "webhook_secret", label: text.t("panel.webhookSecret"), type: "password", required: true, hint: text.t("panel.secretHelp") }, text)}
      <p class="form-error" role="alert" hidden></p>
      <div class="settings-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
    </form>
    ${
      org.webhook_path
        ? `<div class="settings-row">
            <div class="settings-info">
              <h4>${escapeHTML(text.t("panel.webhookPath"))}</h4>
              <p>${escapeHTML(text.t("panel.webhookPathHelp"))}</p>
              <code class="fact-value" data-webhook>${escapeHTML(origin + org.webhook_path)}</code>
            </div>
            ${actionButton("copy", text.t("action.copy"), "data-copy-webhook")}
          </div>`
        : ""
    }`,
  );
}

/** Everything the panel's page for one repository shows, as the server says it. */
export interface RepositoryPanel {
  slug: string;
  name: string;
  forge_repo: string;
  accepts_bugs: boolean;
  accepts_ideas: boolean;
  /** Which channel tells the team, never what it stores. Empty for none. */
  channel: string;
  pending: number;
  organization: { slug: string; name: string; forge: string };
  can_manage: boolean;
  can_delete: boolean;
}

/** Who works in an organisation, as the team endpoint returns it. */
export interface TeamMember {
  username: string;
  avatar?: string;
  admin: boolean;
  roles: { role: string; repository: string; where: string }[];
}

/**
 * One repository, in one place: what it takes, who is told, who works on it,
 * and what is waiting there.
 *
 * These used to be four places — a queue screen of its own, a pencil wired to
 * nothing, a channel picker on the organisation's page and People — and a
 * team member had to know all four to run one board. Each card is Lastboard's
 * settings card, the one recipe the panel uses for everything it configures.
 */
export function repositoryScreen(
  repo: RepositoryPanel,
  /** The team of the organisation, or null when the reader may not see it. */
  team: { members: TeamMember[]; accounts: { username: string }[] } | null,
  text: Text,
): string {
  const org = repo.organization;
  const board = `${org.slug}/${repo.slug}`;
  const hasForge = !!org.forge && org.forge !== "none";

  // This board's reports, and what waits among them: both are the reports
  // screen filtered to it. A second list of the same reports here would be a
  // second place for the same work.
  const review = `/review?${new URLSearchParams({ mode: "all", moderation: "pending", board }).toString()}`;
  const reports = `/review?${new URLSearchParams({ mode: "all", board }).toString()}`;
  const summary = settingsCard(
    repo.name,
    `<p class="org-tags"><span class="mono-tag">/${escapeHTML(board)}</span>${acceptsMarks(repo, text)}</p>
    <div class="settings-row">
      <div class="settings-info">
        <h4>${escapeHTML(text.t("review.title"))}</h4>
        <p>${escapeHTML(text.t("repo.reportsHelp"))}</p>
      </div>
      <a class="button primary" href="${escapeAttr(reports)}" data-repo-reports>${escapeHTML(text.t("action.open"))}</a>
    </div>
    <div class="settings-row">
      <div class="settings-info">
        <h4>${escapeHTML(text.t("repo.pending", { count: text.number(repo.pending) }))}</h4>
        <p>${escapeHTML(text.t("repo.pendingHelp"))}</p>
      </div>
      <a class="button ghost" href="${escapeAttr(review)}" data-repo-review>${escapeHTML(text.t("action.open"))}</a>
    </div>
    <div class="settings-row">
      <div class="settings-info">
        <h4>${escapeHTML(text.t("panel.export", { repository: repo.name }))}</h4>
        <p>${escapeHTML(text.t("repo.exportHelp"))}</p>
      </div>
      <a class="button ghost with-icon" href="/api/orgs/${escapeAttr(org.slug)}/repos/${escapeAttr(repo.slug)}/export"
         download data-export>${icons.download()}<span>${escapeHTML(text.t("repo.exportDownload"))}</span></a>
    </div>`,
    {
      action: `<a class="button ghost" href="/${escapeAttr(board)}">${escapeHTML(text.t("shell.publicBoard"))}</a>`,
    },
  );

  if (!repo.can_manage) {
    return `${repositoryCrumb(org, text)}
      <h1 class="visually-hidden">${escapeHTML(repo.name)}</h1>
      <div class="settings">${summary}</div>`;
  }

  const settings = settingsCard(
    text.t("repo.settings"),
    `<form class="repo-settings">
      ${field({ id: "rs-name", name: "name", label: text.t("panel.name"), hint: text.t("panel.nameHelp"), required: true, value: repo.name }, text)}
      ${hasForge ? field({ id: "rs-forge", name: "forge_repo", label: text.t("panel.forgeRepo"), hint: text.t("panel.forgeRepoHelp"), value: repo.forge_repo }, text) : ""}
      ${acceptsFields(text, { bugs: repo.accepts_bugs, ideas: repo.accepts_ideas })}
      <p class="form-error" role="alert" hidden></p>
      <div class="settings-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
    </form>`,
  );

  const kinds: [string, string][] = [
    ["", text.t("panel.channelNone")],
    ["ntfy", "ntfy"],
    ["gotify", "Gotify"],
    ["webhook", "Webhook"],
    ["smtp", "Email"],
  ];
  const now = kinds.find(([value]) => value === repo.channel)?.[1] ?? repo.channel;
  // The channel chosen shows as chosen; what it stores never comes back, so
  // the fields are empty and saving writes the whole channel again.
  const channel = settingsCard(
    text.t("panel.channel"),
    `<form class="channel">
      <input type="hidden" name="repo" value="${escapeAttr(repo.slug)}" />
      ${choiceField({ id: "chan-kind", name: "kind", label: text.t("panel.channel"), options: kinds, selected: repo.channel })}
      <div class="channel-fields" hidden>
        ${field({ id: "chan-url", name: "url", label: text.t("panel.address2"), example: "https://ntfy.sh/acme-feedback" }, text)}
        ${field({ id: "chan-token", name: "token", label: text.t("panel.token"), type: "password" }, text)}
      </div>
      <div class="channel-mail" hidden>
        ${field({ id: "chan-to", name: "to", label: text.t("panel.mailTo"), type: "email", example: "team@example.com" }, text)}
      </div>
      ${repo.channel ? `<p class="hint" data-channel-now>${escapeHTML(text.t("repo.channelNow", { channel: now }))}</p>` : ""}
      <p class="form-error" role="alert" hidden></p>
      <div class="settings-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
    </form>`,
    { lead: text.t("panel.channelHelp") },
  );

  const danger = repo.can_delete
    ? settingsCard(
        text.t("repo.danger"),
        `<div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("repo.delete"))}</h4>
            <p class="settings-danger">${escapeHTML(text.t("repo.deleteHelp"))}</p>
          </div>
          ${actionButton("remove", text.t("action.delete"), `data-delete-repo="${escapeAttr(repo.slug)}" data-name="${escapeAttr(repo.name)}"`, "danger")}
        </div>`,
      )
    : "";

  return `${repositoryCrumb(org, text)}
    <h1 class="visually-hidden">${escapeHTML(repo.name)}</h1>
    <div class="settings">
      ${summary}
      ${settings}
      ${team ? repositoryTeam(repo, team, text) : ""}
      ${channel}
      ${danger}
    </div>`;
}

function repositoryCrumb(org: { slug: string; name: string }, text: Text): string {
  return `<p class="crumb"><a href="/panel/organizations">${escapeHTML(text.t("shell.organizations"))}</a>
    <span aria-hidden="true">/</span>
    <a href="/panel/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a></p>`;
}

/**
 * Who works on one repository: the organisation's owners, who work on all of
 * it and are changed on the organisation's page, and its collaborators, who
 * are added and taken off here.
 */
function repositoryTeam(
  repo: RepositoryPanel,
  team: { members: TeamMember[]; accounts: { username: string }[] },
  text: Text,
): string {
  const owners = team.members.filter((m) => m.roles.some((r) => r.role === "owner" && r.repository === ""));
  const collaborators = team.members.filter(
    (m) =>
      m.roles.some((r) => r.role === "collaborator" && r.repository === repo.slug) &&
      !owners.includes(m),
  );
  const row = (m: TeamMember, role: string, removable: boolean) =>
    settingsItem({
      attrs: `data-member="${escapeAttr(m.username)}"`,
      lead: avatar(m.username, m.avatar),
      name: m.username,
      tags: `<span class="mono-tag${role === "owner" ? " accent" : ""}" data-role="${role}">${escapeHTML(text.t(`role.${role}`))}</span>`,
      actions: removable
        ? iconButton("remove", text.t("people.revoke"), `class="icon-button danger" data-revoke="${escapeAttr(m.username)}" data-name="${escapeAttr(m.username)}"`)
        : "",
    });

  const taken = new Set([...owners, ...collaborators].map((m) => m.username));
  const candidates = team.accounts.filter((a) => !taken.has(a.username));
  const add =
    candidates.length === 0
      ? ""
      : `<form class="team-add">
          ${choiceField({
            id: "team-who",
            name: "username",
            label: text.t("repo.addCollaborator"),
            options: candidates.map((a) => [a.username, a.username] as [string, string]),
          })}
          <div class="settings-actions">
            <button class="button primary" type="submit">${escapeHTML(text.t("people.grant"))}</button></div>
        </form>`;

  return settingsCard(
    text.t("repo.team"),
    `<ul class="settings-list team">
      ${owners.map((m) => row(m, "owner", false)).join("")}
      ${collaborators.map((m) => row(m, "collaborator", true)).join("")}
    </ul>
    ${collaborators.length === 0 ? `<p class="card-sub">${escapeHTML(text.t("repo.noCollaborators"))}</p>` : ""}
    ${add}`,
    { lead: text.t("repo.teamLead", { organization: repo.organization.name }) },
  );
}

/**
 * One report, as the team works on it.
 *
 * It used to be the public page with one card of "team tools" appended: the
 * status, the forge issue, a rewrite form, a reply box and a merge form in a
 * single box, three widths of field and three kinds of button — and a second
 * box to write in underneath, the public one, which posted a team member's
 * words as an anonymous visitor's and held them for that same person to
 * approve. Now each job is a card of Lastboard's recipe, in the order the work
 * goes: decide whether it is published, read it, set where it stands, bridge
 * it, answer, and fold a duplicate away. There is one place to write.
 */
export function itemTeamView(
  item: ItemDetail,
  /** What the server says this reader may do; can_delete is the owners' and the administrator's. */
  statuses: { current: string; statuses: string[]; can_delete?: boolean },
  /** The other reports on the same board, the ones this one can be merged into. */
  others: { public_id: string; title: string }[],
  text: Text,
): string {
  const board = item.repository;
  const org = board?.organization;
  const crumb =
    board && org
      ? `<p class="crumb"><a href="/panel/organizations">${escapeHTML(text.t("shell.organizations"))}</a>
          <span aria-hidden="true">/</span>
          <a href="/panel/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a>
          <span aria-hidden="true">/</span>
          <a href="/panel/${escapeAttr(org.slug)}/repos/${escapeAttr(board.slug)}">${escapeHTML(board.name)}</a></p>`
      : "";

  const merged = item.merged_into
    ? `<p class="notice">${icons.info()}<a href="/i/${escapeAttr(item.merged_into)}">${escapeHTML(text.t("item.merged"))}</a></p>`
    : "";

  // The one decision here the page cannot take back, so it comes first and
  // only while there is something to decide.
  const decide =
    item.moderation && item.moderation !== "published"
      ? settingsCard(
          text.t("item.waiting"),
          `<div class="settings-row">
            <div class="settings-info"><p>${escapeHTML(text.t("item.waitingHelp"))}</p></div>
            <div class="decide-actions">
              <button class="button primary with-icon" data-moderate="published">${icons.check()}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
              <button class="button ghost with-icon" data-moderate="rejected">${icons.close()}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
              <button class="button danger with-icon" data-moderate="spam">${icons.warning()}<span>${escapeHTML(text.t("panel.spam"))}</span></button>
            </div>
          </div>`,
        )
      : "";

  // Editing and, for whoever runs the organisation, deleting: both at the end
  // of the report's meta line, where the thing they act on begins. Deleting
  // was a danger card at the foot of the page, a long way from the report.
  const pencil = `<span class="detail-tools">
    ${iconButton("edit", text.t("panel.rewrite"), 'data-rewrite')}
    ${
      statuses.can_delete
        ? iconButton("remove", text.t("item.delete"), `class="icon-button danger" data-delete-item="${escapeAttr(item.public_id)}"`)
        : ""
    }
  </span>`;

  const status = settingsCard(
    text.t("panel.status"),
    `<div class="settings-row">
      <div class="settings-info"><p>${escapeHTML(text.t("item.statusHelp"))}</p></div>
      <select id="status-select" aria-label="${escapeAttr(text.t("panel.status"))}">
        ${statuses.statuses
          .map(
            (s) =>
              `<option value="${escapeAttr(s)}"${s === statuses.current ? " selected" : ""}>${escapeHTML(text.t(`status.${s}`))}</option>`,
          )
          .join("")}
      </select>
    </div>`,
  );

  const issue = settingsCard(
    text.t("item.issue"),
    item.issue
      ? `<div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("item.trackedAs", { number: text.number(item.issue.number) }))}</h4>
            <p>${escapeHTML(text.t("item.issueLinkedHelp"))}</p>
          </div>
          <a class="button ghost with-icon" href="${escapeAttr(item.issue.url)}" target="_blank" rel="noopener" data-issue-link>${icons.open(15)}<span>${escapeHTML(text.t("item.issueOpenIt"))}</span></a>
        </div>`
      : `<div class="settings-row">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("panel.createIssue"))}</h4>
            <p>${escapeHTML(text.t("item.issueCreateHelp"))}</p>
          </div>
          <button class="button primary" type="button" data-create-issue>${escapeHTML(text.t("item.issueCreate"))}</button>
        </div>
        <form class="settings-row attach">
          <div class="settings-info">
            <h4>${escapeHTML(text.t("panel.attachIssue"))}</h4>
            <p>${escapeHTML(text.t("item.issueAttachHelp"))}</p>
          </div>
          <div class="inline-field">
            <input name="issue" required placeholder="${escapeAttr(text.t("panel.attachPlaceholder"))}"
                   aria-label="${escapeAttr(text.t("panel.attachIssue"))}" />
            <button class="button ghost" type="submit">${escapeHTML(text.t("item.issueAttach"))}</button>
          </div>
        </form>`,
  );

  const thread =
    item.comments.length === 0
      ? `<p class="card-sub">${escapeHTML(text.t("item.threadEmpty"))}</p>`
      : `<ul class="comments">${item.comments
          .map((c) => {
            const who = c.name || text.t(`comment.${c.author}`);
            return `<li class="comment comment-${escapeAttr(c.author)}">
              <p class="who">
                ${avatar(who)}
                <span class="who-name">${escapeHTML(who)}</span>
                ${c.author === "team" ? `<span class="who-badge">${escapeHTML(text.t("comment.team"))}</span>` : ""}
                <span class="when">${escapeHTML(text.date(c.created_at))}</span>
                ${
                  statuses.can_delete && c.id !== undefined
                    ? iconButton("remove", text.t("action.delete"), `class="icon-button danger comment-delete" data-delete-comment="${c.id}"`)
                    : ""
                }
              </p>
              <p class="what">${escapeHTML(c.body)}</p>
            </li>`;
          })
          .join("")}</ul>`;
  const conversation = settingsCard(
    text.t("item.comments"),
    `${thread}
    <form class="team-reply">
      <div class="field">
        <label for="team-reply-body">${escapeHTML(text.t("panel.reply"))}</label>
        <textarea id="team-reply-body" name="body" rows="3" required
                  placeholder="${escapeAttr(text.t("item.commentPlaceholder"))}"></textarea>
        <p class="hint">${escapeHTML(text.t("item.replyHelp"))}</p>
      </div>
      <div class="settings-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("form.send"))}</button></div>
    </form>`,
    { action: `<span class="count-chip">${escapeHTML(text.number(item.comments.length))}</span>` },
  );

  // Picked by title from this board's reports, filtered as you type: nobody
  // knows another report's identifier, and asking for one made the control a
  // form nobody could fill in.
  const merge = item.merged_into
    ? ""
    : settingsCard(
        text.t("panel.merge"),
        others.length === 0
          ? `<p class="card-sub">${escapeHTML(text.t("item.mergeNone"))}</p>`
          : `<form class="merge">
              <div class="field">
                <label for="merge-search">${escapeHTML(text.t("item.mergePick"))}</label>
                <input id="merge-search" type="search" data-merge-search
                       placeholder="${escapeAttr(text.t("item.mergeSearch"))}" autocomplete="off" />
              </div>
              ${choiceField({
                id: "merge-into",
                name: "into",
                label: text.t("item.mergeInto"),
                options: others.map((o) => [o.public_id, o.title] as [string, string]),
              })}
              <div class="settings-actions">
                <button class="button primary" type="submit">${escapeHTML(text.t("item.mergeAction"))}</button></div>
            </form>`,
        { lead: text.t("panel.mergeHelp") },
      );

  return `${crumb}${merged}
    <h1 class="visually-hidden">${escapeHTML(item.title)}</h1>
    <div class="settings item-team">
      ${decide}
      ${reportArticle(item, text, pencil)}
      ${status}
      ${issue}
      ${conversation}
      ${merge}
    </div>`;
}

/** The report's words as a form, where they stood. Cancel puts them back. */
export function rewriteForm(current: { title: string; body: string }, text: Text): string {
  return `<form class="rewrite">
    <p class="hint">${escapeHTML(text.t("panel.rewriteHelp"))}</p>
    <div class="field">
      <label for="rewrite-title">${escapeHTML(text.t("form.title"))}</label>
      <input id="rewrite-title" name="title" value="${escapeAttr(current.title)}" maxlength="200" required />
    </div>
    <div class="field">
      <label for="rewrite-body">${escapeHTML(text.t("form.body"))}</label>
      <textarea id="rewrite-body" name="body" rows="6">${escapeHTML(current.body)}</textarea>
    </div>
    <div class="settings-actions">
      <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button>
      <button class="button ghost" type="button" data-rewrite-cancel>${escapeHTML(text.t("action.cancel"))}</button>
    </div>
  </form>`;
}

/** Who may act here, what each of them is, and where. */
export function peopleScreen(
  people: {
    id: number;
    username: string;
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
        lead: avatar(p.username, p.avatar),
        name: p.username,
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
          ${iconButton("edit", text.t("action.edit"), `data-edit-person="${p.id}" data-username="${escapeAttr(p.username)}"`)}
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
        ${link("https://support.codigosh.com/codigosh/lasthelp", icons.comment(15), text.t("about.support"))}
        ${link("https://opencollective.com/codigosh", icons.heart(15), text.t("about.donate"))}
      </nav>
    </div>`;
}

/** What /api/update says. */
export interface UpdateStatus {
  current: string;
  latest?: string;
  available?: boolean;
  /** Running in a container: updated by pulling an image, never from here. */
  container?: boolean;
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
  if (status.container) {
    // Lastboard's Docker notice: what manages this installation, and whether
    // there is a newer image to pull.
    return `<div class="about-docker" role="status">
      ${icons.info(20)}
      <div>
        <h4>${escapeHTML(text.t("update.dockerMode"))}</h4>
        <p>${escapeHTML(text.t("update.dockerDesc"))}<br>${
          status.available
            ? `<strong>${escapeHTML(text.t("update.newVersion"))} (${escapeHTML(status.latest ?? "")})</strong>`
            : escapeHTML(text.t("update.upToDateDocker"))
        }</p>
      </div>
    </div>`;
  }
  if (status.available) {
    // Lastboard's card: the badge whose dot pulses, the version beside it, and
    // one button. What is new is read in the dialog it opens, where reading it
    // can still change a mind.
    return `<div class="about-new">
      <span class="about-new-glow" aria-hidden="true"></span>
      <div class="about-new-head">
        <span class="about-badge">${escapeHTML(text.t("update.badge"))}</span>
        <span class="about-new-version">${escapeHTML(status.latest ?? "")}</span>
      </div>
      <button type="button" class="button primary" data-update>${escapeHTML(text.t("update.install"))}</button>
    </div>`;
  }
  if (status.latest || status.reason === "error.update_none") {
    return `<p class="about-state ok" role="status">${icons.done(18)}<span>${escapeHTML(text.t("update.upToDate"))}</span></p>`;
  }
  // It could not look, and says why: an empty answer where a reason belongs
  // reads as "you are up to date", which it must never say by accident.
  return `<p class="about-state${status.reason === "error.update_unreachable" ? " warn" : ""}" role="status">${icons.info(16)}<span>${escapeHTML(
    text.t(status.reason ?? "error.update_unreachable"),
  )}</span></p>`;
}

/** Lastboard's "New update!" beside Info in the menu. */
export function sideUpdateBadge(text: Text): string {
  return `<span class="side-update">${escapeHTML(text.t("update.badge"))}</span>`;
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
