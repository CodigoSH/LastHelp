/**
 * Escaping for text that goes into HTML.
 *
 * Everything on a public board was written by a stranger. Titles, bodies and
 * comments all pass through here before they reach innerHTML, and the reason it
 * is one exported function rather than a habit is that a habit is what gets
 * forgotten in the one template nobody reviewed.
 */
export function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** For a value going into an attribute that is itself inside quotes. */
export function escapeAttr(value: string): string {
  return escapeHTML(value);
}
