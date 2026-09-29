/**
 * The front page editor's own markup: the bar above the page, the panel beside
 * it and the two pickers. The page itself is drawn by ./landing, with editing
 * on; nothing here draws a second version of it.
 *
 * Pure functions, like every view, so the text comes in as a parameter.
 */
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";
import {
  KIND_NAMES,
  LANDING_LAYOUTS,
  type LandingBand,
  type LandingBoard,
  type LandingColumns,
  type LandingKind,
  type LandingLink,
  type LandingModule,
  type LandingSettings,
} from "./landing";
import type { Text } from "./templates";

// --- the bar ------------------------------------------------------------------

export interface EditBarState {
  canUndo: boolean;
  canRedo: boolean;
  phone: boolean;
  saving: "idle" | "saving" | "failed";
  /** Why the page cannot be saved as it is, in words for the operator. */
  problem?: string;
  /** Whether the page settings are what the panel shows. */
  pageOpen?: boolean;
}

/**
 * The editor's bar: undo and redo, the width it is previewed at, and whether
 * the last change is saved. There is nothing to publish: every change is the
 * page.
 */
export function editBar(s: EditBarState, text: Text): string {
  const state = s.problem
    ? `<span class="lpe-state problem" role="alert">${icons.warning(14)}<span>${escapeHTML(s.problem)}</span></span>`
    : s.saving === "failed"
      ? `<span class="lpe-state problem" role="alert">${icons.warning(14)}<span>${escapeHTML(
          text.t("landing.notSaved"),
        )}</span></span>`
      : s.saving === "saving"
        ? `<span class="lpe-state draft"><i aria-hidden="true"></i><span>${escapeHTML(
            text.t("landing.saving"),
          )}</span></span>`
        : `<span class="lpe-state live"><i aria-hidden="true"></i><span>${escapeHTML(
            text.t("landing.saved"),
          )}</span></span>`;
  const barTool = (act: string, label: string, icon: string, on = true, pressed?: boolean) =>
    `<button type="button" class="icon-button${on ? "" : " cannot"}" data-bar="${act}" ${on ? "" : "disabled"}
       ${pressed === undefined ? "" : `aria-pressed="${pressed}"`}
       title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}">${icon}</button>`;
  return `<div class="lpe-bar" role="toolbar" aria-label="${escapeAttr(text.t("landing.compose"))}">
    <div class="lpe-bar-group">
      ${barTool("undo", `${text.t("words.undo")} (Ctrl+Z)`, icons.undo(16), s.canUndo)}
      ${barTool("redo", `${text.t("words.redo")} (Ctrl+Shift+Z)`, icons.redo(16), s.canRedo)}
    </div>
    <div class="lpe-bar-group">
      ${barTool("desktop", text.t("landing.previewDesktop"), icons.monitor(16), true, !s.phone)}
      ${barTool("phone", text.t("landing.previewPhone"), phoneIcon(), true, s.phone)}
    </div>
    ${state}
    <div class="lpe-bar-group end">
      ${barTool("settings", text.t("landing.pageSettings"), icons.palette(16), true, s.pageOpen === true)}
    </div>
  </div>`;
}

function phoneIcon(): string {
  return `<svg class="icon" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
    stroke-width="1.8" stroke-linecap="round" aria-hidden="true" focusable="false">
    <rect x="7" y="3" width="10" height="18" rx="2"/><path d="M11 18h2"/></svg>`;
}

// --- the panel ----------------------------------------------------------------

export interface PanelContext {
  boards: LandingBoard[];
  instance: { name?: string };
  tab: "content" | "appearance";
  /** The default name a band of the default page wears, or "". */
  defaultName: (band: LandingBand) => string;
  /** Whether a public address is configured, which a share picture needs. */
  shareReady: boolean;
}

/**
 * The panel beside the page: what the chosen band or module is made of.
 *
 * Every control writes to the draft as it is changed, and the page beside it
 * redraws — there is no Save here, because the page IS the preview and a save
 * button would make it a question whether what is on it is real.
 */
