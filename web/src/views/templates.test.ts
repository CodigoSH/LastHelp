/**
 * The templates are pure functions, so they can be asked what they draw without
 * a browser. These are the cases where the right answer is not the obvious one.
 */
import { describe, expect, test } from "bun:test";
import { accountRole, avatar, commentEditForm, itemTeamView, itemView, reportForm, rewriteForm, similarItems, type Text, updateZone } from "./templates";
import type { ItemDetail } from "../api";

/** A catalogue that answers with the key, so a test can see which one was used. */
const text: Text = {
  t: (key, params) =>
    params ? `[${key} ${JSON.stringify(params)}]` : `[${key}]`,
  markup: (key, markup) => `[${key} ${JSON.stringify(markup)}]`,
  date: () => "a date",
  dateTime: () => "a date and time",
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

  test("an answered idea keeps its count and has nothing to press", () => {
    const html = itemView(report({ kind: "idea", votes: 12, votable: false }), text);
    expect(html).toContain("vote-block closed");
    expect(html).toContain(">12<");
    expect(html).not.toContain("data-vote");
    expect(html).not.toContain("<button class=\"vote-block");
  });

  test("an idea with no word about it is still votable", () => {
    // The team's own views carry no `votable`; absent means open.
    expect(itemView(report({ kind: "idea" }), text)).toContain("data-vote");
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

  test("in a container it is the Docker notice, never a button to install", () => {
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

describe("the conversation", () => {
  const allowed = { current: "new", statuses: ["new", "closed"] };

  test("an open report has a form for a comment and one for the team", () => {
    expect(itemView(report(), text)).toContain("comment-form");
    expect(itemTeamView(report(), allowed, [], text)).toContain("team-reply");
  });

  test("an answered report keeps its thread and loses both forms", () => {
    const answered = report({
      commentable: false,
      comments: [{ id: 1, author: "visitor", name: "", body: "me too", created_at: 0 }],
    });
    for (const html of [itemView(answered, text), itemTeamView(answered, allowed, [], text)]) {
      expect(html).toContain("me too");
      expect(html).toContain("item.commentsClosed");
      expect(html).not.toContain("<form class=\"comment-form");
      expect(html).not.toContain("<form class=\"team-reply");
    }
  });
});

describe("the window after an answer", () => {
  const allowed = { current: "closed", statuses: ["new", "closed"] };

  test("an answered report that still takes words from this reader says until when", () => {
    const window = report({ status: "closed", commentable: true, comment_until: 1_700_000_000 });
    const html = itemView(window, text);
    expect(html).toContain("comment-form");
    expect(html).toContain("item.commentsUntil");
    expect(html).toContain("a date and time");
    expect(itemTeamView(window, allowed, [], text)).toContain("item.commentsUntil");
  });

  test("a report that is open says nothing about a deadline", () => {
    expect(itemView(report(), text)).not.toContain("item.commentsUntil");
  });
});

describe("the suggestions under a title", () => {
  test("a closed report wears what the team decided, and its help says what a closed one is", () => {
    const html = similarItems(
      [
        { public_id: "a", title: "keyboard shortcuts", status: "declined", closed: true },
        { public_id: "b", title: "keyboard layout", status: "new", closed: false },
      ],
      text,
    );
    expect(html).toContain("status-declined");
    expect(html).toContain("form.similarClosedHelp");
    expect(html.match(/class="status /g)?.length).toBe(1);
  });

  test("a list of open reports has no chip and no talk of closing", () => {
    const html = similarItems([{ public_id: "b", title: "keyboard layout", status: "new", closed: false }], text);
    expect(html).not.toContain("status-");
    expect(html).not.toContain("form.similarClosedHelp");
    expect(html).toContain("form.similarHelp");
  });
});

describe("pictures", () => {
  const shot = (id: string, comment = 0) => ({
    id,
    comment_id: comment,
    width: 1600,
    height: 900,
    url: `/api/items/abc/images/${id}`,
    thumb: `/api/items/abc/images/${id}?size=thumb`,
  });

  test("a board that takes pictures offers a tray with its ceiling, and one that does not, none", () => {
    const repo = { slug: "atlas", name: "Atlas", accepts_bugs: true, accepts_ideas: true, can_notify: false };
    const org = { slug: "acme", name: "Acme" };
    const on = reportForm(org, repo, "bug", text, 5);
    expect(on).toContain("data-tray");
    expect(on).toContain('data-max="5"');
    expect(reportForm(org, repo, "bug", text, 0)).not.toContain("data-tray");
  });

  test("the report shows its own pictures and each message its own", () => {
    const html = itemView(
      report({
        images: [shot("a"), shot("b", 7)],
        comments: [{ id: 7, author: "team", name: "alex", body: "this?", created_at: 0 }],
      }),
      text,
    );
    expect(html.match(/class="picture"/g)?.length).toBe(2);
    // The message's picture sits after its words, not on the report.
    expect(html.indexOf("images/b?size=thumb")).toBeGreaterThan(html.indexOf("this?"));
  });

  test("a stranger is offered no bin; the reporter only on their own", () => {
    const item = report({
      images: [shot("mine"), shot("team", 7)],
      comments: [{ id: 7, author: "team", name: "alex", body: "ours", created_at: 0 }],
    });
    expect(itemView(item, text)).not.toContain("data-remove-picture");
    const theirs = itemView(item, text, { remove: "reporter", token: "t0k" });
    expect(theirs).toContain('data-remove-picture="mine"');
    expect(theirs).not.toContain('data-remove-picture="team"');
    // Their own picture is reached with their follow link: it may not be public yet.
    expect(theirs).toContain("images/mine?size=thumb&amp;k=t0k");
  });

  test("the team may take out any of them", () => {
    const item = report({
      images: [shot("mine"), shot("team", 7)],
      comments: [{ id: 7, author: "team", name: "alex", body: "ours", created_at: 0 }],
    });
    const html = itemTeamView(item, { current: "new", statuses: ["new"] }, [], text, 3);
    expect(html).toContain('data-remove-picture="mine"');
    expect(html).toContain('data-remove-picture="team"');
    expect(html).toContain("team-reply-pictures");
  });
});

describe("the limits the editing forms keep", () => {
  // Written twice: here, so the browser stops before sending, and in the
  // server, which decides. Two copies of a number drift apart without a sound,
  // so this reads the other one.
  const limit = async (name: string) => {
    const go = await Bun.file(new URL("../../../internal/core/item/item.go", import.meta.url)).text();
    const comment = await Bun.file(new URL("../../../internal/core/item/comment.go", import.meta.url)).text();
    const found = (go + comment).match(new RegExp(`\\b${name}\\s*=\\s*([0-9_]+)`));
    if (!found) throw new Error(`${name} is not in the item package any more; this test is reading the wrong file`);
    return Number((found[1] ?? "").replaceAll("_", ""));
  };

  test("the title of a rewrite is as long as the server allows", async () => {
    const attribute = rewriteForm({ title: "t", body: "b" }, text).match(/name="title"[^>]*maxlength="(\d+)"/)?.[1];
    expect(Number(attribute)).toBe(await limit("maxTitle"));
  });

  test("an edited message is as long as the server allows", async () => {
    const attribute = commentEditForm("hello", text).match(/maxlength="(\d+)"/)?.[1];
    expect(Number(attribute)).toBe(await limit("maxComment"));
  });
});

describe("the picture beside a name", () => {
  const thread = [
    { id: 1, author: "reporter", name: "Sam", body: "it breaks", created_at: 0 },
    { id: 2, author: "visitor", name: "", body: "me too", created_at: 0 },
    { id: 3, author: "team", name: "alex", body: "on it", created_at: 0 },
    { id: 4, author: "team", name: "olga", avatar: "/avatars/9", body: "fixed", created_at: 0 },
  ];
  const allowed = { current: "new", statuses: ["new"] };
  const circles = (html: string) => html.match(/<span class="avatar[^"]*"[^>]*>.*?<\/span>/gs) ?? [];

  test("never draws initials: a name alone is not a picture", () => {
    for (const html of [itemView(report({ comments: thread }), text), itemTeamView(report({ comments: thread }), allowed, [], text)]) {
      expect(circles(html).length).toBe(4);
      for (const c of circles(html)) expect(c).not.toMatch(/>\s*[A-Za-z]{1,2}\s*</);
    }
  });

  test("everybody but the team gets the plain default, the team its own", () => {
    const html = itemView(report({ comments: thread }), text);
    const [reporter, visitor, team, teamWithOwn] = circles(html);
    expect(reporter).toContain("is-guest");
    expect(visitor).toContain("is-guest");
    expect(team).toContain("is-team");
    // The same figure for everybody: what tells the team apart is how it is worn.
    expect(team).toContain("avatar-fig");
    expect(reporter).toContain("avatar-fig");
    expect(team).not.toContain("is-guest");
    expect(reporter).not.toContain("is-team");
    expect(teamWithOwn).toContain("is-team");
  });

  test("a teammate's own picture replaces the default, by its address", () => {
    const html = itemView(report({ comments: thread }), text);
    const teamWithOwn = circles(html)[3]!;
    expect(teamWithOwn).toContain('<img src="/avatars/9"');
    expect(teamWithOwn).not.toContain("avatar-fig");
    expect(circles(html)[2]).not.toContain("<img");
  });

  test("a picture address is escaped, so it cannot close its own attribute", () => {
    expect(avatar('/x"><script>alert(1)</script>')).not.toContain("<script>");
  });

  test("a reporter that carries a picture field still gets the plain default", () => {
    // Only the team has pictures; a stranger's message never has one, whatever arrives.
    const html = itemView(report({ comments: [{ id: 1, author: "visitor", name: "", avatar: "/avatars/3", body: "x", created_at: 0 }] }), text);
    expect(circles(html)[0]).toContain("is-guest");
    expect(circles(html)[0]).not.toContain("<img");
  });
});
