import { describe, expect, it } from "bun:test";
import { historyRows, rankedRows } from "./roadmapState";

describe("rankedRows", () => {
  const kinds = ["idea", "bug", "idea", "idea", "bug", "idea", "idea", "idea"];

  it("shows the first rows while it is shut and says how many are left", () => {
    const r = rankedRows(kinds, "", 6);
    expect(r.show).toEqual([true, true, true, true, true, true, false, false]);
    expect(r.hidden).toBe(2);
    expect(r.matching).toBe(8);
  });

  it("shows everything once it is open", () => {
    const r = rankedRows(kinds, "", null);
    expect(r.show.every(Boolean)).toBe(true);
    expect(r.hidden).toBe(0);
  });

  // The count of the limit is of what matches: a filter must not spend it on rows it hides.
  it("counts only what matches against the limit, and renumbers", () => {
    const r = rankedRows(kinds, "idea", 3);
    expect(r.show).toEqual([true, false, true, true, false, false, false, false]);
    expect(r.rank.filter((n) => n > 0)).toEqual([1, 2, 3, 4, 5, 6]);
    expect(r.hidden).toBe(3);
    expect(r.matching).toBe(6);
  });

  it("says nothing matches rather than showing a row", () => {
    const r = rankedRows(["idea", "idea"], "bug", 6);
    expect(r.matching).toBe(0);
    expect(r.show).toEqual([false, false]);
  });
});

describe("historyRows", () => {
  const months = [["idea", "bug"], ["idea"], ["bug", "bug"], ["idea"]];

  it("shows the latest months and counts the ones left", () => {
    const h = historyRows(months, "", 2);
    expect(h.group).toEqual([true, true, false, false]);
    expect(h.hiddenGroups).toBe(2);
    expect(h.matching).toBe(6);
  });

  it("does not spend the allowance on a month the filter empties", () => {
    // Ideas only: the third month has none, so the two shown are the first, second... and the fourth is one left.
    const h = historyRows(months, "idea", 2);
    expect(h.group).toEqual([true, true, false, false]);
    expect(h.hiddenGroups).toBe(1);
    const bugs = historyRows(months, "bug", 2);
    expect(bugs.group).toEqual([true, false, true, false]);
    expect(bugs.hiddenGroups).toBe(0);
  });

  it("hides the rows that do not match inside a month that does", () => {
    const h = historyRows(months, "bug", null);
    expect(h.row[0]).toEqual([false, true]);
  });
});
