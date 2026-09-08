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
export function landing(
  orgs: OrganizationSummary[],
  isAdmin: boolean,
  text: Text,
  instance?: { name?: string; logo?: string },
): string {
  // A mark and a sentence, not a paragraph of prose: three lines that each say
  // one true thing about how this works.
  const point = (icon: string, words: string) =>
    `<li><span class="point-icon">${icon}</span><span>${escapeHTML(words)}</span></li>`;

  const hero = `<section class="hero">
    <span class="hero-mark">${brandMark(instance?.logo, 44)}</span>
    <h1>${escapeHTML(instance?.name || text.t("app.name"))}</h1>
    <p class="lead">${escapeHTML(text.t("app.tagline"))}</p>
    ${
      orgs.length === 0 && isAdmin
        ? `<div class="hero-actions">
             <a class="button primary" href="/panel/new">${escapeHTML(text.t("landing.configure"))}</a>
           </div>`
        : ""
    }
  </section>
  <ul class="points">
    ${point(icons.person(18), text.t("landing.pointNoAccount"))}
    ${point(icons.idea(18), text.t("landing.pointVotes"))}
    ${point(icons.open(18), text.t("landing.pointForge"))}
  </ul>`;

  if (orgs.length === 0) {
    return `<div class="page">${hero}
      ${emptyState(text.t(isAdmin ? "landing.emptyOperator" : "portal.empty"), icons.organization(28))}
    </div>`;
  }

  return `<div class="page">${hero}
    <section>
      ${sectionHead(text.t("shell.organizations"), text.number(orgs.length))}
      ${organizationList(orgs, text)}
    </section>
  </div>`;
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
  return `<div class="section-head">
    <h2>${escapeHTML(title)}</h2>
    ${count !== undefined ? `<span class="count-chip">${escapeHTML(count)}</span>` : ""}
  </div>`;
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
    <section>
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
    return `<h1>${escapeHTML(text.t("shell.overview"))}</h1>
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

  return `<h1>${escapeHTML(text.t("shell.overview"))}</h1>
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


/**
 * The update panel.
 *
 * It says why it cannot check as plainly as it says there is something to
 * install. An empty answer where a reason belongs reads as "you are up to
 * date", which is the one thing it must never say by accident.
 */
/** The beta switch, which decides what "latest" means for this installation. */
export function channelSwitch(beta: boolean, text: Text): string {
  return `<li class="bento-card">
    <div class="card-head">${icons.update()}<h3>${escapeHTML(text.t("update.channel"))}</h3></div>
    <span class="card-sub">${escapeHTML(text.t("update.channelHelp"))}</span>
    <label class="check">
      <input type="checkbox" data-beta ${beta ? "checked" : ""} />
      <span>${escapeHTML(text.t("update.beta"))}</span>
    </label>
  </li>`;
}

export function updatePanel(
  status: {
    current: string;
    latest?: string;
    available?: boolean;
    notes?: string;
    reason?: string;
  },
  text: Text,
): string {
  // A card like every other card, rather than three loose paragraphs under one.
  // What is running is the number somebody came for, so it is the number: the
  // version at the size of a fact, and under it what there is to do about it.
  const state = status.reason
    ? `<p class="notice">${icons.info()}<span>${escapeHTML(text.t(status.reason))}</span></p>`
    : status.available
      ? `<p class="available">${escapeHTML(text.t("update.available", { version: status.latest ?? "" }))}</p>
         ${status.notes ? `<pre class="notes">${escapeHTML(status.notes)}</pre>` : ""}
         <button class="button primary with-icon" data-update>${icons.download()}<span>${escapeHTML(text.t("update.install"))}</span></button>
         <p class="update-note" role="status" hidden></p>`
      : `<p class="hint">${icons.check(14)} ${escapeHTML(text.t("update.upToDate"))}</p>`;

  return `<li class="bento-card">
    <div class="card-head">${icons.info()}<h3>${escapeHTML(text.t("about.version"))}</h3></div>
    <p class="running fact">${escapeHTML(status.current)}</p>
    ${state}
  </li>`;
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

  return `<h2>${escapeHTML(text.t("panel.settings"))}</h2>
    <ul class="bento wide">${forgeCard}${address}${channelCard}</ul>`;
}

/** The team's triage controls on one item: reply, and merge a duplicate away. */
export function triageControls(text: Text): string {
  return `
    <section class="triage">
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
      : `<ul class="items">${items.map((i) => itemRow(i, text)).join("")}</ul>`;

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
  return `<section class="signin">
    <header class="signin-head">
      ${brandMark(instance?.logo, 56)}
      <h1>${escapeHTML(instance?.name || text.t("app.name"))}</h1>
    </header>
    <form class="auth">
      <div class="wizard-card">
        ${field({ id: "auth-username", name: "username", label: text.t("auth.username"), required: true, autocomplete: "username" }, text)}
        ${field({ id: "auth-password", name: "password", label: text.t("auth.password"), type: "password", required: true, autocomplete: "current-password" }, text)}
      </div>
      <p class="form-error" role="alert" hidden></p>
      <button class="button primary signin-go" type="submit">${escapeHTML(text.t("nav.signIn"))}</button>
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
  const commentQueue =
    comments.length === 0
      ? ""
      : `<h2 class="queue-head">${escapeHTML(text.t("panel.commentsWaiting", { count: comments.length }))}</h2>
        <ul class="queue">${comments
          .map(
            (c) => `<li class="bento-card" data-comment="${escapeAttr(String(c.id))}">
              <p class="meta">
                <span class="who-badge">${escapeHTML(text.t(`comment.${c.author}`))}</span>
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
      <div class="theme-choice" role="group" aria-label="${escapeAttr(text.t("shell.theme"))}">
        ${(["system", "light", "dark"] as const)
          .map(
            (value) =>
              `<button type="button" data-theme-choice="${value}" aria-pressed="${theme === value}">${escapeHTML(
                text.t(
                  value === "system"
                    ? "shell.themeSystem"
                    : value === "light"
                      ? "shell.themeLight"
                      : "shell.themeDark",
                ),
              )}</button>`,
          )
          .join("")}
      </div>
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
  instance: string;
  logo?: string;
  avatar?: string;
  content: string;
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
  const item = (href: string, label: string, icon: string, extra = ""): string => {
    const current = spec.path === href || (href !== "/panel" && spec.path.startsWith(href + "/"));
    return `<a class="side-item ${extra}" href="${escapeAttr(href)}"${current ? ' aria-current="page"' : ""}>${icon}<span>${escapeHTML(label)}</span></a>`;
  };

  const group = (label: string, items: string): string =>
    items.trim() === "" ? "" : `<p class="side-label">${escapeHTML(label)}</p>${items}`;

  return `<div class="admin">
    <aside class="side">
      <a class="brand" href="/">
        ${brandMark(spec.logo, 26)}
        <span>${escapeHTML(spec.instance)}</span>
      </a>
      <nav aria-label="${escapeAttr(text.t("nav.panel"))}">
        ${group(
          text.t("shell.groupWork"),
          item("/panel", text.t("shell.overview"), icons.overview()) +
            // The daily work, and the screen this panel did not have: what is
            // waiting, what is open and what was closed, across every board
            // this account has a role in.
            item("/panel/reports", text.t("shell.reports"), icons.queue()) +
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
                item("/panel/updates", text.t("shell.updates"), icons.update()) +
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
        <div class="side-who">
          ${avatar(spec.who, spec.avatar)}
          <span class="who">${escapeHTML(spec.who)}</span>
        </div>
        <button class="link" data-signout>${escapeHTML(text.t("nav.signOut"))}</button>
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
  // The size travels as a variable: the class sets one, and a width attribute
  // does not beat a stylesheet.
  return `<img class="brand-logo" style="--mark-size:${size}px" src="${escapeAttr(src)}" alt=""
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
  series: { day: number; reports: number }[];
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
                <span class="bar-track"><span class="bar-fill" style="inline-size: ${Math.round((n / most) * 100)}%"></span></span>
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
      <h1>${escapeHTML(text.t("shell.overview"))}</h1>
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
  board?: { org: string; repo: string; name: string; organization: string };
}

export interface ReportsData {
  items: ReportRow[];
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

export function reportsScreen(data: ReportsData, filters: ReportFilters, text: Text): string {
  const counts = data.counts ?? { moderation: {}, status: {}, kind: {} };
  const totalOf = (of: Record<string, number>) => Object.values(of).reduce((a, b) => a + b, 0);

  // A tab carries its own number. The point of this screen is knowing there are
  // three things waiting without having to go and look.
  const tab = (attr: string, value: string, label: string, count?: number) =>
    `<button class="tab${(filters as unknown as Record<string, string>)[attr] === value ? " on" : ""}"
       data-${escapeAttr(attr)}="${escapeAttr(value)}">${escapeHTML(label)}${
       count === undefined ? "" : `<span class="count-chip">${escapeHTML(text.number(count))}</span>`
     }</button>`;

  const moderationTabs = [
    tab("moderation", "", text.t("board.all"), totalOf(counts.moderation)),
    tab("moderation", "pending", text.t("moderation.pending"), counts.moderation.pending ?? 0),
    tab("moderation", "published", text.t("moderation.published"), counts.moderation.published ?? 0),
    tab("moderation", "rejected", text.t("moderation.rejected"), counts.moderation.rejected ?? 0),
    tab("moderation", "spam", text.t("panel.spam"), counts.moderation.spam ?? 0),
  ].join("");

  const kindTabs = [
    tab("kind", "", text.t("board.all")),
    tab("kind", "bug", text.t("kind.bug"), counts.kind.bug ?? 0),
    tab("kind", "idea", text.t("kind.idea"), counts.kind.idea ?? 0),
  ].join("");

  const option = (value: string, label: string, selected: boolean) =>
    `<option value="${escapeAttr(value)}"${selected ? " selected" : ""}>${escapeHTML(label)}</option>`;

  const statusSelect = `<select data-status aria-label="${escapeAttr(text.t("panel.status"))}">
    ${option("", text.t("panel.status"), filters.status === "")}
    ${everyStatus
      .filter((s) => (counts.status[s] ?? 0) > 0 || filters.status === s)
      .map((s) => option(s, text.t(`status.${s}`), filters.status === s))
      .join("")}
  </select>`;

  const boardSelect =
    data.boards.length < 2
      ? ""
      : `<select data-board aria-label="${escapeAttr(text.t("portal.boards"))}">
          ${option("", text.t("portal.boards"), filters.board === "")}
          ${data.boards
            .map((b) =>
              option(`${b.org}/${b.repo}`, `${b.organization} / ${b.name}`, filters.board === `${b.org}/${b.repo}`),
            )
            .join("")}
        </select>`;

  const row = (r: ReportRow) => {
    const board = r.board
      ? `<span class="chip">${icons.repository(12)}<span>${escapeHTML(r.board.name)}</span></span>`
      : "";
    return `<li class="report" data-id="${escapeAttr(r.public_id)}">
      <a class="item" href="/i/${escapeAttr(r.public_id)}">
        ${
          r.kind === "idea"
            ? `<span class="score"><span class="count">${escapeHTML(text.number(r.votes))}</span>
                 <span class="label">${escapeHTML(text.t("item.votesLabel"))}</span></span>`
            : `<span class="score mark kind-bug" aria-hidden="true">${icons.bug(20)}</span>`
        }
        <span class="item-main">
          <span class="item-title">${escapeHTML(r.title)}</span>
          <span class="item-meta">
            ${board}
            <span class="kind kind-${escapeAttr(r.kind)}">
              ${r.kind === "bug" ? icons.bug(13) : icons.idea(13)}
              <span class="kind-name">${escapeHTML(text.t(`kind.${r.kind}`))}</span>
            </span>
            <span class="when">${escapeHTML(text.date(r.created_at))}</span>
          </span>
        </span>
        ${stateChip(r, text)}
      </a>
      ${
        // The decision, where the thing to decide about is. Only on what is
        // actually waiting: three buttons on a report published last March are
        // three ways to undo somebody's work by mistake.
        r.moderation === "pending"
          ? `<div class="row-actions">
               <button class="button primary with-icon" data-act="published">${icons.check()}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
               <button class="button with-icon" data-act="rejected">${icons.close()}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
               <button class="button danger with-icon" data-act="spam">${icons.warning()}<span>${escapeHTML(text.t("panel.spam"))}</span></button>
             </div>`
          : ""
      }
    </li>`;
  };

  return `<div class="screen-head">
      <h1>${escapeHTML(text.t("shell.reports"))}</h1>
    </div>
    <div class="controls">
      <div class="tabs">${moderationTabs}</div>
      <div class="tabs">${kindTabs}</div>
      ${statusSelect}
      ${boardSelect}
      <label class="search-wrap">
        ${icons.search(16)}
        <input class="search" type="search" data-q value="${escapeAttr(filters.q)}"
               placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(text.t("board.search"))}" />
      </label>
    </div>
    ${
      data.items.length === 0
        ? emptyState(text.t("reports.empty"), icons.queue(28))
        : `<ul class="items reports">${data.items.map(row).join("")}</ul>`
    }`;
}

export function profileScreen(
  me: { username: string; name?: string; avatar?: string; admin: boolean },
  text: Text,
): string {
  return `<h1>${escapeHTML(text.t("shell.profile"))}</h1>

    <div class="identity">
      ${imagePicker(
        {
          what: "avatar",
          current: me.avatar,
          initials: me.name || me.username,
          label: text.t("profile.picture"),
          big: true,
        },
        text,
      )}
      <div>
        <div class="name">${escapeHTML(me.name || me.username)}</div>
        <div class="card-sub">
          <span class="tag">${escapeHTML(me.username)}</span>
          ${me.admin ? `<span class="chip accent">${escapeHTML(text.t("people.administrator"))}</span>` : ""}
        </div>
      </div>
    </div>

    <ul class="bento">
      <li class="bento-card">
        <div class="card-head">${icons.person()}<h3>${escapeHTML(text.t("profile.name"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("profile.nameHelp"))}</span>
        <form class="profile-name">
          ${field({ id: "display-name", name: "display_name", label: text.t("profile.name"), value: me.name ?? "", example: "Alex" }, text)}
          <p class="form-error" role="alert" hidden></p>
          <div class="wizard-actions"><span class="spacer"></span>
            <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
        </form>
      </li>

      <li class="bento-card">
        <div class="card-head">${icons.warning()}<h3>${escapeHTML(text.t("profile.password"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("profile.passwordHelp"))}</span>
        <form class="profile-password">
          ${field({ id: "current-password", name: "current", label: text.t("profile.current"), type: "password", required: true, autocomplete: "current-password" }, text)}
          ${field({ id: "new-password", name: "new", label: text.t("profile.new"), type: "password", required: true, autocomplete: "new-password" }, text)}
          ${field({ id: "new-password-again", name: "again", label: text.t("auth.passwordAgain"), type: "password", required: true, autocomplete: "new-password" }, text)}
          <p class="form-error" role="alert" hidden></p>
          <div class="wizard-actions"><span class="spacer"></span>
            <button class="button primary" type="submit">${escapeHTML(text.t("profile.change"))}</button></div>
        </form>
      </li>
    </ul>`;
}

/** Language and look, which belong to the reader rather than to the server. */
export function appearanceScreen(
  languages: { code: string; name: string }[],
  language: string,
  theme: string,
  text: Text,
): string {
  return `<h1>${escapeHTML(text.t("shell.appearance"))}</h1>
    <p class="lead">${escapeHTML(text.t("appearance.lead"))}</p>
    <div class="wizard-card">
      ${choiceField({
        id: "appearance-language",
        name: "language",
        label: text.t("shell.language"),
        options: languages.map((l) => [l.code, l.name] as [string, string]),
        selected: language,
      })}
      <div class="field">
        <label>${escapeHTML(text.t("shell.theme"))}</label>
        ${themeChoice(theme, text)}
      </div>
    </div>`;
}

function themeChoice(theme: string, text: Text): string {
  return `<div class="theme-choice" role="group" aria-label="${escapeAttr(text.t("shell.theme"))}">
    ${(["system", "light", "dark"] as const)
      .map(
        (value) =>
          `<button type="button" data-theme-choice="${value}" aria-pressed="${theme === value}">${escapeHTML(
            text.t(
              value === "system"
                ? "shell.themeSystem"
                : value === "light"
                  ? "shell.themeLight"
                  : "shell.themeDark",
            ),
          )}</button>`,
      )
      .join("")}
  </div>`;
}

/** Everything about this installation that an operator chose rather than typed. */
export function personalizationScreen(
  instance: { name: string; logo: string; accent: string },
  text: Text,
): string {
  return `<h1>${escapeHTML(text.t("shell.personalization"))}</h1>
    <p class="lead">${escapeHTML(text.t("personalization.lead"))}</p>

    <form class="instance-settings">
      <div class="wizard-card">
        ${field({ id: "instance-name", name: "name", label: text.t("setup.instanceName"), hint: text.t("setup.instanceNameHelp"), value: instance.name, example: "Acme" }, text)}

        <div class="field">
          <label>${escapeHTML(text.t("personalization.logo"))}</label>
          ${imagePicker(
            { what: "logo", current: instance.logo, initials: instance.name || "L", label: text.t("personalization.logo") },
            text,
          )}
          <p class="hint">${escapeHTML(text.t("personalization.logoHelp"))}</p>
        </div>

        <div class="field">
          <label for="accent">${escapeHTML(text.t("personalization.accent"))}</label>
          <div class="accent-row">
            <input type="color" id="accent" name="accent" value="${escapeAttr(instance.accent || "#e8520e")}" />
            <button type="button" class="button ghost" data-reset-accent>${escapeHTML(text.t("personalization.accentReset"))}</button>
          </div>
          <p class="hint">${escapeHTML(text.t("personalization.accentHelp"))}</p>
        </div>
      </div>
      <p class="form-error" role="alert" hidden></p>
      <div class="wizard-actions"><span class="spacer"></span>
        <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
    </form>`;
}

/** The organisations, as cards. */
export function organizationsScreen(
  orgs: OrganizationSummary[],
  may: { canAdd: boolean; canManage: (slug: string) => boolean; canDelete: boolean },
  text: Text,
): string {
  const cards = orgs
    .map(
      (o) => `<li class="bento-card">
        <a class="card-link" href="/panel/${escapeAttr(o.slug)}">
          <div class="card-head">${icons.organization()}<h3>${escapeHTML(o.name)}</h3></div>
          <span class="card-sub">/${escapeHTML(o.slug)}</span>
        </a>
        ${
          may.canManage(o.slug)
            ? `<div class="card-actions">
                 ${iconButton("edit", text.t("action.edit"), `data-edit-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`)}
                 ${may.canDelete ? iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`) : ""}
               </div>`
            : ""
        }
      </li>`,
    )
    .join("");

  return `<div class="screen-head">
      <h1>${escapeHTML(text.t("shell.organizations"))}</h1>
      ${may.canAdd ? actionButton("add", text.t("panel.newOrganization"), 'data-add-org data-open-modal="org-modal"', "primary") : ""}
    </div>
    <p class="lead">${escapeHTML(text.t("organizations.lead"))}</p>
    ${orgs.length === 0 ? `${emptyState(text.t("panel.firstStep"), icons.organization(28))}` : `<ul class="bento">${cards}</ul>`}`;
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
  const cards = repos
    .map(
      (r) => `<li class="bento-card">
        <a class="card-link" href="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}/queue">
          <div class="card-head">${icons.repository()}<h3>${escapeHTML(r.name)}</h3></div>
          <span class="card-sub">/${escapeHTML(org.slug)}/${escapeHTML(r.slug)}</span>
          <span class="card-sub">${[
            r.accepts_bugs ? escapeHTML(text.t("repo.acceptsBugs")) : "",
            r.accepts_ideas ? escapeHTML(text.t("repo.acceptsIdeas")) : "",
          ]
            .filter(Boolean)
            .join(" · ")}</span>
        </a>
        ${
          canManage
            ? `<div class="card-actions">
                 ${iconButton("open", text.t("shell.publicBoard"), `data-open="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}"`)}
                 ${iconButton("edit", text.t("action.edit"), `data-edit-repo="${escapeAttr(r.slug)}"`)}
                 ${canDelete ? iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-repo="${escapeAttr(r.slug)}" data-name="${escapeAttr(r.name)}"`) : ""}
               </div>`
            : ""
        }
      </li>`,
    )
    .join("");

  return `<p class="crumb"><a href="/panel/organizations">${escapeHTML(text.t("shell.organizations"))}</a></p>
    <div class="screen-head">
      <h1>${escapeHTML(org.name)}</h1>
      ${canManage ? actionButton("add", text.t("panel.newRepository"), 'data-add-repo data-open-modal="repo-modal"', "primary") : ""}
    </div>
    <p class="lead"><span class="tag">/${escapeHTML(org.slug)}</span>
      ${org.forge && org.forge !== "none" ? `<span class="chip accent">${escapeHTML(org.forge)}</span>` : ""}</p>

    ${repos.length === 0 ? `${emptyState(text.t("organizations.noRepositories"), icons.repository(28))}` : `<ul class="bento repositories">${cards}</ul>`}`;
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
    .map(
      (p) => `<li class="bento-card person-card" data-person="${p.id}">
        <div class="person-head">
          ${avatar(p.name || p.username, p.avatar)}
          <div>
            <div class="name">${escapeHTML(p.name || p.username)}</div>
            <div class="card-sub"><span class="tag">${escapeHTML(p.username)}</span></div>
          </div>
        </div>
        <div class="person-tags">
          ${p.admin ? `<span class="chip accent">${escapeHTML(text.t("people.administrator"))}</span>` : ""}
          ${p.disabled ? `<span class="chip">${escapeHTML(text.t("people.disabled"))}</span>` : ""}
          ${
            p.grants.length === 0 && !p.admin
              ? `<span class="chip">${escapeHTML(text.t("people.noRoles"))}</span>`
              : p.grants
                  .map(
                    (g) =>
                      `<span class="chip">${escapeHTML(text.t(`role.${g.role}`))} · ${escapeHTML(g.where ?? "")}</span>`,
                  )
                  .join("")
          }
        </div>
        <div class="card-actions">
          ${iconButton("person", text.t("people.roles"), `data-roles-person="${p.id}" data-username="${escapeAttr(p.username)}"`)}
          ${iconButton("edit", text.t("action.edit"), `data-edit-person="${p.id}" data-name="${escapeAttr(p.name)}" data-username="${escapeAttr(p.username)}"`)}
          ${iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-person="${p.id}" data-name="${escapeAttr(p.username)}"`)}
        </div>
      </li>`,
    )
    .join("");

  return `<div class="screen-head">
      <h1>${escapeHTML(text.t("shell.people"))}</h1>
      ${actionButton("add", text.t("people.add"), 'data-open-modal="person-modal"', "primary")}
    </div>
    <p class="lead">${escapeHTML(text.t("people.lead"))}</p>
    <ul class="bento wide">${rows}</ul>`;
}

/**
 * Backups: one file out, one file in.
 *
 * Restoring is deliberately the noisier half. Taking a copy is safe and can be
 * done at any time; putting one back replaces everything anybody has written
 * since, which is why it says so and asks again.
 */
export function backupScreen(text: Text): string {
  return `<h1>${escapeHTML(text.t("shell.backups"))}</h1>
    <p class="lead">${escapeHTML(text.t("backups.lead"))}</p>

    <ul class="bento">
      <li class="bento-card">
        <div class="card-head">${icons.download()}<h3>${escapeHTML(text.t("backups.take"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("backups.help"))}</span>
        <p><a class="button primary with-icon" href="/api/backup" download>${icons.download()}<span>${escapeHTML(text.t("backups.download"))}</span></a></p>
      </li>

      <li class="bento-card">
        <div class="card-head">${icons.upload()}<h3>${escapeHTML(text.t("backups.restore"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("backups.restoreHelp"))}</span>
        <p class="notice">${icons.warning()} ${escapeHTML(text.t("backups.restoreWarning"))}</p>
        <input type="file" accept=".db,application/octet-stream" id="restore-file" hidden />
        ${actionButton("upload", text.t("backups.chooseFile"), "data-pick-restore", "danger")}
        <p class="restore-note" hidden></p>
      </li>
    </ul>`;
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
 * About: what the server is doing, for the person who has to keep it running.
 *
 * It used to show the version, the commit hash, the schema number and the name
 * of the installation. The name is a setting and lives on the Personalization
 * screen; the commit answers a question nobody was asking on this page. What an
 * operator opens this for is whether the thing is healthy — how long it has
 * been up, how big the database has grown, whether the disk under it is
 * filling, what it costs in memory.
 *
 * The schema number stays, at the bottom and quietly: it is the one number that
 * matters when a backup will not restore.
 */
export function aboutScreen(facts: ServerFacts, text: Text): string {
  const card = (label: string, value: string, icon: string, sub = "") =>
    `<li class="bento-card">
      <div class="card-head">${icon}<h3>${escapeHTML(label)}</h3></div>
      <strong class="fact">${escapeHTML(value)}</strong>
      ${sub ? `<span class="card-sub">${escapeHTML(sub)}</span>` : ""}
    </li>`;

  const cards: string[] = [];

  cards.push(
    card(
      text.t("about.version"),
      facts.version || "—",
      icons.info(),
      facts.os ? `${facts.os} · ${facts.go ?? ""}`.trim() : "",
    ),
  );

  if (facts.uptime_seconds !== undefined) {
    cards.push(
      card(
        text.t("about.uptime"),
        text.since(facts.uptime_seconds),
        icons.queue(),
        facts.started_at !== undefined ? text.date(facts.started_at) : "",
      ),
    );
  }

  if (facts.database_bytes !== undefined) {
    cards.push(card(text.t("about.database"), text.bytes(facts.database_bytes), icons.backup()));
  }

  if (facts.disk_free_bytes !== undefined) {
    // Free first, and the total underneath: the number that decides whether to
    // do something today is how much is left, not how big the disk is.
    cards.push(
      card(
        text.t("about.disk"),
        text.bytes(facts.disk_free_bytes),
        icons.download(),
        facts.disk_total_bytes !== undefined
          ? text.t("about.diskOf", { total: text.bytes(facts.disk_total_bytes) })
          : "",
      ),
    );
  }

  if (facts.memory_bytes !== undefined) {
    cards.push(card(text.t("about.memory"), text.bytes(facts.memory_bytes), icons.overview()));
  }

  return `<h1>${escapeHTML(text.t("shell.about"))}</h1>
    <p class="lead">${escapeHTML(text.t("about.lead"))}</p>
    <ul class="bento">${cards.join("")}</ul>
    <p class="hint">${escapeHTML(text.t("about.source"))}${
      facts.schema !== undefined
        ? ` ${escapeHTML(text.t("about.schemaIs", { version: text.number(facts.schema) }))}`
        : ""
    }</p>`;
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
