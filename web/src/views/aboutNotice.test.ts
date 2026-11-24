import { expect, test } from "bun:test";
import { aboutScreen, type Text } from "./templates";

const text: Text = {
  t: (key, params) => (params ? `[${key} ${JSON.stringify(params)}]` : `[${key}]`),
  markup: (key) => `[${key}]`,
  date: () => "a date",
  dateTime: () => "a date and time",
  number: (value) => String(value),
  bytes: (value) => `${value} bytes`,
  since: (seconds) => `${seconds}s ago`,
  age: (seconds) => `${seconds}s`,
  month: (value) => `month:${value}`,
  shortDate: (value) => `day:${value}`,
};

test("About warns of an undeclared proxy only when the server saw one, and names the setting", () => {
  expect(aboutScreen({ version: "1.4.0" }, text)).not.toContain("about.proxyUnread");
  const html = aboutScreen({ version: "1.4.0", proxy_unread: true }, text);
  expect(html).toContain('class="notice"');
  expect(html).toContain("[about.proxyUnread {&quot;setting&quot;:&quot;LASTHELP_TRUST_PROXY&quot;}]");
});
