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

/** Everything a template needs to say something. */
export interface Text {
  t(key: string, params?: Record<string, string | number>): string;
  date(value: number): string;
  number(value: number): string;
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
export function panelHome(
  orgs: OrganizationSummary[],
  text: Text,
): string {
  const forges = [
    ["none", text.t("panel.forgeNone")],
    ["github", "GitHub"],
    ["gitlab", "GitLab"],
    ["gitea", "Gitea"],
    ["forgejo", "Forgejo"],
  ];

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

    <h2>${escapeHTML(text.t("panel.newOrganization"))}</h2>
    <form class="new-org">
      <label for="org-name">${escapeHTML(text.t("panel.name"))}</label>
      <input id="org-name" name="name" required />

      <label for="org-slug">${escapeHTML(text.t("panel.address"))}</label>
      <input id="org-slug" name="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" />
      <p class="hint">${escapeHTML(text.t("panel.addressHelp"))}</p>

      <label for="org-forge">${escapeHTML(text.t("panel.forge"))}</label>
      <select id="org-forge" name="forge">
        ${forges.map(([value, label]) => `<option value="${escapeAttr(value ?? "")}">${escapeHTML(label ?? "")}</option>`).join("")}
      </select>

      <div class="forge-fields" hidden>
        <label for="org-url">${escapeHTML(text.t("panel.forgeURL"))}</label>
        <input id="org-url" name="base_url" placeholder="https://git.example.com" />

        <label for="org-owner">${escapeHTML(text.t("panel.forgeOwner"))}</label>
        <input id="org-owner" name="forge_owner" />
      </div>

      <p class="form-error" role="alert" hidden></p>
      <button class="button primary" type="submit">${escapeHTML(text.t("auth.create"))}</button>
    </form>`;
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

  return `
    <p class="crumb"><a href="/panel">${escapeHTML(text.t("nav.panel"))}</a></p>
    <h1>${escapeHTML(org.name)}</h1>
    ${list}

    <h2>${escapeHTML(text.t("panel.newRepository"))}</h2>
    <form class="new-repo">
      <label for="repo-name">${escapeHTML(text.t("panel.name"))}</label>
      <input id="repo-name" name="name" required />

      <label for="repo-slug">${escapeHTML(text.t("panel.address"))}</label>
      <input id="repo-slug" name="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" />

      <label for="repo-forge">${escapeHTML(text.t("panel.forgeRepo"))}</label>
      <input id="repo-forge" name="forge_repo" placeholder="owner/repository" />

      <p class="form-error" role="alert" hidden></p>
      <button class="button primary" type="submit">${escapeHTML(text.t("auth.create"))}</button>
    </form>`;
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

export function signInForm(needsSetup: boolean, text: Text): string {
  return `
    <h1>${escapeHTML(needsSetup ? text.t("auth.setupTitle") : text.t("nav.signIn"))}</h1>
    ${needsSetup ? `<p class="hint">${escapeHTML(text.t("auth.setupHelp"))}</p>` : ""}
    <form class="auth">
      <label for="auth-email">${escapeHTML(text.t("auth.email"))}</label>
      <input id="auth-email" name="email" type="email" autocomplete="username" required />

      <label for="auth-password">${escapeHTML(text.t("auth.password"))}</label>
      <input id="auth-password" name="password" type="password"
             autocomplete="${needsSetup ? "new-password" : "current-password"}" required />

      <p class="form-error" role="alert" hidden></p>
      <button class="button primary" type="submit">
        ${escapeHTML(needsSetup ? text.t("auth.create") : text.t("nav.signIn"))}
      </button>
    </form>`;
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
