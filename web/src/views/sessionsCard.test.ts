import { describe, expect, test } from "bun:test";
import { deviceName, profileScreen, sessionsCard, sessionsList, type SessionRow, type Text } from "./templates";

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

describe("the sessions card and its dialog", () => {
  test("the card says how many and opens the dialog; the list is in the dialog, not on the page", () => {
    const html = sessionsCard([row({ current: true }), row({ id: "aaaaaaaaaaaaaaaa" })], text);
    const card = html.slice(0, html.indexOf("<dialog"));
    expect(card).toContain('[sessions.count {"count":"2"}]'.replace(/"/g, "&quot;"));
    expect(card).toContain('data-open-modal="sessions-modal"');
    expect(card).not.toContain("data-session=");
    expect(html).toContain('<dialog class="modal ov-modal sessions-modal" id="sessions-modal"');
    expect(html).toContain("data-sessions-chip>2<");
  });

  test("this browser comes first, marked and without a button; the others as the server gave them", () => {
    const html = sessionsList(
      [row({ id: "aaaaaaaaaaaaaaaa", last_seen: 90 }), row({ id: "bbbbbbbbbbbbbbbb", last_seen: 10, current: true }), row({ id: "cccccccccccccccc", last_seen: 50 })],
      text,
    );
    const order = [...html.matchAll(/data-session="([a-f0-9]+)"/g)].map((m) => m[1]);
    expect(order).toEqual(["bbbbbbbbbbbbbbbb", "aaaaaaaaaaaaaaaa", "cccccccccccccccc"]);
    expect(html).not.toContain('data-session-close="bbbbbbbbbbbbbbbb"');
    expect(html).toContain('data-session-close="aaaaaaaaaaaaaaaa"');
    expect(html.match(/\[sessions\.thisDevice\]/g)?.length).toBe(1);
    expect(html.match(/is-current/g)?.length).toBe(1);
    expect(html).toContain("data-sessions-close-others");
  });

  test("alone, there is nothing else to close", () => {
    const html = sessionsList([row({ current: true })], text);
    expect(html).not.toContain("data-session-close=");
    expect(html).not.toContain("data-sessions-close-others");
  });

  test("a session in use in the last minutes has the green dot; one from hours ago does not", () => {
    const now = Math.floor(Date.now() / 1000);
    expect(sessionsList([row({ last_seen: now - 30 })], text)).toContain("sessions-dot--on");
    expect(sessionsList([row({ last_seen: now - 3 * 3600 })], text)).not.toContain("sessions-dot--on");
  });

  test("what the browser and the network said is written as text, not markup", () => {
    const html = sessionsList([row({ ip: '<img src=x onerror="1">', user_agent: "<b>" })], text);
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<b>");
  });

  test("a session without an address shows a dash, and its confirmation names it without one", () => {
    const html = sessionsList([row({ ip: "" })], text);
    expect(html).toContain('dir="ltr">—<');
    expect(html).toContain('data-device="Firefox · Linux"');
    expect(sessionsList([row({})], text)).toContain('data-device="Firefox · Linux (192.0.2.7)"');
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
