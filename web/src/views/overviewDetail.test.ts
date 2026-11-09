/**
 * What "see more" opens is a pure function of what the server sent, so it is
 * asked here what it draws: the rows, their order, and what a stranger wrote.
 */
import { describe, expect, test } from "bun:test";
import { arrivedDetail, overviewDialog, repoDetail, repliesDetail, unansweredDetail, votedDetail, waitingDetail, type BoardDetail } from "./overviewDetail";
import type { ReportsData, Text } from "./templates";

const text: Text = {
  t: (key, params) => (params ? `[${key} ${JSON.stringify(params)}]` : `[${key}]`),
  markup: (key) => `[${key}]`,
  date: () => "a date",
  dateTime: () => "a date and time",
  number: (value) => String(value),
  bytes: (value) => `${value}b`,
  since: (seconds) => `${seconds}s ago`,
  age: (seconds) => `${seconds}s`,
};

const board = { org: "acme", repo: "atlas", name: "Atlas", organization: "Acme" };

function detail(over: Partial<BoardDetail> = {}): BoardDetail {
  return {
    ...board,
    published: 71, votes: 412,
    waiting_reports: 5, waiting_messages: 2, waiting_oldest: 100,
    unanswered: 4, unanswered_oldest: 50,
    answerable: 40, answered: 36, first_reply: 3600,
    ideas: 20, bugs: 12,
    statuses_by_kind: { idea: { new: 3 }, bug: { closed: 2 } },
    forge: "forgejo", linked: 18, channel: "ntfy",
    ...over,
  };
}

const none: ReportsData = { items: [], boards: [], counts: { moderation: {}, status: {}, kind: {} } };

describe("what is waiting", () => {
  const data: ReportsData = {
    ...none,
    items: [
      { public_id: "r1", title: "A <b>report</b>", kind: "bug", status: "new", moderation: "pending", votes: 0, created_at: 900, board },
    ],
    comments: [{ id: 1, author: "x", body: "hi", created_at: 400, item: { public_id: "p", title: "An idea" }, board: { name: "Docs" } }],
  };

  test("reports and comments come together, the oldest first", () => {
    const html = waitingDetail(data, 1000, text).body;
    expect(html.indexOf("An idea")).toBeLessThan(html.indexOf("A &lt;b&gt;report"));
    expect(html).toContain("[overview.typeComment]");
    expect(html).toContain("[kind.bug]");
  });

  test("how long each has waited is counted from the clock it is given", () => {
    const html = waitingDetail(data, 1000, text).body;
    expect(html).toContain(">600s<");
    expect(html).toContain(">100s<");
  });

  test("what a stranger wrote is escaped", () => {
    expect(waitingDetail(data, 1000, text).body).not.toContain("<b>report</b>");
  });

  test("it says how many and leads to the queue", () => {
    const filled = waitingDetail(data, 1000, text);
    expect(filled.count).toBe("2");
    expect(filled.body).toContain('href="/review?mode=pending"');
  });

  test("nothing waiting is said, not an empty table", () => {
    const filled = waitingDetail(none, 1000, text);
    expect(filled.body).toContain("[overview.nothingYet]");
    expect(filled.body).not.toContain("ov-tbl");
  });
});

describe("what nobody answered", () => {
  test("counts the whole and links to the list it was taken from", () => {
    const data: ReportsData = { ...none, items: [{ public_id: "a", title: "T", kind: "idea", status: "new", moderation: "published", votes: 9, created_at: 0, board }] };
    const filled = unansweredDetail(data, 100, 29, text);
    expect(filled.count).toBe("29");
    expect(filled.body).toContain('href="/review?mode=all&amp;reply=none&amp;state=open"');
    expect(filled.body).toContain(">9<");
  });
});

describe("by repository", () => {
  test("a repository nobody answered in shows a dash, not a made-up wait", () => {
    const html = repliesDetail([detail({ answered: 0, answerable: 3, first_reply: 0 })], text).body;
    expect(html).toContain("—");
    expect(html).not.toContain(">0s<");
  });

  test("how many were answered says out of how many", () => {
    expect(repliesDetail([detail()], text).body).toContain("&quot;answered&quot;:&quot;36&quot;");
  });

  test("what arrived adds the two kinds", () => {
    const html = arrivedDetail([detail({ ideas: 20, bugs: 12 })], text).body;
    expect(html).toContain(">32<");
  });
});

describe("the most voted", () => {
  test("each idea leads to its own page", () => {
    const data: ReportsData = { ...none, items: [{ public_id: "abc", title: "Dark mode", kind: "idea", status: "planned", moderation: "published", votes: 31, created_at: 0, board }] };
    const html = votedDetail(data, text).body;
    expect(html).toContain('href="/i/abc"');
    expect(html).toContain("[status.planned]");
  });
});

describe("one repository", () => {
  test("says where its work goes and what tells its team, and never a secret", () => {
    const html = repoDetail(detail(), 1000, text).body;
    expect(html).toContain("Forgejo");
    expect(html).toContain("ntfy");
    expect(html).not.toMatch(/token|secret|http/i);
  });

  test("a repository with no forge says so", () => {
    const html = repoDetail(detail({ forge: "none", channel: "" }), 1000, text).body;
    expect(html).toContain("[overview.forgeNone]");
    expect(html).toContain("[overview.channelNone]");
  });

  test("the server's channel is named as such", () => {
    expect(repoDetail(detail({ channel: "server" }), 1000, text).body).toContain("[overview.channelServer]");
  });

  test("a kind with nothing in it draws no list", () => {
    const html = repoDetail(detail({ statuses_by_kind: { idea: { new: 3 }, bug: {} } }), 1000, text).body;
    expect(html).toContain("[overview.ideasByStatus]");
    expect(html).not.toContain("[overview.bugsByStatus]");
  });

  test("its links carry the repository the way the address expects it", () => {
    const html = repoDetail(detail({ org: "a b", repo: "c/d" }), 1000, text).body;
    expect(html).toContain("board=a%20b%2Fc%2Fd");
  });

  test("a wait that has no start is a dash", () => {
    expect(repoDetail(detail({ waiting_oldest: 0 }), 1000, text).body).toContain("—");
  });
});

describe("the dialog", () => {
  test("is one element with a name for the reader of a screen", () => {
    const html = overviewDialog(text);
    expect(html).toContain("<dialog");
    expect(html).toContain('aria-labelledby="ov-modal-title"');
    expect(html).toContain("data-ov-close");
  });
});
