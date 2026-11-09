/**
 * The detail behind the overview: what "see more" opens.
 *
 * Pure functions of what the server sent, like the rest of the templates, so
 * they can be asked what they draw without a browser. What a stranger wrote is
 * escaped here and nowhere else; the words and the number formats come from the
 * catalogue through `text`.
 */
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";
import type { ReportsData, Text } from "./templates";

/** One repository, as /api/overview/boards describes it. */
export interface BoardDetail {
  org: string;
  repo: string;
  name: string;
  organization: string;
  published: number;
  votes: number;
  waiting_reports: number;
  waiting_messages: number;
  waiting_oldest: number;
  unanswered: number;
  unanswered_oldest: number;
  answerable: number;
  answered: number;
  first_reply: number;
  ideas: number;
  bugs: number;
  statuses_by_kind: Record<string, Record<string, number>>;
  forge: string;
  linked: number;
  channel: string;
}

/** What each detail is called, in the order the page offers them. */
export type DetailName = "waiting" | "unanswered" | "replies" | "arrived" | "voted" | "repo";

/** The shell of the dialog, drawn once with the page and filled for each question. */
export function overviewDialog(text: Text): string {
  return `<dialog class="modal ov-modal" id="ov-modal" aria-labelledby="ov-modal-title">
    <header class="ov-modal-head">
      <h2 id="ov-modal-title"></h2>
      <span class="ov-chip" data-ov-count hidden></span>
      <button type="button" class="icon-button" data-ov-close aria-label="${escapeAttr(text.t("action.close"))}">${icons.close()}</button>
    </header>
    <div class="ov-modal-body" data-ov-body></div>
  </dialog>`;
}

/** What the dialog is called and counts, and what goes in it. */
export interface Filled {
  title: string;
  count?: string;
  body: string;
}

const link = (href: string, label: string) =>
  `<a class="ov-more" href="${escapeAttr(href)}">${escapeHTML(label)}${icons.chevron(12)}</a>`;

const foot = (...links: string[]) => `<div class="ov-modal-foot">${links.join("")}</div>`;

const table = (cols: string, head: string[], rows: string[][]) => `<div class="ov-tbl ${cols}">
  <div class="ov-tr head" aria-hidden="true">${head.map((h) => `<span>${escapeHTML(h)}</span>`).join("")}</div>
  ${rows.map((cells) => `<div class="ov-tr">${cells.join("")}</div>`).join("")}
</div>`;

const name = (html: string) => `<span class="ov-td name">${html}</span>`;
const cell = (html: string) => `<span class="ov-td">${html}</span>`;
/** A cell that holds a name somebody chose, which is the one kind that may be cut. */
const user = (html: string) => `<span class="ov-td user">${html}</span>`;

const empty = (text: Text) => `<p class="ov-empty">${escapeHTML(text.t("overview.nothingYet"))}</p>`;

/** What is waiting for review: reports and comments together, the oldest first. */
export function waitingDetail(data: ReportsData, now: number, text: Text): Filled {
  const rows: Array<{ at: number; cells: string[] }> = [];
  const board = (b?: { name?: string }) => user(escapeHTML(b?.name ?? ""));
  const age = (at: number) => cell(escapeHTML(text.age(Math.max(0, now - at))));

  for (const r of data.items ?? []) {
    rows.push({
      at: r.created_at,
      cells: [name(escapeHTML(r.title)), cell(escapeHTML(text.t(`kind.${r.kind}`))), board(r.board), age(r.created_at)],
    });
  }
  for (const c of data.comments ?? []) {
    rows.push({
      at: c.created_at,
      cells: [name(escapeHTML(c.item?.title ?? "")), cell(escapeHTML(text.t("overview.typeComment"))), board({ name: c.board?.name }), age(c.created_at)],
    });
  }
  rows.sort((a, b) => a.at - b.at);

  return {
    title: text.t("overview.modalWaiting"),
    count: text.number(rows.length),
    body:
      rows.length === 0
        ? empty(text)
        : table(
            "cols-wait",
            [text.t("overview.colTitle"), text.t("overview.colType"), text.t("overview.colRepository"), text.t("overview.colWaiting")],
            rows.map((r) => r.cells),
          ) + foot(link("/review?mode=pending", text.t("overview.openQueue"))),
  };
}

/** The published reports nobody on the team has answered, as the listing shows them. */
export function unansweredDetail(data: ReportsData, now: number, total: number, text: Text): Filled {
  const rows = data.items ?? [];
  return {
    title: text.t("overview.modalUnanswered"),
    count: text.number(total),
    body:
      rows.length === 0
        ? empty(text)
        : table(
            "cols-unanswered",
            [text.t("overview.colTitle"), text.t("overview.colType"), text.t("overview.colRepository"), text.t("overview.colVotes"), text.t("overview.colAge")],
            rows.map((r) => [
              name(escapeHTML(r.title)),
              cell(escapeHTML(text.t(`kind.${r.kind}`))),
              user(escapeHTML(r.board?.name ?? "")),
              cell(escapeHTML(text.number(r.votes))),
              cell(escapeHTML(text.age(Math.max(0, now - r.created_at)))),
            ]),
          ) + foot(link("/review?mode=all&reply=none&state=open", text.t("overview.seeUnanswered"))),
  };
}

