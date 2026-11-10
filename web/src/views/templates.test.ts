/**
 * The templates are pure functions, so they can be asked what they draw without
 * a browser. These are the cases where the right answer is not the obvious one.
 */
import { describe, expect, test } from "bun:test";
import { accountRole, avatar, board, boardDialogList, commentEditForm, monthGroups, roadmap, roadmapDialogList, type RoadmapLane, itemTeamView, itemView, overviewNeeds, overviewScreen, type OverviewData, reportForm, rewriteForm, similarItems, type Text, updateZone } from "./templates";
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
  month: (value) => `month:${value}`,
  shortDate: (value) => `day:${value}`,
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


function overview(over: Partial<OverviewData> = {}): OverviewData {
  return {
    server: true,
    organizations: 2,
    repositories: 3,
    people: 5,
    pending: 12,
    published: 148,
    votes: 573,
    kinds: { idea: 98, bug: 63 },
    statuses: { new: 21, done: 52 },
    moderation: { published: 148, pending: 12, rejected: 1, spam: 2 },
    last_report: 1000,
    waiting_reports: 8,
    waiting_messages: 4,
    waiting_oldest: 1000,
    unanswered: 7,
    unanswered_oldest: 500,
    unanswered_most_votes: 31,
    answerable: 40,
    answered: 36,
    first_reply: 21600,
    not_on_forge: 2,
    failed_jobs: 3,
    series: [{ day: 1, reports: 3, bugs: 1, ideas: 2 }, { day: 2, reports: 0, bugs: 0, ideas: 0 }],
    top: [{ public_id: "a", title: "Dark mode", kind: "idea", status: "planned", votes: 31, created_at: 500 }],
    recent: [{ public_id: "b", title: "Crash <b>", kind: "bug", status: "new", moderation: "pending", votes: 0, created_at: 900 }],
    boards: [{ org: "acme", repo: "atlas", name: "Atlas", organization: "Acme", published: 71, pending: 5, votes: 412 }],
    ...over,
  };
}

