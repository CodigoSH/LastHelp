/**
 * Writing a block of words where it is seen.
 *
 * The dashboard in this house edits its note widget on the board itself rather
 * than in a dialog, and this is that: while the page is being arranged, a block
 * of words is typed into directly, with a floating bar of tools that follows
 * whichever block has the cursor.
 *
 * Two things had to be redesigned rather than copied, and both come from the
 * policy this page is served under — `style-src 'self'`, with no unsafe-inline:
 *
 *   - Alignment and colour cannot be written as inline styles. The browser
 *     drops a style attribute without a word, so the buttons would appear to
 *     work and change nothing. They are written as classes out of a closed set
 *     instead, painted from the stylesheet, which says the same thing and is
 *     the same set the server will accept.
 *   - There is one bar, moved to the block being written, rather than one per
 *     block. Not for the policy — because a page of eight modules would
 *     otherwise build eight of everything on every redraw.
 *
 * The markup that leaves here is not trusted by anything downstream: the server
 * cleans what it is sent and cleans again what it reads back. This is the
 * convenience half; `internal/core/landing/clean.go` is the half that matters.
 */

/** The alignments, which are the four a paragraph can take. */
const ALIGNMENTS = ["align-start", "align-center", "align-end", "align-justify"] as const;

/**
 * The palette, and it is a palette rather than a colour wheel on purpose: every
 * one of these is a token the theme already defines, so the words stay legible
 * when somebody switches to the dark theme. A free colour picker cannot promise
 * that — it would let an operator write dark grey on a page that later turns
 * dark grey.
 */
export const INKS = [
  "ink-plain",
  "ink-dim",
  "ink-accent",
  "ink-danger",
  "ink-success",
  "ink-warning",
  "ink-info",
] as const;

type Command =
  | { kind: "exec"; command: string; value?: string }
  | { kind: "align"; value: (typeof ALIGNMENTS)[number] }
  | { kind: "ink"; value: (typeof INKS)[number] }
  | { kind: "link" };

/** The block element the cursor is inside, which is what an alignment applies to. */
function blockAround(node: Node | null, within: HTMLElement): HTMLElement | null {
  let at: Node | null = node;
  while (at && at !== within) {
    if (at instanceof HTMLElement && /^(P|H1|H2|H3|LI|BLOCKQUOTE)$/.test(at.tagName)) return at;
    at = at.parentNode;
  }
  return null;
}

/**
 * Wraps what is selected in a span carrying one of the palette's classes.
 *
 * surroundContents alone is not enough: it throws when the selection starts in
 * one element and ends in another, which is most selections a person makes.
 * Taking the contents out and putting them back inside the span works in both
 * cases.
 */
function paint(ink: string): void {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return;
  const range = selection.getRangeAt(0);
  const span = document.createElement("span");
  span.className = ink;
  span.append(range.extractContents());
  range.insertNode(span);
  selection.removeAllRanges();
  const after = document.createRange();
  after.selectNodeContents(span);
  selection.addRange(after);
}

function align(where: HTMLElement, value: string): void {
  const selection = window.getSelection();
  const block = blockAround(selection?.anchorNode ?? null, where) ?? where;
  for (const name of ALIGNMENTS) block.classList.remove(name);
  // The start alignment is what a paragraph does anyway, so it is the absence
  // of a class rather than a class of its own: fewer classes stored, and the
  // reading direction keeps deciding.
  if (value !== "align-start") block.classList.add(value);
}

/**
 * Turns what is selected into a link.
 *
 * createLink is the one execCommand here whose output needs checking: it will
 * happily write whatever it is given, javascript: included. The server refuses
 * that anyway, but a link that looks saved and vanishes on reload is worse than
 * one that was never made, so it is refused here too.
 */
function linkUp(ask: (message: string) => string | null): void {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed) return;
  const raw = (ask("https://") ?? "").trim();
  if (raw === "") return;
  let target: URL;
  try {
    target = new URL(raw, window.location.origin);
  } catch {
    return;
  }
  if (!["http:", "https:", "mailto:"].includes(target.protocol)) return;
  document.execCommand("createLink", false, target.href);
}

function run(what: Command, where: HTMLElement, ask: (message: string) => string | null): void {
  where.focus();
  switch (what.kind) {
    case "exec":
      document.execCommand(what.command, false, what.value);
      break;
    case "align":
      align(where, what.value);
      break;
    case "ink":
      paint(what.value);
      break;
    case "link":
      linkUp(ask);
      break;
  }
}

/**
 * Wires every editable block on the page to one toolbar.
 *
 * Called after each redraw. The listeners go on the blocks, which are replaced
 * on every redraw, so there is nothing to accumulate; the bar is found in the
 * document and is not rebuilt.
 */
export function wireWords(options: {
  root: ParentNode;
  bar: HTMLElement | null;
  onChange: (id: string, html: string) => void;
  ask?: (message: string) => string | null;
}): void {
  const { root, bar, onChange } = options;
  const ask = options.ask ?? ((message: string) => window.prompt(message));
  if (!bar) return;

  const blocks = [...root.querySelectorAll<HTMLElement>("[data-words-body][contenteditable]")];
  if (blocks.length === 0) {
    bar.hidden = true;
    return;
  }

  let writing: HTMLElement | null = null;

  const place = (block: HTMLElement): void => {
    writing = block;
    bar.hidden = false;
    // The bar belongs to the block being written, so it goes inside it rather
    // than being positioned against the window: a module that scrolls takes its
    // bar with it, and nothing has to be recalculated as the page moves.
    block.parentElement?.append(bar);
  };

  for (const block of blocks) {
    block.addEventListener("focusin", () => place(block));
    block.addEventListener("input", () => {
      const id = block.dataset.wordsBody ?? "";
      if (id !== "") onChange(id, block.innerHTML);
    });
    // Enter inside a block of prose should start a paragraph, not a div, which
    // is what a browser does left alone.
    block.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        document.execCommand("insertParagraph");
      }
    });
    // Pasting carries the styling of wherever it came from. Taking the text
    // means what arrives is what the person can see they are pasting.
    block.addEventListener("paste", (event) => {
      event.preventDefault();
      const text = event.clipboardData?.getData("text/plain") ?? "";
      document.execCommand("insertText", false, text);
    });
  }

  if (bar.dataset.wired === "1") return;
  bar.dataset.wired = "1";

  // On pointerdown, not on click: clicking a button would take the cursor out
  // of the block first, and a formatting command with nothing selected does
  // nothing at all.
  bar.addEventListener("pointerdown", (event) => event.preventDefault());

  bar.addEventListener("click", (event) => {
    const button = (event.target as HTMLElement).closest<HTMLButtonElement>("[data-do]");
    if (!button || !writing) return;
    const [kind, value] = (button.dataset.do ?? "").split(":");
    const block = writing;
    if (kind === "align") run({ kind: "align", value: value as never }, block, ask);
    else if (kind === "ink") run({ kind: "ink", value: value as never }, block, ask);
    else if (kind === "link") run({ kind: "link" }, block, ask);
    else if (kind !== undefined) run({ kind: "exec", command: kind, value }, block, ask);
    const id = block.dataset.wordsBody ?? "";
    if (id !== "") onChange(id, block.innerHTML);
  });
}
