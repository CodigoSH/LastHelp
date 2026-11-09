/**
 * The charts are pure functions of numbers, so this is where the arithmetic is
 * checked — including the one thing that only shows up in another language.
 */
import { describe, expect, test } from "bun:test";
import { niceMax, trendChart } from "./charts";

const ideas = { key: "idea", values: [1, 2, 3, 4] };
const bugs = { key: "bug", values: [0, 1, 0, 2] };

describe("the trend chart", () => {
  test("writes coordinates a browser can read, in any language", () => {
    // The reason this exists: an SVG coordinate written with a decimal comma is
    // truncated at the comma and the rest of the path is dropped silently — a
    // chart that is blank in Spanish and correct in English. Nothing in the
    // paths may contain one.
    const svg = trendChart([{ key: "idea", values: [0.5, 1.25, 3.333, 2] }]);
    for (const [, d] of svg.matchAll(/class="tc-line[^"]*" d="([^"]+)"/g)) {
      expect(d).not.toContain(",");
    }
  });

  test("draws a line and a fill for every series", () => {
    const svg = trendChart([ideas, bugs]);
    expect(svg.match(/class="tc-line /g)?.length).toBe(2);
    expect(svg.match(/class="tc-area /g)?.length).toBe(2);
    expect(svg).toContain("tc-idea");
    expect(svg).toContain("tc-bug");
  });

  test("every day is a point on every line and a column to ask", () => {
    const svg = trendChart([ideas, bugs]);
    for (const [, d] of svg.matchAll(/class="tc-line[^"]*" d="([^"]+)"/g)) {
      expect(d!.match(/[ML]/g)?.length).toBe(4);
    }
    expect(svg.match(/class="tc-col"/g)?.length).toBe(4);
  });

  test("a column carries what the caller said about its day", () => {
    const svg = trendChart([ideas], { meta: [{ day: 10, bug: 1 }, { day: 11, bug: 0 }, { day: 12, bug: 2 }, { day: 13, bug: 3 }] });
    expect(svg).toContain('data-day="10"');
    expect(svg).toContain('data-bug="3"');
  });

  test("the scale is shared, so two lines can be compared", () => {
    // 9 is the highest of either series: the labels run to 12, not to 4.
    const svg = trendChart([{ key: "idea", values: [1, 9] }, { key: "bug", values: [1, 2] }]);
    const labels = [...svg.matchAll(/class="tc-ylab"[^>]*>([^<]+)</g)].map((m) => Number(m[1]));
    expect(Math.max(...labels)).toBe(12);
  });

  test("the gridlines are labelled with whole numbers", () => {
    const svg = trendChart([{ key: "idea", values: [1, 2, 5] }]);
    for (const [, label] of svg.matchAll(/class="tc-ylab"[^>]*>([^<]+)</g)) {
      expect(Number.isInteger(Number(label))).toBe(true);
    }
  });

  test("a quiet month is a chart of a quiet month, not an error", () => {
    const svg = trendChart([{ key: "idea", values: [0, 0, 0] }, { key: "bug", values: [0, 0, 0] }]);
    expect(svg).toContain("<svg");
    expect(svg).not.toContain("NaN");
    expect(svg).not.toContain("Infinity");
  });

  test("no data at all still draws the frame", () => {
    const svg = trendChart([]);
    expect(svg).toContain("<svg");
    expect(svg).not.toContain("NaN");
  });

  test("the edge labels are escaped", () => {
    expect(trendChart([ideas], { from: "<b>", to: "&" })).toContain("&lt;b&gt;");
  });
});

describe("the top of the scale", () => {
  test("is a multiple of four, never less than four", () => {
    expect(niceMax([])).toBe(4);
    expect(niceMax([0])).toBe(4);
    expect(niceMax([3])).toBe(4);
    expect(niceMax([5])).toBe(8);
    expect(niceMax([12])).toBe(12);
    expect(niceMax([13])).toBe(16);
  });
});