describe("the overview", () => {
  test("every number it shows is the one the server counted", () => {
    const html = overviewScreen(overview(), text, 4600);
    for (const [id, value] of [["pending", 12], ["unanswered", 7], ["reports", 161]] as const) {
      const tile = html.match(new RegExp(`data-ov="${id}"[^>]*>[\\s\\S]*?<div class="ov-kpi-main"><span class="ov-kpi-v">(\\d+)</span>`))?.[1];
      expect(Number(tile)).toBe(value);
    }
    // The wait is a duration, said as one.
    expect(html).toMatch(/data-ov="reply"[^>]*>[\s\S]*?ov-kpi-v">21600s</);
  });

  test("each indicator opens the detail it summarises", () => {
    const html = overviewScreen(overview(), text, 4600);
    for (const detail of ["waiting", "unanswered", "replies", "arrived", "voted", "repo"]) {
      expect(html).toContain(`data-ov-open="${detail}"`);
    }
    // One dialog for all of them, drawn with the page.
    expect(html.match(/id="ov-modal"/g)?.length).toBe(1);
  });

  test("the tile itself is the button, reachable from the keyboard, with no line of words to aim at", () => {
    const html = overviewScreen(overview(), text, 4600);
    for (const id of ["pending", "unanswered", "reply", "reports"]) {
      const tile = html.match(new RegExp(`<div class="ov-kpi" data-ov="${id}"[^>]*>`))?.[0] ?? "";
      expect(tile).toContain("data-ov-open=");
      expect(tile).toContain('role="button"');
      expect(tile).toContain('tabindex="0"');
    }
    expect(html.slice(html.indexOf('class="ov-kpis"'), html.indexOf('data-ov="activity"'))).not.toContain("ov-more");
  });

  test("a repository tile opens its own detail", () => {
    const html = overviewScreen(overview(), text, 4600);
    expect(html).toContain('data-ov-open="repo" data-board="acme/atlas"');
  });

  test("what needs the team is a way into the list it counted", () => {
    const html = overviewScreen(overview(), text, 4600);
    expect(html).toContain('href="/review?mode=pending"');
    expect(html).toContain('href="/review?mode=all&amp;reply=none&amp;state=open"');
  });

  test("how long something has waited comes from the clock it is given", () => {
    const html = overviewScreen(overview({ waiting_oldest: 1000 }), text, 4600);
    expect(html).toContain("&quot;age&quot;:&quot;3600s&quot;");
  });

  test("a new installation says so and draws no NaN", () => {
    const html = overviewScreen(
      overview({
        pending: 0, published: 0, votes: 0, kinds: {}, statuses: {}, moderation: {}, last_report: 0, series: [], top: [], recent: [], boards: [], repositories: 0,
        waiting_reports: 0, waiting_messages: 0, waiting_oldest: 0, unanswered: 0, unanswered_oldest: 0, unanswered_most_votes: 0,
        answerable: 0, answered: 0, first_reply: 0, not_on_forge: 0, failed_jobs: 0,
      }),
      text,
      4600,
    );
    expect(html).toContain("[overview.emptyLead]");
    expect(html).toContain("[overview.nothingNeedsYou]");
    expect(html).not.toContain("NaN");
    expect(html).not.toContain("Infinity");
  });

  test("what comes from a stranger is escaped", () => {
    const html = overviewScreen(overview(), text, 4600);
    expect(html).toContain("Crash &lt;b&gt;");
    expect(html).not.toContain("Crash <b>");
  });

  test("a team member does not see the server's numbers", () => {
    const html = overviewScreen(overview({ server: false }), text, 4600);
    expect(html).not.toContain('data-ov="server"');
    expect(html).not.toContain("/panel/people");
    expect(overviewScreen(overview(), text, 4600)).toContain("/panel/people");
  });

  test("a bar is as wide as its share of the largest, in twentieths", () => {
    const html = overviewScreen(overview({ statuses_by_kind: { idea: { new: 20, done: 10 }, bug: {} } }), text, 4600);
    expect(html).toContain('hue-new" data-fill="20"');
    expect(html).toContain('hue-done" data-fill="10"');
  });

  test("bugs and ideas are counted apart, each under its own name", () => {
    const html = overviewScreen(
      overview({ statuses_by_kind: { idea: { planned: 3 }, bug: { closed: 2 } } }),
      text,
      4600,
    );
    const box = (id: string) => html.match(new RegExp(`data-ov="${id}">[\\s\\S]*?</section>`))?.[0] ?? "";
    expect(box("idea-statuses")).toContain("[status.planned]");
    expect(box("idea-statuses")).not.toContain("[status.closed]");
    expect(box("bug-statuses")).toContain("[status.closed]");
    expect(box("bug-statuses")).not.toContain("[status.planned]");
    expect(html).toContain("[overview.ideasByStatus]");
    expect(html).toContain("[overview.bugsByStatus]");
  });

  test("every state a kind can be in is listed, the empty ones too", () => {
    // A team deciding what to tell people works from the states that exist. A
    // bug that is being fixed is "in progress" on the roadmap, and a list that
    // only showed the states somebody was in hid that it could be.
    const html = overviewScreen(overview({ statuses_by_kind: { idea: {}, bug: { new: 1 } } }), text, 4600);
    const box = (id: string) => html.match(new RegExp(`data-ov="${id}">[\\s\\S]*?</section>`))?.[0] ?? "";
    for (const state of ["new", "confirmed", "in_progress", "closed", "declined"]) {
      expect(box("bug-statuses")).toContain(`[status.${state}]`);
    }
    for (const state of ["new", "planned", "in_progress", "done", "declined"]) {
      expect(box("idea-statuses")).toContain(`[status.${state}]`);
    }
    // And only its own: an idea is never "fixed" and a bug is never "planned".
    expect(box("bug-statuses")).not.toContain("[status.planned]");
    expect(box("idea-statuses")).not.toContain("[status.closed]");
  });

  test("a list of statuses reads in the order work moves, not by size", () => {
    const html = overviewScreen(
      overview({ statuses_by_kind: { idea: { done: 50, new: 1, planned: 9 }, bug: {} } }),
      text,
      4600,
    );
    const box = html.match(/data-ov="idea-statuses">[\s\S]*?<\/section>/)?.[0] ?? "";
    const order = [...box.matchAll(/\[status\.(\w+)\]/g)].map((m) => m[1]);
    expect(order.indexOf("new")).toBeLessThan(order.indexOf("planned"));
    expect(order.indexOf("planned")).toBeLessThan(order.indexOf("done"));
  });

  test("the chart has a line for each kind and says which range it shows", () => {
    const week = Array.from({ length: 7 }, (_, i) => ({ day: i + 1, reports: 1, bugs: 0, ideas: 1 }));
    const html = overviewScreen(overview({ series: week }), text, 4600);
    expect(html).toContain("tc-line tc-idea");
    expect(html).toContain("tc-line tc-bug");
    expect(html).toMatch(/data-days="7" aria-pressed="true"/);
    expect(html).toMatch(/data-days="30" aria-pressed="false"/);
    expect(html).toMatch(/data-days="90" aria-pressed="false"/);
  });

  test("each day of the chart carries both kinds and the total", () => {
    const html = overviewScreen(overview({ series: [{ day: 5, reports: 3, bugs: 1, ideas: 2 }, { day: 6, reports: 0, bugs: 0, ideas: 0 }] }), text, 4600);
    expect(html).toContain('data-day="5" data-bug="1" data-idea="2" data-reports="3"');
  });

  test("a repository with nothing waiting says so rather than showing a zero", () => {
    const html = overviewScreen(
      overview({ boards: [{ org: "a", repo: "b", name: "B", organization: "A", published: 1, pending: 0, votes: 0 }] }),
      text,
      4600,
    );
    expect(html).toContain("[overview.nothingPending]");
  });
});

describe("what needs the person looking", () => {
  const clear = { waiting_reports: 0, waiting_messages: 0, waiting_oldest: 0, unanswered: 0, unanswered_oldest: 0, unanswered_most_votes: 0, not_on_forge: 0, failed_jobs: 0 };

  test("a thing that is zero is not drawn", () => {
    expect(overviewNeeds(overview(clear), text, 4600)).toEqual([]);
    const only = overviewNeeds(overview({ ...clear, not_on_forge: 3 }), text, 4600);
    expect(only.length).toBe(1);
    expect(only[0]!.href).toContain("kind=bug");
  });

  test("each one leads to the list it counted", () => {
    const needs = overviewNeeds(overview(), text, 4600);
    const href = (match: string) => needs.find((n) => n.title.includes(match))?.href;
    expect(href("needsComments")).toBe("/review?mode=pending");
    expect(href("needsReports")).toBe("/review?mode=pending");
    expect(href("needsUnanswered")).toBe("/review?mode=all&reply=none&state=open");
    expect(href("needsFailed")).toBe("/panel/notifications");
  });

  test("what has waited long is urgent and comes first", () => {
    const day = 86400;
    const now = 20 * day;
    const needs = overviewNeeds(
      overview({ ...{ waiting_reports: 0, waiting_messages: 0, waiting_oldest: 0, failed_jobs: 0, not_on_forge: 2 }, unanswered: 5, unanswered_oldest: now - 9 * day }),
      text,
      now,
    );
    expect(needs[0]!.tone).toBe("bad");
    expect(needs[0]!.title).toContain("needsUnanswered");
    expect(needs[1]!.tone).toBe("warn");

    // A recent wait is listed before an old silence by the order things are
    // checked in, and still comes after it: lateness decides, not the order.
    const mixed = overviewNeeds(
      overview({ ...clear, waiting_messages: 1, waiting_oldest: now - 3600, unanswered: 2, unanswered_oldest: now - 9 * day }),
      text,
      now,
    );
    expect(mixed.map((n) => n.tone)).toEqual(["bad", "warn"]);
    expect(mixed[0]!.title).toContain("needsUnanswered");

    // The same silence a day old is a thing to do, not a late one.
    const fresh = overviewNeeds(overview({ ...{ waiting_reports: 0, waiting_messages: 0, waiting_oldest: 0, failed_jobs: 0, not_on_forge: 0 }, unanswered: 5, unanswered_oldest: now - day }), text, now);
    expect(fresh[0]!.tone).toBe("warn");
  });

  test("a wait is late after two days", () => {
    const day = 86400;
    const late = overviewNeeds(overview({ ...clear, waiting_messages: 1, waiting_oldest: 10 * day - 3 * day }), text, 10 * day);
    const fine = overviewNeeds(overview({ ...clear, waiting_messages: 1, waiting_oldest: 10 * day - day }), text, 10 * day);
    expect(late[0]!.tone).toBe("bad");
    expect(fine[0]!.tone).toBe("warn");
  });
});

describe("the roadmap page", () => {
  const org = { slug: "acme", name: "Acme" };
  const repo = { slug: "atlas", name: "Atlas", accepts_bugs: true, accepts_ideas: true };
  const item = (id: string, kind: "idea" | "bug", over: Record<string, unknown> = {}) => ({
    public_id: id, kind, title: `title ${id}`, status: "planned", votes: 3, voted: false, created_at: 1_700_000_000, ...over,
  });
  const lane = (key: string, items: ReturnType<typeof item>[], ideas = items.filter((i) => i.kind === "idea").length, bugs = items.length - ideas): RoadmapLane =>
    ({ lane: key, ideas, bugs, total: ideas + bugs, items });

  test("draws four tiles and the four lanes, the last one shut", () => {
    const html = roadmap(org as never, repo as never, [
      lane("in_progress", [item("a", "idea")]),
      lane("planned", [item("b", "idea"), item("c", "bug")]),
      lane("done", [item("d", "idea", { closed_at: 1_700_000_000 })]),
      lane("declined", [item("e", "bug", { closed_at: 1_700_000_000 })]),
    ], 1_800_000_000, text);
    for (const key of ["in_progress", "planned", "done", "declined"]) {
      expect(html).toContain(`data-rm-lane="${key}"`);
      expect(html).toContain(`data-rm-box="${key}"`);
    }
    expect(html).toContain("data-rm-declined hidden");
    // What was declined is not what is going to happen: it is behind a button.
    expect(html).toContain("roadmap.declinedShow");
  });

  test("a lane that is empty is still drawn, so the reader is not left to work out which they see", () => {
    const html = roadmap(org as never, repo as never, [lane("planned", [item("b", "idea")])], 1_800_000_000, text);
    expect(html).toContain('data-rm-box="in_progress"');
    expect(html).toContain('data-rm-box="declined"');
  });

  test("a bug is not offered a vote and an idea shows its count", () => {
    const html = roadmap(org as never, repo as never, [lane("planned", [item("i", "idea", { votes: 42 }), item("b", "bug", { votes: 0 })])], 1_800_000_000, text);
    expect(html).toContain("<b>42</b>");
    expect(html.match(/class="rm-vote bug"/g)?.length).toBe(1);
  });

  test("a board with nothing on it says what the page is for instead of four empty lanes", () => {
    const html = roadmap(org as never, repo as never, [], 1_800_000_000, text);
    expect(html).toContain("roadmap.quietTitle");
    expect(html).not.toContain("data-rm-box");
  });

  test("a title is escaped and linked to its report", () => {
    const html = roadmap(org as never, repo as never, [lane("planned", [item("x", "idea", { title: "<b>bold</b> & more" })])], 1_800_000_000, text);
    expect(html).toContain("&lt;b&gt;bold&lt;/b&gt; &amp; more");
    expect(html).toContain('href="/i/x"');
  });
});

describe("months of a history", () => {
  const at = (y: number, m: number, d: number) => Math.floor(new Date(y, m, d, 12).getTime() / 1000);
  const entry = (id: string, closed: number) => ({ public_id: id, kind: "idea" as const, title: id, status: "done", votes: 0, voted: false, created_at: 0, closed_at: closed });

  test("groups what ended in the same month and keeps the newest first", () => {
    const groups = monthGroups([entry("a", at(2026, 10, 4)), entry("b", at(2026, 10, 2)), entry("c", at(2026, 9, 28)), entry("d", at(2026, 9, 1)), entry("e", at(2025, 9, 30))]);
    expect(groups.map((g) => g.items.map((i) => i.public_id))).toEqual([["a", "b"], ["c", "d"], ["e"]]);
  });

  test("the same month of another year is another group", () => {
    const groups = monthGroups([entry("a", at(2026, 9, 5)), entry("b", at(2025, 9, 5))]);
    expect(groups.length).toBe(2);
  });
});

describe("the dialog behind a lane's show more", () => {
  const item = (id: string, kind: "idea" | "bug", over: Record<string, unknown> = {}) => ({
    public_id: id, kind, title: `title ${id}`, status: "planned", votes: 3, voted: false, created_at: 1_700_000_000, ...over,
  });

  test("says what it holds and how much is left to read", () => {
    const html = roadmapDialogList("planned", "votes", [item("a", "idea"), item("b", "bug")], 5, 1_800_000_000, text);
    expect(html).toContain('href="/i/a"');
    expect(html).toContain("roadmap.shown");
    expect(html).toContain("{&quot;shown&quot;:&quot;2&quot;,&quot;total&quot;:&quot;5&quot;}");
    // More to read than has been read: the button is there.
    expect(html).toContain("data-rm-load");
  });

  test("has no button once everything is on screen", () => {
    const html = roadmapDialogList("planned", "votes", [item("a", "idea")], 1, 1_800_000_000, text);
    expect(html).not.toContain("data-rm-load");
  });

  test("a finished lane is grouped by month while it reads by date, and flat once it is ranked", () => {
    const rows = [item("a", "idea", { closed_at: 1_700_000_000 }), item("b", "bug", { closed_at: 1_600_000_000 })];
    expect(roadmapDialogList("done", "recent", rows, 2, 1_800_000_000, text)).toContain("<h3>");
    expect(roadmapDialogList("done", "votes", rows, 2, 1_800_000_000, text)).not.toContain("<h3>");
  });

  test("a declined row is marked as a refusal, not as a delivery", () => {
    const rows = [item("a", "idea", { closed_at: 1_700_000_000 })];
    expect(roadmapDialogList("declined", "recent", rows, 1, 1_800_000_000, text)).toContain("rm-dismissed");
    expect(roadmapDialogList("done", "recent", rows, 1, 1_800_000_000, text)).toContain("rm-check");
  });

  test("an empty answer says nothing matches, and offers no list", () => {
    const html = roadmapDialogList("planned", "votes", [], 0, 1_800_000_000, text);
    expect(html).toContain("review.nothingMatches");
    expect(html).not.toContain("rm-row");
  });

  test("a title is escaped", () => {
    expect(roadmapDialogList("planned", "votes", [item("a", "idea", { title: "<i>x</i>" })], 1, 1_800_000_000, text)).toContain("&lt;i&gt;x&lt;/i&gt;");
  });
});

describe("the board page", () => {
  const org = { slug: "acme", name: "Acme" };
  const repo = { slug: "atlas", name: "Atlas", accepts_bugs: true, accepts_ideas: true };
  const item = (id: string, kind: "idea" | "bug", over: Record<string, unknown> = {}) => ({
    public_id: id, kind, title: `title ${id}`, status: "new", votes: 4, voted: false, created_at: 1_700_000_000, ...over,
  });
  const filters: { state: "open" | "closed"; outcome: "" | "done" | "undone"; kind: string; sort: string; query: string } = { state: "open", outcome: "", kind: "", sort: "new", query: "" };
  const tiles = { open: 27, closed: 13, done: 11, undone: 2, ideas: 18, bugs: 9 };
  const lanes = [
    { key: "idea" as const, total: 18, items: [item("a", "idea"), item("b", "idea")] },
    { key: "bug" as const, total: 9, items: [item("c", "bug")] },
  ];
  const html = (over: Partial<typeof filters> = {}, l = lanes, t = tiles, road: { inProgress: number; planned: number; done: number } | null = { inProgress: 3, planned: 14, done: 11 }) =>
    board(org as never, repo as never, l as never, { ...filters, ...over }, t, road, "idea", text);

  test("draws two lanes while the view has a split, and one across when a filter chose", () => {
    const two = html();
    expect(two).toContain('data-bd-lane="idea"');
    expect(two).toContain('data-bd-lane="bug"');
    expect(two).toContain('class="bd-lanes"');
    const one = html({ kind: "bug" }, [lanes[1]!]);
    expect(one).toContain('class="bd-lanes one"');
    expect(one).not.toContain('data-bd-lane="idea"');
  });

  test("the way to the roadmap is a link on a tile, and the header has no button for it", () => {
    const page = html();
    expect(page).toContain('<a class="bd-tile road" href="/acme/atlas/roadmap">');
    // The page's actions are the feed and a new report: the roadmap is the tile.
    const header = page.slice(page.indexOf("head-actions"), page.indexOf("</header>"));
    expect(header).not.toContain("/roadmap");
    expect(header).toContain("/new");
    // It stays a link when the roadmap could not be read.
    expect(html({}, lanes, tiles, null)).toContain('href="/acme/atlas/roadmap"');
  });

  test("the roadmap tile says what was delivered as the big number, and what is under way and coming under it", () => {
    const page = html();
    const tile = page.slice(page.indexOf('<a class="bd-tile road"'), page.indexOf("</a>", page.indexOf('<a class="bd-tile road"')));
    expect(tile).toContain(">11<span"); // delivered
    expect(tile).toContain("3 · "); // in progress
    expect(tile).toContain(" 14"); // planned
    expect(html({}, lanes, tiles, null)).not.toContain(">11<span");
  });

  test("only the roadmap tile is a control: the two that count open and closed are information", () => {
    const page = html();
    const tiles = page.slice(page.indexOf('<div class="bd-tiles">'), page.indexOf('<a class="bd-tile road"'));
    expect(tiles).not.toContain("<button");
    expect(tiles).not.toContain("data-bd-state");
    expect(tiles).not.toContain("aria-pressed");
    expect(tiles).toContain('data-tile="open"');
    expect(tiles).toContain('data-tile="closed"');
  });

  test("open and closed are tabs above the bar, one pressed, each with its count", () => {
    const open = html();
    expect(open).toContain('data-bd-state="open" aria-pressed="true"');
    expect(open).toContain('data-bd-state="closed" aria-pressed="false"');
    // The tabs come before the bar, and are not part of it.
    expect(open.indexOf("bd-tabs")).toBeLessThan(open.indexOf('class="fb"'));
    expect(open.slice(open.indexOf('class="fb"'), open.indexOf("bd-lanes"))).not.toContain("data-bd-state");
    expect(html({ state: "closed" })).toContain('data-bd-state="closed" aria-pressed="true"');
  });

  test("the bar is a search, a menu of filters and an order, in that order", () => {
    const page = html();
    // The lane picker for a phone follows the bar and is not part of it.
    const bar = page.slice(page.indexOf('class="fb"'), page.indexOf("bd-seg bd-pick"));
    expect(bar.indexOf("data-bd-search")).toBeLessThan(bar.indexOf("data-fb-toggle"));
    expect(bar).toContain('data-fb-group="kind" data-fb-value="bug"');
    expect(bar).toContain('data-fb-group="sort" data-fb-value="votes"');
    // No loose row of segments any more: each choice is an option of a menu.
    expect(bar).not.toContain("bd-seg");
    expect(bar).not.toContain("data-bd-kind");
  });

  test("what is closed can be narrowed to what was done or what was not, and only while looking at it", () => {
    const page = html({ state: "closed", outcome: "undone" });
    expect(page).toContain('data-fb-group="outcome" data-fb-value="undone"');
    expect(page).toMatch(/aria-checked="true" data-fb-was="true" data-fb-group="outcome" data-fb-value="undone"/);
    expect(page).toMatch(/aria-checked="false" data-fb-was="false" data-fb-group="outcome" data-fb-value="done"/);
    // The outcome is a question of the closed side: on the open side there is nothing to choose.
    expect(html({ state: "open", outcome: "undone" })).not.toContain('data-fb-group="outcome"');
  });

  test("the type is in sight, not in the menu, and the filters button counts what is in the menu", () => {
    const page = html();
    // Three answers asked all the time: a segmented choice beside the search, one press.
    expect(page).toContain('class="fb-seg"');
    expect(page).toContain('data-fb-group="kind" data-fb-value="" aria-pressed="true"'); // one of them is on
    // And it is not also a group of the menu.
    expect(page).not.toContain('<p class="fb-group-name">[overview.colType]</p>');
    // With nothing but the type to choose there is no menu to open, and no button for it.
    expect(page).not.toContain("data-fb-filters");
    expect(html({ kind: "bug" })).not.toContain("fb-count");
    // Closed has a result to choose, so it has the menu, and the button counts it.
    const closed = html({ state: "closed", kind: "bug", outcome: "done" });
    expect(closed).toContain("data-fb-filters");
    expect(closed).toContain('<b class="fb-count">1</b>');
  });

  test("only a board that takes both kinds offers a kind", () => {
    expect(html()).toContain('data-fb-group="kind"');
    const bugs = board(org as never, { ...repo, accepts_ideas: false } as never, [lanes[1]!] as never, filters, tiles, null, "bug", text);
    expect(bugs).not.toContain('data-fb-group="kind"');
  });

  test("what narrows the list is said, with a way to undo each and to clear them all", () => {
    expect(html()).not.toContain("data-fb-undo");
    const narrowed = html({ state: "closed", outcome: "done", query: "csv" });
    expect(narrowed).toContain('data-fb-undo="outcome"');
    expect(narrowed).toContain('data-fb-undo="q"');
    expect(narrowed).toContain("data-fb-clear");
    // The type is already on show in its own control, so it is not said twice.
    expect(html({ kind: "bug" })).not.toContain('data-fb-undo="kind"');
    // One letter is not a search: it narrows nothing and says nothing.
    expect(html({ query: "c" })).not.toContain("data-fb-undo");
  });

  test("an empty board says what it is for instead of drawing lanes", () => {
    const page = html({}, [{ key: "idea", total: 0, items: [] }, { key: "bug", total: 0, items: [] }], { open: 0, closed: 0, done: 0, undone: 0, ideas: 0, bugs: 0 });
    expect(page).toContain("bd-quiet");
    expect(page).not.toContain("data-bd-lane");
    expect(page).not.toContain("data-bd-search");
  });

  test("a lane that finds nothing under a filter says so, and a quiet one says nothing yet", () => {
    const empty = [{ key: "idea" as const, total: 0, items: [] }, { key: "bug" as const, total: 0, items: [] }];
    expect(html({ query: "zz" }, empty)).toContain("review.nothingMatches");
    expect(html({}, empty)).toContain("overview.nothingYet");
  });

  test("a title is escaped and every row leads to its report", () => {
    const page = html({}, [{ key: "idea", total: 1, items: [item("x", "idea", { title: "<b>x</b>" })] }, lanes[1]!]);
    expect(page).toContain("&lt;b&gt;x&lt;/b&gt;");
    expect(page).toContain('href="/i/x"');
  });

  test("a bug is not given a vote count, and an idea is", () => {
    const page = html();
    expect(page.match(/class="score mark kind-bug"/g)?.length).toBe(1);
    expect(page).toContain('<span class="count">4</span>');
  });

  test("the lane picker for a phone names each lane and its count, only when there are two", () => {
    expect(html()).toContain("data-bd-pick");
    expect(html({ kind: "bug" }, [lanes[1]!])).not.toContain("data-bd-pick");
  });
});

describe("the dialog of a board lane", () => {
  const item = (id: string) => ({ public_id: id, kind: "idea" as const, title: `title ${id}`, status: "planned", votes: 2, voted: false, created_at: 1_700_000_000 });
  test("says how much is shown and offers the rest only when there is some", () => {
    const partial = boardDialogList([item("a"), item("b")], 5, text);
    expect(partial).toContain("data-bd-load");
    expect(partial).toContain("board.showing");
    expect(boardDialogList([item("a")], 1, text)).not.toContain("data-bd-load");
  });
  test("says nothing matches rather than drawing an empty list", () => {
    expect(boardDialogList([], 0, text)).toContain("review.nothingMatches");
  });
});
