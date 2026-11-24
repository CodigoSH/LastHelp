import { describe, expect, test } from "bun:test";
import { ONLINE_WINDOW_S, sessionIsOnline, sessionsInOrder } from "./sessionsOrder";

const s = (id: number, current = false) => ({ id, current });

describe("sessionsInOrder", () => {
  test("this device first, the others as they came", () => {
    expect(sessionsInOrder([s(1), s(2), s(3, true), s(4)]).map((x) => x.id)).toEqual([3, 1, 2, 4]);
  });
  test("does not touch what it was given, and survives having no current one", () => {
    const input = [s(1), s(2)];
    expect(sessionsInOrder(input).map((x) => x.id)).toEqual([1, 2]);
    expect(input.map((x) => x.id)).toEqual([1, 2]);
    expect(sessionsInOrder([])).toEqual([]);
  });
});

describe("sessionIsOnline", () => {
  const now = 1_795_000_000;
  const seen = (ago: number, current = false) => ({ current, last_seen: now - ago });

  test("a session seen a moment ago is connected", () => {
    expect(sessionIsOnline(seen(30), now)).toBe(true);
    expect(sessionIsOnline(seen(ONLINE_WINDOW_S), now)).toBe(true);
  });
  test("one not seen within the window is not", () => {
    expect(sessionIsOnline(seen(ONLINE_WINDOW_S + 1), now)).toBe(false);
    expect(sessionIsOnline(seen(3 * 3600), now)).toBe(false);
  });
  test("this device always is, whatever its stored time says", () => {
    expect(sessionIsOnline(seen(3 * 3600, true), now)).toBe(true);
  });
  test("a server clock a little ahead is still now; a time far in the future, or none, is not a connection", () => {
    expect(sessionIsOnline(seen(-30), now)).toBe(true);
    expect(sessionIsOnline(seen(-3600), now)).toBe(false);
    expect(sessionIsOnline({ current: false, last_seen: Number.NaN }, now)).toBe(false);
  });
});
