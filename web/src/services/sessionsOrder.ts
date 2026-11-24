/**
 * The order the sessions dialog lists sign-ins in: this device first, so it is
 * the one that is found without looking, then the rest as the server gave them
 * (the most recently used first). A new array; the input is not touched.
 */
export function sessionsInOrder<T extends { current: boolean }>(items: T[]): T[] {
  return [...items.filter((s) => s.current), ...items.filter((s) => !s.current)];
}

/**
 * How long after its last request a session still counts as connected. The
 * server writes a session's last-seen time at most once a minute, so anything
 * shorter than a few of those would flicker; five minutes still reads as "now".
 */
export const ONLINE_WINDOW_S = 5 * 60;

/**
 * How far ahead of this browser's clock a time may be and still be "now": the
 * server stamps the time, and two clocks a few seconds apart are normal.
 */
const SKEW_S = 60;

/** Whether a session was seen within the window. The one this page runs in always is. */
export function sessionIsOnline(session: { current: boolean; last_seen: number }, now = Date.now() / 1000): boolean {
  if (session.current) return true;
  const ago = now - session.last_seen;
  return Number.isFinite(ago) && ago >= -SKEW_S && ago <= ONLINE_WINDOW_S;
}
