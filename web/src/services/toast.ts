/**
 * Toasts, the way Lastboard does them.
 *
 * The behaviour is ported rather than reinvented — bottom centre, three at a
 * time with the rest queued, a draining bar that pauses under the pointer, a
 * close button, an optional action, and errors announced assertively while
 * everything else is announced politely. That is the house's, and somebody who
 * uses two CodigoSH products should not have to learn a second set of manners.
 *
 * The implementation is this project's: a module and a container on the page,
 * not a custom element with a shadow root. LastHelp has no web components, and
 * adding one for this would mean the only component in the product carries the
 * only shadow root, the only adopted stylesheet and the only reason to explain
 * both.
 *
 * What is deliberately NOT a toast: the validation error next to a field. It
 * belongs beside the box it is about, where somebody is already looking, and a
 * message that flies away three seconds later is the wrong shape for one that
 * has to be read and acted on.
 */
import { i18n } from "./i18n";

type Kind = "success" | "error" | "info" | "warning";

interface Options {
  message: string;
  kind?: Kind;
  action?: { label: string; onClick: () => void };
  duration?: number;
}

const MAX_VISIBLE = 3;
const DEFAULT_MS = 3000;
/** With an action, longer: it has to be reachable before it leaves. */
const ACTION_MS = 6000;
/** Must match the CSS leave transition. */
import { setRule } from "./dynamicStyles";

const EXIT_MS = 260;

/** How many have been drawn, which is where each one's id comes from. */
let drawn = 0;

/** How long this toast's bar takes to empty. */
function drain(el: HTMLElement, duration: number): void {
  setRule(
    `toast-${el.dataset.toast}`,
    `.toast[data-toast="${el.dataset.toast}"] .toast-bar { animation-duration: ${Math.round(duration)}ms; }`,
  );
}

interface Timer {
  remaining: number;
  startedAt: number;
  handle: number;
}

const queue: Options[] = [];
const timers = new Map<HTMLElement, Timer>();
let container: HTMLElement | null = null;

function surface(): HTMLElement {
  if (container?.isConnected) return container;
  container = document.createElement("div");
  container.className = "toasts";
  document.body.appendChild(container);
  return container;
}

const keyOf = (o: Options) => `${o.kind ?? "success"}|${o.message}`;

const visible = () => surface().querySelectorAll(".toast:not(.toast-leaving)").length;

/** Show a toast. `toast("Saved")`, or `toast({ message, kind, action })`. */
export function toast(input: string | Options, kind: Kind = "success"): void {
  const opts: Options = typeof input === "string" ? { message: input, kind } : { kind: "success", ...input };
  if (!opts.message) return;

  // The same message twice is one message that happened twice. Restart its
  // countdown instead of stacking a copy — except when it carries an action,
  // which is a distinct intent every time.
  if (!opts.action) {
    const existing = [...surface().querySelectorAll<HTMLElement>(".toast:not(.toast-leaving)")].find(
      (t) => t.dataset.key === keyOf(opts),
    );
    if (existing) {
      restart(existing, opts.duration ?? DEFAULT_MS);
      return;
    }
  }

  if (visible() >= MAX_VISIBLE) {
    queue.push(opts);
    return;
  }
  draw(opts);
}

/** The failure of a request, in the reader's language. */
export function toastError(message: string): void {
  toast({ message, kind: "error" });
}

function draw(opts: Options): void {
  const kind = opts.kind ?? "success";
  const duration = opts.duration ?? (opts.action ? ACTION_MS : DEFAULT_MS);

  const el = document.createElement("div");
  el.className = `toast toast-${kind}`;
  el.dataset.key = keyOf(opts);
  // An error interrupts; everything else waits its turn.
  el.setAttribute("role", kind === "error" ? "alert" : "status");
  // How long its bar takes to drain, as a rule of its own rather than a style
  // attribute: three of these exist at a time and each one is told apart by an
  // id, which is what the rule is keyed on.
  el.dataset.toast = String(++drawn);
  drain(el, duration);

  const message = document.createElement("span");
  message.className = "toast-message";
  message.textContent = opts.message;
  el.appendChild(message);

  if (opts.action) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "toast-action";
    button.textContent = opts.action.label;
    button.addEventListener("click", () => {
      try {
        opts.action!.onClick();
      } catch (err) {
        console.error("a toast action failed", err);
      }
      dismiss(el);
    });
    el.appendChild(button);
  }

  const close = document.createElement("button");
  close.type = "button";
  close.className = "toast-close";
  close.setAttribute("aria-label", i18n.t("action.close"));
  close.innerHTML = `<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor"
    stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M18 6L6 18M6 6l12 12"/></svg>`;
  close.addEventListener("click", () => dismiss(el));
  el.appendChild(close);

  const bar = document.createElement("div");
  bar.className = "toast-bar";
  el.appendChild(bar);

  // The bar pauses in CSS; the timer has to pause with it, or the toast leaves
  // while its own countdown says it has not.
  el.addEventListener("mouseenter", () => pause(el));
  el.addEventListener("mouseleave", () => resume(el));

  surface().appendChild(el);
  requestAnimationFrame(() => el.classList.add("toast-visible"));
  start(el, duration);
}

function start(el: HTMLElement, remaining: number): void {
  timers.set(el, { remaining, startedAt: Date.now(), handle: window.setTimeout(() => dismiss(el), remaining) });
}

function pause(el: HTMLElement): void {
  const t = timers.get(el);
  if (!t) return;
  clearTimeout(t.handle);
  t.remaining = Math.max(0, t.remaining - (Date.now() - t.startedAt));
}

function resume(el: HTMLElement): void {
  const t = timers.get(el);
  if (!t) return;
  t.startedAt = Date.now();
  t.handle = window.setTimeout(() => dismiss(el), t.remaining);
}

function restart(el: HTMLElement, duration: number): void {
  drain(el, duration);
  const bar = el.querySelector<HTMLElement>(".toast-bar");
  if (bar) {
    // A residual animation wins by source order and would never replay. The
    // class is removed after a forced reflow, which is the only way to make an
    // animation start again — and it is a class rather than an inline style,
    // because the policy this page is served under allows neither.
    bar.classList.add("no-drain");
    void bar.offsetWidth;
    bar.classList.remove("no-drain");
  }
  const t = timers.get(el);
  if (t) clearTimeout(t.handle);
  start(el, duration);
}

function dismiss(el: HTMLElement): void {
  if (el.classList.contains("toast-leaving")) return;
  const t = timers.get(el);
  if (t) clearTimeout(t.handle);
  timers.delete(el);

  el.classList.add("toast-leaving");
  el.classList.remove("toast-visible");
  window.setTimeout(() => {
    el.remove();
    // And its rule goes with it: a sheet that only ever grows is a leak with a
    // slow fuse.
    setRule(`toast-${el.dataset.toast}`);
    if (queue.length && visible() < MAX_VISIBLE) {
      const next = queue.shift();
      if (next) draw(next);
    }
  }, EXIT_MS);
}
