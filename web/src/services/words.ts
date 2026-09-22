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
  | { kind: "link" }
  | { kind: "image" };

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

/**
 * Puts a picture where the cursor is.
 *
 * The file is scaled and re-encoded before it is sent — see landingImage.ts —
 * so the caller hands over whatever was picked and gets back a path. It goes
 * in at 480 wide: a starting size that looks deliberate in a module, and the
 * handle on its corner is there to change it.
 */
async function placeImage(
  where: HTMLElement,
  pick: () => Promise<File | null>,
  upload: (file: File) => Promise<string>,
): Promise<void> {
  const file = await pick();
  if (!file) return;
  const at = await upload(file);
  where.focus();
  const img = document.createElement("img");
  img.src = at;
  img.width = 480;
  img.alt = "";

  const selection = window.getSelection();
  const range =
    selection && selection.rangeCount > 0 && where.contains(selection.anchorNode)
      ? selection.getRangeAt(0)
      : null;
  if (range) {
    range.deleteContents();
    range.insertNode(img);
    range.setStartAfter(img);
    range.collapse(true);
    selection?.removeAllRanges();
    selection?.addRange(range);
  } else {
    where.append(img);
  }
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
    case "image":
      // Handled by the caller: it needs to wait for a file and for an upload,
      // and this one is deliberately synchronous so that every command that
      // can run inside a selection does.
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
  /** Picks a file and gives back the path it ended up at. */
  image?: {
    pick: () => Promise<File | null>;
    upload: (file: File) => Promise<string>;
    failed: (err: unknown) => void;
  };
}): void {
  const { root, bar, onChange } = options;
  const ask = options.ask ?? ((message: string) => window.prompt(message));
  if (!bar) return;

  const blocks = [...root.querySelectorAll<HTMLElement>("[data-words-body][contenteditable=true]")];
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

  /**
   * Dragging a picture wider or narrower.
   *
   * The size is written as the width ATTRIBUTE, not as a style: this page is
   * served with style-src 'self', so a style written by a drag is dropped by
   * the browser and the handle would move while nothing happened. HTML has
   * carried a width attribute on images since before CSS existed, it is not a
   * style, and the server's policy accepts it as a number of pixels.
   *
   * The handle is drawn by the stylesheet on the selected picture rather than
   * being an element, so there is nothing to insert into what somebody wrote
   * and nothing to remember to take out again before saving.
   */
  const resizing = (block: HTMLElement): void => {
    let dragging: { img: HTMLImageElement; fromX: number; fromWidth: number } | null = null;

    block.addEventListener("pointerdown", (event) => {
      const img = (event.target as HTMLElement).closest("img");
      if (!(img instanceof HTMLImageElement)) return;
      const box = img.getBoundingClientRect();
      // The bottom-right corner, the same corner the grid resizes a module
      // from. Physical rather than logical on purpose: it is where the pointer
      // is, and the pointer does not mirror.
      const onCorner = event.clientX > box.right - 18 && event.clientY > box.bottom - 18;
      img.classList.toggle("chosen", true);
      for (const other of block.querySelectorAll("img")) {
        if (other !== img) other.classList.remove("chosen");
      }
      if (!onCorner) return;
      event.preventDefault();
      dragging = { img, fromX: event.clientX, fromWidth: box.width };
      block.setPointerCapture(event.pointerId);
    });

    block.addEventListener("pointermove", (event) => {
      if (!dragging) return;
      const widest = block.clientWidth - 8;
      const wanted = Math.round(dragging.fromWidth + (event.clientX - dragging.fromX));
      dragging.img.width = Math.max(40, Math.min(widest, wanted));
    });

    const done = (event: PointerEvent): void => {
      if (!dragging) return;
      block.releasePointerCapture?.(event.pointerId);
      dragging = null;
      const id = block.dataset.wordsBody ?? "";
      if (id !== "") onChange(id, block.innerHTML);
    };
    block.addEventListener("pointerup", done);
    block.addEventListener("pointercancel", done);
  };

  for (const block of blocks) {
    resizing(block);
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
    if (kind === "image") {
      const put = options.image;
      if (!put) return;
      void placeImage(block, put.pick, put.upload)
        .then(() => {
          const id = block.dataset.wordsBody ?? "";
          if (id !== "") onChange(id, block.innerHTML);
        })
        .catch(put.failed);
      return;
    }
    if (kind === "align") run({ kind: "align", value: value as never }, block, ask);
    else if (kind === "ink") run({ kind: "ink", value: value as never }, block, ask);
    else if (kind === "link") run({ kind: "link" }, block, ask);
    else if (kind !== undefined) run({ kind: "exec", command: kind, value }, block, ask);
    const id = block.dataset.wordsBody ?? "";
    if (id !== "") onChange(id, block.innerHTML);
  });
}
