/**
 * The pure half of i18n: no fetch, no localStorage, no singleton, so every rule
 * below can be tested without a browser.
 */

/**
 * Look a key up in the current table, fall back to English, and finally to the
 * key itself — a missing string shows as `board.report` rather than as a blank
 * space, because a blank space is a bug nobody reports.
 */
export function translate(
  key: string,
  current: Record<string, string> | undefined,
  fallback: Record<string, string> | undefined,
  params?: Record<string, string | number>,
): string {
  let text = current?.[key] ?? fallback?.[key];
  if (!text) return key;

  // One pass over the template, never over what was substituted into it.
  //
  // Replacing the placeholders one at a time re-scans the text after each
  // substitution, so a value that happens to contain `{count}` — and on this
  // board every title is written by a stranger — gets a later parameter
  // interpolated into it. That corrupts the text and, worse, nests the bidi
  // isolates, which is the exact invariant this file exists to hold.
  if (params) {
    text = text.replace(/\{([a-zA-Z0-9_]+)\}/g, (placeholder, name: string) =>
      name in params ? isolate(String(params[name])) : placeholder,
    );
  }
  return text;
}

/**
 * Wrap an interpolated value in FIRST STRONG ISOLATE … POP DIRECTIONAL ISOLATE.
 *
 * A value dropped into a translated sentence carries its own direction: a
 * version number, a count, a repository name is left-to-right inside an Arabic
 * sentence. Without the isolate, the neutral characters around it — the comma,
 * the colon, the dash — are placed by the direction of the paragraph rather
 * than of the value, so the punctuation drifts to the wrong end and a list of
 * otherwise correct strings comes out with two different alignments.
 *
 * The characters are invisible and inert left-to-right, so this costs nothing
 * in the other seventeen languages.
 *
 * This is the only interpolator. A second one — a `.replace()` at a call site,
 * building a sentence out of two keys — skips the isolate, and that is the bug
 * this house has now shipped twice. An architecture test greps for it.
 */
export function isolate(value: string): string {
  return `⁨${value}⁩`;
}

/** Prefer an explicit choice, then the browser, then English — but only among
 *  languages this build actually ships. */
export function resolveLanguage(
  saved: string | null,
  browserLang: string,
  available: string[],
): string {
  if (saved && available.includes(saved)) return saved;
  const short = (browserLang || "").split("-")[0];
  if (short && available.includes(short)) return short;
  return "en";
}

/**
 * Formatters are cached because building one is expensive and a list redraws
 * asks for the same one on every row.
 */
const dateFormatters = new Map<string, Intl.DateTimeFormat>();
const numberFormatters = new Map<string, Intl.NumberFormat>();
const byteFormatters = new Map<string, Intl.NumberFormat>();
const durationFormatters = new Map<string, Intl.NumberFormat>();

/**
 * Format in the language the reader chose, not the browser's.
 *
 * `toLocaleString()` with no argument uses the browser's locale, so somebody
 * running the interface in Spanish on an English system would see English
 * months and full stops where their numbers take commas.
 */
export function formatDate(
  code: string,
  value: Date,
  options: Intl.DateTimeFormatOptions = { dateStyle: "medium" },
): string {
  const cacheKey = code + JSON.stringify(options);
  let formatter = dateFormatters.get(cacheKey);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(code, options);
    dateFormatters.set(cacheKey, formatter);
  }
  return formatter.format(value);
}

export function formatNumber(code: string, value: number): string {
  let formatter = numberFormatters.get(code);
  if (!formatter) {
    formatter = new Intl.NumberFormat(code);
    numberFormatters.set(code, formatter);
  }
  return formatter.format(value);
}

/**
 * A size in bytes, in the reader's language and at a scale a person can read.
 *
 * Intl knows the unit names in every language this ships in, so "1,2 GB" and
 * "1.2 GB" and "١٫٢ غيغابايت" come out right without a table of our own. The
 * scale is chosen here because Intl formats a number, not a magnitude.
 */
const byteUnits: Array<[number, string]> = [
  [1024 ** 3, "gigabyte"],
  [1024 ** 2, "megabyte"],
  [1024, "kilobyte"],
  [1, "byte"],
];

export function formatBytes(code: string, value: number): string {
  const [scale, unit] = byteUnits.find(([s]) => value >= s) ?? byteUnits[byteUnits.length - 1]!;
  const scaled = value / scale;
  const cacheKey = `${code}|${unit}`;
  let formatter = byteFormatters.get(cacheKey);
  if (!formatter) {
    formatter = new Intl.NumberFormat(code, {
      style: "unit",
      unit,
      unitDisplay: "short",
      maximumFractionDigits: unit === "byte" ? 0 : 1,
    });
    byteFormatters.set(cacheKey, formatter);
  }
  return formatter.format(scaled);
}

/**
 * How long something has lasted, in the largest unit that still says something.
 *
 * "3 days" rather than "4,320 minutes": the question this answers is whether a
 * server has been up since this morning or since March, and every extra digit
 * makes that harder to see rather than easier.
 *
 * A duration, not a relative time. RelativeTimeFormat was the obvious reach and
 * the wrong one: it renders "3 seconds ago", so under the label "Running for"
 * the screen read "Running for 3 seconds ago". It compiled, it translated, and
 * it was only wrong once somebody read the sentence.
 */
const durationUnits: Array<[number, string]> = [
  [86400 * 365, "year"],
  [86400 * 30, "month"],
  [86400, "day"],
  [3600, "hour"],
  [60, "minute"],
  [1, "second"],
];

export function formatDuration(code: string, seconds: number): string {
  const [scale, unit] = durationUnits.find(([s]) => seconds >= s) ?? durationUnits[durationUnits.length - 1]!;
  const cacheKey = `${code}|${unit}`;
  let formatter = durationFormatters.get(cacheKey);
  if (!formatter) {
    formatter = new Intl.NumberFormat(code, { style: "unit", unit, unitDisplay: "long" });
    durationFormatters.set(cacheKey, formatter);
  }
  return formatter.format(Math.floor(seconds / scale));
}

/**
 * How old something is, as short as the language allows: "3d", "5h", "12m".
 *
 * For a column of rows in a queue, where the question is only which of these
 * has been waiting longest. A full date ("23 Sept 2026") is three times the
 * width and makes the reader do the subtraction; the full date is still there,
 * in the row's title, for whoever needs it.
 *
 * Minutes at the least: "40s" on a queue is noise that changes while you read.
 */
const ageFormatters = new Map<string, Intl.NumberFormat>();

export function formatAge(code: string, seconds: number): string {
  const units = durationUnits.slice(0, -1);
  const [scale, unit] = units.find(([s]) => seconds >= s) ?? units[units.length - 1]!;
  const cacheKey = `${code}|${unit}`;
  let formatter = ageFormatters.get(cacheKey);
  if (!formatter) {
    formatter = new Intl.NumberFormat(code, { style: "unit", unit, unitDisplay: "narrow" });
    ageFormatters.set(cacheKey, formatter);
  }
  return formatter.format(Math.max(1, Math.floor(seconds / scale)));
}

/**
 * Machine values never go through a locale formatter, and this is the reminder
 * of why: a decimal comma inside an SVG `points` attribute cuts the number in
 * half and the browser discards the rest without a word, so a chart in Spanish
 * would render blank. Same for CSS lengths.
 */
export function machineNumber(value: number): string {
  return String(value);
}
