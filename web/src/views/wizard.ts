/**
 * The step wizard, and the fields that go inside it.
 *
 * Pure functions like the rest of the templates: they take their text as a
 * parameter rather than importing the i18n singleton, which is what keeps them
 * testable without a browser.
 *
 * This exists because the panel used to be a wall of inputs with no order and
 * no examples. The operator who asked for it put it plainly: he did not know
 * what went in each box, because the boxes asked for formats and showed none.
 */
import { escapeAttr, escapeHTML } from "../escape";
import type { Text } from "./templates";

/**
 * Real examples, per forge.
 *
 * Deliberately not translated. A URL, an account name and a repository name are
 * machine values: translating them would invent accounts that do not exist, and
 * the point of an example is that the reader can recognise the shape.
 *
 * They are also real projects rather than `example.com/my-org/my-repo`, which
 * teaches nothing about what a forge owner actually looks like. Real, and
 * somebody else's: an example is a shape to recognise, not an invitation to
 * read whoever runs this installation.
 */
export interface ForgeExample {
  url: string;
  owner: string;
  repo: string;
  /** The narrowest scope that works, which is what the docs tell people to make. */
  scope: string;
  /** Where the token is created, in that forge's own words. */
  where: string;
}

export const forgeExamples: Record<string, ForgeExample> = {
  github: {
    url: "https://github.com",
    owner: "immich-app",
    repo: "immich",
    scope: "repo",
    where: "Settings → Developer settings → Personal access tokens",
  },
  gitlab: {
    url: "https://gitlab.com",
    owner: "gitlab-org",
    repo: "gitlab",
    scope: "api",
    where: "Preferences → Access tokens",
  },
  gitea: {
    url: "https://gitea.com",
    owner: "gitea",
    repo: "gitea",
    scope: "write:issue",
    where: "Settings → Applications → Access tokens",
  },
  forgejo: {
    url: "https://codeberg.org",
    owner: "forgejo",
    repo: "forgejo",
    scope: "write:issue",
    where: "Settings → Applications → Access tokens",
  },
};

/** The example for a forge, falling back to Forgejo's shape for an unknown one. */
export function exampleFor(forge: string): ForgeExample {
  return forgeExamples[forge] ?? forgeExamples.forgejo!;
}

export interface FieldSpec {
  id: string;
  name: string;
  label: string;
  /** What goes in the box, in one sentence. Optional: obvious fields need none. */
  hint?: string;
  /** A real value of the right shape. Shown as the placeholder and beside it. */
  example?: string;
  type?: string;
  required?: boolean;
  value?: string;
  pattern?: string;
  autocomplete?: string;
}

/**
 * One labelled field.
 *
 * The example goes in the hint line as well as in the placeholder. A
 * placeholder alone disappears at the first keystroke — exactly when somebody
 * is checking the shape of what they are typing — and it is grey-on-grey for
 * anybody who needs contrast. Showing it twice at rest was worse: the same word
 * in the box and again underneath reads as two different things.
 */
export function field(spec: FieldSpec, text?: Text): string {
  const attrs = [
    `id="${escapeAttr(spec.id)}"`,
    `name="${escapeAttr(spec.name)}"`,
    `type="${escapeAttr(spec.type ?? "text")}"`,
    spec.required ? "required" : "",
    spec.value !== undefined ? `value="${escapeAttr(spec.value)}"` : "",
    spec.pattern ? `pattern="${escapeAttr(spec.pattern)}"` : "",
    spec.example ? `placeholder="${escapeAttr(spec.example)}"` : "",
    spec.autocomplete ? `autocomplete="${escapeAttr(spec.autocomplete)}"` : "",
  ]
    .filter(Boolean)
    .join(" ");

  const example =
    spec.example && text
      ? ` ${escapeHTML(text.t("wizard.example"))} <code class="example">${escapeHTML(spec.example)}</code>`
      : "";

  return `<div class="field">
    <label for="${escapeAttr(spec.id)}">${escapeHTML(spec.label)}</label>
    <input ${attrs} />
    ${spec.hint || example ? `<p class="hint">${escapeHTML(spec.hint ?? "")}${example}</p>` : ""}
  </div>`;
}

/**
 * The same field, when the answer is more than a line.
 *
 * Its own function rather than a flag on `field`, because the two differ in
 * more than a tag: a textarea has no `value` attribute and no `placeholder`
 * that survives, and half of `field`'s attribute list means nothing here.
 */
