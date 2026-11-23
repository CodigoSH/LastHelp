/**
 * Reads a User-Agent header into a browser and a system, for a person to
 * recognise their own devices in a list.
 *
 * Returns identifiers, not words: the caller translates "unknown" and joins
 * the pieces in the user"s language. Brave and other Chromium builds report
 * as Chrome, which is all the header can tell us.
 *
 * Order matters, because user agents lie politely:
 *  - Edge and Opera also say "Chrome", and Chrome also says "Safari".
 *  - Android also says "Linux", and iPhone also says "Mac OS X".
 */

export type BrowserId = "firefox" | "chrome" | "edge" | "safari" | "opera" | "unknown";
export type OsId = "windows" | "macos" | "linux" | "android" | "ios" | "chromeos" | "unknown";

export function userAgentLabel(ua: string): { browser: BrowserId; os: OsId } {
  const s = ua || "";

  let browser: BrowserId = "unknown";
  if (/\bEdg(e|A|iOS)?\//.test(s)) browser = "edge";
  else if (/\bOPR\/|\bOpera\b/.test(s)) browser = "opera";
  else if (/\bFirefox\/|\bFxiOS\//.test(s)) browser = "firefox";
  else if (/(?:\b|Headless)Chrome\/|\bCriOS\//.test(s)) browser = "chrome";
  else if (/\bSafari\//.test(s)) browser = "safari";

  let os: OsId = "unknown";
  if (/iPhone|iPad|iPod/.test(s)) os = "ios";
  else if (/Android/.test(s)) os = "android";
  else if (/\bCrOS\b/.test(s)) os = "chromeos";
  else if (/Windows/.test(s)) os = "windows";
  else if (/Macintosh|Mac OS X/.test(s)) os = "macos";
  else if (/Linux|X11/.test(s)) os = "linux";

  return { browser, os };
}