export function panel(
  chosen:
    | { band: LandingBand }
    | { module: LandingModule }
    | { page: LandingSettings }
    | { footer: LandingSettings }
    | null,
  ctx: PanelContext,
  text: Text,
): string {
  if (!chosen) {
    return `<div class="lpe-panel-empty">
      ${icons.module(28)}
      <p>${escapeHTML(text.t("landing.chooseSomething"))}</p>
    </div>`;
  }
  if ("page" in chosen) return pagePanel(chosen.page, ctx, text);
  if ("footer" in chosen) return footerPanel(chosen.footer, ctx, text);
  if ("band" in chosen) return bandPanel(chosen.band, ctx, text);
  return modulePanel(chosen.module, ctx, text);
}

function panelHead(icon: string, title: string, text: Text): string {
  return `<header class="lpe-panel-head">
    <span class="lpe-panel-icon" aria-hidden="true">${icon}</span>
    <h2>${escapeHTML(title)}</h2>
    <button type="button" class="lpe-close" data-bar="deselect"
      title="${escapeAttr(text.t("action.close"))}" aria-label="${escapeAttr(text.t("action.close"))}">${icons.close(16)}</button>
  </header>`;
}

/** A segmented choice: a short closed list, all visible at once. */
function choice(bind: string, current: string, options: Array<[string, string]>, label: string): string {
  return `<div class="field">
    <span class="label">${escapeHTML(label)}</span>
    <div class="segmented lpe-seg" role="radiogroup" aria-label="${escapeAttr(label)}">
      ${options
        .map(
          ([value, name]) => `<button type="button" class="segment${value === current ? " on" : ""}" role="radio"
            aria-checked="${value === current}" data-set="${escapeAttr(bind)}" data-value="${escapeAttr(value)}">
            <span>${escapeHTML(name)}</span></button>`,
        )
        .join("")}
    </div>
  </div>`;
}

function input(bind: string, label: string, value: string, opts: { placeholder?: string; hint?: string; max?: number; required?: boolean; type?: string } = {}): string {
  const id = `lpe-${bind.replace(/\W/g, "-")}`;
  return `<div class="field">
    <label for="${escapeAttr(id)}">${escapeHTML(label)}</label>
    <input id="${escapeAttr(id)}" type="${escapeAttr(opts.type ?? "text")}" data-bind="${escapeAttr(bind)}"
      value="${escapeAttr(value)}" ${opts.placeholder ? `placeholder="${escapeAttr(opts.placeholder)}"` : ""}
      ${opts.max ? `maxlength="${opts.max}"` : ""} ${opts.required ? "required" : ""} autocomplete="off" />
    ${opts.hint ? `<p class="hint">${escapeHTML(opts.hint)}</p>` : ""}
  </div>`;
}

function check(bind: string, label: string, on: boolean, invert = false): string {
  return `<label class="lpe-check">
    <input type="checkbox" data-bind="${escapeAttr(bind)}" ${invert ? 'data-invert="1"' : ""} ${on ? "checked" : ""} />
    <span>${escapeHTML(label)}</span>
  </label>`;
}

function select(bind: string, label: string, current: string, options: Array<[string, string]>, attrs = ""): string {
  const id = `lpe-${bind.replace(/\W/g, "-")}`;
  return `<div class="field">
    <label for="${escapeAttr(id)}">${escapeHTML(label)}</label>
    <select id="${escapeAttr(id)}" data-bind="${escapeAttr(bind)}" ${attrs}>
      ${options
        .map(
          ([value, name]) =>
            `<option value="${escapeAttr(value)}"${value === current ? " selected" : ""}>${escapeHTML(name)}</option>`,
        )
        .join("")}
    </select>
  </div>`;
}

/**
 * Where a data module reads from: everything, one organisation, or one board.
 * A board that no longer exists keeps its line, marked, so the choice on the
 * page is still the one shown here.
 */
