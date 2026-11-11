/**
 * What the menus of `views/filterBar.ts` do: open on a press, close on another
 * press anywhere, on Escape or on choosing, move between their options with the
 * arrow keys and give the focus back to the button that opened them.
 *
 * Installed once on the document, because the screens draw their bars again
 * whenever they redraw and a listener on the bar would be lost with it. A choice
 * leaves as an `fb-pick` event from the option, which bubbles to whoever drew
 * the bar and which says the group and the value.
 */

/** What a staged menu says when it is applied: the value of every group in it, by name. Several values are joined by commas. */
export interface FilterApply {
  values: Record<string, string>;
}

export interface FilterPick {
  group: string;
  value: string;
}

import { escapeHTML } from "../escape";
import { icons } from "../views/icons";

const MENU = ".fb-menu";

/** Close every open menu, or every one but `keep`. */
export function closeMenus(keep?: Element | null): void {
  document.querySelectorAll<HTMLElement>(`${MENU} .fb-pop:not([hidden])`).forEach((pop) => {
    if (keep && pop.closest(MENU) === keep) return;
    pop.hidden = true;
    pop.closest(MENU)?.querySelector("[data-fb-toggle]")?.setAttribute("aria-expanded", "false");
  });
}

/**
 * Open toward the side where there is room. A menu hangs from the start of its button; if that runs
 * past the window's end it hangs from the button's end instead. Where a button sits depends on the
 * screen — at the far end of a bar on one, at the start of a narrow column on another — so which side
 * has room is asked when it opens, not decided when it was written.
 */
function place(pop: HTMLElement): void {
  pop.classList.remove("end");
  const r = pop.getBoundingClientRect();
  const rtl = getComputedStyle(pop).direction === "rtl";
  const past = rtl ? r.left < 8 : r.right > innerWidth - 8;
  if (!past) return;
  pop.classList.add("end");
  const after = pop.getBoundingClientRect();
  // If hanging from the other side is worse, stay where it was.
  const worse = rtl ? after.right > innerWidth - 8 : after.left < 8;
  if (worse) pop.classList.remove("end");
}

/** How many boxes are ticked in each group that takes several, shown on its heading. */
function refreshTicked(pop: Element): void {
  pop.querySelectorAll<HTMLElement>(".fb-group").forEach((group) => {
    const badge = group.querySelector<HTMLElement>("[data-fb-on]");
    if (!badge) return;
    const n = [...group.querySelectorAll<HTMLElement>(".fb-opt")].filter((o) => (o.dataset.fbValue ?? "") !== "" && o.getAttribute("aria-checked") === "true").length;
    badge.textContent = new Intl.NumberFormat(document.documentElement.lang || undefined).format(n);
    badge.hidden = n === 0;
  });
}

const options = (menu: Element) => [...menu.querySelectorAll<HTMLButtonElement>(".fb-opt")];

let installed = false;

