/**
 * Light, dark, or whatever the system says.
 *
 * The choice is stamped on <html data-theme> and the stylesheet does the rest:
 * with nothing stamped the media query decides, and an explicit choice wins in
 * both directions — including choosing dark on a light system, which a toggle
 * that only ever darkens cannot do.
 *
 * Stored in localStorage, which can throw outright in a private window or with
 * site data blocked, so every read and write is guarded. A reader with no
 * stored choice is not an error: it is the default.
 */
export type Theme = "system" | "light" | "dark";

const KEY = "lasthelp_theme";

export function currentTheme(): Theme {
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === "light" || saved === "dark") return saved;
  } catch {
    // Blocked or unavailable. The system default is the right answer anyway.
  }
  return "system";
}

export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  if (theme === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);

  try {
    if (theme === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, theme);
  } catch {
    // The page still looks right for this visit; only the memory of it is lost.
  }
}

/** Called once at start, before the first paint the reader notices. */
export function restoreTheme(): void {
  applyTheme(currentTheme());
}
