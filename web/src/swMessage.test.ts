import { describe as suite, expect, it } from "bun:test";
import { describe, target } from "./swMessage";

suite("target", () => {
  it("keeps a path inside the instance", () => {
    expect(target("/review")).toBe("/review");
    expect(target("/")).toBe("/");
  });

  // A push is signed by this server, but the page it opens is chosen by what the message says.
  it("sends anything that leaves the instance to the home page", () => {
    for (const bad of ["https://evil.example/", "//evil.example", "/\\evil.example", "javascript:alert(1)", "review", "", null, undefined, 42, {}]) {
      expect(target(bad)).toBe("/");
    }
  });
});

suite("describe", () => {
  it("shows what the server said", () => {
    const { title, options } = describe(JSON.stringify({ title: "Web: idea", body: "Dark mode", url: "/review" }));
    expect(title).toBe("Web: idea");
    expect(options.body).toBe("Dark mode");
    expect((options.data as { url: string }).url).toBe("/review");
  });

  it("still says something when the message is empty or is not JSON", () => {
    expect(describe(null).title).toBe("LastHelp");
    expect(describe("").title).toBe("LastHelp");
    expect(describe("plain words").options.body).toBe("plain words");
    expect(describe("[1,2]").title).toBe("LastHelp");
    expect(describe("null").title).toBe("LastHelp");
  });

  it("does not take a field of the wrong type for text", () => {
    const { title, options } = describe(JSON.stringify({ title: 5, body: { a: 1 }, url: 7 }));
    expect(title).toBe("LastHelp");
    expect(options.body).toBe("");
    expect((options.data as { url: string }).url).toBe("/");
  });

  it("stacks the messages that share a tag, and still notices each one", () => {
    const { options } = describe(JSON.stringify({ title: "a", tag: "board:3" }));
    expect(options.tag).toBe("board:3");
    expect(options.renotify).toBe(true);
  });

  it("stands alone without a tag, and a tag of the wrong type is none", () => {
    for (const raw of [JSON.stringify({ title: "a" }), JSON.stringify({ title: "a", tag: 5 }), JSON.stringify({ title: "a", tag: "" })]) {
      const { options } = describe(raw);
      expect(options.tag).toBeUndefined();
      expect(options.renotify).toBe(false);
    }
  });

  it("carries the badge for the status bar", () => {
    expect(describe(JSON.stringify({ title: "a" })).options.badge).toBe("/icon-192.png");
  });
});