export function scopeOptions(boards: LandingBoard[], current: string, text: Text, boardsOnly = false): Array<[string, string]> {
  const out: Array<[string, string]> = boardsOnly ? [] : [["", text.t("landing.scopeAll")]];
  const orgs = [...new Map(boards.map((b) => [b.org, b.org_name ?? b.org])).entries()];
  for (const [org, orgName] of orgs) {
    if (!boardsOnly) out.push([org, text.t("landing.scopeOrg", { org: orgName })]);
    for (const b of boards.filter((x) => x.org === org)) {
      out.push([`${b.org}/${b.slug}`, orgs.length > 1 || !boardsOnly ? `${orgName} / ${b.name}` : b.name]);
    }
  }
  if (current && !out.some(([v]) => v === current)) out.push([current, text.t("landing.scopeGone", { scope: current })]);
  return out;
}

/**
 * The page as a whole: what a browser tab and a shared link call it, the
 * picture a shared link shows, the links at its foot, and the way back to the
 * page every installation starts with.
 */
function pagePanel(set: LandingSettings, ctx: PanelContext, text: Text): string {
  return `${panelHead(icons.palette(16), text.t("landing.pageSettings"), text)}
    <div class="lpe-panel-body">
      ${input("page.title", text.t("landing.pageTitle"), set.title ?? "", {
        placeholder: ctx.instance.name || text.t("app.name"),
        hint: text.t("landing.pageTitleHint"),
        max: 120,
      })}
      ${input("page.description", text.t("landing.pageDescription"), set.description ?? "", {
        hint: text.t("landing.pageDescriptionHint"),
        max: 200,
      })}
      <div class="field">
        <span class="label">${escapeHTML(text.t("landing.shareImage"))}</span>
        <div class="lpe-image">
          ${
            set.share_image
              ? `<img src="${escapeAttr(set.share_image)}" alt="" />`
              : `<div class="lpe-image-none">${icons.image(28)}</div>`
          }
          <div class="lpe-image-actions">
            <button type="button" class="button ghost small" data-act="pick-share">${icons.upload(14)}<span>${escapeHTML(
              text.t(set.share_image ? "landing.replaceImage" : "landing.uploadImage"),
            )}</span></button>
            ${
              set.share_image
                ? `<button type="button" class="button ghost small danger" data-act="drop-share">${escapeHTML(text.t("action.remove"))}</button>`
                : ""
            }
          </div>
        </div>
        <p class="hint">${escapeHTML(text.t(ctx.shareReady ? "landing.shareImageHint" : "landing.shareImageNoURL"))}</p>
      </div>
      <div class="lpe-restore">
        <p class="hint">${escapeHTML(text.t("landing.restoreHint"))}</p>
        <button type="button" class="button ghost small danger" data-act="restore">${escapeHTML(text.t("landing.restore"))}</button>
      </div>
    </div>`;
}

/**
 * The foot of the page, every part of it: the line of words, which is also
 * written in place on the page; the name and mark and the credit, each kept or
 * dropped; and the links.
 */
function footerPanel(set: LandingSettings, ctx: PanelContext, text: Text): string {
  return `${panelHead(icons.section(16), text.t("landing.footer"), text)}
    <div class="lpe-panel-body">
      ${input("page.footer_text", text.t("landing.footerText"), set.footer_text ?? "", {
        placeholder: text.t("landing.footerTextPlaceholder"),
        max: 240,
      })}
      ${check("page.footer_hide_brand", text.t("landing.showBrand"), set.footer_hide_brand !== true, true)}
      ${check("page.footer_hide_credit", text.t("landing.showCredit"), set.footer_hide_credit !== true, true)}
      <div class="field">
        <span class="label">${escapeHTML(text.t("landing.footerLinks"))}</span>
        ${linksFields(set.footer ?? [], ctx, text)}
      </div>
    </div>`;
}

