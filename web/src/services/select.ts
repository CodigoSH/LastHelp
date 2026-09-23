/**
 * The house dropdown, over every <select> in the product.
 *
 * A port of Lastboard's app-select: a trigger with the field's own look and a
 * chevron that turns, and a list that opens under it with the chosen option in
 * the accent. The browser's own dropdown is a grey system menu that looks like
 * no part of either product, and it was the one control on the review screen
 * that did not read as ours.
 *
 * It is an enhancement, not a replacement, and that is the whole design:
 *
 *   - The <select> stays in the page, hidden, and remains the truth. A form
 *     still submits it, code still reads `.value`, and a choice made here is
 *     written into it and announced with the same `change` event the browser
 *     would fire — so nothing that already listened has to learn this exists.
 *   - It is applied to every select, wherever it is drawn, by watching the
 *     document rather than by each screen remembering to call it. Four screens
 *     had native selects when this was written; a fifth added tomorrow is
 *     covered without anybody knowing.
 *
 * Two things differ from Lastboard, both for this product's policy
 * (`style-src 'self'`, no unsafe-inline): the list is positioned with a rule
 * inserted through CSSOM instead of by writing `el.style`, and it is never
 * portalled to <body> — it stays beside its trigger, so inside a <dialog> it
 * is in the top layer with it.
 */

import { setRule } from "./dynamicStyles";

/** Mirrors `.sel-list.open { max-block-size }` in theme.css. */
const LIST_MAX_HEIGHT = 300;

let open: { wrap: HTMLElement; close: () => void } | null = null;
let counter = 0;

const chevron = `<svg class="sel-icon" viewBox="0 0 24 24" aria-hidden="true"><polyline points="6 9 12 15 18 9"></polyline></svg>`;

function labelOf(select: HTMLSelectElement): string {
  return select.selectedOptions[0]?.textContent?.trim() ?? select.options[0]?.textContent?.trim() ?? "";
}

/**
 * Where the list goes, worked out when it opens.
 *
 * Fixed rather than absolute, so no scrolling ancestor can clip it. A fixed
 * element is placed against the window unless an ancestor has a transform or
 * a filter, and a dialog arriving has both — so, as Lastboard does, a probe is
 * placed at the origin and whatever offset it lands at is taken off.
 */
function place(wrap: HTMLElement, trigger: HTMLElement, list: HTMLElement, id: string): void {
  const anchor = trigger.getBoundingClientRect();
  const probe = document.createElement("span");
  probe.className = "sel-probe";
  wrap.append(probe);
  const origin = probe.getBoundingClientRect();
  probe.remove();

  const gap = 6;
  const height = Math.min(list.scrollHeight, LIST_MAX_HEIGHT);
  const dialog = wrap.closest("dialog")?.getBoundingClientRect();
  const bottom = dialog ? Math.min(dialog.bottom, window.innerHeight) : window.innerHeight;
  const top = dialog ? Math.max(dialog.top, 0) : 0;
  const below = bottom - anchor.bottom;
  const above = anchor.top - top;
  // Above the trigger when it does not fit below and there is more room up
  // there: a list near the foot of the screen otherwise runs off it.
  const flip = below < height + gap && above > below;
  const y = flip ? anchor.top - gap - height : anchor.bottom + gap;

  setRule(
    "select-open",
    `.sel-list[data-sel="${id}"]{top:${y - origin.top}px;left:${anchor.left - origin.left}px;` +
      `width:${anchor.width}px}`,
  );
}