export function areaField(
  spec: Omit<FieldSpec, "type" | "pattern" | "autocomplete"> & { rows?: number },
  text?: Text,
): string {
  const example =
    spec.example && text
      ? ` ${escapeHTML(text.t("wizard.example"))} <code class="example">${escapeHTML(spec.example)}</code>`
      : "";

  return `<div class="field">
    <label for="${escapeAttr(spec.id)}">${escapeHTML(spec.label)}</label>
    <textarea id="${escapeAttr(spec.id)}" name="${escapeAttr(spec.name)}" rows="${spec.rows ?? 5}"
      ${spec.required ? "required" : ""}
      ${spec.example ? `placeholder="${escapeAttr(spec.example)}"` : ""}>${escapeHTML(spec.value ?? "")}</textarea>
    ${spec.hint || example ? `<p class="hint">${escapeHTML(spec.hint ?? "")}${example}</p>` : ""}
  </div>`;
}

/** A labelled <select>, with the same shape as a field so a form reads evenly. */
export function choiceField(
  spec: Omit<FieldSpec, "example"> & { options: [string, string][]; selected?: string },
): string {
  return `<div class="field">
    <label for="${escapeAttr(spec.id)}">${escapeHTML(spec.label)}</label>
    <select id="${escapeAttr(spec.id)}" name="${escapeAttr(spec.name)}">
      ${spec.options
        .map(
          ([value, label]) =>
            `<option value="${escapeAttr(value)}"${value === spec.selected ? " selected" : ""}>${escapeHTML(label)}</option>`,
        )
        .join("")}
    </select>
    ${spec.hint ? `<p class="hint">${escapeHTML(spec.hint)}</p>` : ""}
  </div>`;
}

export interface WizardSpec {
  /** 1-based. */
  step: number;
  /** The name of each step, in order. Its length is how many there are. */
  names: string[];
  title: string;
  lead?: string;
  /** The markup inside the card — fields, usually. */
  body: string;
  /** The label on the button that submits. */
  submit: string;
  /** Shown as a plain link beside the button, if this step can be left. */
  back?: { href: string; label: string };
  /** Extra class on the form, so main.ts can find it. */
  formClass: string;
  /** Drawn above the steps when given: the first run, and nowhere else. */
  mark?: string;
}

/**
 * The whole step: dots, heading, card, actions.
 *
 * The dots are an ordered list with aria-current on the one you are on, so the
 * progress is in the markup and not only in the shape — a screen reader gets
 * "step 2 of 3" without being able to see a row of circles.
 */
export function wizard(spec: WizardSpec, text: Text): string {
  const total = spec.names.length;
  const dots = spec.names
    .map((name, index) => {
      const n = index + 1;
      const state = n < spec.step ? "done" : n === spec.step ? "now" : "";
      return `<li class="step ${state}"${n === spec.step ? ' aria-current="step"' : ""}>
        <span class="dot" aria-hidden="true"></span>
        <span class="step-name">${escapeHTML(name)}</span>
      </li>`;
    })
    .join("");

  return `<section class="wizard">
    <!-- The mark, above the steps, on the first run only. A wizard inside the
         panel is already under a sidebar wearing it; a second one in the middle
         of the page is the same logo twice on one screen. -->
    ${
      spec.mark
        ? `<img class="wizard-mark brand-logo mark-52" src="${escapeAttr(spec.mark)}" alt="" width="52" height="52" />`
        : ""
    }
    <ol class="steps" aria-label="${escapeAttr(text.t("wizard.progress", { step: spec.step, total }))}">
      ${dots}
    </ol>
    <header class="wizard-head">
      <h1>${escapeHTML(spec.title)}</h1>
      ${spec.lead ? `<p class="lead">${escapeHTML(spec.lead)}</p>` : ""}
    </header>
    <form class="${escapeAttr(spec.formClass)}">
      <div class="wizard-card">${spec.body}</div>
      <p class="form-error" role="alert" hidden></p>
      <div class="wizard-actions">
        ${spec.back ? `<a class="button ghost" href="${escapeAttr(spec.back.href)}">${escapeHTML(spec.back.label)}</a>` : ""}
        <span class="spacer"></span>
        <button class="button primary" type="submit">${escapeHTML(spec.submit)}</button>
      </div>
    </form>
  </section>`;
}
