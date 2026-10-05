/**
 * The "go to search" shortcut: Ctrl+K, or Cmd+K on a Mac, from anywhere in the
 * product, aimed at the search box the screen in front of the person has.
 *
 * Kept apart from the page code so it can be tested without a browser.
 */

export interface ShortcutKey {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

/** Whether a key press is the shortcut: K with Ctrl or Cmd, and nothing else held. */
export function isSearchShortcut(e: ShortcutKey): boolean {
  return e.key.toLowerCase() === "k" && (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey;
}

/**
 * A place where Ctrl+K means something of its own — a link in an editor, a
 * paragraph being written — and the shortcut keeps out of the way. A plain field
 * is not one of those: somebody adding a word who wants to look for another
 * should not have to leave the field with the mouse first.
 */
export function ownsCtrlK(el: Element | null | undefined): boolean {
  if (!el) return false;
  const tag = el.tagName;
  if (tag === "TEXTAREA" || tag === "SELECT") return true;
  return (el as HTMLElement).isContentEditable === true;
}

/** The name of the modifier on this machine, for the legend that teaches the shortcut. */
export function modKey(platform?: string): string {
  const where = platform ?? (typeof navigator !== "undefined" ? navigator.platform : "");
  return /Mac|iPhone|iPad|iPod/i.test(where) ? "⌘" : "Ctrl";
}

/**
 * The search box the shortcut goes to: the first one a person can see, inside the
 * dialog when one is open — a box behind a modal is a box they cannot use.
 */
export function findSearchField(doc: Document): HTMLInputElement | null {
  const scope: ParentNode = doc.querySelector("dialog[open]") ?? doc;
  const fields = scope.querySelectorAll<HTMLInputElement>('input[type="search"]');
  for (const field of fields) {
    if (!field.disabled && field.getClientRects().length > 0) return field;
  }
  return null;
}

/** Wires the shortcut once for the whole document, and returns the way to take it off. */
export function installSearchShortcut(doc: Document): () => void {
  const handler = (event: KeyboardEvent) => {
    if (!isSearchShortcut(event) || event.defaultPrevented) return;
    if (ownsCtrlK(event.target as Element | null)) return;
    const field = findSearchField(doc);
    if (!field) return;
    // The browser's own Ctrl+K goes to its address bar; here it is ours.
    event.preventDefault();
    field.focus();
    field.select();
  };
  doc.addEventListener("keydown", handler);
  return () => doc.removeEventListener("keydown", handler);
}