function bandPanel(b: LandingBand, ctx: PanelContext, text: Text): string {
  const fallback = ctx.defaultName(b);
  return `${panelHead(icons.section(16), text.t("landing.section"), text)}
    <div class="lpe-panel-body">
      ${input("band.name", text.t("landing.name"), b.name ?? "", {
        placeholder: fallback || text.t("landing.noName"),
        max: 80,
      })}
      ${fallback ? check("band.hide_name", text.t("landing.hideName"), b.hide_name === true) : ""}
      <div class="field">
        <span class="label">${escapeHTML(text.t("landing.layout"))}</span>
        <div class="lpe-layouts" role="radiogroup" aria-label="${escapeAttr(text.t("landing.layout"))}">
          ${LANDING_LAYOUTS.map(
            (l) => `<button type="button" class="lpe-layout${l === b.columns ? " on" : ""}" role="radio"
              aria-checked="${l === b.columns}" data-set="band.columns" data-value="${l}"
              title="${escapeAttr(layoutName(l, text))}" aria-label="${escapeAttr(layoutName(l, text))}">${layoutDiagram(l)}</button>`,
          ).join("")}
        </div>
        <p class="hint">${escapeHTML(text.t("landing.layoutHint"))}</p>
      </div>
      ${choice("band.background", b.background ?? "", [
        ["", text.t("landing.bgNone")],
        ["soft", text.t("landing.bgSoft")],
        ["accent", text.t("landing.bgAccent")],
      ], text.t("landing.background"))}
      ${choice("band.width", b.width ?? "", [
        ["", text.t("landing.widthPage")],
        ["full", text.t("landing.widthFull")],
      ], text.t("landing.width"))}
      ${choice("band.spacing", b.spacing ?? "", [
        ["compact", text.t("landing.spaceCompact")],
        ["", text.t("landing.spaceNormal")],
        ["airy", text.t("landing.spaceAiry")],
      ], text.t("landing.spacing"))}
    </div>`;
}

export function layoutName(l: LandingColumns, text: Text): string {
  return text.t(`landing.layouts.${l.replace(/-/g, "_")}`);
}

/** A layout, drawn as its columns. */
export function layoutDiagram(l: LandingColumns): string {
  return `<span class="lpe-diagram" aria-hidden="true">${l
    .split("-")
    .map((w) => `<i class="w${w}"></i>`)
    .join("")}</span>`;
}

const KIND_ICONS: Record<LandingKind, (s?: number) => string> = {
  heading: icons.overview,
  text: icons.edit,
  search: icons.search,
  boards: icons.repository,
  reports: icons.idea,
  roadmap: icons.open,
  statistics: icons.monitor,
  links: icons.link,
  image: icons.image,
};

export function kindIcon(kind: LandingKind, size = 16): string {
  return KIND_ICONS[kind](size);
}

const NAMED = new Set<LandingKind>(["boards", "reports", "roadmap", "statistics", "links"]);

function modulePanel(m: LandingModule, ctx: PanelContext, text: Text): string {
  const tabs = `<div class="lpe-tabs" role="tablist">
    ${(["content", "appearance"] as const)
      .map(
        (t) => `<button type="button" role="tab" class="lpe-tab${ctx.tab === t ? " on" : ""}" aria-selected="${ctx.tab === t}"
          data-tab="${t}">${escapeHTML(text.t(t === "content" ? "landing.tabContent" : "landing.tabAppearance"))}</button>`,
      )
      .join("")}
  </div>`;
  const body = ctx.tab === "content" ? contentFields(m, ctx, text) : appearanceFields(m, text);
  return `${panelHead(kindIcon(m.kind), text.t(KIND_NAMES[m.kind]), text)}
    ${tabs}
    <div class="lpe-panel-body">${body}</div>`;
}

function appearanceFields(m: LandingModule, text: Text): string {
  const named = NAMED.has(m.kind);
  return `${choice("style", m.style ?? "", [
    ["", text.t("landing.styleCard")],
    ["soft", text.t("landing.styleSoft")],
    ["plain", text.t("landing.stylePlain")],
  ], text.t("landing.style"))}
  ${
    named
      ? `${input("name", text.t("landing.name"), m.name ?? "", { placeholder: text.t("landing.nameDefault"), max: 80 })}
         ${check("hide_name", text.t("landing.hideName"), m.hide_name === true)}`
      : ""
  }`;
}

