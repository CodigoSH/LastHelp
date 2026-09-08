/**
 * The markup, as pure functions.
 *
 * They take their text as a parameter rather than reaching for the i18n
 * singleton, which is what lets them be tested without a browser: importing the
 * singleton drags in fetch and localStorage and the whole file stops counting
 * as testable.
 */
import type { ItemDetail, ItemSummary, OrganizationSummary, RepositorySummary } from "../api";
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";
import { actionButton, iconButton } from "./modal";

export { modal, actionButton, iconButton } from "./modal";
export { field, choiceField } from "./wizard";
import { choiceField, exampleFor, field, wizard } from "./wizard";

/** Everything a template needs to say something. */
export interface Text {
  t(key: string, params?: Record<string, string | number>): string;
  date(value: number): string;
  number(value: number): string;
}

/**
 * The first screen anybody sees, signed in or not.
 *
 * It used to be one grey sentence — "nothing has been set up here yet" — on an
 * otherwise blank page. Nothing said what this is, and an operator who had just
 * installed it read that, correctly, as an unfinished product. A page that
 * admits it is empty before it says what it is has the order backwards.
 *
 * So: what this is, then what there is. An administrator looking at an empty
 * one is offered the way out; a visitor is told plainly, without being sent to
 * a form that would refuse them.
 */
export function landing(orgs: OrganizationSummary[], isAdmin: boolean, text: Text): string {
  const hero = `<section class="hero">
    <h1>LastHelp</h1>
    <p class="lead">${escapeHTML(text.t("app.tagline"))}</p>
    ${
      orgs.length === 0 && isAdmin
        ? `<div class="hero-actions">
             <a class="button primary" href="/panel/new">${escapeHTML(text.t("landing.configure"))}</a>
           </div>`
        : ""
    }
    <ul class="hero-points">
      <li><span>${escapeHTML(text.t("landing.pointNoAccount"))}</span></li>
      <li><span>${escapeHTML(text.t("landing.pointVotes"))}</span></li>
      <li><span>${escapeHTML(text.t("landing.pointForge"))}</span></li>
    </ul>
  </section>`;

  if (orgs.length === 0) {
    return `${hero}<p class="empty">${escapeHTML(
      text.t(isAdmin ? "landing.emptyOperator" : "portal.empty"),
    )}</p>`;
  }

  return `${hero}${organizationList(orgs, text)}`;
}

export function organizationList(orgs: OrganizationSummary[], text: Text): string {
  if (orgs.length === 0) return `<p class="empty">${escapeHTML(text.t("portal.empty"))}</p>`;

  return `<ul class="cards">${orgs
    .map(
      (o) => `<li><a class="card" href="/${escapeAttr(o.slug)}">
        <span class="card-title">${escapeHTML(o.name)}</span>
        <span class="card-sub">/${escapeHTML(o.slug)}</span>
      </a></li>`,
    )
    .join("")}</ul>`;
}

