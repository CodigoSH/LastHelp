/**
 * The templates are pure functions, so they can be asked what they draw without
 * a browser. These are the cases where the right answer is not the obvious one.
 */
import { describe, expect, test } from "bun:test";
import { accountRole, itemView, type Text, updateZone } from "./templates";
import type { ItemDetail } from "../api";

/** A catalogue that answers with the key, so a test can see which one was used. */
const text: Text = {
  t: (key, params) =>
    params ? `[${key} ${JSON.stringify(params)}]` : `[${key}]`,
  markup: (key, markup) => `[${key} ${JSON.stringify(markup)}]`,
  date: () => "a date",
  number: (value) => String(value),
  bytes: (value) => `${value} bytes`,
  since: (seconds) => `${seconds}s ago`,
  age: (seconds) => `${seconds}s`,
};

function report(over: Partial<ItemDetail> = {}): ItemDetail {
  return {
    public_id: "abc",
    kind: "bug",
    title: "Crashes when importing",
    body: "It closes with no message.",
    status: "new",
    votes: 0,
    voted: false,
    created_at: 0,
    mine: false,
    comments: [],
    ...over,
  };
}

describe("the answers to the form", () => {
  test("a question the form still asks is drawn in the reader's language", () => {
    // Not the label stored with the answer: that one is English, because it
    // travels into the forge issue. Drawing it here put English words in the
    // middle of an Arabic page.
    const html = itemView(
      report({ fields: [{ key: "version", label: "Version", value: "1.4.2" }] }),
      text,
    );
    expect(html).toContain("[field.version]");
    expect(html).not.toContain("<dt>Version</dt>");
  });

  test("a question it no longer asks keeps the words it was sent under", () => {
    // The reason the label is stored at all: a form reworded next month must
    // not reword what somebody sent last month.
    const html = itemView(
      report({ fields: [{ key: "browser", label: "Which browser", value: "Firefox" }] }),
      text,
    );
    expect(html).toContain("Which browser");
  });

  test("a report with no answers draws no block at all", () => {
    expect(itemView(report(), text)).not.toContain("answers-head");
  });
});

describe("the body of a report", () => {
  test("blank lines become paragraphs", () => {
    const html = itemView(report({ body: "It closes.\n\nI expected it not to." }), text);
    expect(html.match(/<p>/g)?.length).toBe(2);
  });

  test("and the markup in it is escaped, not rendered", () => {
    const html = itemView(report({ body: "<img src=x onerror=alert(1)>" }), text);
    expect(html).not.toContain("<img");
    expect(html).toContain("&lt;img");
  });
});

describe("the vote", () => {
  test("an idea is offered one", () => {
    expect(itemView(report({ kind: "idea" }), text)).toContain("vote-block");
  });

  test("a bug is not", () => {
    // Two hundred people agreeing that something crashes does not make it
    // crash harder.
    expect(itemView(report({ kind: "bug" }), text)).not.toContain("vote-block");
  });
});

describe("a reporter following their own report", () => {
  test("is told it is waiting only while it is", () => {
    const waiting = itemView(report({ mine: true, moderation: "pending" }), text);
    expect(waiting).toContain("[item.waiting]");

    const published = itemView(report({ mine: true, moderation: "published" }), text);
    expect(published).not.toContain("[item.waiting]");
  });

  test("and a stranger is never offered their link back", () => {
    expect(itemView(report({ mine: false }), text)).not.toContain("data-follow-link");
  });
});

describe("the update zone", () => {
  test("is green only when the release server was reached", () => {
    expect(updateZone({ current: "v1", latest: "v1", available: false }, text)).toContain("about-state ok");
    expect(updateZone({ current: "v1", reason: "error.update_none" }, text)).toContain("about-state ok");
    // Not reached, for whatever reason: never the colour of "all is well".
    for (const reason of ["error.update_offline", "error.update_unverifiable", "error.update_unreachable"]) {
      expect(updateZone({ current: "v1", reason }, text)).not.toContain("about-state ok");
    }
    expect(updateZone(null, text)).not.toContain("about-state ok");
  });

  test("in a container it is Lastboard's Docker notice, never a button to install", () => {
    const zone = updateZone({ current: "v1", latest: "v2", available: true, container: true }, text);
    expect(zone).toContain("about-docker");
    expect(zone).not.toContain("data-update");
  });

  test("something newer is the card with its one button, and no notes beside it", () => {
    const zone = updateZone({ current: "v1", latest: "v2", available: true }, text);
    expect(zone).toContain("data-update");
    expect(zone).not.toContain("release-notes");
  });
});

describe("the word under an account's name", () => {
  const owner = { role: "owner" };
  const collaborator = { role: "collaborator" };
  test("is the administrator first, whatever else the account holds", () => {
    expect(accountRole({ admin: true, memberships: [owner] }, text)).toBe("[people.administrator]");
  });
  test("is the team for an owner or a collaborator, whichever they are where", () => {
    expect(accountRole({ admin: false, memberships: [collaborator, owner] }, text)).toBe("[comment.team]");
    expect(accountRole({ admin: false, memberships: [owner] }, text)).toBe("[comment.team]");
    expect(accountRole({ admin: false, memberships: [collaborator] }, text)).toBe("[comment.team]");
  });
  test("is nothing for an account that holds nothing", () => {
    expect(accountRole({ admin: false, memberships: [] }, text)).toBe("");
    expect(accountRole(null, text)).toBe("");
  });
});
