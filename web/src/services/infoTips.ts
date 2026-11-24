/**
 * Opening the "i" tips, once for the whole document.
 *
 * One tip is open at a time. A mouse opens it by resting on the "i" and closes
 * it by leaving; a tap or a click opens it and keeps it open until the next tap
 * anywhere else or Escape; the keyboard opens it on focus. Hover alone would
 * leave a phone with no way in, and a tap that only toggled would leave a
 * mouse clicking for every sentence.
 *
 * Placed after it opens: a sentence that would run past the edge of the window
 * opens the other way instead. Closed tips are not drawn at all, so one near an
 * edge never widens the page while it waits.
 */

function place(tip: HTMLElement): void {
  tip.classList.remove("to-end");
  const text = tip.querySelector<HTMLElement>(".tip-text");
  if (!text) return;
  const r = text.getBoundingClientRect();
  const width = document.documentElement.clientWidth;
  const rtl = getComputedStyle(tip).direction === "rtl";
  if ((!rtl && r.right > width - 8) || (rtl && r.left < 8)) tip.classList.add("to-end");
}

let open: HTMLElement | null = null;
// Opened by a click or a tap: the pointer leaving does not close it.
let pinned: HTMLElement | null = null;

function show(tip: HTMLElement): void {
  if (open && open !== tip) hide(open);
  open = tip;
  tip.classList.add("open");
  tip.setAttribute("aria-expanded", "true");
  place(tip);
}

function hide(tip: HTMLElement): void {
  tip.classList.remove("open", "to-end");
  tip.setAttribute("aria-expanded", "false");
  if (open === tip) open = null;
  if (pinned === tip) pinned = null;
}

/** The tip an event happened on, if any. */
function tipOf(target: EventTarget | null): HTMLElement | null {
  return target instanceof Element ? target.closest<HTMLElement>("[data-tip]") : null;
}

export function installInfoTips(doc: Document = document): void {
  doc.addEventListener("pointerover", (event) => {
    const tip = tipOf(event.target);
    if (tip && event.pointerType === "mouse") show(tip);
  });
  doc.addEventListener("pointerout", (event) => {
    const tip = tipOf(event.target);
    if (!tip || event.pointerType !== "mouse") return;
    if (tip.contains(event.relatedTarget as Node | null) || pinned === tip) return;
    hide(tip);
  });
  doc.addEventListener(
    "click",
    (event) => {
      const tip = tipOf(event.target);
      if (tip) {
        // Inside a form or a label it is still only a tip: never a submit, never a checkbox.
        event.preventDefault();
        event.stopPropagation();
        if (pinned === tip) {
          hide(tip);
        } else {
          show(tip);
          pinned = tip;
        }
        return;
      }
      if (open) hide(open);
    },
    true,
  );
  doc.addEventListener("focusin", (event) => {
    const tip = tipOf(event.target);
    if (tip && tip.matches(":focus-visible")) show(tip);
  });
  doc.addEventListener("focusout", (event) => {
    const tip = tipOf(event.target);
    if (tip && pinned !== tip) hide(tip);
  });
  doc.addEventListener(
    "keydown",
    (event) => {
      if (event.key !== "Escape" || !open) return;
      hide(open);
      // A tip inside a dialog closes before the dialog does.
      event.preventDefault();
      event.stopPropagation();
    },
    true,
  );
}