export function repositoryList(
  org: OrganizationSummary,
  repos: RepositorySummary[],
  text: Text,
): string {
  const header = `<h1>${escapeHTML(org.name)}</h1>`;
  if (repos.length === 0) return `${header}<p class="empty">${escapeHTML(text.t("portal.empty"))}</p>`;

  return `${header}<ul class="cards">${repos
    .map(
      (r) => `<li><a class="card" href="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}">
        <span class="card-title">${escapeHTML(r.name)}</span>
        <span class="card-sub">${[
          r.accepts_bugs ? escapeHTML(text.t("kind.bug")) : "",
          r.accepts_ideas ? escapeHTML(text.t("kind.idea")) : "",
        ]
          .filter(Boolean)
          .join(" · ")}</span>
      </a></li>`,
    )
    .join("")}</ul>`;
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
      <p class="empty">${escapeHTML(text.t("overview.noRoles"))}</p>`;
  }

  const cards = memberships
    .map((m) => {
      const href = m.repository
        ? `/${m.organization}/${m.repository}/queue`
        : `/panel/${m.organization}`;
      return `<li class="bento-card">
        <a class="card-link" href="${escapeAttr(href)}">
          <div class="card-head">${m.repository ? icons.repository() : icons.organization()}<h3>${escapeHTML(m.name ?? "")}</h3></div>
          <span class="card-sub"><span class="tag">${escapeHTML(text.t(`role.${m.role}`))}</span></span>
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
    <label class="switch">
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
  const running = `<p class="running">${escapeHTML(text.t("update.running", { version: status.current }))}</p>`;

  if (status.reason) {
    return `${running}
      <p class="notice">${escapeHTML(text.t(status.reason))}</p>`;
  }

  if (!status.available) {
    return `${running}
      <p class="hint">${escapeHTML(text.t("update.upToDate"))}</p>`;
  }

  return `${running}
    <p class="available">${escapeHTML(text.t("update.available", { version: status.latest ?? "" }))}</p>
    ${status.notes ? `<pre class="notes">${escapeHTML(status.notes)}</pre>` : ""}
    <button class="button primary" data-update>${escapeHTML(text.t("update.install"))}</button>
    <p class="update-note" role="status" hidden></p>`;
}

/** One organisation in the panel: its boards, and a form to add another. */
export function panelOrganization(
  org: OrganizationSummary,
  repos: RepositorySummary[],
  text: Text,
): string {
  const list =
    repos.length === 0
      ? `<p class="empty">${escapeHTML(text.t("portal.empty"))}</p>`
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
              ${field({ id: "chan-url", name: "url", label: text.t("panel.address2"), example: "https://ntfy.sh/lasthelp" }, text)}
              ${field({ id: "chan-token", name: "token", label: text.t("panel.token"), type: "password" }, text)}
            </div>
            <div class="channel-mail" hidden>
              ${field({ id: "chan-to", name: "to", label: text.t("panel.mailTo"), type: "email", example: "equipo@codigosh.com" }, text)}
            </div>
            <p class="form-error" role="alert" hidden></p>
            <div class="wizard-actions"><span class="spacer"></span>
              <button class="button primary" type="submit">${escapeHTML(text.t("action.save"))}</button></div>
            <p class="channel-note" role="status" hidden></p>
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
      <p class="triage-note" role="status" hidden></p>
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

  const list =
    items.length === 0
      ? `<p class="empty">${escapeHTML(text.t("board.empty"))}</p>`
      : `<ul class="items">${items.map((i) => itemRow(i, text)).join("")}</ul>`;

  return `
    <header class="board-head">
      <div>
        <p class="crumb"><a href="/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a></p>
        <h1>${escapeHTML(repo.name)}</h1>
      </div>
      <div class="head-actions">
        <a class="button ghost" href="/feed/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/atom.xml">${escapeHTML(text.t("nav.feed"))}</a>
        <a class="button ghost" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/roadmap">${escapeHTML(text.t("nav.roadmap"))}</a>
        <a class="button primary with-icon" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/new">${icons.add()}<span>${escapeHTML(text.t("board.report"))}</span></a>
      </div>
    </header>
    <div class="controls">
      <div class="tabs">${tabs}</div>
      <div class="sorts">${sorts}</div>
      <input class="search" type="search" value="${escapeAttr(filters.query)}"
             placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(text.t("board.search"))}" />
    </div>
    ${list}`;
}

/**
 * One report in the list.
 *
 * The kind is an icon rather than a word repeated on every row: twenty rows
 * saying "Bug" is twenty words nobody reads, and the shape is faster to scan.
 * The count of votes stays a word, because a number on its own beside a title
 * is a number nobody can name.
 */
