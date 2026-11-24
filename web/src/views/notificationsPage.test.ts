// What each kind of account is shown about being told, one by one: the administrator, an owner of an
// organisation, a collaborator on a repository, and an account that was given no role.
import { describe, expect, test } from "bun:test";
import { adminShell, notificationsScreen, overviewNeeds, profileScreen, type OverviewData, type ShellSpec } from "./templates";
import type { PushView } from "../services/pushClient";

const text = { t: (k: string) => `[${k}]`, number: (n: number) => String(n), dateTime: () => "when", age: () => "1m" } as never;

const channel = { kind: "ntfy", channel_url: "https://ntfy.example/a", channel_to: "", channel_has_token: false };
const ready: PushView = { support: "ready", devices: [], thisDevice: null };

const shell = (over: Partial<ShellSpec>): string =>
  adminShell(
    { path: "/panel", organizations: [], who: "x", role: "", isAdmin: false, onTeam: true, mayAddOrganization: false, content: "", ...over } as ShellSpec,
    text,
  );

/** The menu group a link is written under: the last label before it. */
function groupOf(html: string, href: string): string {
  const at = html.indexOf(`href="${href}"`);
  if (at < 0) return "";
  const labels = [...html.slice(0, at).matchAll(/<p class="side-label">([^<]*)<\/p>/g)];
  return labels.length ? labels[labels.length - 1]![1]! : "";
}

describe("the menu", () => {
  test("Notifications is under the account for the administrator and for anybody on a team, and not under the server", () => {
    for (const spec of [{ isAdmin: true }, { isAdmin: false }]) {
      expect(groupOf(shell(spec), "/panel/notifications")).toBe("[shell.groupAccount]");
    }
    expect(groupOf(shell({ isAdmin: true }), "/panel/antiabuse")).toBe("[shell.groupSystem]");
  });

  test("inside an organization or one of its projects, Organizations is the current place, and only it", () => {
    const orgs = [{ slug: "acme", name: "Acme" }] as ShellSpec["organizations"];
    for (const path of ["/panel/acme", "/panel/acme/repos/atlas"]) {
      const html = shell({ path, organizations: orgs, isAdmin: true });
      expect(html).toContain('href="/panel/organizations" aria-current="page"');
      expect(html.match(/aria-current="page"/g)?.length).toBe(1);
    }
    // A page of the panel is not mistaken for an organization.
    expect(shell({ path: "/panel/about", organizations: orgs, isAdmin: true })).not.toContain('href="/panel/organizations" aria-current');
  });

  test("an account with no role is not offered it: nothing could reach them", () => {
    expect(shell({ onTeam: false })).not.toContain('href="/panel/notifications"');
  });

  test("it is one entry and not two, whoever looks", () => {
    expect(shell({ isAdmin: true }).split('href="/panel/notifications"').length - 1).toBe(1);
  });
});

describe("the page", () => {
  test("the administrator has the devices and the channel of the server", () => {
    const html = notificationsScreen(channel, ready, text);
    expect(html).toContain("[push.title]");
    expect(html).toContain("data-push-toggle");
    expect(html).toContain('class="channel"');
    expect(html).toContain("[panel.channelServer]");
  });

  test("an owner or a collaborator has the devices and no channel to set", () => {
    const html = notificationsScreen(null, ready, text);
    expect(html).toContain("[push.title]");
    expect(html).toContain("data-push-toggle");
    expect(html).not.toContain("[panel.channelServer]");
    expect(html).not.toContain('class="channel"');
  });

  test("a browser that cannot take part says why, and is not offered a switch that does nothing", () => {
    for (const support of ["denied", "needs-install", "unsupported"] as const) {
      const html = notificationsScreen(null, { support, devices: [], thisDevice: null }, text);
      expect(html).toContain(`data-push-state="${support}"`);
      expect(html).not.toContain("data-push-toggle");
    }
  });

  test("the profile no longer holds the devices", () => {
    const html = profileScreen({ username: "ana", role: "[comment.team]" }, text);
    expect(html).not.toContain("data-push-toggle");
    expect(html).not.toContain("[push.title]");
  });
});

describe("the reminder on the overview", () => {
  const base = {
    waiting_reports: 0,
    waiting_messages: 0,
    waiting_oldest: 0,
    unanswered: 0,
    unanswered_oldest: 0,
    unanswered_most_votes: 0,
    not_on_forge: 0,
    failed_jobs: 0,
  };
  const needs = (push_boards: number, pushOff: boolean) => overviewNeeds({ ...base, push_boards } as unknown as OverviewData, text, 4600, pushOff);

  test("it is there when the team is told by Push and this browser is not on the list", () => {
    const got = needs(2, true);
    expect(got.length).toBe(1);
    expect(got[0]!.href).toBe("/panel/notifications");
    expect(got[0]!.title).toBe("[overview.needsPush]");
  });

  test("it is not there when no board uses Push, nor when this browser is already on the list", () => {
    expect(needs(0, true)).toEqual([]);
    expect(needs(2, false)).toEqual([]);
  });
});
