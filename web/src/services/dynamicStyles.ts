/**
 * The handful of values only the browser knows, kept out of style attributes.
 *
 * A content policy that allows inline styles allows an injected one, and this
 * product takes its content from strangers. The way out is not to give up the
 * dynamic parts: **a rule inserted through the CSSOM is not an inline style**.
 * The policy governs the `style` attribute and the `<style>` block; a
 * constructed stylesheet adopted by the document is neither, so an accent
 * colour, a tooltip position and a countdown can still come from the running
 * page under `style-src 'self'`.
 *
 * Everything with a small, known set of values — the size of a mark, the width
 * of a bar — is a class in the stylesheet instead. This is for what is genuinely
 * open-ended.
 */

/** What each caller has set, by name, so a rule can be replaced rather than piled on. */
const rules = new Map<string, string>();

let sheet: CSSStyleSheet | null = null;

function adopted(): CSSStyleSheet | null {
  if (sheet) return sheet;
  try {
    sheet = new CSSStyleSheet();
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
    return sheet;
  } catch {
    // A browser without constructed stylesheets. The alternative — writing a
    // <style> element — is the inline CSS this file exists to avoid, so the
    // dynamic values simply fall back to what the stylesheet already says.
    return null;
  }
}

/**
 * Set — or with no body, clear — one named rule.
 *
 * The whole sheet is rewritten because it holds a handful of rules and
 * replaceSync is one call: an index bookkeeping scheme over insertRule and
 * deleteRule is more code and one off-by-one away from writing somebody else's
 * rule.
 */
export function setRule(name: string, css?: string): void {
  if (css === undefined) rules.delete(name);
  else rules.set(name, css);

  const target = adopted();
  if (!target) return;
  target.replaceSync([...rules.values()].join("\n"));
}
