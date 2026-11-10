/**
 * The bar every list is searched and narrowed with, drawn the same way on every
 * screen: the search box first and growing, a Filters button that opens a menu
 * of what can narrow the list, an order button, and under them what is chosen,
 * as chips that each come off with one press.
 *
 * Pure markup, like the rest of the views. What opens and closes a menu is
 * `services/filterMenus.ts`, installed once for the whole page; a screen only
 * listens for `fb-pick`, which says which option of which group was chosen.
 */
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";
import type { Text } from "./templates";

export interface FilterOption {
  value: string;
  label: string;
  /** How many the list would hold with this option; absent when it is not known. */
  count?: number;
}

/** One question the list can be narrowed by, with the answer it has now. */
export interface FilterGroup {
  name: string;
  label: string;
  options: FilterOption[];
  /** What is chosen. For a group that takes several, the values joined by commas, as the address has them. */
  value: string;
  /** Several options can be on at once: they are boxes to tick, and are applied together. */
  multi?: boolean;
}

/** The values a group has on: one for a choice, any number for a group that takes several, none for "everything". */
export function chosenValues(g: FilterGroup): string[] {
  return g.value === "" ? [] : g.multi ? g.value.split(",").filter((v) => v !== "") : [g.value];
}

/** From this many options a group has a box to find one in: a list of thirty is not read, it is searched. */
export const FIND_FROM = 10;

const option = (group: FilterGroup, o: FilterOption, text: Text) => {
  const on = group.multi ? (o.value === "" ? chosenValues(group).length === 0 : chosenValues(group).includes(o.value)) : group.value === o.value;
  return `<button type="button" class="fb-opt${group.multi ? " box" : ""}" role="${group.multi ? "menuitemcheckbox" : "menuitemradio"}" aria-checked="${on}" data-fb-was="${on}" data-fb-group="${escapeAttr(group.name)}" data-fb-value="${escapeAttr(o.value)}"${group.multi ? " data-fb-multi" : ""}>
      <span class="fb-opt-mark" aria-hidden="true"></span><span class="fb-opt-name">${escapeHTML(o.label)}</span>${
        o.count === undefined ? "" : `<b class="fb-opt-n">${escapeHTML(text.number(o.count))}</b>`
      }</button>`;
};

/** How many of the groups hold something other than their first option, which is "everything". */
export function activeFilters(groups: FilterGroup[]): number {
  return groups.filter((g) => g.options.length > 0 && (g.multi ? chosenValues(g).length > 0 : g.value !== g.options[0]!.value)).length;
}

/**
 * The button that opens the menu of filters, with how many are on, and the menu.
 * Absent when there is nothing to choose: a board that takes one kind has no
 * filter to offer, and an empty menu is a button that does nothing.
 */
export function filterMenu(groups: FilterGroup[], label: string, text: Text): string {
  const usable = groups.filter((g) => g.options.length > 1);
  if (usable.length === 0) return "";
  const on = activeFilters(usable);
  // A menu with a group that takes several is staged: what is ticked is applied together, with one button.
  const staged = usable.some((g) => g.multi);
  return `<div class="fb-menu">
    <button type="button" class="fb-btn${on > 0 ? " on" : ""}" data-fb-toggle data-fb-filters aria-haspopup="menu" aria-expanded="false">
      ${icons.filter(14)}<span>${escapeHTML(label)}</span>${on > 0 ? `<b class="fb-count">${escapeHTML(text.number(on))}</b>` : ""}${icons.caretDown(12)}
    </button>
    <div class="fb-pop${usable.length >= 3 ? " cols" : ""}" role="menu"${staged ? " data-fb-staged" : ""} hidden>${usable
      .map(
        (g) => `<div class="fb-group" role="group" aria-label="${escapeAttr(g.label)}"><p class="fb-group-name">${escapeHTML(g.label)}</p>${
          g.options.length >= FIND_FROM
            ? `<input type="search" class="fb-find" data-fb-find placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(g.label + ": " + text.t("board.search"))}" autocomplete="off" />`
            : ""
        }<div class="fb-opts">${g.options.map((o) => option(g, o, text)).join("")}</div></div>`,
      )
      .join("")}${staged ? `<div class="fb-apply"><button type="button" class="fb-apply-btn" data-fb-apply>${escapeHTML(text.t("board.apply"))}</button></div>` : ""}</div>
  </div>`;
}

