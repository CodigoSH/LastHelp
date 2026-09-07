/**
 * Everything on a public board was written by a stranger, so this function is
 * the last thing standing between a report's title and innerHTML. The tests
 * below are the payloads, not the pretty cases.
 */
import { describe, expect, test } from "bun:test";
import { escapeAttr, escapeHTML } from "./escape";

describe("escapeHTML", () => {
  test("neutralises a script tag", () => {
    expect(escapeHTML("<script>alert(1)</script>")).toBe(
      "&lt;script&gt;alert(1)&lt;/script&gt;",
    );
  });

  test("escapes the ampersand first", () => {
    // If & were replaced last, "<" would already be "&lt;" and would become
    // "&amp;lt;" — the page would render the escape sequence as text. The order
    // in the implementation is load-bearing, and nothing else would catch it.
    expect(escapeHTML("<")).toBe("&lt;");
    expect(escapeHTML("&lt;")).toBe("&amp;lt;");
  });

  test("closes both kinds of quote", () => {
    // A value landing inside an attribute has to survive either delimiter, or
    // it breaks out of the attribute and writes its own.
    expect(escapeHTML('" onerror="alert(1)')).toBe(
      "&quot; onerror=&quot;alert(1)",
    );
    expect(escapeHTML("' onerror='alert(1)")).toBe(
      "&#39; onerror=&#39;alert(1)",
    );
  });

  test("the classic attribute break-out", () => {
    const escaped = escapeAttr('"><img src=x onerror=alert(1)>');
    expect(escaped).not.toContain("<");
    expect(escaped).not.toContain('"');
  });

  test("leaves ordinary text alone", () => {
    expect(escapeHTML("Crashes when importing a folder")).toBe(
      "Crashes when importing a folder",
    );
    // Non-Latin text is not markup and must not be mangled.
    expect(escapeHTML("لا يعمل")).toBe("لا يعمل");
  });

  test("is idempotent in the sense that matters: no character escapes twice", () => {
    // Escaping twice should visibly double the ampersands rather than silently
    // producing something that renders as the original — which is how a
    // double-escape bug hides.
    expect(escapeHTML(escapeHTML("<b>"))).toBe("&amp;lt;b&amp;gt;");
  });
});
