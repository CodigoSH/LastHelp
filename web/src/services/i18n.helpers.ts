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

  if (params) {
    for (const name of Object.keys(params)) {
      text = text.replaceAll(`{${name}}`, isolate(String(params[name])));
    }
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
 * Machine values never go through a locale formatter, and this is the reminder
 * of why: a decimal comma inside an SVG `points` attribute cuts the number in
 * half and the browser discards the rest without a word, so a chart in Spanish
 * would render blank. Same for CSS lengths.
 */
export function machineNumber(value: number): string {
  return String(value);
}
