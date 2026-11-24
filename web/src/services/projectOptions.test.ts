import { describe, expect, it } from "bun:test";
import {
  applyPreset,
  defaultOptions,
  fromProject,
  isBlocked,
  OPTION_GROUPS,
  presetOf,
  PRESET_IDS,
  receivesSomething,
  setOption,
  summary,
  toRequest,
} from "./projectOptions";

describe("presets", () => {
  it("a new project starts on the default preset, public, with its roadmap open", () => {
    const o = defaultOptions();
    expect(presetOf(o)).toBe("software");
    expect(o.private).toBe(false);
    expect(o.roadmap).toBe(true);
  });

  it("each preset is recognised as itself", () => {
    for (const id of PRESET_IDS) expect(presetOf(applyPreset(defaultOptions(), id))).toBe(id);
  });

  it("a preset leaves who may see the project alone", () => {
    const hidden = { ...defaultOptions(), private: true, roadmap: false };
    const after = applyPreset(hidden, "community");
    expect(after.private).toBe(true);
    expect(after.roadmap).toBe(false);
  });

  it("changing one report option makes it custom, and visibility does not", () => {
    const o = defaultOptions();
    expect(presetOf(setOption(o, "images", false))).toBe("custom");
    expect(presetOf(setOption(o, "private", true))).toBe("software");
    expect(presetOf(setOption(o, "roadmap", false))).toBe("software");
  });
});

describe("dependencies", () => {
  it("the problem questions go with problems, and votes with ideas", () => {
    const noBugs = setOption(defaultOptions(), "bugs", false);
    expect(noBugs.version).toBe(false);
    expect(noBugs.steps).toBe(false);
    expect(isBlocked(noBugs, "version")).toBe(true);
    const noIdeas = setOption(defaultOptions(), "ideas", false);
    expect(noIdeas.votes).toBe(false);
    expect(isBlocked(noIdeas, "votes")).toBe(true);
    expect(isBlocked(noIdeas, "images")).toBe(false);
  });

  it("a blocked option cannot be turned on by itself", () => {
    const noBugs = setOption(defaultOptions(), "bugs", false);
    expect(setOption(noBugs, "steps", true).steps).toBe(false);
  });

  it("a project with neither kind receives nothing", () => {
    const o = setOption(setOption(defaultOptions(), "bugs", false), "ideas", false);
    expect(receivesSomething(o)).toBe(false);
    expect(receivesSomething(defaultOptions())).toBe(true);
  });
});

describe("summary and requests", () => {
  it("lists every option but who may see it, on one side or the other", () => {
    const all = OPTION_GROUPS.flatMap((g) => g.options.map((o) => o.key)).filter((k) => k !== "private");
    const s = summary(applyPreset(defaultOptions(), "ideas"));
    expect([...s.on, ...s.off].sort()).toEqual([...all].sort());
    expect(s.off).toContain("bugs");
    expect(s.on).toContain("votes");
  });

  it("sends what the server names, and reads back what it sends", () => {
    const o = { ...applyPreset(defaultOptions(), "community"), private: true, roadmap: false };
    const body = toRequest(o);
    expect(body).toEqual({
      accepts_bugs: true,
      accepts_ideas: true,
      accepts_images: true,
      asks_version: false,
      asks_steps: false,
      votes: true,
      roadmap_public: false,
      private: true,
    });
    expect(fromProject(body)).toEqual(o);
  });

  it("a project from before the options keeps votes and its roadmap open", () => {
    const o = fromProject({ accepts_bugs: true, accepts_ideas: true });
    expect(o.votes).toBe(true);
    expect(o.roadmap).toBe(true);
    expect(o.images).toBe(true);
  });
});
