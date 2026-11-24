/**
 * What a project takes and who sees it, as one set of choices.
 *
 * The create dialog and the project's settings draw the same groups from this
 * catalogue, so a project can be made the way it will stay without opening its
 * settings afterwards. The presets are shortcuts over the reports group only:
 * who may see the project is a separate decision, and choosing "Community"
 * should not quietly make a private project public.
 */

export interface ProjectOptions {
  bugs: boolean;
  /** The problem form asks which version, and how to make it happen again. */
  version: boolean;
  steps: boolean;
  ideas: boolean;
  /** Visitors vote on ideas. */
  votes: boolean;
  images: boolean;
  /** Hidden from everybody who is not on the team. */
  private: boolean;
  /** The roadmap is open to visitors; off, only the team reads it. */
  roadmap: boolean;
}

export type OptionKey = keyof ProjectOptions;
export type PresetId = "software" | "community" | "ideas";
export type PresetName = PresetId | "custom";

export const PRESET_IDS: PresetId[] = ["software", "community", "ideas"];

/** The keys a preset sets. */
export const PRESET_KEYS: OptionKey[] = ["bugs", "version", "steps", "ideas", "votes", "images"];

export const PRESETS: Record<PresetId, Pick<ProjectOptions, "bugs" | "version" | "steps" | "ideas" | "votes" | "images">> = {
  software: { bugs: true, version: true, steps: true, ideas: true, votes: true, images: true },
  community: { bugs: true, version: false, steps: false, ideas: true, votes: true, images: true },
  ideas: { bugs: false, version: false, steps: false, ideas: true, votes: true, images: true },
};

export const DEFAULT_PRESET: PresetId = "software";

export interface OptionDef {
  key: OptionKey;
  /** Only means something while this one is on. */
  needs?: OptionKey;
}

export interface OptionGroup {
  id: "reports" | "visibility";
  options: OptionDef[];
}

/** The catalogue, grouped, in the order the dialog shows it. */
export const OPTION_GROUPS: OptionGroup[] = [
  {
    id: "reports",
    options: [
      { key: "bugs" },
      { key: "version", needs: "bugs" },
      { key: "steps", needs: "bugs" },
      { key: "ideas" },
      { key: "votes", needs: "ideas" },
      { key: "images" },
    ],
  },
  { id: "visibility", options: [{ key: "private" }, { key: "roadmap" }] },
];

const ALL: OptionDef[] = OPTION_GROUPS.flatMap((g) => g.options);

/** What a new project starts with. */
export function defaultOptions(): ProjectOptions {
  return { ...PRESETS[DEFAULT_PRESET], private: false, roadmap: true };
}

/** Whether an option is switched off by the one it depends on. */
export function isBlocked(o: ProjectOptions, key: OptionKey): boolean {
  const needs = ALL.find((d) => d.key === key)?.needs;
  return !!needs && !o[needs];
}

/** Every dependent option cleared while the one it needs is off. */
export function normalize(o: ProjectOptions): ProjectOptions {
  const out = { ...o };
  for (const d of ALL) if (d.needs && !out[d.needs]) out[d.key] = false;
  return out;
}

export function setOption(o: ProjectOptions, key: OptionKey, on: boolean): ProjectOptions {
  return normalize({ ...o, [key]: on });
}

/** A preset changes what the project receives and leaves who may see it alone. */
export function applyPreset(o: ProjectOptions, id: PresetId): ProjectOptions {
  return normalize({ ...o, ...PRESETS[id] });
}

/** Which preset these choices are exactly, or custom. */
export function presetOf(o: ProjectOptions): PresetName {
  const n = normalize(o);
  for (const id of PRESET_IDS) {
    if (PRESET_KEYS.every((k) => n[k] === PRESETS[id][k as keyof (typeof PRESETS)[PresetId]])) return id;
  }
  return "custom";
}

/** Whether the server would take it: a project that receives nothing is refused. */
export function receivesSomething(o: ProjectOptions): boolean {
  return o.bugs || o.ideas;
}

/**
 * What the project will and will not have, one entry per option, in the
 * catalogue's order. Who may see it is not on either side: the summary says
 * that in a sentence of its own.
 */
export function summary(o: ProjectOptions): { on: OptionKey[]; off: OptionKey[] } {
  const n = normalize(o);
  const on: OptionKey[] = [];
  const off: OptionKey[] = [];
  for (const d of ALL) {
    if (d.key === "private") continue;
    (n[d.key] ? on : off).push(d.key);
  }
  return { on, off };
}

/** The choices as the server's create and update bodies name them. */
export interface ProjectRequest {
  accepts_bugs: boolean;
  accepts_ideas: boolean;
  accepts_images: boolean;
  asks_version: boolean;
  asks_steps: boolean;
  votes: boolean;
  roadmap_public: boolean;
  private: boolean;
}

export function toRequest(o: ProjectOptions): ProjectRequest {
  const n = normalize(o);
  return {
    accepts_bugs: n.bugs,
    accepts_ideas: n.ideas,
    accepts_images: n.images,
    asks_version: n.version,
    asks_steps: n.steps,
    votes: n.votes,
    roadmap_public: n.roadmap,
    private: n.private,
  };
}

/** The choices a project has, from what the server says about it. */
export function fromProject(p: Pick<ProjectRequest, "accepts_bugs" | "accepts_ideas"> & Partial<ProjectRequest>): ProjectOptions {
  return {
    bugs: p.accepts_bugs,
    ideas: p.accepts_ideas,
    images: p.accepts_images ?? true,
    version: p.asks_version ?? false,
    steps: p.asks_steps ?? false,
    votes: p.votes ?? true,
    roadmap: p.roadmap_public ?? true,
    private: p.private ?? false,
  };
}
