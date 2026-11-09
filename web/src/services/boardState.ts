/**
 * What the board page asks of the server, and what it makes of the answers, as
 * pure rules.
 *
 * The page is not one list: it is the lanes of the view. What is open splits by
 * kind — ideas, which are voted, and bugs, which are not — and what is closed
 * splits by what was done about it. A filter that has chosen one side of the
 * split leaves a single lane, across the page. Each lane is read on its own,
 * bounded, and opens whole in a dialog.
 */

export type LaneKey = "idea" | "bug" | "done" | "undone";

/** The lane's question to the list endpoint, besides the words and the order. */
export interface LaneSpec {
  key: LaneKey;
  params: Record<string, string>;
}

/** How many rows a lane shows before "show more": fewer when there are two side by side. */
export function laneCap(lanes: number): number {
  return lanes > 1 ? 6 : 10;
}

export interface PlanFilters {
  state: "open" | "closed";
  outcome: "" | "done" | "undone";
  kind: string;
}

/**
 * The lanes of a view. Open splits by the kinds the board accepts — or the one
 * the filter chose; closed splits into done and not done — or the one chosen,
 * with the kind narrowing both.
 */
export function lanePlan(f: PlanFilters, accepts: { ideas: boolean; bugs: boolean }): LaneSpec[] {
  if (f.state === "open") {
    const kinds = f.kind === "bug" || f.kind === "idea" ? [f.kind] : [...(accepts.ideas ? ["idea"] : []), ...(accepts.bugs ? ["bug"] : [])];
    return kinds.map((k) => ({ key: k as LaneKey, params: { state: "open", kind: k } }));
  }
  const outcomes = f.outcome ? [f.outcome] : (["done", "undone"] as const);
  return outcomes.map((o) => ({
    key: o,
    params: { state: "closed", outcome: o, ...(f.kind ? { kind: f.kind } : {}) },
  }));
}

/** The counts the list endpoint sends with every answer. */
export interface Counts {
  open?: number;
  closed?: number;
  done?: number;
  undone?: number;
  open_idea?: number;
  open_bug?: number;
}

export interface Tiles {
  open: number;
  closed: number;
  done: number;
  undone: number;
  ideas: number;
  bugs: number;
}

/**
 * The numbers on the tiles, from the answers the lanes got.
 *
 * When the lanes are split by kind, each was counted under its own kind, so the
 * sides are summed; otherwise every lane was counted under the same question and
 * any one of them says it. The ideas and bugs of what is open are the server's
 * and do not move with the kind.
 */
export function tileNumbers(f: PlanFilters, answers: Array<{ total: number; counts?: Counts }>): Tiles {
  const splitByKind = f.state === "open" && answers.length > 1;
  const pick = (name: keyof Counts): number =>
    splitByKind ? answers.reduce((n, a) => n + (a.counts?.[name] ?? 0), 0) : (answers[0]?.counts?.[name] ?? 0);
  return {
    open: pick("open"),
    closed: pick("closed"),
    done: pick("done"),
    undone: pick("undone"),
    ideas: answers[0]?.counts?.open_idea ?? 0,
    bugs: answers[0]?.counts?.open_bug ?? 0,
  };
}

/** What the dialog of a lane asks: the lane's question, narrowed by a state of its own when it has chosen one. */
export function dialogParams(lane: LaneSpec, status: string): Record<string, string> {
  // A state of its own replaces the side: it already says what is open.
  if (status) {
    const { kind } = lane.params;
    return { status, ...(kind ? { kind } : {}) };
  }
  return { ...lane.params };
}

/** The states an open lane can be narrowed to in its dialog: the ones its kind has, and that are not an end. */
export function laneStatuses(key: LaneKey): string[] {
  if (key === "idea") return ["new", "planned", "in_progress"];
  if (key === "bug") return ["new", "confirmed", "in_progress"];
  return [];
}
