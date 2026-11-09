import { setRule } from "./dynamicStyles";

/**
 * What a day of the chart was, under the pointer.
 *
 * The chart is thirty lines of nothing and one spike until somebody can ask it
 * which day was which. The answer comes from the column that was drawn for the
 * day — every one carries its numbers — rather than from arithmetic repeated
 * here: the drawing already decided which value sits where, and a second copy
 * of that decision is a second place for it to be wrong.
 */

/**
 * What to say about a day, built by whoever knows the reader's language.
 *
 * It is handed the column's own data attributes and returns finished markup:
 * this file positions a box and does not know what a bug or an idea is, and
 * the words and the number formats stay where the catalogue is.
 */
export type Describe = (of: DOMStringMap) => string;

/** The frame a position is waiting for, or null when none is. */
let queued: number | null = null;

export function attachTrendHover(root: ParentNode | null, describe: Describe): void {
  const wrap = root?.querySelector<HTMLElement>(".tc-wrap");
  const svg = wrap?.querySelector("svg");
  if (!wrap || !svg) return;

  const columns = [...svg.querySelectorAll<SVGRectElement>(".tc-col")];
  const cursor = svg.querySelector<SVGLineElement>(".tc-cursor");
  if (columns.length === 0) return;

  const tip = document.createElement("div");
  tip.className = "chart-tip";
  tip.hidden = true;
  // Read out as a live region rather than as a shape: a tooltip that only
  // exists as a position is a tooltip only a mouse can use.
  tip.setAttribute("role", "status");
  wrap.append(tip);

  // The drawing keeps its proportions, so a coordinate in it is a fraction of
  // its viewBox and a coordinate on screen is the same fraction of the box.
  const view = svg.viewBox.baseVal;

  const show = (event: PointerEvent) => {
    const box = svg.getBoundingClientRect();
    if (box.width === 0) return;
    const fraction = (event.clientX - box.left) / box.width;

    let nearest = columns[0]!;
    let best = Infinity;
    for (const column of columns) {
      const distance = Math.abs(Number(column.dataset.x) / view.width - fraction);
      if (distance < best) {
        best = distance;
        nearest = column;
      }
    }

    const x = Number(nearest.dataset.x);
    if (cursor) {
      cursor.setAttribute("x1", String(x));
      cursor.setAttribute("x2", String(x));
      // An SVG element has no hidden property, only the attribute — and the
      // stylesheet's [hidden] rule is what actually takes it off the screen.
      cursor.removeAttribute("hidden");
    }

    tip.innerHTML = describe(nearest.dataset);
    tip.hidden = false;

    // Positioned in physical pixels because that is what was measured. The
    // chart runs oldest to newest from left to right in every language, which
    // is why the axis under it is pinned the same way. To the side of the
    // guide and never past the card's edge, which clips.
    const at = (x / view.width) * box.width;
    const room = Math.max(box.width - tip.offsetWidth - 4, 0);
    const left = at + 12 + tip.offsetWidth > box.width ? at - 12 - tip.offsetWidth : at + 12;

    // Written as a rule rather than onto the element. The page is served with
    // no 'unsafe-inline' in style-src, and an assignment to element.style is an
    // inline style to the browser; a rule in the document's own adopted sheet
    // is not. Once per frame, because a pointer produces more events than a
    // screen has frames.
    const place = { x: Math.min(Math.max(left, 0), room) };
    if (queued === null) {
      queued = requestAnimationFrame(() => {
        queued = null;
        setRule("chart-tip", `.chart-tip { --tip-x: ${Math.round(place.x)}px; }`);
      });
    }
  };

  const hide = () => {
    tip.hidden = true;
    if (cursor) cursor.setAttribute("hidden", "");
  };

  wrap.addEventListener("pointermove", show);
  wrap.addEventListener("pointerleave", hide);
  // A tap shows it and the next tap elsewhere takes it away, which is the most
  // a chart this size can usefully do on a touch screen.
  wrap.addEventListener("pointerdown", show);
}
