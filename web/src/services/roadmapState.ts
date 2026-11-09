/**
 * What the roadmap page shows of what it holds, as pure rules over kinds.
 *
 * The page carries every row and hides the ones that do not fit: the filter and
 * the "show more" buttons only change what is hidden. The rules are here so
 * each has a test, and the page is left with nothing to decide but the DOM.
 */

export type RoadmapFilter = "" | "idea" | "bug";

const matches = (filter: RoadmapFilter, kind: string): boolean => filter === "" || kind === filter;

/**
 * Whether a title holds what somebody typed. A query under two characters is not
 * a search — it would match half of everything — so it matches all, the same
 * rule the server's search has.
 */
export function titleHas(title: string, query: string): boolean {
  const q = query.trim().toLocaleLowerCase();
  return q.length < 2 || title.toLocaleLowerCase().includes(q);
}

/**
 * A ranked list: which rows show. `limit` is how many show while it is shut and
 * null once it is open. The rank is of what is shown, so a filter renumbers: the
 * first idea is first of the ideas.
 */
export function rankedRows(
  kinds: string[],
  filter: RoadmapFilter,
  limit: number | null,
  /** Rows the search lets through; absent means all of them. */
  hits?: boolean[],
): { show: boolean[]; rank: number[]; hidden: number; matching: number } {
  const show: boolean[] = [];
  const rank: number[] = [];
  let seen = 0;
  for (const [i, kind] of kinds.entries()) {
    if (!matches(filter, kind) || hits?.[i] === false) {
      show.push(false);
      rank.push(0);
      continue;
    }
    seen++;
    const visible = limit === null || seen <= limit;
    show.push(visible);
    rank.push(seen);
  }
  return { show, rank, hidden: seen - show.filter(Boolean).length, matching: seen };
}

/**
 * Months of a history: which groups and which rows show. A month with nothing
 * that matches is not shown, and the months that count against `months` are the
 * ones that have something — a filter must not spend the allowance on empty ones.
 * `months` is how many show while shut, null once open.
 */
export function historyRows(
  groups: string[][],
  filter: RoadmapFilter,
  months: number | null,
  /** Rows the search lets through, by group; absent means all of them. */
  hits?: boolean[][],
): { group: boolean[]; row: boolean[][]; hiddenGroups: number; matching: number } {
  const group: boolean[] = [];
  const row: boolean[][] = [];
  let withRows = 0;
  let matching = 0;
  for (const [g, kinds] of groups.entries()) {
    const keep = kinds.map((k, i) => matches(filter, k) && hits?.[g]?.[i] !== false);
    const n = keep.filter(Boolean).length;
    matching += n;
    if (n === 0) {
      group.push(false);
      row.push(keep);
      continue;
    }
    withRows++;
    const visible = months === null || withRows <= months;
    group.push(visible);
    row.push(keep);
  }
  return { group, row, hiddenGroups: withRows - group.filter(Boolean).length, matching };
}
