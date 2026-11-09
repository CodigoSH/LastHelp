/**
 * Whether this device can be told about a new report while LastHelp is closed,
 * and the small pure pieces the push client needs. No network and no storage, so
 * each rule here has a test.
 */

/** What the browser reports about itself. */
export interface PushEnv {
  serviceWorker: boolean;
  pushManager: boolean;
  notification: boolean;
  permission: NotificationPermission;
  ios: boolean;
  /** Running as an installed app (added to the home screen) and not in a tab. */
  standalone: boolean;
}

/**
 * - `ready`: it can be turned on here.
 * - `needs-install`: an iPhone or iPad only gets push for an app added to the Home Screen, so a tab
 *   in Safari has to be told to install first. Without this the switch would be there and do nothing.
 * - `denied`: the person said no to notifications for this site; only they can undo it, in the
 *   browser's settings.
 * - `unsupported`: the browser has no Web Push.
 */
export type PushSupport = "ready" | "needs-install" | "denied" | "unsupported";

export function pushSupport(e: PushEnv): PushSupport {
  // Checked before the features: Safari in a tab does not expose PushManager at all, so "unsupported"
  // would be the wrong thing to tell somebody who only has to add the app to the Home Screen.
  if (e.ios && !e.standalone) return "needs-install";
  if (!e.serviceWorker || !e.pushManager || !e.notification) return "unsupported";
  if (e.permission === "denied") return "denied";
  return "ready";
}

/** iPhones and iPads, including an iPad that says it is a Mac (iPadOS does, and has a touch screen). */
export function isIOS(userAgent: string, maxTouchPoints: number): boolean {
  return /iPad|iPhone|iPod/.test(userAgent) || (/Macintosh/.test(userAgent) && maxTouchPoints > 1);
}

/** The server's public key, base64url, as the bytes a subscription asks for. */
export function decodeKey(key: string): Uint8Array<ArrayBuffer> {
  const padded = key.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (key.length % 4)) % 4);
  const raw = atob(padded);
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

/**
 * A name for a subscription that says which one it is without being it: the first eight bytes of the
 * SHA-256 of its address, in hex. The server computes the same for its list, so the page can tell
 * which row is this device without the server ever sending an address back.
 */
export async function deviceRef(endpoint: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(endpoint));
  return [...new Uint8Array(digest).slice(0, 8)].map((b) => b.toString(16).padStart(2, "0")).join("");
}