/** Wire the menus for the whole page. Safe to call more than once. */
export function installFilterMenus(): void {
  if (installed) return;
  installed = true;

  document.addEventListener("click", (event) => {
    const target = event.target as Element | null;
    if (!target) return;
    // A choice drawn in sight is a choice of the same kind as one in a menu: it says the same thing, with no menu to close.
    const seg = target.closest<HTMLElement>(".fb-seg button");
    if (seg) {
      closeMenus();
      const detail: FilterPick = { group: seg.dataset.fbGroup ?? "", value: seg.dataset.fbValue ?? "" };
      seg.dispatchEvent(new CustomEvent<FilterPick>("fb-pick", { bubbles: true, detail }));
      return;
    }
    const toggle = target.closest<HTMLElement>("[data-fb-toggle]");
    const opt = target.closest<HTMLElement>(".fb-opt");
    if (toggle) {
      const menu = toggle.closest(MENU);
      const pop = menu?.querySelector<HTMLElement>(".fb-pop");
      if (!menu || !pop) return;
      const open = pop.hidden;
      closeMenus(open ? menu : null);
      pop.hidden = !open;
      if (open) place(pop);
      // A menu opens with every option in sight, whatever was typed into a group's box the last time.
      if (open) {
        menu.querySelectorAll<HTMLInputElement>("[data-fb-find]").forEach((f) => (f.value = ""));
        // And with what is applied ticked, not what was ticked and never applied the last time.
        options(menu).forEach((o) => {
          o.hidden = false;
          if (o.dataset.fbWas) o.setAttribute("aria-checked", o.dataset.fbWas);
        });
        refreshTicked(pop);
      }
      toggle.setAttribute("aria-expanded", String(open));
      if (open) {
        const checked = options(menu).find((o) => o.getAttribute("aria-checked") === "true") ?? options(menu)[0];
        checked?.focus({ preventScroll: true });
      }
      return;
    }
    // Clear: every group back to "everything", still staged until it is applied.
    const reset = target.closest<HTMLElement>("[data-fb-reset]");
    if (reset) {
      const pop = reset.closest(".fb-pop");
      pop?.querySelectorAll<HTMLElement>(".fb-opt").forEach((o) => o.setAttribute("aria-checked", String((o.dataset.fbValue ?? "") === "")));
      if (pop) refreshTicked(pop);
      return;
    }
    // The button of a staged menu: everything ticked is applied together, and the menu closes.
    const apply = target.closest<HTMLElement>("[data-fb-apply]");
    if (apply) {
      const pop = apply.closest(".fb-pop");
      const values: Record<string, string> = {};
      pop?.querySelectorAll<HTMLElement>(".fb-group").forEach((group) => {
        const all = [...group.querySelectorAll<HTMLElement>(".fb-opt")];
        const name = all[0]?.dataset.fbGroup;
        if (!name) return;
        const on = all.filter((o) => o.getAttribute("aria-checked") === "true" && (o.dataset.fbValue ?? "") !== "").map((o) => o.dataset.fbValue ?? "");
        // A group that takes several gives every value; one that takes a single answer gives that one, or nothing for "everything".
        values[name] = all[0]?.hasAttribute("data-fb-multi") ? on.join(",") : (on[0] ?? "");
      });
      const button = apply.closest(MENU)?.querySelector<HTMLElement>("[data-fb-toggle]");
      closeMenus();
      button?.focus({ preventScroll: true });
      apply.dispatchEvent(new CustomEvent<FilterApply>("fb-apply", { bubbles: true, detail: { values } }));
      return;
    }
    // In a staged menu an option is ticked or chosen, not sent: the menu stays open until it is applied.
    if (opt && opt.closest(".fb-pop")?.hasAttribute("data-fb-staged")) {
      const group = opt.closest(".fb-group");
      const all = [...(group?.querySelectorAll<HTMLElement>(".fb-opt") ?? [])];
      const set = (o: HTMLElement, on: boolean) => o.setAttribute("aria-checked", String(on));
      if (opt.hasAttribute("data-fb-multi")) {
        const everything = (opt.dataset.fbValue ?? "") === "";
        if (everything) all.forEach((o) => set(o, o === opt));
        else {
          set(opt, opt.getAttribute("aria-checked") !== "true");
          const real = all.filter((o) => (o.dataset.fbValue ?? "") !== "");
          // "Everything" is on exactly when nothing else is.
          all.filter((o) => (o.dataset.fbValue ?? "") === "").forEach((o) => set(o, !real.some((r) => r.getAttribute("aria-checked") === "true")));
        }
      } else all.forEach((o) => set(o, o === opt));
      refreshTicked(opt.closest(".fb-pop") as Element);
      return;
    }
    if (opt) {
      const menu = opt.closest(MENU);
      const button = menu?.querySelector<HTMLElement>("[data-fb-toggle]");
      closeMenus();
      button?.focus({ preventScroll: true });
      const detail: FilterPick = { group: opt.dataset.fbGroup ?? "", value: opt.dataset.fbValue ?? "" };
      opt.dispatchEvent(new CustomEvent<FilterPick>("fb-pick", { bubbles: true, detail }));
      return;
    }
    // On a phone the veil behind a sheet is the menu element itself, and pressing it closes the sheet.
    if (target.matches(MENU)) {
      closeMenus();
      return;
    }
    // A press anywhere else closes what is open.
    if (!target.closest(MENU)) closeMenus();
  });

  // A group's own box narrows its options to the ones that hold what was typed. The first option
  // is "everything" and stays, so a search never leaves a group with no way back.
  document.addEventListener("input", (event) => {
    const box = (event.target as Element | null)?.closest<HTMLInputElement>("[data-fb-find]");
    if (!box) return;
    const words = box.value.trim().toLocaleLowerCase();
    const group = box.closest(".fb-group");
    group?.querySelectorAll<HTMLElement>(".fb-opt").forEach((o, i) => {
      o.hidden = i > 0 && words !== "" && !(o.querySelector(".fb-opt-name")?.textContent ?? "").toLocaleLowerCase().includes(words);
    });
  });

  document.addEventListener("keydown", (event) => {
    const target = event.target as Element | null;
    const open = document.querySelector<HTMLElement>(`${MENU} .fb-pop:not([hidden])`);
    if (!open) return;
    const menu = open.closest(MENU);
    if (!menu) return;
    if (event.key === "Escape") {
      // Closing the menu must not also close the dialog it may be in.
      event.preventDefault();
      event.stopPropagation();
      closeMenus();
      menu.querySelector<HTMLElement>("[data-fb-toggle]")?.focus({ preventScroll: true });
      return;
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    if (!target || !menu.contains(target)) return;
    event.preventDefault();
    // Only what is in sight: an option a search has hidden is not somewhere the arrows stop.
    const all = options(menu).filter((o) => !o.hidden);
    // From a group's box, down goes to the first option of that group.
    if (target.matches("[data-fb-find]")) {
      if (event.key === "ArrowDown") target.closest(".fb-group")?.querySelector<HTMLButtonElement>(".fb-opt:not([hidden])")?.focus({ preventScroll: true });
      return;
    }
    const at = all.indexOf(target as HTMLButtonElement);
    const next = event.key === "ArrowDown" ? (at + 1) % all.length : (at - 1 + all.length) % all.length;
    all[next]?.focus({ preventScroll: true });
  }, true);
}

/**
 * Bring a bar drawn once up to date with what a screen holds: the option each
 * group has now, how many filters are on, the order the order button names, and
 * the chips. For the screens that narrow their list in the browser, where the
 * page is not drawn again with every choice.
 *
 * `values` has a value for each group by name; the first option of a group is
 * "everything", so a group that holds it narrows nothing. `query` is the search
 * as typed, which is a chip of its own once it is a search.
 */
export function syncFilterBar(bar: Element, values: Record<string, string>, query = ""): void {
  const active: Array<{ name: string; label: string }> = [];
  // The choices in sight follow what the screen holds, and are not chips: they are already on show.
  bar.querySelectorAll<HTMLElement>(".fb-seg button").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.fbValue === (values[b.dataset.fbGroup ?? ""] ?? ""))));
  const groups = new Set<string>();
  bar.querySelectorAll<HTMLElement>(".fb-opt").forEach((o) => groups.add(o.dataset.fbGroup ?? ""));
  for (const group of groups) {
    const all = [...bar.querySelectorAll<HTMLElement>(`.fb-opt[data-fb-group="${group}"]`)];
    const value = values[group] ?? all[0]?.dataset.fbValue ?? "";
    all.forEach((o) => o.setAttribute("aria-checked", String(o.dataset.fbValue === value)));
    const chosen = all.find((o) => o.dataset.fbValue === value);
    if (group === "sort") {
      const label = chosen?.querySelector(".fb-opt-name")?.textContent ?? "";
      const button = chosen?.closest(".fb-menu")?.querySelector<HTMLElement>("[data-fb-toggle] span");
      if (button && label) button.textContent = label;
      continue;
    }
    if (chosen && all[0] && chosen !== all[0]) active.push({ name: group, label: chosen.querySelector(".fb-opt-name")?.textContent ?? "" });
  }

  const button = bar.querySelector<HTMLElement>("[data-fb-filters]");
  if (button) {
    button.classList.toggle("on", active.length > 0);
    let count = button.querySelector<HTMLElement>(".fb-count");
    if (active.length > 0) {
      if (!count) {
        count = document.createElement("b");
        count.className = "fb-count";
        button.querySelector("svg:last-child")?.before(count);
      }
      count.textContent = new Intl.NumberFormat(document.documentElement.lang || undefined).format(active.length);
    } else count?.remove();
  }

  // The chips sit right after the bar they belong to.
  const next = bar.nextElementSibling;
  const host = next?.matches("[data-fb-chips]") ? (next as HTMLElement) : bar.querySelector<HTMLElement>("[data-fb-chips]");
  if (host) {
    const chips = [...active, ...(query.trim().length >= 2 ? [{ name: "q", label: `«${query.trim()}»` }] : [])];
    host.hidden = chips.length === 0;
    host.replaceChildren();
    for (const c of chips) {
      const b = document.createElement("button");
      b.type = "button";
      b.className = "fb-chip";
      b.dataset.fbUndo = c.name;
      b.setAttribute("aria-label", c.label);
      b.innerHTML = `${escapeHTML(c.label)}${icons.close(12)}`;
      host.append(b);
    }
    if (chips.length > 0) {
      const clear = document.createElement("button");
      clear.type = "button";
      clear.className = "fb-clear";
      clear.dataset.fbClear = "";
      clear.textContent = host.dataset.fbClearLabel ?? "";
      host.append(clear);
    }
  }
}
