/**
 * The pure half of i18n. Every test here names a bug this house has actually
 * shipped, in this project or another, rather than exercising the happy path.
 */
import { describe, expect, test } from "bun:test";
import {
  formatDate,
  formatNumber,
  isolate,
  machineNumber,
  resolveLanguage,
  translate,
} from "./i18n.helpers";

const FSI = "⁨";
const PDI = "⁩";

describe("translate", () => {
  test("falls back to English, then to the key itself", () => {
    const en = { "board.report": "Report" };
    expect(translate("board.report", { "board.report": "Reportar" }, en)).toBe("Reportar");
    expect(translate("board.report", {}, en)).toBe("Report");
    // A missing string shows as the key, never as a blank: a blank is a bug
    // nobody reports.
    expect(translate("board.missing", {}, en)).toBe("board.missing");
    expect(translate("board.missing", undefined, undefined)).toBe("board.missing");
  });

  test("isolates every interpolated value", () => {
    // Without this the neutral characters around the value — the dash, the
    // colon, the comma — are placed by the paragraph's direction rather than
    // the value's, and an Arabic list comes out with two alignments.
    const out = translate("k", { k: "{name} — {count}" }, {}, { name: "Lastboard", count: 3 });
    expect(out).toBe(`${FSI}Lastboard${PDI} — ${FSI}3${PDI}`);
  });

  test("a value carrying a placeholder is not re-interpolated", () => {
    // On this board every title is written by a stranger, and one of them will
    // contain {count}. Substituting the parameters one at a time re-scans the
    // text after each pass, so a later parameter lands inside an earlier
    // value — corrupting it and nesting the isolates.
    const out = translate(
      "k",
      { k: "{title} — {count}" },
      {},
      { title: "Crash in {count}", count: 42 },
    );
    expect(out).toBe(`${FSI}Crash in {count}${PDI} — ${FSI}42${PDI}`);
    // Exactly one isolate pair per value, never nested.
    expect([...out].filter((c) => c === FSI)).toHaveLength(2);
    expect([...out].filter((c) => c === PDI)).toHaveLength(2);
  });

  test("a placeholder with no parameter is left visible", () => {
    // Left as-is rather than blanked, so the gap is reported instead of read
    // as an oddly worded sentence.
    expect(translate("k", { k: "{a} and {b}" }, {}, { a: "x" })).toBe(`${FSI}x${PDI} and {b}`);
  });

  test("the same placeholder twice gets both", () => {
    expect(translate("k", { k: "{n}/{n}" }, {}, { n: 2 })).toBe(`${FSI}2${PDI}/${FSI}2${PDI}`);
  });
});

describe("isolate", () => {
  test("wraps in FIRST STRONG ISOLATE … POP DIRECTIONAL ISOLATE", () => {
    expect(isolate("1.2.0")).toBe(`${FSI}1.2.0${PDI}`);
  });
});

describe("resolveLanguage", () => {
  const available = ["en", "es", "ar"];

  test("an explicit choice wins", () => {
    expect(resolveLanguage("ar", "es-ES", available)).toBe("ar");
  });

  test("a choice this build does not ship falls through", () => {
    // Otherwise a language dropped from the build strands whoever had picked
    // it on a page of raw keys.
    expect(resolveLanguage("ja", "es-ES", available)).toBe("es");
  });

  test("the browser's region is trimmed", () => {
    expect(resolveLanguage(null, "es-419", available)).toBe("es");
  });

  test("English is the floor", () => {
    expect(resolveLanguage(null, "ja-JP", available)).toBe("en");
    expect(resolveLanguage(null, "", available)).toBe("en");
  });
});

describe("formatting", () => {
  test("uses the language of the application, not of the browser", () => {
    // toLocaleString() with no argument reads the browser's locale, so someone
    // running the interface in Spanish on an English system saw English months
    // and full stops where their numbers take commas.
    expect(formatNumber("es", 1234.5)).toBe("1234,5");
    expect(formatNumber("en", 1234.5)).toBe("1,234.5");
    expect(formatNumber("de", 1234.5)).toBe("1.234,5");
  });

  test("a date formats in the chosen language", () => {
    const day = new Date(Date.UTC(2026, 8, 7, 12, 0, 0));
    const options: Intl.DateTimeFormatOptions = { month: "long", timeZone: "UTC" };
    expect(formatDate("es", day, options)).toBe("septiembre");
    expect(formatDate("en", day, options)).toBe("September");
  });

  test("the cache returns the same formatter, not a stale answer", () => {
    // Formatters are cached because a redrawing list asks for the same one on
    // every row. The cache is keyed by language, so a second language must not
    // get the first one's output.
    expect(formatNumber("es", 1000)).toBe("1000");
    expect(formatNumber("en", 1000)).toBe("1,000");
    expect(formatNumber("es", 1000)).toBe("1000");
  });

  test("machine values never see a locale", () => {
    // A decimal comma inside an SVG points attribute cuts the number in half
    // and the browser discards the rest without a word: a chart in Spanish
    // would render blank. Same for CSS lengths.
    expect(machineNumber(42.5)).toBe("42.5");
    expect(machineNumber(42.5)).not.toContain(",");
  });
});
