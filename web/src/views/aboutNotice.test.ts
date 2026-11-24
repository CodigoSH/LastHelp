import { describe, expect, test } from "bun:test";
import { aboutScreen, attentionCard, attentionInOrder, attentionList, serverDialog, supportText, type AttentionItem, type Text } from "./templates";

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

const items: AttentionItem[] = [
  { key: "https", tone: "info" },
  { key: "proxy", tone: "warn" },
  { key: "disk", tone: "bad", params: { free_bytes: 1, total_bytes: 10 } },
  { key: "public_url", tone: "warn" },
];

describe("what needs the administrator", () => {
  test("worst first, and in the server's order within one tone", () => {
    expect(attentionInOrder(items).map((i) => i.key)).toEqual(["disk", "proxy", "public_url", "https"]);
  });

  test("the card names the worst three and counts the rest, and opens the dialog", () => {
    const html = attentionCard(items, text);
    expect(html).toContain('data-open-modal="attention-modal"');
    expect(html.indexOf("[attention.disk]")).toBeLessThan(html.indexOf("[attention.proxy]"));
    expect(html).not.toContain("[attention.https]");
    expect(html).toContain("[attention.more {&quot;count&quot;:&quot;1&quot;}]");
  });

  test("with nothing to say it says so and is not a button", () => {
    const html = attentionCard([], text);
    expect(html).toContain("[attention.allGood]");
    expect(html).not.toContain("data-open-modal");
  });

  test("a kind this page does not know is left out, not shown as a key", () => {
    expect(attentionCard([{ key: "from-the-future", tone: "bad" }], text)).toContain("[attention.allGood]");
    expect(attentionList([{ key: "from-the-future", tone: "bad" }], text)).not.toContain("from-the-future");
  });

  test("each item in the dialog says what to set, where there is something to set", () => {
    const html = attentionList(items, text);
    expect(html).toContain("<code>LASTHELP_TRUST_PROXY=true</code>");
    expect(html).toContain("<code>LASTHELP_PUBLIC_URL=https://…</code>");
  });
});

describe("the server dialog", () => {
  const facts = {
    version: "1.4.0",
    go: "go1.27.4",
    os: "linux/amd64",
    schema: 19,
    host: { cpu_percent: 12.4, cores: 4, mem_total_bytes: 8, mem_used_bytes: 3 },
    config: { trust_proxy: false, proxy_hops: 1, public_url: "<b>x</b>", mail: false, offline: true, https: false },
  };

  test("holds the machine, the installation and the setup, with what a person typed escaped", () => {
    const html = serverDialog(facts, text);
    for (const k of ["[about.machine]", "[about.install]", "[about.config]", "[about.cpu]", "[about.proxyRow]", "data-copy-support"]) {
      expect(html).toContain(k);
    }
    expect(html).not.toContain("<b>x</b>");
  });

  test("leaves out the machine where the system says nothing", () => {
    expect(serverDialog({ version: "1.4.0" }, text)).not.toContain("[about.machine]");
  });

  test("the text for support carries the version, the platform and the schema", () => {
    const s = supportText(facts);
    expect(s).toContain("LastHelp 1.4.0");
    expect(s).toContain("linux/amd64 · go1.27.4");
    expect(s).toContain("schema 19");
  });

  test("About holds the card, both dialogs and the discreet button, and no wall of numbers", () => {
    const html = aboutScreen(facts, items, text);
    expect(html).toContain('id="attention-modal"');
    expect(html).toContain('id="server-modal"');
    expect(html).toContain('data-open-modal="server-modal"');
    expect(html).not.toContain("about-stats");
  });
});
