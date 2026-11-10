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
  value: string;
}

const option = (group: FilterGroup, o: FilterOption, text: Text) =>
  `<button type="button" class="fb-opt" role="menuitemradio" aria-checked="${group.value === o.value}" data-fb-group="${escapeAttr(group.name)}" data-fb-value="${escapeAttr(o.value)}">
      <span class="fb-opt-mark" aria-hidden="true"></span><span class="fb-opt-name">${escapeHTML(o.label)}</span>${
        o.count === undefined ? "" : `<b class="fb-opt-n">${escapeHTML(text.number(o.count))}</b>`
      }</button>`;

/** How many of the groups hold something other than their first option, which is "everything". */
export function activeFilters(groups: FilterGroup[]): number {
  return groups.filter((g) => g.options.length > 0 && g.value !== g.options[0]!.value).length;
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
  return `<div class="fb-menu">
    <button type="button" class="fb-btn${on > 0 ? " on" : ""}" data-fb-toggle data-fb-filters aria-haspopup="menu" aria-expanded="false">
      ${icons.filter(14)}<span>${escapeHTML(label)}</span>${on > 0 ? `<b class="fb-count">${escapeHTML(text.number(on))}</b>` : ""}${icons.caretDown(12)}
    </button>
    <div class="fb-pop" role="menu" hidden>${usable
      .map(
        (g) => `<div class="fb-group" role="group" aria-label="${escapeAttr(g.label)}"><p class="fb-group-name">${escapeHTML(g.label)}</p>${g.options.map((o) => option(g, o, text)).join("")}</div>`,
      )
      .join("")}</div>
  </div>`;
}

/** The order button: it says the order the list has now, and opens the others. */
export function sortMenu(group: FilterGroup, label: string, text: Text): string {
  const current = group.options.find((o) => o.value === group.value) ?? group.options[0];
  return `<div class="fb-menu">
    <button type="button" class="fb-btn" data-fb-toggle aria-haspopup="menu" aria-expanded="false" aria-label="${escapeAttr(label)}">
      ${icons.sort(14)}<span>${escapeHTML(current?.label ?? label)}</span>${icons.caretDown(12)}
    </button>
    <div class="fb-pop end" role="menu" hidden><div class="fb-group" role="group" aria-label="${escapeAttr(label)}">${group.options.map((o) => option(group, o, text)).join("")}</div></div>
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