function contentFields(m: LandingModule, ctx: PanelContext, text: Text): string {
  const scope = (label = text.t("landing.scope")) => select("scope", label, m.scope ?? "", scopeOptions(ctx.boards, m.scope ?? "", text));
  switch (m.kind) {
    case "heading":
      return `${input("title", text.t("landing.title"), m.title ?? "", {
        placeholder: ctx.instance.name || text.t("app.name"),
        max: 120,
      })}
        ${input("subtitle", text.t("landing.subtitle"), m.subtitle ?? "", { placeholder: text.t("app.tagline"), max: 240 })}
        ${choice("align", m.align ?? "", [
          ["", text.t("landing.alignCenter")],
          ["start", text.t("landing.alignStart")],
        ], text.t("landing.align"))}
        ${check("hide_logo", text.t("landing.showLogo"), m.hide_logo !== true, true)}
        ${m.hide_logo ? "" : choice("logo_size", m.logo_size ?? "", [
          ["small", text.t("landing.logoSmall")],
          ["", text.t("landing.logoMedium")],
          ["large", text.t("landing.logoLarge")],
        ], text.t("landing.logoSize"))}
        ${check("with_search", text.t("landing.showSearch"), m.with_search === true)}
        <p class="hint">${escapeHTML(text.t("landing.writeOnPage"))}</p>`;
    case "text":
      return `${choice("tone", m.tone ?? "", [
        ["", text.t("landing.toneNormal")],
        ["note", text.t("landing.toneNote")],
        ["notice", text.t("landing.toneNotice")],
      ], text.t("landing.tone"))}
        <p class="hint">${escapeHTML(text.t("landing.writeOnPage"))}</p>`;
    case "search":
      return `<p class="hint">${escapeHTML(text.t("landing.aboutSearch"))}</p>`;
    case "boards":
      return `${choice("display", m.display ?? "", [
        ["", text.t("landing.displayCards")],
        ["list", text.t("landing.displayList")],
      ], text.t("landing.display"))}
        ${scope()}`;
    case "reports":
      return `${select("show", text.t("landing.show"), m.show === "most_voted" ? "" : (m.show ?? ""), [
        ["", text.t("landing.moduleVoted")],
        ["recently_done", text.t("landing.moduleDone")],
        ["latest", text.t("landing.moduleActivity")],
        ["planned", text.t("landing.showPlanned")],
      ])}
        ${input("rows", text.t("landing.rows"), String(m.rows || 5), { type: "number", max: 2 })}
        ${scope()}`;
    case "roadmap":
    case "statistics":
      return scope();
    case "links":
      return linksFields(m.links ?? [], ctx, text);
    case "image":
      return imageFields(m, text);
  }
}

function linksFields(links: LandingLink[], ctx: PanelContext, text: Text): string {
  const boards = scopeOptions(ctx.boards, "", text, true);
  const rows = links
    .map((l, i) => {
      const where =
        l.to === "url"
          ? `<input type="url" data-link="${i}" data-link-field="url" value="${escapeAttr(l.url ?? "")}"
               placeholder="https://" aria-label="${escapeAttr(text.t("landing.linkURL"))}" />`
          : `<select data-link="${i}" data-link-field="scope" aria-label="${escapeAttr(text.t("landing.linkBoard"))}">
               <option value="">${escapeHTML(text.t("landing.linkPickBoard"))}</option>
               ${[...boards, ...(l.scope && !boards.some(([v]) => v === l.scope) ? [[l.scope, l.scope] as [string, string]] : [])]
                 .map(([v, n]) => `<option value="${escapeAttr(v)}"${v === l.scope ? " selected" : ""}>${escapeHTML(n)}</option>`)
                 .join("")}
             </select>`;
      return `<li class="lpe-link">
        <div class="lpe-link-head">
          <input type="text" data-link="${i}" data-link-field="label" value="${escapeAttr(l.label)}" maxlength="80"
            placeholder="${escapeAttr(text.t("landing.linkLabel"))}" aria-label="${escapeAttr(text.t("landing.linkLabel"))}" />
          <button type="button" class="lpe-tool" data-link="${i}" data-link-act="up" data-turn="up"
            title="${escapeAttr(text.t("landing.moveUp"))}" aria-label="${escapeAttr(text.t("landing.moveUp"))}" ${i === 0 ? "disabled" : ""}>${icons.chevron(14)}</button>
          <button type="button" class="lpe-tool" data-link="${i}" data-link-act="remove" data-danger="1"
            title="${escapeAttr(text.t("action.remove"))}" aria-label="${escapeAttr(text.t("action.remove"))}">${icons.remove(14)}</button>
        </div>
        <select data-link="${i}" data-link-field="to" aria-label="${escapeAttr(text.t("landing.linkTo"))}">
          ${(
            [
              ["form", text.t("landing.toForm")],
              ["board", text.t("landing.toBoard")],
              ["roadmap", text.t("landing.toRoadmap")],
              ["url", text.t("landing.toURL")],
            ] as Array<[string, string]>
          )
            .map(([v, n]) => `<option value="${v}"${v === l.to ? " selected" : ""}>${escapeHTML(n)}</option>`)
            .join("")}
        </select>
        ${where}
        <label class="lpe-check">
          <input type="checkbox" data-link="${i}" data-link-field="button" ${l.button ? "checked" : ""} />
          <span>${escapeHTML(text.t("landing.linkButton"))}</span>
        </label>
      </li>`;
    })
    .join("");
  return `<ul class="lpe-links">${rows}</ul>
    <button type="button" class="button ghost small" data-act="add-link" ${links.length >= 8 ? "disabled" : ""}>
      ${icons.add(14)}<span>${escapeHTML(text.t("landing.addLink"))}</span></button>`;
}