function itemRow(item: ItemSummary, text: Text): string {
  // "Votes: 3", not an icon and a bare 3. A number on its own beside a title is
  // a number nobody can name, and the house writes counters as label-then-value
  // for exactly that reason. Adding the icon and dropping the word was a trade
  // that gave up the meaning for the decoration.
  const votes =
    item.kind === "idea"
      ? `<span class="votes${item.voted ? " voted" : ""}">${escapeHTML(text.t("item.votes", { count: text.number(item.votes) }))}</span>`
      : "";

  return `<li><a class="item" href="/i/${escapeAttr(item.public_id)}">
    <span class="kind kind-${escapeAttr(item.kind)}" title="${escapeAttr(text.t(`kind.${item.kind}`))}">
      ${item.kind === "bug" ? icons.bug(14) : icons.idea(14)}
      <span class="kind-name">${escapeHTML(text.t(`kind.${item.kind}`))}</span>
    </span>
    <span class="item-title">${escapeHTML(item.title)}</span>
    <span class="status status-${escapeAttr(item.status)}">${escapeHTML(text.t(`status.${item.status}`))}</span>
    ${votes}
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
  return `
    <p class="crumb"><a href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}">${escapeHTML(repo.name)}</a></p>
    <h1>${escapeHTML(text.t("nav.roadmap"))}</h1>
    <p class="lead">${escapeHTML(text.t("roadmap.lead"))}</p>
    <div class="roadmap">
      ${columns
        .map(
          (c) => `<section class="column">
            <h2>${escapeHTML(text.t(`roadmap.${c.column}`))}</h2>
            ${
              c.items.length === 0
                ? `<p class="empty">${escapeHTML(text.t("roadmap.emptyColumn"))}</p>`
                : `<ul>${c.items
                    .map(
                      (i) => `<li><a href="/i/${escapeAttr(i.public_id)}">
                        <span class="kind kind-${escapeAttr(i.kind)}">${escapeHTML(text.t(`kind.${i.kind}`))}</span>
                        <span class="item-title">${escapeHTML(i.title)}</span>
                      </a></li>`,
                    )
                    .join("")}</ul>`
            }
          </section>`,
        )
        .join("")}
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
    <p class="issue-note" role="status" hidden></p>
  </div>`;
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

export function reportForm(repo: RepositorySummary, kind: string, text: Text): string {
  const choice = (value: string, label: string) =>
    `<button type="button" class="choice${kind === value ? " on" : ""}" data-choice="${escapeAttr(value)}">${escapeHTML(label)}</button>`;

  return `
    <h1>${escapeHTML(text.t("board.report"))}</h1>
    <form class="report">
      <div class="choices">
        ${repo.accepts_bugs ? choice("bug", text.t("form.kindBug")) : ""}
        ${repo.accepts_ideas ? choice("idea", text.t("form.kindIdea")) : ""}
      </div>

      <!--
        The honeypot. Hidden from people by CSS and from assistive technology by
        aria-hidden and tabindex, so nobody who can see or hear the page is ever
        offered it; something filling in every input it finds will fill it in.
      -->
      <div class="trap" aria-hidden="true">
        <label for="report-website">Website</label>
        <input id="report-website" name="website" type="text" tabindex="-1" autocomplete="off" />
      </div>

      <label for="report-title">${escapeHTML(text.t("form.title"))}</label>
      <input id="report-title" name="title" maxlength="200" required
             placeholder="${escapeAttr(text.t("form.titlePlaceholder"))}" />

      <div class="similar" hidden></div>

      <label for="report-body">${escapeHTML(text.t("form.body"))}</label>
      <textarea id="report-body" name="body" rows="6"
                placeholder="${escapeAttr(text.t("form.bodyPlaceholder"))}"></textarea>

      ${
        // Only when there is a relay to notify with. Asking for an address the
        // server cannot use is collecting personal data for nothing.
        repo.can_notify
          ? `<label for="report-email">${escapeHTML(text.t("form.email"))}</label>
             <input id="report-email" name="email" type="email" />
             <p class="hint">${escapeHTML(text.t("form.emailHelp"))}</p>`
          : ""
      }

      <p class="form-error" role="alert" hidden></p>
      <button class="button primary" type="submit">${escapeHTML(text.t("form.send"))}</button>
    </form>`;
}

export function similarItems(items: Array<{ public_id: string; title: string }>, text: Text): string {
  if (items.length === 0) return "";
  return `<p class="similar-head">${escapeHTML(text.t("form.similar"))}</p>
    <ul>${items
      .map((i) => `<li><a href="/i/${escapeAttr(i.public_id)}">${escapeHTML(i.title)}</a></li>`)
      .join("")}</ul>
    <p class="hint">${escapeHTML(text.t("form.similarHelp"))}</p>`;
}

export function submitted(link: string, text: Text): string {
  return `
    <h1>${escapeHTML(text.t("sent.title"))}</h1>
    <p>${escapeHTML(text.t("sent.waiting"))}</p>
    <p class="keep">${escapeHTML(text.t("sent.keepLink"))}</p>
    <div class="follow-link">
      <input readonly value="${escapeAttr(link)}" aria-label="${escapeAttr(text.t("sent.copy"))}" />
      <button class="button" data-copy>${escapeHTML(text.t("sent.copy"))}</button>
    </div>`;
}

export function itemView(item: ItemDetail, text: Text): string {
  const merged = item.merged_into
    ? `<p class="notice"><a href="/i/${escapeAttr(item.merged_into)}">${escapeHTML(text.t("item.merged"))}</a></p>`
    : "";

  // Only the reporter ever sees an item that is still waiting, and they should
  // be told why it is not on the board rather than left wondering.
  const waiting = item.mine ? `<p class="notice">${escapeHTML(text.t("item.waiting"))}</p>` : "";

  const vote =
    item.kind === "idea"
      ? `<button class="button vote${item.voted ? " voted" : ""}" data-vote>
           ${escapeHTML(item.voted ? text.t("item.voted") : text.t("item.vote"))}
           <span class="count">${escapeHTML(text.number(item.votes))}</span>
         </button>`
      : "";

  const thread = item.comments
    .map(
      (c) => `<li class="comment comment-${escapeAttr(c.author)}">
        <p class="who">${escapeHTML(c.name || text.t(`comment.${c.author}`))} · ${escapeHTML(text.date(c.created_at))}</p>
        <p class="what">${escapeHTML(c.body)}</p>
      </li>`,
    )
    .join("");

  return `
    ${merged}${waiting}
    <article class="detail">
      <p class="meta">
        <span class="kind kind-${escapeAttr(item.kind)}">${escapeHTML(text.t(`kind.${item.kind}`))}</span>
        <span class="status">${escapeHTML(text.t(`status.${item.status}`))}</span>
        <span class="when">${escapeHTML(text.date(item.created_at))}</span>
      </p>
      <h1>${escapeHTML(item.title)}</h1>
      <p class="body">${escapeHTML(item.body)}</p>
      ${vote}
    </article>

    <section class="thread">
      <h2>${escapeHTML(text.t("item.comments"))}</h2>
      <ul class="comments">${thread}</ul>
      <form class="comment-form">
        <textarea name="body" rows="3" placeholder="${escapeAttr(text.t("item.commentPlaceholder"))}"></textarea>
        <button class="button" type="submit">${escapeHTML(text.t("form.send"))}</button>
        <p class="comment-sent" role="status" hidden>${escapeHTML(text.t("item.commentWaiting"))}</p>
      </form>
    </section>`;
}

/**
 * Signing in, once the instance has been claimed.
 *
 * Claiming goes through the wizard, so this no longer takes a flag for it: a
 * parameter that nothing passes is a branch nobody tests. It wears the same card
 * as the wizard steps, because a login that looks like a different product is
 * how a page reads as bolted on.
 */
export function signInForm(text: Text): string {
  return `<section class="wizard">
    <header class="wizard-head">
      <h1>${escapeHTML(text.t("nav.signIn"))}</h1>
    </header>
    <form class="auth">
      <div class="wizard-card">
        ${field({ id: "auth-username", name: "username", label: text.t("auth.username"), required: true, autocomplete: "username" }, text)}
        ${field({ id: "auth-password", name: "password", label: text.t("auth.password"), type: "password", required: true, autocomplete: "current-password" }, text)}
      </div>
      <p class="form-error" role="alert" hidden></p>
      <div class="wizard-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("nav.signIn"))}</button>
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
export function queueScreen(
  org: OrganizationSummary,
  repo: RepositorySummary,
  items: ItemSummary[],
  bodies: Map<string, string>,
  text: Text,
): string {
  const header = `<p class="crumb"><a href="/panel/${escapeAttr(org.slug)}">${escapeHTML(org.name)}</a></p>
    <div class="screen-head">
      <h1>${escapeHTML(repo.name)}</h1>
      <a class="button ghost with-icon" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}">
        ${icons.open()}<span>${escapeHTML(text.t("shell.publicBoard"))}</span></a>
    </div>
    <p class="lead">${escapeHTML(
      items.length === 0 ? text.t("queue.empty") : text.t("queue.lead", { count: items.length }),
    )}</p>`;

  if (items.length === 0) return header;

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
          <div class="actions">
            <button class="button primary with-icon" data-act="published">${icons.check()}<span>${escapeHTML(text.t("panel.approve"))}</span></button>
            <button class="button ghost with-icon" data-act="rejected">${icons.close()}<span>${escapeHTML(text.t("panel.reject"))}</span></button>
            <button class="button danger with-icon" data-act="spam">${icons.warning()}<span>${escapeHTML(text.t("panel.spam"))}</span></button>
          </div>
        </li>`,
      )
      .join("")}</ul>`;
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

function accountFields(text: Text, picture = ""): string {
  return (
    // The picture first, because it is the one optional thing here and putting
    // it after two required fields makes it look like a third.
    `<div class="field">
      <label>${escapeHTML(text.t("profile.picture"))}</label>
      <div class="identity" style="margin:0">
        ${avatar("?", picture)}
        ${actionButton("upload", text.t("setup.choosePicture"), "data-pick-avatar")}
        <input type="file" accept="image/*" id="avatar-file" hidden />
      </div>
      <p class="hint">${escapeHTML(text.t("setup.pictureHelp"))}</p>
    </div>` +
    field({
      id: "username",
      name: "username",
      label: text.t("auth.username"),
      required: true,
      autocomplete: "username",
      hint: text.t("auth.usernameHelp"),
      example: "k4wit0",
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
      example: "CodigoSH",
      required: true,
    }, text) +
    field({
      id: "org-slug",
      name: "slug",
      label: text.t("panel.address"),
      hint: text.t("panel.addressHelp"),
      example: "codigosh",
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
  return `<div class="field">
    <label>${escapeHTML(text.t("repo.accepts"))}</label>
    <p class="choices">
      <label class="choice"><input type="checkbox" name="accepts_bugs" checked /> ${escapeHTML(text.t("kind.bug"))}</label>
      <label class="choice"><input type="checkbox" name="accepts_ideas" checked /> ${escapeHTML(text.t("kind.idea"))}</label>
    </p>
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
        title: text.t("setup.languageTitle"),
        lead: text.t("setup.languageLead"),
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
        title: text.t("auth.setupTitle"),
        lead: text.t("auth.setupHelp"),
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
      title: text.t("setup.instanceTitle"),
      lead: text.t("setup.instanceLead"),
      body:
        field(
          {
            id: "instance-name",
            name: "name",
            label: text.t("setup.instanceName"),
            hint: text.t("setup.instanceNameHelp"),
            example: "CodigoSH",
          },
          text,
        ) +
        `<div class="field">
          <label>${escapeHTML(text.t("personalization.logo"))}</label>
          <div class="identity" style="margin:0">
            ${prefs.logo ? `<img class="logo-preview" src="${escapeAttr(prefs.logo)}" alt="" />` : avatar("L")}
            ${actionButton("upload", text.t("personalization.chooseLogo"), "data-pick-logo")}
            <input type="file" accept="image/*" id="logo-file" hidden />
          </div>
          <p class="hint">${escapeHTML(text.t("personalization.logoHelp"))}</p>
        </div>`,
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
        ${spec.logo ? `<img class="brand-logo" src="${escapeAttr(spec.logo)}" alt="" />` : ""}
        <span>${escapeHTML(spec.instance)}</span>
      </a>
      <nav aria-label="${escapeAttr(text.t("nav.panel"))}">
        ${group(
          text.t("shell.groupWork"),
          item("/panel", text.t("shell.overview"), icons.overview()) +
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

/** Initials when there is no picture, which is most of the time. */
export function avatar(name: string, src?: string, big = false): string {
  const initials = (name || "?")
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => [...part][0]?.toUpperCase() ?? "")
    .join("");
  return `<span class="avatar${big ? " big" : ""}" aria-hidden="true">${
    src ? `<img src="${escapeAttr(src)}" alt="" />` : escapeHTML(initials)
  }</span>`;
}

/**
 * The front page of the panel: what this installation holds, in numbers.
 *
 * Counted by the server rather than by fetching everything and counting here,
 * so a portal that has been useful for two years opens as fast as one installed
 * this morning.
 */
export function overviewScreen(
  data: {
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
  },
  text: Text,
): string {
  const stat = (value: number, label: string, icon: string, href?: string) => {
    const inner = `<span class="value">${escapeHTML(text.number(value))}</span>
      <span class="label">${icon}${escapeHTML(label)}</span>`;
    // Always <li> outside and the card inside, whether the card is a link or
    // not. Two different shapes meant one of them lost its box the moment the
    // list learned to get out of the way of the other.
    return href
      ? `<li><a class="stat" href="${escapeAttr(href)}">${inner}</a></li>`
      : `<li><div class="stat">${inner}</div></li>`;
  };

  // The label comes in as a function rather than as a prefix to glue a key
  // together from. A key built out of a variable is a key no search can follow,
  // and the scan that finds strings nobody uses reported all four moderation
  // states as dead while the screen was drawing them.
  const bars = (title: string, counts: Record<string, number>, label: (key: string) => string) => {
    const entries = Object.entries(counts).filter(([, n]) => n > 0);
    if (entries.length === 0) return "";
    const most = Math.max(...entries.map(([, n]) => n));
    return `<h2>${escapeHTML(title)}</h2>
      <div class="bars">${entries
        .sort((a, b) => b[1] - a[1])
        .map(
          ([key, n]) => `<div class="bar-row">
            <span>${escapeHTML(label(key))}</span>
            <span class="bar-track"><span class="bar-fill" style="inline-size: ${Math.round((n / most) * 100)}%"></span></span>
            <span class="count">${escapeHTML(text.number(n))}</span>
          </div>`,
        )
        .join("")}</div>`;
  };

  const total = Object.values(data.kinds).reduce((a, b) => a + b, 0);

  return `<h1>${escapeHTML(text.t("shell.overview"))}</h1>
    <p class="lead">${escapeHTML(
      total === 0 ? text.t("overview.emptyLead") : text.t("overview.lead"),
    )}</p>

    <ul class="stats">
      ${stat(total, text.t("overview.reports"), icons.repository())}
      ${stat(data.pending, text.t("panel.queue"), icons.queue())}
      ${stat(data.published, text.t("overview.published"), icons.check())}
      ${stat(data.votes, text.t("overview.votes"), icons.idea())}
      ${stat(data.organizations, text.t("shell.organizations"), icons.organization(), "/panel/organizations")}
      ${stat(data.repositories, text.t("portal.boards"), icons.repository(), "/panel/organizations")}
      ${stat(data.people, text.t("shell.people"), icons.people(), "/panel/people")}
    </ul>

    ${bars(text.t("overview.byKind"), data.kinds, (k) => text.t(`kind.${k}`))}
    ${bars(text.t("overview.byStatus"), data.statuses, (k) => text.t(`status.${k}`))}
    ${bars(text.t("overview.byModeration"), data.moderation, (k) => text.t(`moderation.${k}`))}

    ${
      data.last_report > 0
        ? `<p class="hint">${escapeHTML(text.t("overview.lastReport", { when: text.date(data.last_report) }))}</p>`
        : ""
    }`;
}

/**
 * Your own account.
 *
 * Identity first — the picture, the name, what you are — then the two things
 * that can be changed. The old version was three stacked forms with a heading
 * each, which reads as a settings file rather than as a page about a person.
 */
export function profileScreen(
  me: { username: string; name?: string; avatar?: string; admin: boolean },
  text: Text,
): string {
  return `<h1>${escapeHTML(text.t("shell.profile"))}</h1>

    <div class="identity">
      ${avatar(me.name || me.username, me.avatar, true)}
      <div>
        <div class="name">${escapeHTML(me.name || me.username)}</div>
        <div class="card-sub">
          <span class="tag">${escapeHTML(me.username)}</span>
          ${me.admin ? `<span class="tag accent">${escapeHTML(text.t("people.administrator"))}</span>` : ""}
        </div>
      </div>
      <span class="spacer"></span>
      ${actionButton("upload", text.t("profile.picture"), 'data-pick-avatar')}
      ${me.avatar ? actionButton("remove", text.t("profile.pictureClear"), "data-clear-avatar") : ""}
      <input type="file" accept="image/*" id="avatar-file" hidden />
    </div>

    <ul class="bento">
      <li class="bento-card">
        <div class="card-head">${icons.person()}<h3>${escapeHTML(text.t("profile.name"))}</h3></div>
        <span class="card-sub">${escapeHTML(text.t("profile.nameHelp"))}</span>
        <form class="profile-name">
          ${field({ id: "display-name", name: "display_name", label: text.t("profile.name"), value: me.name ?? "", example: "k4wit0" }, text)}
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
        ${field({ id: "instance-name", name: "name", label: text.t("setup.instanceName"), hint: text.t("setup.instanceNameHelp"), value: instance.name, example: "CodigoSH" }, text)}

        <div class="field">
          <label>${escapeHTML(text.t("personalization.logo"))}</label>
          <div class="identity" style="margin:0">
            ${instance.logo ? `<img class="logo-preview" src="${escapeAttr(instance.logo)}" alt="" />` : avatar(instance.name || "L")}
            ${actionButton("upload", text.t("personalization.chooseLogo"), "data-pick-logo")}
            ${instance.logo ? actionButton("remove", text.t("personalization.clearLogo"), "data-clear-logo") : ""}
            <input type="file" accept="image/*" id="logo-file" hidden />
          </div>
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
export function organizationsScreen(orgs: OrganizationSummary[], canManage: boolean, text: Text): string {
  const cards = orgs
    .map(
      (o) => `<li class="bento-card">
        <a class="card-link" href="/panel/${escapeAttr(o.slug)}">
          <div class="card-head">${icons.organization()}<h3>${escapeHTML(o.name)}</h3></div>
          <span class="card-sub">/${escapeHTML(o.slug)}</span>
        </a>
        ${
          canManage
            ? `<div class="card-actions">
                 ${iconButton("edit", text.t("action.edit"), `data-edit-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`)}
                 ${iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-org="${escapeAttr(o.slug)}" data-name="${escapeAttr(o.name)}"`)}
               </div>`
            : ""
        }
      </li>`,
    )
    .join("");

  return `<div class="screen-head">
      <h1>${escapeHTML(text.t("shell.organizations"))}</h1>
      ${canManage ? actionButton("add", text.t("panel.newOrganization"), 'data-add-org data-open-modal="org-modal"', "primary") : ""}
    </div>
    <p class="lead">${escapeHTML(text.t("organizations.lead"))}</p>
    ${orgs.length === 0 ? `<p class="empty">${escapeHTML(text.t("panel.firstStep"))}</p>` : `<ul class="bento">${cards}</ul>`}`;
}

/** One organisation: its repositories, and what can be done to it. */
export function organizationScreen(
  org: OrganizationSummary & { forge?: string },
  repos: RepositorySummary[],
  canManage: boolean,
  text: Text,
): string {
  const cards = repos
    .map(
      (r) => `<li class="bento-card">
        <a class="card-link" href="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}/queue">
          <div class="card-head">${icons.repository()}<h3>${escapeHTML(r.name)}</h3></div>
          <span class="card-sub">/${escapeHTML(org.slug)}/${escapeHTML(r.slug)}</span>
          <span class="card-sub">${[
            r.accepts_bugs ? escapeHTML(text.t("kind.bug")) : "",
            r.accepts_ideas ? escapeHTML(text.t("kind.idea")) : "",
          ]
            .filter(Boolean)
            .join(" · ")}</span>
        </a>
        ${
          canManage
            ? `<div class="card-actions">
                 ${iconButton("open", text.t("shell.publicBoard"), `data-open="/${escapeAttr(org.slug)}/${escapeAttr(r.slug)}"`)}
                 ${iconButton("edit", text.t("action.edit"), `data-edit-repo="${escapeAttr(r.slug)}"`)}
                 ${iconButton("remove", text.t("action.delete"), `class="icon-button danger" data-delete-repo="${escapeAttr(r.slug)}" data-name="${escapeAttr(r.name)}"`)}
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
      ${org.forge && org.forge !== "none" ? `<span class="tag accent">${escapeHTML(org.forge)}</span>` : ""}</p>

    ${repos.length === 0 ? `<p class="empty">${escapeHTML(text.t("organizations.noRepositories"))}</p>` : `<ul class="bento repositories">${cards}</ul>`}`;
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
          ${p.admin ? `<span class="tag accent">${escapeHTML(text.t("people.administrator"))}</span>` : ""}
          ${p.disabled ? `<span class="tag">${escapeHTML(text.t("people.disabled"))}</span>` : ""}
          ${
            p.grants.length === 0 && !p.admin
              ? `<span class="tag">${escapeHTML(text.t("people.noRoles"))}</span>`
              : p.grants
                  .map(
                    (g) =>
                      `<span class="tag">${escapeHTML(text.t(`role.${g.role}`))} · ${escapeHTML(g.where ?? "")}</span>`,
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
export function aboutScreen(
  version: { current: string; commit?: string; schema?: number },
  instance: string,
  text: Text,
): string {
  const fact = (label: string, value: string, icon: string) =>
    `<li class="bento-card">
      <div class="card-head">${icon}<h3>${escapeHTML(label)}</h3></div>
      <code class="fact-value">${escapeHTML(value)}</code>
    </li>`;

  return `<h1>${escapeHTML(text.t("shell.about"))}</h1>
    <p class="lead">${escapeHTML(text.t("about.lead"))}</p>
    <ul class="bento">
      ${fact(text.t("about.version"), version.current || "—", icons.info())}
      ${version.commit ? fact(text.t("about.commit"), version.commit, icons.repository()) : ""}
      ${version.schema !== undefined ? fact(text.t("about.schema"), String(version.schema), icons.backup()) : ""}
      ${fact(text.t("setup.instanceName"), instance, icons.organization())}
    </ul>
    <p class="hint">${escapeHTML(text.t("about.source"))}</p>`;
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
              <span class="tag">${escapeHTML(text.t(`role.${g.role}`))}</span>
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