/** How fast each repository answers. */
export function repliesDetail(boards: BoardDetail[], text: Text): Filled {
  return {
    title: text.t("overview.modalReplies"),
    count: text.number(boards.length),
    body:
      boards.length === 0
        ? empty(text)
        : table(
            "cols-replies",
            [text.t("overview.colRepository"), text.t("overview.colFirstReply"), text.t("overview.colAnswered"), text.t("overview.colUnanswered")],
            boards.map((b) => [
              name(escapeHTML(b.name)),
              cell(b.answered > 0 ? escapeHTML(text.age(b.first_reply)) : "—"),
              cell(escapeHTML(text.t("overview.answeredOf", { answered: text.number(b.answered), total: text.number(b.answerable) }))),
              cell(escapeHTML(text.number(b.unanswered))),
            ]),
          ),
  };
}

/** What arrived in each repository this month, by kind. */
export function arrivedDetail(boards: BoardDetail[], text: Text): Filled {
  return {
    title: text.t("overview.modalArrived"),
    count: text.number(boards.length),
    body:
      boards.length === 0
        ? empty(text)
        : table(
            "cols-arrived",
            [text.t("overview.colRepository"), text.t("overview.reports"), text.t("overview.ideas"), text.t("overview.bugs")],
            boards.map((b) => [
              name(escapeHTML(b.name)),
              cell(escapeHTML(text.number(b.ideas + b.bugs))),
              cell(escapeHTML(text.number(b.ideas))),
              cell(escapeHTML(text.number(b.bugs))),
            ]),
          ),
  };
}

/** The ideas people want most. */
export function votedDetail(data: ReportsData, text: Text): Filled {
  const rows = data.items ?? [];
  return {
    title: text.t("overview.modalVoted"),
    count: text.number(rows.length),
    body:
      rows.length === 0
        ? empty(text)
        : table(
            "cols-voted",
            [text.t("overview.colTitle"), text.t("overview.colRepository"), text.t("overview.colStatus"), text.t("overview.colVotes")],
            rows.map((r) => [
              name(`<a href="/i/${escapeAttr(r.public_id)}">${escapeHTML(r.title)}</a>`),
              user(escapeHTML(r.board?.name ?? "")),
              cell(escapeHTML(text.t(`status.${r.status}`))),
              cell(escapeHTML(text.number(r.votes))),
            ]),
          ) + foot(link("/review?mode=all&kind=idea&sort=votes", text.t("overview.seeReports"))),
  };
}

const forgeNames: Record<string, string> = { forgejo: "Forgejo", gitea: "Gitea", github: "GitHub", gitlab: "GitLab" };
const channelNames: Record<string, string> = { ntfy: "ntfy", gotify: "Gotify", webhook: "Webhook", smtp: "Email" };

/** One repository: its figures, where its work goes and how far each kind has got. */
export function repoDetail(b: BoardDetail, now: number, text: Text): Filled {
  const fact = (label: string, value: string) =>
    `<div class="ov-tr"><span class="ov-td label">${escapeHTML(label)}</span><span class="ov-td">${value}</span></div>`;
  const age = (at: number) => (at > 0 ? escapeHTML(text.age(Math.max(0, now - at))) : "—");
  const forge = b.forge in forgeNames ? forgeNames[b.forge]! : text.t("overview.forgeNone");
  const channel = b.channel in channelNames ? channelNames[b.channel]! : b.channel === "server" ? text.t("overview.channelServer") : text.t("overview.channelNone");

  const flow = (kind: string, title: string) => {
    const entries = Object.entries(b.statuses_by_kind?.[kind] ?? {}).filter(([, n]) => n > 0);
    if (entries.length === 0) return "";
    return `<p class="ov-sub">${escapeHTML(title)}</p><div class="ov-tbl cols-facts">${entries
      .map(([state, n]) => fact(text.t(`status.${state}`), escapeHTML(text.number(n))))
      .join("")}</div>`;
  };

  return {
    title: `${b.name} · ${b.organization}`,
    body:
      `<div class="ov-tbl cols-facts">
        ${fact(text.t("panel.queue"), escapeHTML(text.number(b.waiting_reports + b.waiting_messages)))}
        ${fact(text.t("overview.oldestWait"), age(b.waiting_oldest))}
        ${fact(text.t("overview.firstReply"), b.answered > 0 ? escapeHTML(text.age(b.first_reply)) : "—")}
        ${fact(text.t("overview.unanswered"), escapeHTML(text.number(b.unanswered)))}
        ${fact(text.t("overview.published"), escapeHTML(text.number(b.published)))}
        ${fact(text.t("overview.forge"), escapeHTML(forge))}
        ${fact(text.t("overview.linked"), escapeHTML(text.number(b.linked)))}
        ${fact(text.t("overview.channel"), escapeHTML(channel))}
      </div>` +
      flow("idea", text.t("overview.ideasByStatus")) +
      flow("bug", text.t("overview.bugsByStatus")) +
      foot(
        link(`/review?mode=all&board=${encodeURIComponent(`${b.org}/${b.repo}`)}`, text.t("overview.openRepoReports")),
        link(`/${encodeURIComponent(b.org)}/${encodeURIComponent(b.repo)}`, text.t("shell.publicBoard")),
      ),
  };
}