function imageFields(m: LandingModule, text: Text): string {
  return `<div class="lpe-image">
      ${
        m.image
          ? `<img src="${escapeAttr(m.image)}" alt="" />`
          : `<div class="lpe-image-none">${icons.image(28)}</div>`
      }
      <div class="lpe-image-actions">
        <button type="button" class="button ghost small" data-act="pick-image">${icons.upload(14)}<span>${escapeHTML(
          text.t(m.image ? "landing.replaceImage" : "landing.uploadImage"),
        )}</span></button>
        ${
          m.image
            ? `<button type="button" class="button ghost small danger" data-act="drop-image">${escapeHTML(text.t("action.remove"))}</button>`
            : ""
        }
      </div>
    </div>
    ${input("alt", text.t("landing.alt"), m.alt ?? "", { hint: text.t("landing.altHint"), max: 200, required: !!m.image })}
    ${input("caption", text.t("landing.caption"), m.caption ?? "", { max: 200 })}`;
}

// --- the pickers ----------------------------------------------------------------

type Card = [kind: string, icon: string, name: string, about: string];

function pickerDialog(id: string, title: string, lead: string, groups: Array<[string, string]>, text: Text): string {
  return `<dialog class="modal chooser lpe-picker" id="${escapeAttr(id)}">
    <div class="picker-head">
      <div class="title-group">
        <h2>${escapeHTML(title)}</h2>
        <p>${escapeHTML(lead)}</p>
      </div>
      <button type="button" class="picker-close" data-close
        aria-label="${escapeAttr(text.t("action.close"))}">${icons.close(20)}</button>
    </div>
    ${groups
      .map(
        ([name, cards]) => `<section class="lpe-pick-group">
          <h3 class="lpe-group-name">${escapeHTML(name)}</h3>
          <div class="picker-grid">${cards}</div>
        </section>`,
      )
      .join("")}
  </dialog>`;
}

function cards(list: Card[], attr: string): string {
  return list
    .map(
      ([kind, icon, name, about]) => `<button type="button" class="picker-card" ${attr}="${escapeAttr(kind)}">
        <span class="picker-icon" aria-hidden="true">${icon}</span>
        <span class="picker-name">${escapeHTML(name)}</span>
        <span class="picker-about">${escapeHTML(about)}</span>
      </button>`,
    )
    .join("");
}

/** The nine kinds, in two groups: what the operator writes, and what the portal fills in. */
export function modulePicker(text: Text): string {
  const card = (kind: LandingKind, about: string): Card => [kind, kindIcon(kind, 32), text.t(KIND_NAMES[kind]), text.t(about)];
  const content = cards(
    [
      card("heading", "landing.aboutHeading"),
      card("text", "landing.aboutText"),
      card("links", "landing.aboutLinks"),
      card("image", "landing.aboutImage"),
      card("search", "landing.aboutSearch"),
    ],
    "data-pick",
  );
  const portal = cards(
    [
      card("boards", "landing.aboutBoards"),
      card("reports", "landing.aboutReports"),
      card("roadmap", "landing.aboutRoadmap"),
      card("statistics", "landing.aboutStats"),
    ],
    "data-pick",
  );
  return pickerDialog(
    "lpe-module-picker",
    text.t("landing.newModule"),
    text.t("landing.newModuleLead"),
    [
      [text.t("landing.groupContent"), content],
      [text.t("landing.groupPortal"), portal],
    ],
    text,
  );
}

