/**
 * The charts are pure functions of numbers, so this is where the arithmetic is
 * checked — including the one thing that only shows up in another language.
 */
import { describe, expect, test } from "bun:test";
import { areaChart, donut } from "./charts";

describe("the area chart", () => {
  test("writes coordinates a browser can read, in any language", () => {
    // The reason this exists: an SVG coordinate written with a decimal comma is
    // truncated at the comma and the rest of the path is dropped silently — a
    // chart that is blank in Spanish and correct in English. Nothing in the
    // output may carry a comma at all.
    const svg = areaChart([1, 7, 3, 9, 0, 4]);
    expect(svg).not.toContain(",");
    expect(svg).not.toContain("NaN");
  });

  test("a series of zeros draws a flat line rather than nothing", () => {
    const svg = areaChart([0, 0, 0]);
    expect(svg).not.toContain("NaN");
    expect(svg).toContain("chart-line");
  });

  test("an empty series still draws", () => {
    expect(areaChart([])).toContain("<svg");
  });

  test("every day is a point on the line", () => {
    const svg = areaChart([1, 2, 3, 4]);
    const line = svg.match(/class="chart-line" d="([^"]+)"/)?.[1] ?? "";
    expect(line.match(/[ML]/g)?.length).toBe(4);
  });
});

describe("the ring", () => {
  test("segments add up to the whole circle", () => {
    const svg = donut([
      { value: 3, className: "a" },
      { value: 1, className: "b" },
    ]);
    const lengths = [...svg.matchAll(/stroke-dasharray="([\d.]+) /g)].map((m) => Number(m[1]));
    expect(lengths.length).toBe(2);
    // Two decimals: the lengths are rounded for the markup, so three times a
    // quarter of a circle is 2.9999 and not 3. Close enough to catch a wrong
    // ratio, loose enough not to fail on the rounding it is written with.
    expect(lengths[0]! / lengths[1]!).toBeCloseTo(3, 2);
  });

  test("a segment of nothing draws nothing", () => {
    const svg = donut([
      { value: 0, className: "a" },
      { value: 2, className: "b" },
    ]);
    expect(svg).toContain('class="arc b"');
    expect(svg).not.toContain('class="arc a"');
  });

  test("no segments at all is a track and no arcs", () => {
    const svg = donut([]);
    expect(svg).toContain("arc-track");
    expect(svg).not.toContain('class="arc ');
    expect(svg).not.toContain("NaN");
  });
});
