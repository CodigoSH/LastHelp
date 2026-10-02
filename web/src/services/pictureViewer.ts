/**
 * A picture on a report, opened large over the page.
 *
 * Every small picture is a link to the full one, so a middle click, a new tab
 * or a page without this script still reaches it. A plain click is taken here
 * instead and opens the viewer: the picture as large as the screen allows,
 * its neighbours in the same row a step away, Escape or a click outside it to
 * go back — and the focus returns to the picture it was opened from.
 *
 * The steps follow the reading direction: in Arabic the next picture is to the
 * left, and so is the key that reaches it.
 */

export interface ViewerText {
  /** The viewer's markup, from views.pictureViewer. */
  markup: () => string;
  /** "Image 2 of 5". */
  count: (n: number, of: number) => string;
}

export function attachPictureViewer(words: ViewerText): void {
  let dialog: HTMLDialogElement | null = null;
  let group: HTMLAnchorElement[] = [];
  let at = 0;
  let opener: HTMLElement | null = null;

  const show = () => {
    const link = group[at];
    if (!dialog || !link) return;
    const picture = dialog.querySelector<HTMLImageElement>(".viewer-picture");
    if (picture) {
      picture.src = link.href;
      picture.alt = link.querySelector("img")?.alt ?? "";
    }
    const count = dialog.querySelector<HTMLElement>(".viewer-count");
    if (count) count.textContent = group.length > 1 ? words.count(at + 1, group.length) : "";
    dialog.querySelectorAll<HTMLButtonElement>("[data-viewer-step]").forEach((b) => {
      b.hidden = group.length < 2;
    });
  };

  const step = (by: number) => {
    if (group.length < 2) return;
    at = (at + by + group.length) % group.length;
    show();
  };

  const build = (): HTMLDialogElement => {
    const holder = document.createElement("div");
    holder.innerHTML = words.markup();
    const built = holder.firstElementChild as HTMLDialogElement;
    document.body.append(built);

    built.querySelector("[data-viewer-close]")?.addEventListener("click", () => built.close());
    built.querySelectorAll<HTMLButtonElement>("[data-viewer-step]").forEach((b) =>
      b.addEventListener("click", () => step(Number(b.dataset.viewerStep))),
    );
    // Outside the picture is the way back, as it is on every viewer people
    // already know.
    built.addEventListener("click", (event) => {
      const target = event.target as HTMLElement;
      if (target === built || target.classList.contains("viewer-figure")) built.close();
    });
    built.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      const rtl = getComputedStyle(built).direction === "rtl";
      const forward = event.key === (rtl ? "ArrowLeft" : "ArrowRight");
      event.preventDefault();
      step(forward ? 1 : -1);
    });
    built.addEventListener("close", () => {
      const picture = built.querySelector<HTMLImageElement>(".viewer-picture");
      if (picture) picture.removeAttribute("src");
      opener?.focus();
    });
    return built;
  };

  document.addEventListener("click", (event) => {
    const link = (event.target as HTMLElement).closest<HTMLAnchorElement>("a.picture-open");
    if (!link) return;
    // A new tab, a new window, a download: what the person asked for.
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    group = Array.from(link.closest(".pictures")?.querySelectorAll<HTMLAnchorElement>("a.picture-open") ?? [link]);
    at = Math.max(0, group.indexOf(link));
    opener = link;
    // Built again each time: it is cheap, and its words are then always in the
    // language the page is in now, not the one it was first opened in.
    dialog?.remove();
    dialog = build();
    show();
    dialog.showModal();
  });
}
