/**
 * The charts are pure functions of numbers, so this is where the arithmetic is
 * checked — including the one thing that only shows up in another language.
 */
import { describe, expect, test } from "bun:test";
import { areaChart } from "./charts";

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
