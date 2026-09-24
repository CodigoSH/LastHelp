/**
 * The templates are pure functions, so they can be asked what they draw without
 * a browser. These are the cases where the right answer is not the obvious one.
 */
import { describe, expect, test } from "bun:test";
import { itemView, memberHome, releaseNotes, type Text, updateZone } from "./templates";
import type { ItemDetail } from "../api";

/** A catalogue that answers with the key, so a test can see which one was used. */
const text: Text = {
  t: (key, params) =>
    params ? `[${key} ${JSON.stringify(params)}]` : `[${key}]`,
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

describe("the release notes on About", () => {
  test("are drawn as headings and lists, not shown as markdown", () => {
    const html = releaseNotes("### Added\n- One thing\n- Another\n\n### Fixed\n- A third");
    expect(html).toBe(
      "<h4>Added</h4><ul><li>One thing</li><li>Another</li></ul><h4>Fixed</h4><ul><li>A third</li></ul>",
    );
  });

  test("and every piece of text is still escaped", () => {
    // They come from the release server. Drawing their shape must not turn
    // into drawing whatever markup somebody put in a commit subject.
    const html = releaseNotes("### <img src=x onerror=alert(1)>\n- <b>bold</b>\nplain <i>line</i>");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<b>");
    expect(html).not.toContain("<i>");
    expect(html).toContain("&lt;b&gt;bold&lt;/b&gt;");
  });
});

describe("the update zone", () => {
  test("is green only when the release server was reached", () => {
    expect(updateZone({ current: "v1", latest: "v1", available: false }, text)).toContain("about-state ok");
    expect(updateZone({ current: "v1", reason: "error.update_none" }, text)).toContain("about-state ok");
    // Not reached, for whatever reason: never the colour of "all is well".
    for (const reason of ["error.update_offline", "error.update_unverifiable", "error.update_container", "error.update_unreachable"]) {
      expect(updateZone({ current: "v1", reason }, text)).not.toContain("about-state ok");
    }
    expect(updateZone(null, text)).not.toContain("about-state ok");
  });
});

describe("the overview for somebody on a team", () => {
  const board = { slug: "atlas", name: "Atlas", accepts_bugs: true, accepts_ideas: true, pending: 2 };
  const row = {
    slug: "acme", name: "Acme", forge: "forgejo", repositories: 1, pending: 2, can_manage: false, boards: [board],
  };

  test("an owner is told when the forge is not connected, and led to where it is", () => {
    const html = memberHome({ waiting: 2, organizations: [{ ...row, role: "owner", can_manage: true, connected: false }] }, text);
    expect(html).toContain('data-connect="acme"');
    expect(html).toContain('href="/panel/acme"');
  });

  test("a collaborator is not, since connecting is not theirs", () => {
    const html = memberHome({ waiting: 2, organizations: [{ ...row, role: "collaborator" }] }, text);
    expect(html).not.toContain("data-connect");
  });

  test("each board leads to its own page and says what waits in it", () => {
    const html = memberHome({ waiting: 2, organizations: [{ ...row, role: "collaborator" }] }, text);
    expect(html).toContain('href="/panel/acme/repos/atlas"');
    expect(html).toContain('data-pending="2"');
    expect(html).not.toContain("/queue");
  });

  test("somebody with no organisation is told so", () => {
    expect(memberHome({ waiting: 0, organizations: [] }, text)).toContain("overview.noRoles");
  });
});