/** The order button: it says the order the list has now, and opens the others. */
export function sortMenu(group: FilterGroup, label: string, text: Text): string {
  const current = group.options.find((o) => o.value === group.value) ?? group.options[0];
  return `<div class="fb-menu">
    <button type="button" class="fb-btn" data-fb-toggle aria-haspopup="menu" aria-expanded="false" aria-label="${escapeAttr(label)}">
      ${icons.sort(14)}<span>${escapeHTML(current?.label ?? label)}</span>${icons.caretDown(12)}
    </button>
    <div class="fb-pop" role="menu" hidden><div class="fb-group" role="group" aria-label="${escapeAttr(label)}"><div class="fb-opts">${group.options.map((o) => option(group, o, text)).join("")}</div></div></div>
  </div>`;
}

/**
 * A choice of a few answers, all in sight: "All / Bugs / Ideas" is one press, not a menu to open and
 * an option to find. For a question with three answers or fewer that is asked all the time; what has
 * more answers is a menu.
 */
export function segmented(group: FilterGroup): string {
  if (group.options.length < 2) return "";
  return `<div class="fb-seg" role="group" aria-label="${escapeAttr(group.label)}">${group.options
    .map(
      (o) => `<button type="button" data-fb-group="${escapeAttr(group.name)}" data-fb-value="${escapeAttr(o.value)}" aria-pressed="${group.value === o.value}">${escapeHTML(o.label)}</button>`,
    )
    .join("")}</div>`;
}

/**
 * A choice that is a control of its own, beside the filters: one question with a button that
 * says the answer it has now ("All" until something is chosen) and a menu of the others. For
 * what has many answers and is chosen often — a repository among thirty — where one more group
 * in the filters menu would make it long and hide the one thing people reach for first.
 */
export function choiceMenu(group: FilterGroup, text: Text, icon: string): string {
  if (group.options.length < 2) return "";
  const chosen = group.value !== "" && group.value !== group.options[0]!.value ? group.options.find((o) => o.value === group.value) : undefined;
  return `<div class="fb-menu">
    <button type="button" class="fb-btn${chosen ? " on" : ""}" data-fb-toggle aria-haspopup="menu" aria-expanded="false" aria-label="${escapeAttr(group.label)}">
      ${icon}<span class="fb-btn-name"${chosen ? ` title="${escapeAttr(chosen.label)}"` : ""}>${escapeHTML(chosen?.label ?? group.label)}</span>${icons.caretDown(12)}
    </button>
    <div class="fb-pop wide" role="menu" hidden><div class="fb-group" role="group" aria-label="${escapeAttr(group.label)}">${
      group.options.length >= FIND_FROM
        ? `<input type="search" class="fb-find" data-fb-find placeholder="${escapeAttr(text.t("board.search"))}" aria-label="${escapeAttr(group.label + ": " + text.t("board.search"))}" autocomplete="off" />`
        : ""
    }<div class="fb-opts">${group.options.map((o) => option(group, o, text)).join("")}</div></div></div>
  </div>`;
}

/** One thing that narrows the list, said, with a way to take it off. */
export interface FilterChip {
  /** What the screen hears when the chip is pressed. */
  name: string;
  label: string;
}

/** What is chosen, as chips, and one press to clear them all. Empty when nothing narrows the list. */
export function filterChips(chips: FilterChip[], clear: string, extraAttr = ""): string {
  if (chips.length === 0) return "";
  return `<div class="fb-chips" ${extraAttr}>${chips
    .map(
      (c) => `<button type="button" class="fb-chip" data-fb-undo="${escapeAttr(c.name)}" aria-label="${escapeAttr(c.label)}">${escapeHTML(c.label)}${icons.close(12)}</button>`,
    )
    .join("")}<button type="button" class="fb-clear" data-fb-clear>${escapeHTML(clear)}</button></div>`;
}

/**
 * Where the chips go on a screen that narrows its list in the browser, without
 * asking the server: empty until something is chosen, and filled by
 * `syncFilterBar`. `clear` is the word on the button that takes them all off.
 */
export function filterChipsHost(clear: string): string {
  return `<div class="fb-chips" data-fb-chips data-fb-clear-label="${escapeAttr(clear)}" hidden></div>`;
}