/** The ready-made sections a new one can start from, by id. */
export const READY_MADE = ["heading", "where", "lists", "points", "roadmap", "text", "links"] as const;
export type ReadyMade = (typeof READY_MADE)[number];

/** A section: empty in one of the six layouts, or one of the ready-made ones. */
export function sectionPicker(text: Text): string {
  const layouts = cards(
    LANDING_LAYOUTS.map((l): Card => [
      `layout:${l}`,
      layoutDiagram(l),
      layoutName(l, text),
      text.t("landing.aboutEmpty"),
    ]),
    "data-pick-section",
  );
  const readyIcons: Record<ReadyMade, string> = {
    heading: icons.overview(32),
    where: icons.repository(32),
    lists: icons.idea(32),
    points: icons.check(32),
    roadmap: icons.open(32),
    text: icons.edit(32),
    links: icons.link(32),
  };
  const ready = cards(
    READY_MADE.map((r): Card => [
      `ready:${r}`,
      readyIcons[r],
      text.t(`landing.ready.${r}`),
      text.t(`landing.readyAbout.${r}`),
    ]),
    "data-pick-section",
  );
  return pickerDialog(
    "lpe-section-picker",
    text.t("landing.newSection"),
    text.t("landing.newSectionLead"),
    [
      [text.t("landing.groupReady"), ready],
      [text.t("landing.groupLayouts"), layouts],
    ],
    text,
  );
}

// --- writing --------------------------------------------------------------------

/** The bar that follows whichever block of words has the cursor. */
export function wordsBar(text: Text): string {
  const tool = (does: string, label: string, icon: string, wide = false) =>
    `<button type="button" class="words-tool${wide ? " wide" : ""}" data-do="${escapeAttr(does)}"
      title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}">${icon}</button>`;

  const ink = (name: string, label: string) =>
    `<button type="button" class="words-ink ${name}" data-do="ink:${escapeAttr(name)}"
      title="${escapeAttr(label)}" aria-label="${escapeAttr(label)}"><span></span></button>`;

  return `<div class="words-bar" hidden>
    <div class="words-group">
      ${tool("formatBlock:h2", text.t("words.h2"), "H2", true)}
      ${tool("formatBlock:h3", text.t("words.h3"), "H3", true)}
      ${tool("formatBlock:p", text.t("words.paragraph"), "P", true)}
    </div>
    <div class="words-group">
      ${tool("bold", text.t("words.bold"), icons.bold(15))}
      ${tool("italic", text.t("words.italic"), icons.italic(15))}
      ${tool("underline", text.t("words.underline"), icons.underline(15))}
      ${tool("strikeThrough", text.t("words.strike"), icons.strike(15))}
    </div>
    <div class="words-group">
      ${tool("align:align-start", text.t("words.alignStart"), icons.alignStart(15))}
      ${tool("align:align-center", text.t("words.alignCenter"), icons.alignCenter(15))}
      ${tool("align:align-end", text.t("words.alignEnd"), icons.alignEnd(15))}
    </div>
    <div class="words-group">
      ${tool("insertUnorderedList", text.t("words.bullets"), icons.bullets(15))}
      ${tool("insertOrderedList", text.t("words.numbers"), icons.numbers(15))}
      ${tool("link:", text.t("words.link"), icons.link(15))}
      ${tool("image:", text.t("words.image"), icons.image(15))}
    </div>
    <div class="words-group">
      ${ink("ink-plain", text.t("words.inkPlain"))}
      ${ink("ink-dim", text.t("words.inkDim"))}
      ${ink("ink-accent", text.t("words.inkAccent"))}
      ${ink("ink-success", text.t("words.inkSuccess"))}
      ${ink("ink-info", text.t("words.inkInfo"))}
    </div>
  </div>`;
}
