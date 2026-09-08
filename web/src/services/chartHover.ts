import { setRule } from "./dynamicStyles";

/**
 * What a day of the chart was, under the pointer.
 *
 * The chart is thirty bars of nothing and one spike until somebody can ask it
 * which day was which. The answer comes from the point that was drawn — every
 * one carries its day and its count — rather than from arithmetic repeated
 * here: the drawing already decided which value sits where, and a second copy
 * of that decision is a second place for it to be wrong.
 */

/**
 * What to say about a point, built by whoever knows the reader's language.
 *
 * It is handed the point's own data attributes and returns finished markup:
 * this file positions a box and does not know what a bug or an idea is, and
 * the words and the number formats stay where the catalogue is.
 */
export type Describe = (of: DOMStringMap) => string;

/** The frame a position is waiting for, or null when none is. */
let queued: number | null = null;

export function attachChartHover(root: ParentNode | null, describe: Describe): void {
  const wrap = root?.querySelector<HTMLElement>(".chart-wrap");
  const svg = wrap?.querySelector("svg");
  if (!wrap || !svg) return;

  const dots = [...svg.querySelectorAll<SVGCircleElement>(".chart-dot")];
  const cursor = svg.querySelector<SVGLineElement>(".chart-cursor");
  if (dots.length === 0) return;

  const tip = document.createElement("div");
  tip.className = "chart-tip";
  tip.hidden = true;
  // Read out as a live region rather than as a shape: a tooltip that only
  // exists as a position is a tooltip only a mouse can use.
  tip.setAttribute("role", "status");
  wrap.append(tip);

  // The drawing is stretched to the box it is in (preserveAspectRatio="none"),
  // so a coordinate in the chart is a fraction of its viewBox and a coordinate
  // on screen is the same fraction of the rendered box.
  const view = svg.viewBox.baseVal;

  const show = (event: PointerEvent) => {
    const box = svg.getBoundingClientRect();
    if (box.width === 0) return;
    const fraction = (event.clientX - box.left) / box.width;

    let nearest = dots[0]!;
    let best = Infinity;
    for (const dot of dots) {
      const at = dot.cx.baseVal.value / view.width;
      const distance = Math.abs(at - fraction);
      if (distance < best) {
        best = distance;
        nearest = dot;
      }
    }

    for (const dot of dots) dot.classList.toggle("on", dot === nearest);

    const x = nearest.cx.baseVal.value;
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
    // is why the axis under it is pinned the same way.
    //
    // And kept inside the card, which clips: the busiest day is the tallest
    // point and sits against the top edge, where a box drawn above it is a box
    // nobody can read. Above when there is room, below when there is not, and
    // never further out than half its own width from either side.
    const left = (x / view.width) * box.width;
    const top = (nearest.cy.baseVal.value / view.height) * box.height;
    const half = tip.offsetWidth / 2;
    const above = top - tip.offsetHeight - 10 >= 0;
    tip.classList.toggle("below", !above);

    // Written as a rule rather than onto the element. The page is served with
    // no 'unsafe-inline' in style-src, and an assignment to element.style is an
    // inline style to the browser; a rule in the document's own adopted sheet
    // is not. Once per frame, because a pointer produces more events than a
    // screen has frames.
    const at = {
      x: Math.min(Math.max(left, half + 4), Math.max(box.width - half - 4, half + 4)),
      y: top,
    };
    if (queued === null) {
      queued = requestAnimationFrame(() => {
        queued = null;
        setRule("chart-tip", `.chart-tip { --tip-x: ${Math.round(at.x)}px; --tip-y: ${Math.round(at.y)}px; }`);
      });
    }
  };

  const hide = () => {
    tip.hidden = true;
    if (cursor) cursor.setAttribute("hidden", "");
    for (const dot of dots) dot.classList.remove("on");
  };

  wrap.addEventListener("pointermove", show);
  wrap.addEventListener("pointerleave", hide);
  // A tap shows it and the next tap elsewhere takes it away, which is the most
  // a chart this size can usefully do on a touch screen.
  wrap.addEventListener("pointerdown", show);
}
