/**
 * The charts, drawn as SVG by hand.
 *
 * No chart library: this house ships one binary with no external anything, and
 * a dependency for four shapes is a dependency. They are pure functions of
 * numbers, so they can be asked what they draw without a browser.
 *
 * The one rule that matters here: **an SVG coordinate is a machine value and
 * never goes through the locale formatter.** A decimal comma cuts the number in
 * half and the browser drops the rest of the path without saying anything — a
 * chart that is blank in Spanish and fine in English. Nothing in this file
 * calls toLocaleString, and nothing in it emits a comma at all, which is what
 * the test asserts.
 */

/** A number as SVG writes it: at most two decimals, and always with a dot. */
function n(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return String(Math.round(value * 100) / 100);
}

export interface AreaChartOptions {
  /** The drawing area. It scales to the box it is put in; this is its shape. */
  width?: number;
  height?: number;
  /** A label under the first and the last point, if there is one. */
  from?: string;
  to?: string;
  /**
   * What each point is, beyond its height: the day it belongs to and whatever
   * the caller wants the reader to be able to ask it — here, the split by kind.
   *
   * It travels into the markup as data attributes on the point that was drawn,
   * so whatever reads the chart back takes the numbers from the drawing rather
   * than recomputing which value belongs where. Two places doing that
   * arithmetic is one place for them to disagree.
   */
  meta?: Array<Record<string, string | number>>;
}

/**
 * An area chart of one series, filled under the line.
 *
 * Every day is expected to be in the array, including the empty ones: a chart
 * drawn only from the days something happened has no gaps and therefore says a
 * quiet month was a busy one. That is the server's job, and this draws what it
 * is given.
 */
export function areaChart(values: number[], options: AreaChartOptions = {}): string {
  const width = options.width ?? 320;
  const height = options.height ?? 90;
  const pad = 8;
  const floor = height - pad;

  // A flat line at the bottom rather than a division by zero, which is what an
  // installation on its first day has.
  const max = Math.max(...values, 1);
  const span = width - pad * 2;
  const step = values.length > 1 ? span / (values.length - 1) : 0;

  const at = (value: number, index: number): [number, number] => [
    pad + index * step,
    floor - (value / max) * (height - pad * 2),
  ];

  const points = values.length > 0 ? values.map(at) : [at(0, 0)];
  const line = points.map(([x, y], i) => `${i === 0 ? "M" : "L"} ${n(x)} ${n(y)}`).join(" ");
  const first = points[0]!;
  const last = points[points.length - 1]!;
  const area = `${line} L ${n(last[0])} ${n(floor)} L ${n(first[0])} ${n(floor)} Z`;

  // The stops are coloured from the stylesheet rather than here: the accent is
  // an instance setting, and a colour written into the markup is one an
  // operator's own colour never reaches. The gradient's id is fixed because
  // there is one of these per screen; a second chart would share an identical
  // gradient, which is what it would have drawn anyway.
  // Three lines behind the series. Without them an empty month is a chart with
  // one stroke across the bottom and nothing to read it against; with them it
  // is a chart of a quiet month, which is a different thing.
  const grid = [0.25, 0.5, 0.75]
    .map((at) => {
      const y = pad + (height - pad * 2) * at;
      return `<line class="chart-grid" x1="${n(pad)}" y1="${n(y)}" x2="${n(width - pad)}" y2="${n(y)}" />`;
    })
    .join("");

  // preserveAspectRatio="none", so the chart fills the card it is given rather
  // than sitting letterboxed in the middle of it with the drawing scaled to its
  // height. Every stroke in here is non-scaling, so nothing thickens with the
  // stretch — and the last point is a tick rather than a circle, because a
  // circle in a stretched box is an ellipse.
  return `<svg class="chart" viewBox="0 0 ${n(width)} ${n(height)}"
    preserveAspectRatio="none" role="img" aria-hidden="true">
    ${grid}
    <defs>
      <linearGradient id="area-chart" x1="0" y1="0" x2="0" y2="1">
        <stop class="chart-stop-top" offset="0%" />
        <stop class="chart-stop-bottom" offset="100%" />
      </linearGradient>
    </defs>
    <path class="chart-area" d="${area}" fill="url(#area-chart)" />
    <path class="chart-line" d="${line}" fill="none" vector-effect="non-scaling-stroke" />
    <line class="chart-last" x1="${n(last[0])}" y1="${n(last[1])}" x2="${n(last[0])}" y2="${n(floor)}"
      vector-effect="non-scaling-stroke" />
    <line class="chart-cursor" x1="0" y1="${n(pad)}" x2="0" y2="${n(floor)}"
      vector-effect="non-scaling-stroke" hidden />
    <g class="chart-points">${points
      .map(
        ([x, y], i) =>
          `<circle class="chart-dot" cx="${n(x)}" cy="${n(y)}" r="3" data-i="${i}"
             data-count="${values[i] ?? 0}"${Object.entries(options.meta?.[i] ?? {})
               .map(([key, value]) => ` data-${key}="${String(value).replace(/"/g, "&quot;")}"`)
               .join("")} />`,
      )
      .join("")}</g>
  </svg>${
    options.from || options.to
      ? `<p class="chart-axis"><span>${options.from ?? ""}</span><span>${options.to ?? ""}</span></p>`
      : ""
  }`;
}

export interface Segment {
  value: number;
  /** A class rather than a colour: the palette lives in the stylesheet, which
   *  is also where an operator's accent reaches it. */
  className: string;
}

/**
 * A ring, one arc per segment, drawn with dash offsets on a single circle.
 *
 * Arcs by dasharray rather than by path arithmetic: no trigonometry, no
 * large-arc flags, and a segment of zero simply draws nothing instead of
 * becoming a full circle, which is what the sweep-flag version does.
 */
export function donut(segments: Segment[], middle = ""): string {
  const size = 120;
  const radius = 46;
  const circumference = 2 * Math.PI * radius;
  const total = segments.reduce((sum, s) => sum + Math.max(s.value, 0), 0);

  let offset = 0;
  const arcs = segments
    .filter((s) => s.value > 0)
    .map((s) => {
      const length = total > 0 ? (s.value / total) * circumference : 0;
      const arc = `<circle class="arc ${s.className}" cx="${n(size / 2)}" cy="${n(size / 2)}" r="${n(radius)}"
        fill="none" stroke-width="14" stroke-linecap="butt"
        stroke-dasharray="${n(length)} ${n(circumference - length)}"
        stroke-dashoffset="${n(-offset)}" />`;
      offset += length;
      return arc;
    })
    .join("");

  return `<div class="donut">
    <svg viewBox="0 0 ${n(size)} ${n(size)}" role="img" aria-hidden="true">
      <circle class="arc-track" cx="${n(size / 2)}" cy="${n(size / 2)}" r="${n(radius)}"
        fill="none" stroke-width="14" />
      <g transform="rotate(-90 ${n(size / 2)} ${n(size / 2)})">${arcs}</g>
    </svg>
    ${middle ? `<span class="donut-middle">${middle}</span>` : ""}
  </div>`;
}
