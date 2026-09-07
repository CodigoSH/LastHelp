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
export function panelHome(orgs: OrganizationSummary[], text: Text): string {
  const list =
    orgs.length === 0
      ? `<p class="empty">${escapeHTML(text.t("panel.firstStep"))}</p>`
      : `<ul class="cards">${orgs
          .map(
            (o) => `<li><a class="card" href="/panel/${escapeAttr(o.slug)}">
              <span class="card-title">${escapeHTML(o.name)}</span>
              <span class="card-sub">/${escapeHTML(o.slug)}</span>
            </a></li>`,
          )
          .join("")}</ul>`;

  return `
    <h1>${escapeHTML(text.t("nav.panel"))}</h1>
    ${list}
    <p class="head-actions">
      <a class="button primary" href="/panel/new">${escapeHTML(text.t("panel.newOrganization"))}</a>
    </p>`;
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
    return `<h2>${escapeHTML(text.t("update.title"))}</h2>${running}
      <p class="notice">${escapeHTML(text.t(status.reason))}</p>`;
  }

  if (!status.available) {
    return `<h2>${escapeHTML(text.t("update.title"))}</h2>${running}
      <p class="hint">${escapeHTML(text.t("update.upToDate"))}</p>`;
  }

  return `<h2>${escapeHTML(text.t("update.title"))}</h2>${running}
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
                  download>${escapeHTML(text.t("panel.export", { board: r.name }))}</a>`,
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
  text: Text,
): string {
  const forgeBlock =
    !org.forge || org.forge === "none"
      ? ""
      : `<h2>${escapeHTML(text.t("panel.connection"))}</h2>
         <form class="credentials">
           <label for="cred-token">${escapeHTML(text.t("panel.token"))}</label>
           <input id="cred-token" name="token" type="password" autocomplete="off" required />
           <p class="hint">${escapeHTML(text.t("panel.tokenHelp"))}</p>

           <label for="cred-secret">${escapeHTML(text.t("panel.webhookSecret"))}</label>
           <input id="cred-secret" name="webhook_secret" type="password" autocomplete="off" required />

           <p class="form-error" role="alert" hidden></p>
           <button class="button primary" type="submit">${escapeHTML(text.t("auth.create"))}</button>
           <p class="webhook-path" hidden></p>
         </form>`;

  const kinds = [
    ["", text.t("panel.channelNone")],
    ["ntfy", "ntfy"],
    ["gotify", "Gotify"],
    ["webhook", "Webhook"],
    ["smtp", "Email"],
  ];

  const channelBlock =
    repos.length === 0
      ? ""
      : `<h2>${escapeHTML(text.t("panel.channel"))}</h2>
         <p class="hint">${escapeHTML(text.t("panel.channelHelp"))}</p>
         <form class="channel">
           <label for="chan-repo">${escapeHTML(text.t("portal.boards"))}</label>
           <select id="chan-repo" name="repo">
             ${repos.map((r) => `<option value="${escapeAttr(r.slug)}">${escapeHTML(r.name)}</option>`).join("")}
           </select>

           <label for="chan-kind">${escapeHTML(text.t("panel.channel"))}</label>
           <select id="chan-kind" name="kind">
             ${kinds.map(([v, l]) => `<option value="${escapeAttr(v ?? "")}">${escapeHTML(l ?? "")}</option>`).join("")}
           </select>

           <div class="channel-fields" hidden>
             <label for="chan-url">${escapeHTML(text.t("panel.address2"))}</label>
             <input id="chan-url" name="url" />

             <label for="chan-token">${escapeHTML(text.t("panel.token"))}</label>
             <input id="chan-token" name="token" type="password" autocomplete="off" />
           </div>

           <div class="channel-mail" hidden>
             <label for="chan-to">${escapeHTML(text.t("auth.email"))}</label>
             <input id="chan-to" name="to" type="email" />
           </div>

           <p class="form-error" role="alert" hidden></p>
           <button class="button" type="submit">${escapeHTML(text.t("auth.create"))}</button>
           <p class="channel-note" role="status" hidden></p>
         </form>`;

  return forgeBlock + channelBlock;
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
        <a class="button" href="/feed/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/atom.xml">${escapeHTML(text.t("nav.feed"))}</a>
        <a class="button" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/roadmap">${escapeHTML(text.t("nav.roadmap"))}</a>
        <a class="button primary" href="/${escapeAttr(org.slug)}/${escapeAttr(repo.slug)}/new">${escapeHTML(text.t("board.report"))}</a>
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

function itemRow(item: ItemSummary, text: Text): string {
  const votes =
    item.kind === "idea"
      ? `<span class="votes${item.voted ? " voted" : ""}">${escapeHTML(text.t("item.votes", { count: text.number(item.votes) }))}</span>`
      : "";

  return `<li><a class="item" href="/i/${escapeAttr(item.public_id)}">
    <span class="kind kind-${escapeAttr(item.kind)}">${escapeHTML(text.t(`kind.${item.kind}`))}</span>
    <span class="item-title">${escapeHTML(item.title)}</span>
    <span class="status">${escapeHTML(text.t(`status.${item.status}`))}</span>
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
        ${field({ id: "auth-email", name: "email", label: text.t("auth.email"), type: "email", required: true, autocomplete: "username" }, text)}
        ${field({ id: "auth-password", name: "password", label: text.t("auth.password"), type: "password", required: true, autocomplete: "current-password" }, text)}
      </div>
      <p class="form-error" role="alert" hidden></p>
      <div class="wizard-actions">
        <button class="button primary" type="submit">${escapeHTML(text.t("nav.signIn"))}</button>
      </div>
    </form>
  </section>`;
}

export function queueView(items: ItemSummary[], bodies: Map<string, string>, text: Text): string {
  if (items.length === 0) return `<p class="empty">${escapeHTML(text.t("panel.empty"))}</p>`;

  return `<ul class="queue">${items
    .map(
      (i) => `<li data-id="${escapeAttr(i.public_id)}">
        <p class="meta">
          <span class="kind kind-${escapeAttr(i.kind)}">${escapeHTML(text.t(`kind.${i.kind}`))}</span>
          <span class="when">${escapeHTML(text.date(i.created_at))}</span>
        </p>
        <h3>${escapeHTML(i.title)}</h3>
        <p class="body">${escapeHTML(bodies.get(i.public_id) ?? "")}</p>
        <div class="actions">
          <button class="button primary" data-act="published">${escapeHTML(text.t("panel.approve"))}</button>
          <button class="button" data-act="rejected">${escapeHTML(text.t("panel.reject"))}</button>
          <button class="button danger" data-act="spam">${escapeHTML(text.t("panel.spam"))}</button>
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

function accountFields(text: Text): string {
  return (
    field({
      id: "email",
      name: "email",
      label: text.t("auth.email"),
      type: "email",
      required: true,
      autocomplete: "username",
      example: "you@example.com",
    }, text) +
    field({
      id: "password",
      name: "password",
      label: text.t("auth.password"),
      type: "password",
      required: true,
      autocomplete: "new-password",
      hint: text.t("auth.passwordHelp"),
    }, text)
  );
}

function organizationFields(text: Text, forge: string): string {
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

function repositoryFields(text: Text, forge: string, hasForge: boolean): string {
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
function acceptsFields(text: Text): string {
  return `<div class="field">
    <label>${escapeHTML(text.t("repo.accepts"))}</label>
    <p class="choices">
      <label class="choice"><input type="checkbox" name="accepts_bugs" checked /> ${escapeHTML(text.t("kind.bug"))}</label>
      <label class="choice"><input type="checkbox" name="accepts_ideas" checked /> ${escapeHTML(text.t("kind.idea"))}</label>
    </p>
    <p class="hint">${escapeHTML(text.t("repo.acceptsHelp"))}</p>
  </div>`;
}

/** The first run: account, organisation, board. */
export function setupWizard(step: number, text: Text, forge = "none"): string {
  const names = [
    text.t("setup.stepAccount"),
    text.t("setup.stepOrganization"),
    text.t("setup.stepBoard"),
  ];

  if (step === 1) {
    return wizard(
      {
        step: 1,
        names,
        title: text.t("auth.setupTitle"),
        lead: text.t("auth.setupHelp"),
        body: accountFields(text),
        submit: text.t("wizard.continue"),
        formClass: "setup-account",
      },
      text,
    );
  }

  if (step === 2) {
    return wizard(
      {
        step: 2,
        names,
        title: text.t("setup.orgTitle"),
        lead: text.t("setup.orgLead"),
        body: organizationFields(text, forge),
        submit: text.t("wizard.continue"),
        formClass: "setup-organization",
      },
      text,
    );
  }

  return wizard(
    {
      step: 3,
      names,
      title: text.t("setup.boardTitle"),
      lead: text.t("setup.boardLead"),
      body: repositoryFields(text, forge, forge !== "none") + acceptsFields(text),
      submit: text.t("wizard.finish"),
      formClass: "setup-repository",
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
        lead: text.t("setup.boardLead"),
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

/** The end of the first run, which has to say where the thing now lives. */
export function wizardDone(
  orgSlug: string,
  repoSlug: string,
  hasForge: boolean,
  text: Text,
): string {
  const address = `/${orgSlug}/${repoSlug}`;
  return `<section class="wizard">
    <header class="wizard-head">
      <p class="step-count">${escapeHTML(text.t("setup.doneStep"))}</p>
      <h1>${escapeHTML(text.t("setup.doneTitle"))}</h1>
      <p class="lead">${escapeHTML(text.t("setup.doneLead"))}</p>
    </header>
    <div class="wizard-card">
      <p><a class="button primary" href="${escapeAttr(address)}">${escapeHTML(text.t("setup.doneOpen"))}</a></p>
      ${hasForge ? `<p class="hint">${escapeHTML(text.t("setup.doneWebhook"))}</p>` : ""}
    </div>
    <div class="wizard-actions">
      <a class="link" href="/panel/${escapeAttr(orgSlug)}">${escapeHTML(text.t("nav.panel"))}</a>
    </div>
  </section>`;
}
