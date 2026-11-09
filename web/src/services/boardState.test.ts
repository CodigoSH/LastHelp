import { describe, expect, it } from "bun:test";
import { dialogParams, laneCap, lanePlan, laneStatuses, tileNumbers } from "./boardState";

const both = { ideas: true, bugs: true };

describe("lanePlan", () => {
  it("splits what is open by kind and what is closed by outcome", () => {
    expect(lanePlan({ state: "open", outcome: "", kind: "" }, both).map((l) => l.key)).toEqual(["idea", "bug"]);
    expect(lanePlan({ state: "closed", outcome: "", kind: "" }, both).map((l) => l.key)).toEqual(["done", "undone"]);
  });

  it("leaves one lane when a filter has chosen the side of the split", () => {
    expect(lanePlan({ state: "open", outcome: "", kind: "bug" }, both).map((l) => l.key)).toEqual(["bug"]);
    expect(lanePlan({ state: "closed", outcome: "undone", kind: "" }, both).map((l) => l.key)).toEqual(["undone"]);
  });

  it("offers only the kinds a board accepts", () => {
    expect(lanePlan({ state: "open", outcome: "", kind: "" }, { ideas: false, bugs: true }).map((l) => l.key)).toEqual(["bug"]);
    expect(lanePlan({ state: "open", outcome: "", kind: "" }, { ideas: true, bugs: false }).map((l) => l.key)).toEqual(["idea"]);
  });

  it("asks the question of each lane, and a closed lane keeps the kind", () => {
    expect(lanePlan({ state: "open", outcome: "", kind: "" }, both)[0]?.params).toEqual({ state: "open", kind: "idea" });
    expect(lanePlan({ state: "closed", outcome: "", kind: "idea" }, both)[1]?.params).toEqual({ state: "closed", outcome: "undone", kind: "idea" });
    // The outcome is a question of the closed side only.
    expect(lanePlan({ state: "open", outcome: "done", kind: "" }, both).every((l) => !l.params.outcome)).toBe(true);
  });
});

describe("laneCap", () => {
  it("shows fewer rows when two lanes share the page", () => {
    expect(laneCap(2)).toBe(6);
    expect(laneCap(1)).toBe(10);
  });
});

describe("tileNumbers", () => {
  it("sums the lanes when they were counted each under its own kind", () => {
    const t = tileNumbers({ state: "open", outcome: "", kind: "" }, [
      { total: 15, counts: { open: 15, closed: 8, done: 6, undone: 2, open_idea: 15, open_bug: 10 } },
      { total: 10, counts: { open: 10, closed: 6, done: 5, undone: 1, open_idea: 15, open_bug: 10 } },
    ]);
    expect(t).toEqual({ open: 25, closed: 14, done: 11, undone: 3, ideas: 15, bugs: 10 });
  });

  it("takes one answer when every lane was counted under the same question", () => {
    const t = tileNumbers({ state: "closed", outcome: "", kind: "" }, [
      { total: 11, counts: { open: 25, closed: 14, done: 11, undone: 3, open_idea: 15, open_bug: 10 } },
      { total: 3, counts: { open: 25, closed: 14, done: 11, undone: 3, open_idea: 15, open_bug: 10 } },
    ]);
    expect(t.closed).toBe(14);
    expect(t.open).toBe(25);
  });

  it("does not make a number of an answer that has none", () => {
    expect(tileNumbers({ state: "open", outcome: "", kind: "" }, [])).toEqual({ open: 0, closed: 0, done: 0, undone: 0, ideas: 0, bugs: 0 });
  });
});

describe("the dialog of a lane", () => {
  const idea = lanePlan({ state: "open", outcome: "", kind: "" }, both)[0]!;
  it("asks the lane's own question", () => {
    expect(dialogParams(idea, "")).toEqual({ state: "open", kind: "idea" });
  });
  it("lets a state replace the side, and keeps the kind", () => {
    expect(dialogParams(idea, "planned")).toEqual({ status: "planned", kind: "idea" });
  });
  it("narrows by the states a kind has, and none for a closed lane", () => {
    expect(laneStatuses("idea")).toEqual(["new", "planned", "in_progress"]);
    expect(laneStatuses("bug")).toEqual(["new", "confirmed", "in_progress"]);
    expect(laneStatuses("done")).toEqual([]);
  });
});
