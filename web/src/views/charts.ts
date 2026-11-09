/**
 * The charts, drawn as SVG by hand.
 *
 * No chart library: this house ships one binary with no external anything, and
 * a dependency for a few shapes is a dependency. They are pure functions of
 * numbers, so they can be asked what they draw without a browser.
 *
 * The one rule that matters here: **an SVG coordinate is a machine value and
 * never goes through the locale formatter.** A decimal comma cuts the number in
 * half and the browser drops the rest of the path without saying anything — a
 * chart that is blank in Spanish and fine in English. Nothing in this file
 * calls toLocaleString, and nothing in it emits a comma at all, which is what
 * the test asserts.
 */

import { escapeHTML } from "../escape";

/** A number as SVG writes it: at most two decimals, and always with a dot. */
function n(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return String(Math.round(value * 100) / 100);
}

/** One line of a trend: what it is called in the stylesheet, and a value per day. */
export interface TrendSeries {
  /** A class, not a colour: the palette lives in the stylesheet, which is also
   *  where an operator's accent reaches it. */
  key: string;
  values: number[];
}

export interface TrendOptions {
  /** The drawing area. It scales to the box it is put in; this is its shape. */
  width?: number;
  height?: number;
  /** A label under the first and the last point, if there is one. */
  from?: string;
  to?: string;
  /**
   * What each day is, beyond its height: the day it belongs to and whatever
   * the caller wants the reader to be able to ask it.
   *
   * It travels into the markup as data attributes on a column drawn for that
   * day, so whatever reads the chart back takes the numbers from the drawing
   * rather than recomputing which value belongs where. Two places doing that
   * arithmetic is one place for them to disagree.
   */
  meta?: Array<Record<string, string | number>>;
}

/** The room the labels take, in the drawing's own units. */
const PAD = { left: 32, right: 8, top: 8, bottom: 22 };

/**
 * The top of the scale: the smallest multiple of four that holds the highest
 * point, and never less than four. Four, so the quarter lines land on whole
 * numbers — a gridline labelled "2.5" over a chart of reports is a gridline
 * somebody has to explain.
 */
export function niceMax(values: number[]): number {
  const top = Math.max(0, ...values.filter(Number.isFinite));
  return Math.max(4, Math.ceil(top / 4) * 4);
}

/**
 * Lines over time, one per series, each filled under itself.
 *
 * Every day is expected to be in the array, including the empty ones: a chart
 * drawn only from the days something happened has no gaps and therefore says a
 * quiet month was a busy one. That is the server's job, and this draws what it
 * is given.
 *
 * A column is drawn for every day and carries its meta; the lines themselves
 * are only shapes. What is under the pointer is decided by the column, and the
 * text it is told in is the caller's.
 */
export function trendChart(series: TrendSeries[], options: TrendOptions = {}): string {
  const width = options.width ?? 640;
  const height = options.height ?? 190;
  const days = Math.max(0, ...series.map((s) => s.values.length));
  const span = width - PAD.left - PAD.right;
  const tall = height - PAD.top - PAD.bottom;
  const top = niceMax(series.flatMap((s) => s.values));

  const x = (index: number) => PAD.left + (days > 1 ? (index / (days - 1)) * span : 0);
  const y = (value: number) => PAD.top + tall - (Math.min(Math.max(value, 0), top) / top) * tall;

  // Behind the series. Without them an empty month is a chart with one stroke
  // across the bottom and nothing to read it against; with them it is a chart
  // of a quiet month, which is a different thing.
  const grid = [0, 0.25, 0.5, 0.75, 1]
    .map((at) => {
      const line = y(top * at);
      return `<line class="tc-grid${at === 0 ? " base" : ""}" x1="${n(PAD.left)}" y1="${n(line)}" x2="${n(width - PAD.right)}" y2="${n(line)}" />
        <text class="tc-ylab" x="${n(PAD.left - 8)}" y="${n(line + 3.5)}">${n(top * at)}</text>`;
    })
    .join("");

  // The gradient stops take their colour from the stylesheet: the accent is an
  // instance setting, and a colour written into the markup is one an
  // operator's own colour never reaches.
  const defs = series
    .map(
      (s) => `<linearGradient id="tc-fill-${escapeHTML(s.key)}" class="tc-${escapeHTML(s.key)}" x1="0" y1="0" x2="0" y2="1">
        <stop class="tc-stop-top" offset="0%" /><stop class="tc-stop-bottom" offset="100%" /></linearGradient>`,
    )
    .join("");

  const lines = series
    .map((s) => {
      if (s.values.length === 0) return "";
      const d = s.values.map((v, i) => `${i === 0 ? "M" : "L"} ${n(x(i))} ${n(y(v))}`).join(" ");
      const floor = n(y(0));
      return `<path class="tc-area tc-${escapeHTML(s.key)}" d="${d} L ${n(x(s.values.length - 1))} ${floor} L ${n(x(0))} ${floor} Z" fill="url(#tc-fill-${escapeHTML(s.key)})" />
        <path class="tc-line tc-${escapeHTML(s.key)}" d="${d}" fill="none" vector-effect="non-scaling-stroke" />`;
    })
    .join("");

  // The edge labels are text in a drawing, so their ends are physical: oldest
  // on the left in every language, which is also the way the line runs.
  const edge = (at: number, anchor: string, text?: string) =>
    text ? `<text class="tc-edge" x="${n(at)}" y="${n(height - 4)}" text-anchor="${anchor}">${escapeHTML(text)}</text>` : "";

  const columns = Array.from({ length: days }, (_, i) => {
    const data = Object.entries(options.meta?.[i] ?? {})
      .map(([key, value]) => ` data-${key}="${escapeHTML(String(value))}"`)
      .join("");
    return `<rect class="tc-col" x="${n(x(i))}" y="${n(PAD.top)}" width="1" height="${n(tall)}" data-i="${i}" data-x="${n(x(i))}"${data} />`;
  }).join("");

  return `<div class="tc-wrap">
    <svg class="tc" viewBox="0 0 ${n(width)} ${n(height)}" role="img" aria-hidden="true">
      <defs>${defs}</defs>${grid}${lines}${edge(PAD.left, "start", options.from)}${edge(width - PAD.right, "end", options.to)}
      <line class="tc-cursor" x1="0" y1="${n(PAD.top)}" x2="0" y2="${n(PAD.top + tall)}" vector-effect="non-scaling-stroke" hidden />
      <g class="tc-cols">${columns}</g>
    </svg>
  </div>`;
}
