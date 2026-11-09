/**
 * The dialog behind a roadmap lane's "show more": every item of the lane, with a
 * search, a kind filter and an order, a page at a time.
 *
 * Pure markup, like the rest of the views. The page holds the dialog's frame and
 * fills the list from what the server answers; the questions it asks are the
 * client's, in `main.ts`.
 */
import { escapeAttr, escapeHTML } from "../escape";
import type { ItemSummary } from "../api";
import { icons } from "./icons";
import type { Text } from "./templates";

/** Rows that ended, grouped by the month they ended in, newest first. A month is the reader's own, local. */
export function monthGroups(items: ItemSummary[]): Array<{ at: number; items: ItemSummary[] }> {
  const groups: Array<{ key: string; at: number; items: ItemSummary[] }> = [];
  for (const item of items) {
    const at = item.closed_at ?? item.created_at;
    const d = new Date(at * 1000);
    const key = `${d.getFullYear()}-${d.getMonth()}`;
    const last = groups[groups.length - 1];
    if (last && last.key === key) last.items.push(item);
    else groups.push({ key, at, items: [item] });
  }
  return groups.map(({ at, items }) => ({ at, items }));
}

/** The word for what a report is: its icon and its name. */
export function roadmapKind(text: Text, kind: string): string {
  return `<span class="kind kind-${escapeAttr(kind)}">${kind === "bug" ? icons.bug(13) : icons.idea(13)}<span class="kind-name">${escapeHTML(text.t(`kind.${kind}`))}</span></span>`;
}

/** An idea's votes in a tile; a bug, which is not voted on, its icon. */
export function voteTile(text: Text, item: ItemSummary): string {
  return item.kind === "idea"
    ? `<span class="rm-vote"><b>${escapeHTML(text.number(item.votes))}</b><small>${escapeHTML(text.t("roadmap.votes"))}</small></span>`
    : `<span class="rm-vote bug" aria-hidden="true">${icons.bug(18)}</span>`;
}

/** What marks a finished row: a tick for what was delivered, a dashed ring for what was declined. */
const marker = (lane: string) => (lane === "declined" ? `<span class="rm-dismissed" aria-hidden="true"></span>` : `<span class="rm-check" aria-hidden="true">${icons.check(14)}</span>`);

/** The frame, drawn once with the page and filled for each lane. */
export function roadmapDialog(text: Text): string {
  return `<dialog class="modal ov-modal rm-modal" id="rm-modal" aria-labelledby="rm-modal-title">
    <header class="ov-modal-head">
      <h2 id="rm-modal-title"></h2>
      <span class="ov-chip" data-rm-count></span>
      <button type="button" class="icon-button" data-rm-close aria-label="${escapeAttr(text.t("action.close"))}">${icons.close()}</button>
    </header>
    <div class="rm-tools">
      <label class="search-box">
        ${icons.search(14)}
        <input type="search" data-rm-q placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(text.t("board.search"))}" autocomplete="off" />
      </label>
      <div class="rm-seg" role="group" aria-label="${escapeAttr(text.t("roadmap.show"))}">
        <button type="button" data-rm-kind="" aria-pressed="true">${escapeHTML(text.t("board.all"))}</button>
        <button type="button" data-rm-kind="idea" aria-pressed="false">${escapeHTML(text.t("overview.ideas"))}</button>
        <button type="button" data-rm-kind="bug" aria-pressed="false">${escapeHTML(text.t("overview.bugs"))}</button>
      </div>
      <div class="rm-seg" role="group">
        <button type="button" data-rm-sort="votes" aria-pressed="false">${escapeHTML(text.t("board.sortVotes"))}</button>
        <button type="button" data-rm-sort="recent" aria-pressed="false">${escapeHTML(text.t("board.sortNew"))}</button>
      </div>
    </div>
    <div class="ov-modal-body" data-rm-modal-body></div>
  </dialog>`;
}

/**
 * The list inside the dialog: what has been read so far, and what is left to
 * read. An open lane is rows; a finished one is grouped by month while it reads
 * by date, and flat once it is ranked by votes, where a month would be a lie.
 */
export function roadmapDialogList(
  lane: string,
  sort: "votes" | "recent",
  items: ItemSummary[],
  total: number,
  now: number,
  text: Text,
): string {
  if (items.length === 0) return `<p class="ov-empty">${escapeHTML(text.t("review.nothingMatches"))}</p>`;

  const finished = lane === "declined" || lane === "done";
  const row = (i: ItemSummary) =>
    finished
      ? `<a class="rm-done-row" href="/i/${escapeAttr(i.public_id)}">
          ${marker(lane)}
          <span class="rm-text"><span class="rm-title">${escapeHTML(i.title)}</span><span class="rm-meta">${roadmapKind(text, i.kind)}${i.kind === "idea" ? `<span class="rm-age">${escapeHTML(`${text.number(i.votes)} ${text.t("roadmap.votes")}`)}</span>` : ""}</span></span>
          <span class="rm-when">${escapeHTML(text.shortDate(i.closed_at ?? i.created_at))}</span>
        </a>`
      : `<a class="rm-row rm-mrow" href="/i/${escapeAttr(i.public_id)}">
          ${voteTile(text, i)}
          <span class="rm-text"><span class="rm-title">${escapeHTML(i.title)}</span>
            <span class="rm-meta">${roadmapKind(text, i.kind)}<span class="rm-age">${escapeHTML(text.age(Math.max(0, now - i.created_at)))}</span></span></span>
          <span class="rm-go" aria-hidden="true">${icons.chevron(16)}</span>
        </a>`;

  const body =
    finished && sort === "recent"
      ? monthGroups(items)
          .map((g) => `<div class="rm-month"><h3>${escapeHTML(text.month(g.at))}<span>${escapeHTML(text.number(g.items.length))}</span></h3>${g.items.map(row).join("")}</div>`)
          .join("")
      : `<div class="rm-rows">${items.map(row).join("")}</div>`;

  const more =
    items.length < total
      ? `<button type="button" class="rm-more" data-rm-load>${escapeHTML(text.t("review.more"))}</button>`
      : "";
  return `${body}<p class="rm-shown">${escapeHTML(text.t("roadmap.shown", { shown: text.number(items.length), total: text.number(total) }))}</p>${more}`;
}