function enhance(select: HTMLSelectElement): void {
  if (select.dataset.enhanced === "1" || select.multiple) return;
  select.dataset.enhanced = "1";
  const id = `s${++counter}`;

  const wrap = document.createElement("div");
  wrap.className = "sel";
  select.before(wrap);
  wrap.append(select);
  select.classList.add("sel-native");
  select.tabIndex = -1;
  select.setAttribute("aria-hidden", "true");

  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "sel-trigger";
  trigger.setAttribute("aria-haspopup", "listbox");
  trigger.setAttribute("aria-expanded", "false");
  trigger.innerHTML = `<span class="sel-value"></span>${chevron}`;
  const value = trigger.querySelector<HTMLElement>(".sel-value")!;
  wrap.append(trigger);

  // The name a person or a screen reader knows this by. A <label for> that
  // pointed at the select now points at the button, which is labelable too —
  // otherwise clicking the label would focus a control nobody can see.
  const aria = select.getAttribute("aria-label");
  if (aria) trigger.setAttribute("aria-label", aria);
  if (select.id) {
    trigger.id = `${select.id}-trigger`;
    for (const label of document.querySelectorAll<HTMLLabelElement>(`label[for="${CSS.escape(select.id)}"]`)) {
      label.htmlFor = trigger.id;
    }
  }

  const sync = () => {
    value.textContent = labelOf(select);
    trigger.disabled = select.disabled;
    // The classes the screen put on the select — `set` on a narrowing that is
    // on — are the trigger's to show now.
    wrap.className = ["sel", ...[...select.classList].filter((c) => c !== "sel-native")].join(" ");
  };
  sync();
  select.addEventListener("change", sync);

  let list: HTMLElement | null = null;
  let active = -1;

  const options = () => [...select.options].filter((o) => !o.hidden);

  const mark = () => {
    if (!list) return;
    const items = [...list.querySelectorAll<HTMLElement>(".sel-option")];
    items.forEach((el, i) => el.classList.toggle("active", i === active));
    const current = items[active];
    if (current) {
      list.setAttribute("aria-activedescendant", current.id);
      current.scrollIntoView({ block: "nearest" });
    }
  };

  const close = (refocus = false) => {
    if (!list) return;
    list.remove();
    list = null;
    setRule("select-open");
    wrap.classList.remove("open");
    trigger.setAttribute("aria-expanded", "false");
    if (open?.wrap === wrap) open = null;
    if (refocus) trigger.focus();
  };

  const choose = (index: number) => {
    const option = options()[index];
    if (!option) return;
    const changed = select.value !== option.value;
    select.value = option.value;
    close(true);
    // The event the browser would have fired, so every listener already
    // written against the select keeps working without knowing about this.
    if (changed) select.dispatchEvent(new Event("change", { bubbles: true }));
  };

  const show = () => {
    if (list || select.disabled) return;
    open?.close();
    list = document.createElement("div");
    list.className = "sel-list";
    list.dataset.sel = id;
    list.setAttribute("role", "listbox");
    list.tabIndex = -1;
    const all = options();
    active = Math.max(0, all.findIndex((o) => o.selected));
    list.innerHTML = all
      .map(
        (o, i) =>
          `<div class="sel-option${o.selected ? " selected" : ""}" role="option" id="${id}-${i}"
               aria-selected="${o.selected}" data-i="${i}"></div>`,
      )
      .join("");
    // Text through textContent, not the template: an option's label is
    // whatever the page put there, and this is the one place it would
    // otherwise be parsed as markup a second time.
    list.querySelectorAll<HTMLElement>(".sel-option").forEach((el, i) => {
      el.textContent = all[i]?.textContent ?? "";
    });
    wrap.append(list);
    place(wrap, trigger, list, id);
    wrap.classList.add("open");
    trigger.setAttribute("aria-expanded", "true");
    open = { wrap, close: () => close() };

    list.addEventListener("pointerdown", (event) => event.preventDefault());
    list.addEventListener("click", (event) => {
      const el = (event.target as HTMLElement).closest<HTMLElement>(".sel-option");
      if (el) choose(Number(el.dataset.i));
    });
    list.addEventListener("keydown", keys);
    list.focus();
    mark();
    // The class that runs the entrance, a frame later so there is a closed
    // state to animate from.
    requestAnimationFrame(() => list?.classList.add("open"));
  };

  // Keys inside the open list. They stop here: the review screen answers
  // single letters and arrows, and a list is where those mean something else.
  const keys = (event: KeyboardEvent) => {
    const count = options().length;
    switch (event.key) {
      case "ArrowDown":
        active = Math.min(count - 1, active + 1);
        mark();
        break;
      case "ArrowUp":
        active = Math.max(0, active - 1);
        mark();
        break;
      case "Home":
        active = 0;
        mark();
        break;
      case "End":
        active = count - 1;
        mark();
        break;
      case "Enter":
      case " ":
        choose(active);
        break;
      case "Escape":
        close(true);
        break;
      case "Tab":
        close();
        return;
      default:
        return;
    }
    event.preventDefault();
    event.stopPropagation();
  };

  trigger.addEventListener("click", () => (list ? close(true) : show()));
  trigger.addEventListener("keydown", (event) => {
    if (["ArrowDown", "ArrowUp", "Enter", " "].includes(event.key)) {
      event.preventDefault();
      event.stopPropagation();
      show();
    }
  });
}

/** Every select under a node, including the node itself. */
export function enhanceSelects(root: ParentNode): void {
  if (root instanceof HTMLSelectElement) enhance(root);
  root.querySelectorAll?.("select").forEach(enhance);
}

/**
 * Once, at start: enhance what is there and whatever is drawn later.
 *
 * A click outside, a scroll anywhere but in the list, and a resize all close
 * the open list — Lastboard's rules, because a list left floating where its
 * trigger used to be is worse than one that closes.
 */
export function watchSelects(): void {
  enhanceSelects(document.body);
  new MutationObserver((changes) => {
    for (const change of changes) {
      for (const node of change.addedNodes) {
        if (node instanceof Element) enhanceSelects(node);
      }
    }
  }).observe(document.body, { childList: true, subtree: true });

  document.addEventListener(
    "pointerdown",
    (event) => {
      if (open && !open.wrap.contains(event.target as Node)) open.close();
    },
    true,
  );
  window.addEventListener(
    "scroll",
    (event) => {
      const list = open?.wrap.querySelector(".sel-list");
      if (open && !(list && list.contains(event.target as Node))) open.close();
    },
    true,
  );
  window.addEventListener("resize", () => open?.close());
}
