import { describe, expect, test } from "bun:test";
import { deviceName, profileScreen, sessionsCard, type SessionRow, type Text } from "./templates";

const text: Text = {
  t: (key, params) => (params ? `[${key} ${JSON.stringify(params)}]` : `[${key}]`),
  markup: (key) => `[${key}]`,
  date: () => "a date",
  dateTime: (value) => `at:${String(value)}`,
  number: (value) => String(value),
  bytes: (value) => `${value} bytes`,
  since: (seconds) => `${seconds}s ago`,
  age: (seconds) => `${seconds}s`,
  month: (value) => `month:${value}`,
  shortDate: (value) => `day:${value}`,
};

const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0";
const row = (over: Partial<SessionRow>): SessionRow => ({ id: "0123456789abcdef", current: false, last_seen: 100, user_agent: firefox, ip: "192.0.2.7", ...over });

describe("the list of sessions", () => {
  test("this browser comes first, without a button to close it; the others by when they were last seen", () => {
    const html = sessionsCard(
      [row({ id: "aaaaaaaaaaaaaaaa", last_seen: 50 }), row({ id: "bbbbbbbbbbbbbbbb", last_seen: 10, current: true }), row({ id: "cccccccccccccccc", last_seen: 90 })],
      text,
    );
    const order = [...html.matchAll(/data-session="([a-f0-9]+)"/g)].map((m) => m[1]);
    expect(order).toEqual(["bbbbbbbbbbbbbbbb", "cccccccccccccccc", "aaaaaaaaaaaaaaaa"]);
    expect(html).not.toContain('data-session-close="bbbbbbbbbbbbbbbb"');
    expect(html).toContain('data-session-close="cccccccccccccccc"');
    expect(html.match(/\[sessions\.thisDevice\]/g)?.length).toBe(1);
    expect(html).toContain("data-sessions-close-others");
  });

  test("alone, there is nothing else to close", () => {
    const html = sessionsCard([row({ current: true })], text);
    expect(html).not.toContain("data-session-close=");
    expect(html).not.toContain("data-sessions-close-others");
  });

  test("what the browser and the network said is written as text, not markup", () => {
    const html = sessionsCard([row({ ip: '<img src=x onerror="1">', user_agent: "<b>" })], text);
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<b>");
  });

  test("a session without an address says so with a dash, and its confirmation names it without one", () => {
    const html = sessionsCard([row({ ip: "" })], text);
    expect(html).toContain("&quot;address&quot;:&quot;—&quot;");
    expect(html).toContain('data-device="Firefox · Linux"');
    expect(sessionsCard([row({})], text)).toContain('data-device="Firefox · Linux (192.0.2.7)"');
  });

  test("the card is on the profile only when the list could be read", () => {
    expect(profileScreen({ username: "ana", role: "" }, text, null)).not.toContain("[sessions.title]");
    expect(profileScreen({ username: "ana", role: "" }, text, [row({ current: true })])).toContain("[sessions.title]");
  });
});

describe("the name of a device", () => {
  test("browser and system, which are names and are not translated", () => {
    expect(deviceName(firefox, text)).toBe("Firefox · Linux");
  });
  test("a description that says nothing is an unknown device, and an unknown browser keeps its system", () => {
    expect(deviceName("", text)).toBe("[sessions.unknownDevice]");
    expect(deviceName("curl/8 (Windows)", text)).toBe("[sessions.unknownBrowser] · Windows");
  });
});
