/**
 * The front page editor.
 *
 * The page being edited is the page: the same markup visitors get, drawn with
 * editing on, with the controls laid over it and a panel beside it. Every change
 * is saved to the page itself as it is made, so what the editor shows is what visitors get, and leaving it is
 * only leaving it.
 *
 * Kept out of main.ts because it is a small program of its own — a model, a
 * history, a save queue and a keyboard — and main.ts is the router.
 */
import { api } from "./api";
import { askConfirm } from "./confirm";
import { escapeHTML } from "./escape";
import { NoWebP, putImage } from "./services/landingImage";
import { wireWords } from "./services/words";
import * as landingViews from "./views/landing";
import { FOOTER_ID } from "./views/landing";
import type {
  LandingBand,
  LandingBoard,
  LandingColumns,
  LandingContent,
  LandingKind,
  LandingLink,
  LandingModule,
  LandingPage,
} from "./views/landing";
import * as editorViews from "./views/landingEditor";
import type { Text } from "./views/templates";

export interface EditorHost {
  root: HTMLElement;
  text: Text;
  instance: () => { name?: string; logo?: string };
  report: (err: unknown) => void;
  toast: (message: string) => void;
  toastError: (message: string) => void;
  /** Redraws the bar above the page, which carries the way out of the editor. */
  renderNav: () => void;
}

/** The server's limits, repeated so the editor can say no before it asks. */
const MAX_BANDS = 12;
const MAX_MODULES = 24;

type Found =
  | { band: LandingBand; bi: number; module?: undefined }
  | { module: LandingModule; band: LandingBand; bi: number; ci: number; mi: number };

interface Composed extends LandingPage {
  content?: LandingContent;
  boards?: LandingBoard[];
  share_ready?: boolean;
}

/** What the panel shows when the page as a whole is chosen: no band or module has this id. */
const PAGE = " page";

/** A fresh id nobody on the page has, for a band or a module. */
function freshID(kind: string): string {
  return `${kind}-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

/** The part of a page the server fills in, as one string, to tell whether it moved. */
function dataSignature(page: LandingPage): string {
  const out: string[] = [];
  for (const b of page.bands)
    for (const cell of b.cells)
      for (const m of cell)
        if (["boards", "reports", "roadmap", "statistics"].includes(m.kind))
          out.push([m.id, m.kind, m.scope ?? "", m.show ?? "", m.rows ?? 0].join(":"));
  return out.join("|");
}

export class LandingEditor {
  private page: LandingPage = { bands: [], settings: {} };
  private content: LandingContent = {};
  private boards: LandingBoard[] = [];
  private shareReady = false;

  private past: string[] = [];
  private future: string[] = [];
  /** What the last change was, so a burst of typing is one step to undo. */
  private lastChange = { key: "", at: 0 };

  private selected = "";
  private tab: "content" | "appearance" = "content";
  private phone = false;

  private saving: "idle" | "saving" | "failed" = "idle";
  private saveTimer: ReturnType<typeof setTimeout> | undefined;
  private inFlight: Promise<void> | null = null;
  /** The data signature the server last filled the content for. */
  private filledFor = "";

  /** Where the next module or section goes, set by the control that asked. */
  private target: { band: string; column: number; after?: number } | { bandAt: number } | null = null;

  private onKey = (e: KeyboardEvent) => this.key(e);
  private stopped = false;

  constructor(private host: EditorHost) {}

  // --- lifecycle ---------------------------------------------------------------

  async start(): Promise<void> {
    await this.load();
    this.drawAll();
    document.addEventListener("keydown", this.onKey);
  }

  /** Leaves the editor, saving what is outstanding first. */
  async stop(): Promise<void> {
    this.stopped = true;
    document.removeEventListener("keydown", this.onKey);
    await this.flush();
  }

  private async load(): Promise<void> {
    const composed = await api.get<Composed>("/api/landing");
    this.page = { bands: composed.bands ?? [], settings: composed.settings ?? {} };
    this.content = composed.content ?? {};
    this.boards = composed.boards ?? [];
    this.shareReady = composed.share_ready === true;
    this.filledFor = dataSignature(this.page);
  }

  // --- the model ---------------------------------------------------------------

  private find(id: string): Found | null {
    for (let bi = 0; bi < this.page.bands.length; bi++) {
      const band = this.page.bands[bi]!;
      if (band.id === id) return { band, bi };
      for (let ci = 0; ci < band.cells.length; ci++) {
        const mi = band.cells[ci]!.findIndex((m) => m.id === id);
        if (mi >= 0) return { module: band.cells[ci]![mi]!, band, bi, ci, mi };
      }
    }
    return null;
  }

  private moduleCount(): number {
    return this.page.bands.reduce((n, b) => n + b.cells.reduce((k, c) => k + c.length, 0), 0);
  }

  /**
   * Changes the page, as one step of history.
   *
   * `key` joins a burst of changes to one field into one step: typing a title
   * letter by letter undoes as the title, not as a letter.
   */
  private change(mutate: () => void, opts: { key?: string; redraw?: "all" | "page" | "none" } = {}): void {
    const before = JSON.stringify(this.page);
    mutate();
    if (JSON.stringify(this.page) === before) return;
    const now = Date.now();
    const joins = opts.key !== undefined && opts.key === this.lastChange.key && now - this.lastChange.at < 1500;
    if (!joins) {
      this.past.push(before);
      if (this.past.length > 100) this.past.shift();
    }
    this.lastChange = { key: opts.key ?? "", at: now };
    this.future = [];
    this.scheduleSave();
    const redraw = opts.redraw ?? "all";
    if (redraw === "all") this.drawAll();
    else if (redraw === "page") {
      this.drawPage();
      this.drawBar();
    } else this.drawBar();
  }

  private undo(): void {
    const previous = this.past.pop();
    if (previous === undefined) return;
    this.future.push(JSON.stringify(this.page));
    this.page = JSON.parse(previous) as LandingPage;
    this.afterTimeTravel();
  }

  private redo(): void {
    const next = this.future.pop();
    if (next === undefined) return;
    this.past.push(JSON.stringify(this.page));
    this.page = JSON.parse(next) as LandingPage;
    this.afterTimeTravel();
  }

  private afterTimeTravel(): void {
    this.lastChange = { key: "", at: 0 };
    if (this.selected && !this.find(this.selected)) this.selected = "";
    this.scheduleSave();
    this.drawAll();
  }

  // --- saving ------------------------------------------------------------------

  /** Why the page cannot be saved as it is, or "" when it can. */
  private problem(): string {
    const t = this.host.text.t;
    const linkOK = (l: LandingLink) =>
      l.label.trim() !== "" && (l.to === "url" ? /^https?:\/\/[^\s/]+/i.test(l.url ?? "") : (l.scope ?? "").includes("/"));
    for (const b of this.page.bands)
      for (const cell of b.cells)
        for (const m of cell) {
          if (m.kind === "image" && m.image && !(m.alt ?? "").trim()) return t("landing.needAlt");
          if (m.kind === "links" && !(m.links ?? []).every(linkOK)) return t("landing.needLink");
        }
    if (!(this.page.settings.footer ?? []).every(linkOK)) return t("landing.needLink");
    return "";
  }

  private scheduleSave(): void {
    clearTimeout(this.saveTimer);
    this.saveTimer = setTimeout(() => void this.save(), 450);
  }

  /** Saves now whatever is waiting, and waits for it. */
  private async flush(): Promise<void> {
    if (this.saveTimer !== undefined) {
      clearTimeout(this.saveTimer);
      this.saveTimer = undefined;
      await this.save();
    }
    await this.inFlight;
  }

  /**
   * One save at a time, always of the page as it is when it leaves: two in
   * flight could land in the wrong order and leave the older one stored.
   */
  private async save(): Promise<void> {
    this.saveTimer = undefined;
    if (this.inFlight) {
      await this.inFlight;
      if (this.saveTimer !== undefined) return;
    }
    if (this.problem()) {
      this.drawBar();
      return;
    }
    const sent = clone(this.page);
    this.saving = "saving";
    this.drawBar();
    this.inFlight = (async () => {
      try {
        await api.put("/api/landing", sent);
        this.saving = "idle";
        // A module that is new, or now reads from somewhere else, has nothing
        // to show until the server says what goes in it.
        const signature = dataSignature(sent);
        if (signature !== this.filledFor && !this.stopped) {
          const composed = await api.get<Composed>("/api/landing");
          this.content = composed.content ?? {};
          this.filledFor = signature;
          this.drawPageWhenIdle();
        }
      } catch (err) {
        this.saving = "failed";
        this.host.report(err);
      } finally {
        this.inFlight = null;
        if (!this.stopped) this.drawBar();
      }
    })();
    await this.inFlight;
  }

  // --- drawing -----------------------------------------------------------------

  private get text(): Text {
    return this.host.text;
  }

  private drawAll(): void {
    const { root } = this.host;
    root.innerHTML = `<div class="lpe${this.phone ? " phone" : ""}">
      <div class="lpe-bar-slot"></div>
      <div class="lpe-stage"><div class="lpe-canvas"></div></div>
      <aside class="lpe-panel" aria-label="${escapeHTML(this.text.t("landing.compose"))}"></aside>
      ${editorViews.wordsBar(this.text)}
      ${editorViews.modulePicker(this.text)}
      ${editorViews.sectionPicker(this.text)}
    </div>`;
    this.wireOnce();
    this.drawBar();
    this.drawPage();
    this.drawPanel();
    this.host.renderNav();
  }

  private el<T extends HTMLElement>(selector: string): T | null {
    return this.host.root.querySelector<T>(selector);
  }

  private drawBar(): void {
    const slot = this.el(".lpe-bar-slot");
    if (!slot) return;
    slot.innerHTML = editorViews.editBar(
      {
        canUndo: this.past.length > 0,
        canRedo: this.future.length > 0,
        phone: this.phone,
        saving: this.saving,
        problem: this.problem() || undefined,
        pageOpen: this.selected === PAGE,
      },
      this.text,
    );
  }

  private drawPage(): void {
    const canvas = this.el(".lpe-canvas");
    if (!canvas) return;
    // The writing bar lives inside whichever block has the cursor; it is taken
    // out first, or replacing the page would take it with it.
    const bar = this.el<HTMLElement>(".words-bar");
    if (bar) {
      bar.hidden = true;
      this.el(".lpe")?.append(bar);
    }
    canvas.innerHTML = landingViews.landingPage(
      this.page,
      { instance: this.host.instance(), content: this.content, editing: true, selected: this.selected },
      this.text,
    );
    this.wirePage();
  }

  /** Redraws the page, unless somebody is typing in it — then when they stop. */
  private drawPageWhenIdle(): void {
    const active = document.activeElement as HTMLElement | null;
    if (active?.isContentEditable && this.el(".lpe-canvas")?.contains(active)) {
      active.addEventListener("blur", () => this.drawPage(), { once: true });
      return;
    }
    this.drawPage();
  }

  private drawPanel(): void {
    const aside = this.el(".lpe-panel");
    if (!aside) return;
    const found = this.selected ? this.find(this.selected) : null;
    const chosen =
      this.selected === PAGE
        ? { page: this.page.settings }
        : this.selected === FOOTER_ID
          ? { footer: this.page.settings }
          : !found
          ? null
          : found.module
            ? { module: found.module }
            : { band: found.band };
    aside.innerHTML = editorViews.panel(
      chosen,
      {
        boards: this.boards,
        instance: this.host.instance(),
        tab: this.tab,
        shareReady: this.shareReady,
        defaultName: (b) => (b.id === "where" ? this.text.t("landing.bandWhere") : b.id === "how" ? this.text.t("landing.bandHow") : ""),
      },
      this.text,
    );
  }

  /** Marks what is chosen without redrawing the page, so a caret stays where it is. */
  private choose(id: string): void {
    if (id === this.selected) return;
    this.selected = id;
    this.tab = "content";
    this.host.root.querySelectorAll(".lpe-canvas .is-selected").forEach((n) => n.classList.remove("is-selected"));
    if (id) this.chosenElement()?.classList.add("is-selected");
    this.drawPanel();
    this.drawBar();
  }

  private focusSelected(): void {
    if (!this.selected) return;
    this.chosenElement()?.focus({ preventScroll: false });
  }

  /** The element on the page that is chosen: a module, a band or the footer. */
  private chosenElement(): HTMLElement | null {
    const id = this.selected;
    if (!id || id === PAGE) return null;
    if (id === FOOTER_ID) return this.host.root.querySelector<HTMLElement>(".lpe-canvas [data-footer]");
    return this.host.root.querySelector<HTMLElement>(
      `.lpe-canvas [data-module="${CSS.escape(id)}"], .lpe-canvas [data-band="${CSS.escape(id)}"]`,
    );
  }

  // --- wiring ------------------------------------------------------------------

  /** The listeners on what survives a redraw of the page: the editor's own frame. */
  private wireOnce(): void {
    const lpe = this.el<HTMLElement>(".lpe");
    if (!lpe) return;

    lpe.addEventListener("click", (e) => void this.click(e));
    const aside = this.el<HTMLElement>(".lpe-panel");
    aside?.addEventListener("input", (e) => this.panelInput(e));
    aside?.addEventListener("change", (e) => this.panelChange(e));

    for (const dialog of lpe.querySelectorAll<HTMLDialogElement>("dialog")) {
      dialog.addEventListener("click", (e) => {
        // A click on the backdrop is a click on the dialog itself.
        if (e.target === dialog) dialog.close();
      });
    }

    // Dragging a module by its grip, from one place in a column to another.
    const canvas = this.el<HTMLElement>(".lpe-canvas");
    if (!canvas) return;
    let dragging = "";
    const marker = document.createElement("div");
    marker.className = "lpe-drop";
    canvas.addEventListener("dragstart", (e) => {
      const grip = (e.target as HTMLElement).closest<HTMLElement>("[data-grip]");
      if (!grip || !e.dataTransfer) return;
      dragging = grip.dataset.grip ?? "";
      const card = grip.closest<HTMLElement>(".lp-mod");
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", dragging);
      if (card) {
        e.dataTransfer.setDragImage(card, 24, 24);
        requestAnimationFrame(() => card.classList.add("dragging"));
      }
    });
    const place = (e: DragEvent): { band: string; column: number; index: number } | null => {
      const col = (e.target as HTMLElement).closest<HTMLElement>(".lp-col");
      const band = col?.closest<HTMLElement>("[data-band]");
      if (!col || !band) return null;
      const cards = [...col.querySelectorAll<HTMLElement>(":scope > .lp-mod")].filter((c) => c.dataset.module !== dragging);
      let index = cards.length;
      for (let i = 0; i < cards.length; i++) {
        const box = cards[i]!.getBoundingClientRect();
        if (e.clientY < box.top + box.height / 2) {
          index = i;
          break;
        }
      }
      const before = cards[index] ?? col.querySelector(":scope > .lpe-add");
      if (before) col.insertBefore(marker, before);
      else col.append(marker);
      return { band: band.dataset.band ?? "", column: Number(col.dataset.column ?? 0), index };
    };
    canvas.addEventListener("dragover", (e) => {
      if (!dragging) return;
      if (place(e)) e.preventDefault();
    });
    canvas.addEventListener("drop", (e) => {
      if (!dragging) return;
      e.preventDefault();
      const at = place(e);
      marker.remove();
      const id = dragging;
      dragging = "";
      if (at) this.moveModule(id, at.band, at.column, at.index);
    });
    canvas.addEventListener("dragend", () => {
      dragging = "";
      marker.remove();
      canvas.querySelectorAll(".dragging").forEach((n) => n.classList.remove("dragging"));
    });
  }

  /** The listeners on the page itself, which is replaced on every redraw. */
  private wirePage(): void {
    const canvas = this.el<HTMLElement>(".lpe-canvas");
    if (!canvas) return;

    // The words of text modules, typed where they are read.
    wireWords({
      root: canvas,
      bar: this.el<HTMLElement>(".words-bar"),
      image: {
        pick: pickImageFile,
        upload: putImage,
        failed: (err) =>
          err instanceof NoWebP ? this.host.toastError(this.text.t("error.no_webp")) : this.host.report(err),
      },
      onChange: (id, html) => {
        const found = this.find(id);
        if (!found || !(found.module)) return;
        this.change(() => (found.module.body = html), { key: `words:${id}`, redraw: "none" });
      },
    });
    // An empty block says where to write; the first click takes the prompt away.
    canvas.querySelectorAll<HTMLElement>("[data-words-body]").forEach((box) =>
      box.addEventListener("focusin", () => {
        if (box.querySelector(".words-prompt") && box.textContent?.trim() === box.querySelector(".words-prompt")?.textContent?.trim()) {
          box.innerHTML = "<p><br></p>";
        }
      }),
    );

    // A heading's title and line, typed in place. Plain text: what arrives
    // is exactly what can be seen.
    canvas.querySelectorAll<HTMLElement>("[data-field][data-for]").forEach((field) => {
      field.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          field.blur();
        }
      });
      field.addEventListener("paste", (e) => {
        e.preventDefault();
        document.execCommand("insertText", false, e.clipboardData?.getData("text/plain").replace(/\s+/g, " ") ?? "");
      });
      field.addEventListener("input", () => {
        const found = this.find(field.dataset.for ?? "");
        if (!found || !(found.module)) return;
        const name = field.dataset.field as "title" | "subtitle";
        const typed = (field.textContent ?? "").replace(/\s+/g, " ").trim();
        const fallback = name === "title" ? this.host.instance().name || this.text.t("app.name") : this.text.t("app.tagline");
        this.change(
          () => {
            if (typed === "" || typed === fallback) delete found.module[name];
            else found.module[name] = typed;
          },
          { key: `${name}:${found.module.id}`, redraw: "none" },
        );
        const mirror = this.el<HTMLInputElement>(`.lpe-panel [data-bind="${name}"]`);
        if (mirror) mirror.value = found.module[name] ?? "";
      });
    });

    // The footer's line, typed in place like a heading. Plain text.
    canvas.querySelectorAll<HTMLElement>("[data-footer-text]").forEach((field) => {
      field.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          field.blur();
        }
      });
      field.addEventListener("paste", (e) => {
        e.preventDefault();
        document.execCommand("insertText", false, e.clipboardData?.getData("text/plain").replace(/\s+/g, " ") ?? "");
      });
      field.addEventListener("input", () => {
        const typed = (field.textContent ?? "").replace(/\s+/g, " ").trim();
        // An emptied line leaves a <br> behind in some browsers, which would
        // keep the placeholder from showing.
        if (typed === "") field.innerHTML = "";
        const settings = this.page.settings;
        this.change(
          () => {
            if (typed === "") delete settings.footer_text;
            else settings.footer_text = typed;
          },
          { key: "page.footer_text", redraw: "none" },
        );
        const mirror = this.el<HTMLInputElement>('.lpe-panel [data-bind="page.footer_text"]');
        if (mirror) mirror.value = settings.footer_text ?? "";
      });
    });

    // A search on a page being edited would open results over the editor.
    canvas.querySelectorAll<HTMLInputElement>(".lp-search input").forEach((i) => (i.readOnly = true));
    // Links on the page being edited choose the module rather than leaving.
    canvas.querySelectorAll<HTMLAnchorElement>("a[href]").forEach((a) => a.setAttribute("tabindex", "-1"));
  }

  private async click(e: MouseEvent): Promise<void> {
    const target = e.target as HTMLElement;

    const bar = target.closest<HTMLElement>("[data-bar]");
    if (bar) {
      switch (bar.dataset.bar) {
        case "undo":
          return this.undo();
        case "redo":
          return this.redo();
        case "desktop":
        case "phone":
          this.phone = bar.dataset.bar === "phone";
          this.el(".lpe")?.classList.toggle("phone", this.phone);
          return this.drawBar();
        case "deselect":
          return this.choose("");
        case "settings":
          return this.choose(this.selected === PAGE ? "" : PAGE);
      }
      return;
    }

    // Anything that leaves the page does not, while it is being edited.
    if (target.closest(".lpe-canvas a[href]")) e.preventDefault();

    const pickModule = target.closest<HTMLElement>("[data-pick]");
    if (pickModule) {
      pickModule.closest("dialog")?.close();
      return this.addModule(pickModule.dataset.pick as LandingKind);
    }
    const pickSection = target.closest<HTMLElement>("[data-pick-section]");
    if (pickSection) {
      pickSection.closest("dialog")?.close();
      return this.addSection(pickSection.dataset.pickSection ?? "layout:1");
    }
    if (target.closest("[data-close]")) {
      target.closest("dialog")?.close();
      return;
    }

    const addModule = target.closest<HTMLElement>("[data-add-module]");
    if (addModule) {
      this.target = { band: addModule.dataset.addModule ?? "", column: Number(addModule.dataset.column ?? 0) };
      return this.openPicker("lpe-module-picker");
    }
    const addBand = target.closest<HTMLElement>("[data-add-band]");
    if (addBand) {
      this.target = { bandAt: Number(addBand.dataset.addBand ?? this.page.bands.length) };
      return this.openPicker("lpe-section-picker");
    }

    // The controls on a band or a module.
    const act = target.closest<HTMLElement>(".lpe-canvas [data-act]");
    if (act?.dataset.act === "select-footer") return this.choose(FOOTER_ID);
    if (act) {
      const owner = act.closest<HTMLElement>("[data-module]") ?? act.closest<HTMLElement>("[data-band]");
      const id = owner?.dataset.module ?? owner?.dataset.band ?? "";
      return this.act(act.dataset.act ?? "", id);
    }

    // The panel's own buttons.
    const set = target.closest<HTMLElement>(".lpe-panel [data-set]");
    if (set) return this.setField(set.dataset.set ?? "", set.dataset.value ?? "", "all");
    const tab = target.closest<HTMLElement>(".lpe-panel [data-tab]");
    if (tab) {
      this.tab = tab.dataset.tab === "appearance" ? "appearance" : "content";
      return this.drawPanel();
    }
    const panelAct = target.closest<HTMLElement>(".lpe-panel [data-act], .lpe-panel [data-link-act]");
    if (panelAct) return this.panelAct(panelAct);

    // Choosing: a module, or a band anywhere outside its modules.
    const inCanvas = target.closest(".lpe-canvas");
    if (!inCanvas) return;
    const mod = target.closest<HTMLElement>("[data-module]");
    if (mod) return this.choose(mod.dataset.module ?? "");
    const band = target.closest<HTMLElement>("[data-band]");
    if (band) return this.choose(band.dataset.band ?? "");
    if (target.closest("[data-footer]")) return this.choose(FOOTER_ID);
    this.choose("");
  }

  private openPicker(id: string): void {
    const picker = this.el<HTMLDialogElement>(`#${id}`);
    picker?.showModal();
  }

  /** The top bar's plus: a module where the chosen thing is, or a section after it. */
  addFromBar(what: "module" | "section"): void {
    const found = this.selected ? this.find(this.selected) : null;
    if (what === "section") {
      this.target = { bandAt: found ? found.bi + 1 : this.page.bands.length };
      return this.openPicker("lpe-section-picker");
    }
    if (found && found.module) this.target = { band: found.band.id, column: found.ci, after: found.mi };
    else if (found) this.target = { band: found.band.id, column: 0 };
    else this.target = null;
    this.openPicker("lpe-module-picker");
  }

  private act(act: string, id: string): void {
    const found = this.find(id);
    if (!found) return;
    const t = this.text.t;
    if (act === "select") return this.choose(id);
    if (found.module) {
      const cell = found.band.cells[found.ci]!;
      if (act === "copy") {
        if (this.moduleCount() >= MAX_MODULES) return this.host.toastError(t("landing.tooManyModules"));
        const copy = { ...clone(found.module), id: freshID(found.module.kind) };
        this.change(() => cell.splice(found.mi + 1, 0, copy));
        return this.choose(copy.id);
      }
      if (act === "remove") {
        this.selected = "";
        return this.change(() => cell.splice(found.mi, 1));
      }
      return;
    }
    const bands = this.page.bands;
    const bi = found.bi;
    switch (act) {
      case "up":
      case "down": {
        const to = act === "up" ? bi - 1 : bi + 1;
        if (to < 0 || to >= bands.length) return;
        this.change(() => bands.splice(to, 0, ...bands.splice(bi, 1)));
        return this.focusSelected();
      }
      case "copy": {
        if (bands.length >= MAX_BANDS) return this.host.toastError(t("landing.tooManySections"));
        const modules = found.band.cells.reduce((n, c) => n + c.length, 0);
        if (this.moduleCount() + modules > MAX_MODULES) return this.host.toastError(t("landing.tooManyModules"));
        const copy = clone(found.band);
        copy.id = freshID("section");
        copy.cells = copy.cells.map((c) => c.map((m) => ({ ...m, id: freshID(m.kind) })));
        return this.change(() => bands.splice(bi + 1, 0, copy));
      }
      case "remove":
        if (bands.length === 1) return this.host.toastError(t("landing.lastSection"));
        this.selected = "";
        return this.change(() => bands.splice(bi, 1));
    }
  }

  private moveModule(id: string, bandID: string, column: number, index: number): void {
    const found = this.find(id);
    const to = this.find(bandID);
    if (!found || !(found.module) || !to || to.module) return;
    this.change(() => {
      const [moving] = found.band.cells[found.ci]!.splice(found.mi, 1);
      if (!moving) return;
      const cell = to.band.cells[column] ?? to.band.cells[0]!;
      cell.splice(Math.min(index, cell.length), 0, moving);
    });
    this.selected = id;
    this.drawPanel();
    this.focusSelected();
  }

  // --- adding ------------------------------------------------------------------

  private newModule(kind: LandingKind): LandingModule {
    const m: LandingModule = { id: freshID(kind), kind };
    switch (kind) {
      case "heading":
        m.with_search = true;
        m.style = "plain";
        break;
      case "reports":
        m.rows = 5;
        break;
      case "links":
        m.links = [];
        break;
    }
    return m;
  }

  private addModule(kind: LandingKind): void {
    if (this.moduleCount() >= MAX_MODULES) return this.host.toastError(this.text.t("landing.tooManyModules"));
    const added = this.newModule(kind);
    const target = this.target;
    this.target = null;
    const into = target && "band" in target ? this.find(target.band) : null;
    if (into && !(into.module)) {
      const cell = into.band.cells[target && "column" in target ? target.column : 0] ?? into.band.cells[0]!;
      const at = target && "after" in target && target.after !== undefined ? target.after + 1 : cell.length;
      this.change(() => cell.splice(at, 0, added));
    } else {
      // Nothing chosen: a section of its own at the foot of the page.
      if (this.page.bands.length >= MAX_BANDS) return this.host.toastError(this.text.t("landing.tooManySections"));
      this.change(() => this.page.bands.push({ id: freshID("section"), columns: "1", cells: [[added]] }));
    }
    this.selected = added.id;
    this.tab = "content";
    this.drawAll();
    this.focusSelected();
  }

  private readyMade(which: editorViews.ReadyMade): LandingBand {
    const t = this.text.t;
    const id = freshID("section");
    const mod = (kind: LandingKind, extra: Partial<LandingModule> = {}): LandingModule => ({
      ...this.newModule(kind),
      ...extra,
    });
    switch (which) {
      case "heading":
        return { id, columns: "1", background: "soft", spacing: "airy", cells: [[mod("heading")]] };
      case "where":
        return { id, columns: "1", name: t("landing.bandWhere"), cells: [[mod("boards", { hide_name: true, style: "plain" })]] };
      case "lists":
        return { id, columns: "1-1", cells: [[mod("reports")], [mod("reports", { show: "recently_done" })]] };
      case "points": {
        const point = () => mod("text", { body: `<h3>${escapeHTML(t("landing.sampleTitle"))}</h3><p>${escapeHTML(t("landing.writeHere"))}</p>` });
        return { id, columns: "1-1-1", name: t("landing.bandHow"), cells: [[point()], [point()], [point()]] };
      }
      case "roadmap":
        return { id, columns: "1", cells: [[mod("roadmap")]] };
      case "text":
        return { id, columns: "1", cells: [[mod("text")]] };
      case "links":
        return { id, columns: "1", cells: [[mod("links")]] };
    }
  }

  private addSection(pick: string): void {
    const t = this.text.t;
    if (this.page.bands.length >= MAX_BANDS) return this.host.toastError(t("landing.tooManySections"));
    const [kind, value] = pick.split(":") as [string, string];
    const band: LandingBand =
      kind === "ready"
        ? this.readyMade(value as editorViews.ReadyMade)
        : {
            id: freshID("section"),
            columns: value as LandingColumns,
            cells: Array.from({ length: landingViews.columnCount(value as LandingColumns) }, () => []),
          };
    const modules = band.cells.reduce((n, c) => n + c.length, 0);
    if (this.moduleCount() + modules > MAX_MODULES) return this.host.toastError(t("landing.tooManyModules"));
    const at = this.target && "bandAt" in this.target ? this.target.bandAt : this.page.bands.length;
    this.target = null;
    this.change(() => this.page.bands.splice(at, 0, band));
    this.selected = band.id;
    this.drawAll();
    this.focusSelected();
  }

  // --- the panel -----------------------------------------------------------------

  /**
   * Writes one field of the chosen thing. An empty value is no value: the page
   * keeps only what differs from the default, which is also what the server
   * checks field by field.
   */
  private setField(bind: string, raw: string | boolean, redraw: "all" | "page"): void {
    if (bind.startsWith("page.")) {
      const key = bind.slice(5) as "title" | "description" | "footer_text" | "footer_hide_brand" | "footer_hide_credit";
      const settings = this.page.settings as Record<string, unknown>;
      return this.change(
        () => {
          if (raw === false || (typeof raw === "string" && raw.trim() === "")) delete settings[key];
          else settings[key] = raw;
        },
        // The footer is on the page; the title and description are not.
        { key: `${bind}`, redraw: key.startsWith("footer") ? "page" : "none" },
      );
    }
    const found = this.selected ? this.find(this.selected) : null;
    if (!found) return;
    const onBand = bind.startsWith("band.");
    const key = onBand ? bind.slice(5) : bind;
    if (onBand && found.module) return;
    const owner = (found.module && !onBand ? found.module : found.band) as unknown as Record<string, unknown>;

    let value: unknown = raw;
    if (key === "rows") {
      const n = Math.round(Number(raw));
      value = Number.isFinite(n) ? Math.min(20, Math.max(1, n)) : 5;
    }
    if (key === "columns") return this.relayout(found.band, raw as LandingColumns);
    this.change(
      () => {
        if (value === "" || value === false) delete owner[key];
        else owner[key] = value;
      },
      { key: `${bind}:${this.selected}`, redraw },
    );
  }

  /** A new layout keeps every module: columns that go hand theirs to the last one kept. */
  private relayout(band: LandingBand, columns: LandingColumns): void {
    if (band.columns === columns) return;
    const n = landingViews.columnCount(columns);
    this.change(() => {
      const cells = band.cells.slice(0, n);
      const spill = band.cells.slice(n).flat();
      while (cells.length < n) cells.push([]);
      cells[n - 1] = [...cells[n - 1]!, ...spill];
      band.cells = cells;
      band.columns = columns;
    });
  }

  private panelInput(e: Event): void {
    const field = e.target as HTMLInputElement;
    if (field.dataset.bind && field.type !== "checkbox" && field.tagName === "INPUT") {
      if (field.type === "number") return;
      this.setField(field.dataset.bind, field.value, "page");
      this.drawBar();
      return;
    }
    if (field.dataset.link !== undefined && field.tagName === "INPUT" && field.type !== "checkbox") {
      this.setLink(Number(field.dataset.link), field.dataset.linkField ?? "", field.value, false);
    }
  }

  private panelChange(e: Event): void {
    const field = e.target as HTMLInputElement | HTMLSelectElement;
    if (field.dataset.bind) {
      if (field instanceof HTMLInputElement && field.type === "checkbox") {
        const on = field.dataset.invert ? !field.checked : field.checked;
        return this.setField(field.dataset.bind, on, "page");
      }
      if (field instanceof HTMLInputElement && field.type === "number") {
        this.setField(field.dataset.bind, field.value, "page");
        field.value = String((this.find(this.selected) as { module?: LandingModule })?.module?.rows ?? 5);
        return;
      }
      if (field instanceof HTMLSelectElement) return this.setField(field.dataset.bind, field.value, "page");
      return;
    }
    if (field.dataset.link !== undefined) {
      const value = field instanceof HTMLInputElement && field.type === "checkbox" ? field.checked : field.value;
      // Changing where a link goes changes which box asks for the address.
      this.setLink(Number(field.dataset.link), field.dataset.linkField ?? "", value, field.dataset.linkField === "to");
    }
  }

  private chosenModule(): LandingModule | null {
    const found = this.selected ? this.find(this.selected) : null;
    return found && found.module ? found.module : null;
  }

  /** The list of links the panel is editing: a links module's, or the page's foot. */
  private chosenLinks(create = false): LandingLink[] | null {
    if (this.selected === FOOTER_ID) {
      if (create && !this.page.settings.footer) this.page.settings.footer = [];
      return this.page.settings.footer ?? [];
    }
    const m = this.chosenModule();
    if (!m || m.kind !== "links") return null;
    if (create && !m.links) m.links = [];
    return m.links ?? [];
  }

  private setLink(i: number, field: string, value: string | boolean, redrawPanel: boolean): void {
    const link = this.chosenLinks()?.[i];
    if (!link) return;
    this.change(
      () => {
        if (field === "label") link.label = String(value);
        else if (field === "url") link.url = String(value).trim();
        else if (field === "scope") link.scope = String(value);
        else if (field === "button") {
          if (value) link.button = true;
          else delete link.button;
        } else if (field === "to") {
          link.to = value as LandingLink["to"];
          if (link.to === "url") delete link.scope;
          else delete link.url;
        }
      },
      { key: `link:${this.selected}:${i}:${field}`, redraw: redrawPanel ? "all" : "page" },
    );
  }

  private async panelAct(button: HTMLElement): Promise<void> {
    switch (button.dataset.act) {
      case "pick-share":
        return this.upload((path) => (this.page.settings.share_image = path));
      case "drop-share":
        return this.change(() => delete this.page.settings.share_image);
      case "restore":
        return this.restore();
    }
    if (button.dataset.linkAct) {
      const i = Number(button.dataset.link);
      const links = this.chosenLinks() ?? [];
      if (button.dataset.linkAct === "remove") this.change(() => links.splice(i, 1));
      else if (button.dataset.linkAct === "up" && i > 0) this.change(() => links.splice(i - 1, 0, ...links.splice(i, 1)));
      return;
    }
    switch (button.dataset.act) {
      case "add-link": {
        const first = this.boards[0];
        this.change(() =>
          this.chosenLinks(true)?.push(
            first ? { label: "", to: "form", scope: `${first.org}/${first.slug}` } : { label: "", to: "url", url: "" },
          ),
        );
        // Straight to the label, which is what a new link is missing.
        const labels = this.host.root.querySelectorAll<HTMLInputElement>('.lpe-panel [data-link-field="label"]');
        labels[labels.length - 1]?.focus();
        return;
      }
    }
    const m = this.chosenModule();
    if (!m) return;
    switch (button.dataset.act) {
      case "pick-image":
        await this.upload((path) => (m.image = path));
        // A picture is not saved without its description, so that is next.
        this.host.root.querySelector<HTMLInputElement>('.lpe-panel [data-bind="alt"]')?.focus();
        return;
      case "drop-image":
        return this.change(() => {
          delete m.image;
          delete m.alt;
          delete m.caption;
        });
    }
  }

  /** Asks for a picture, stores it, and hands its address to `put` as one change. */
  private async upload(put: (path: string) => void): Promise<void> {
    const file = await pickImageFile();
    if (!file) return;
    try {
      const path = await putImage(file);
      this.change(() => put(path));
    } catch (err) {
      if (err instanceof NoWebP) this.host.toastError(this.text.t("error.no_webp"));
      else this.host.report(err);
    }
  }

  /**
   * Back to the page every installation starts with — as a change like any
   * other, saved at once, and Ctrl+Z takes it back.
   */
  private async restore(): Promise<void> {
    if (!(await askConfirm(this.text.t("landing.restore"), this.text.t("landing.confirmRestore")))) return;
    try {
      await this.flush();
      const fresh = await api.post<LandingPage>("/api/landing/restore");
      this.change(() => {
        this.page.bands = fresh.bands;
        this.page.settings = fresh.settings ?? {};
      });
      this.selected = PAGE;
      this.drawPanel();
    } catch (err) {
      this.host.report(err);
    }
  }

  // --- the keyboard ----------------------------------------------------------------

  /**
   * The keys every editor answers to. Inside a field or a block of words they
   * belong to the field — Ctrl+Z there undoes the typing — except Escape,
   * which leaves the field and keeps the module chosen.
   */
  private key(e: KeyboardEvent): void {
    if (this.stopped || document.querySelector("dialog[open]")) return;
    const target = e.target as HTMLElement;
    const typing = target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName);
    const mod = e.ctrlKey || e.metaKey;
    if (typing) {
      if (e.key === "Escape") {
        target.blur();
        this.focusSelected();
      }
      return;
    }
    if (mod && e.key.toLowerCase() === "z") {
      e.preventDefault();
      return e.shiftKey ? this.redo() : this.undo();
    }
    if (mod && e.key.toLowerCase() === "y") {
      e.preventDefault();
      return this.redo();
    }
    if (!this.selected) return;
    const found = this.find(this.selected);
    if (!found) return;
    // Deleting and moving answer on the page only. With the focus on a button
    // in the panel, Backspace removing the module beside it would be a key
    // doing something nobody pointed it at.
    const onPage = target === document.body || !!this.el(".lpe-canvas")?.contains(target);
    if (!onPage && e.key !== "Escape") return;

    if (e.key === "Escape") {
      e.preventDefault();
      return this.choose("");
    }
    if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault();
      return this.act("remove", this.selected);
    }
    if (!e.key.startsWith("Arrow")) return;
    e.preventDefault();
    if (!(found.module)) {
      if (e.key === "ArrowUp") this.act("up", found.band.id);
      if (e.key === "ArrowDown") this.act("down", found.band.id);
      return;
    }
    const rtl = document.documentElement.dir === "rtl";
    const cells = found.band.cells;
    let { ci, mi } = found;
    let bi = found.bi;
    if (e.key === "ArrowUp") {
      if (mi > 0) mi -= 1;
      else if (bi > 0) {
        bi -= 1;
        ci = Math.min(ci, this.page.bands[bi]!.cells.length - 1);
        mi = this.page.bands[bi]!.cells[ci]!.length;
      } else return;
    } else if (e.key === "ArrowDown") {
      if (mi < cells[ci]!.length - 1) mi += 1;
      else if (bi < this.page.bands.length - 1) {
        bi += 1;
        ci = Math.min(ci, this.page.bands[bi]!.cells.length - 1);
        mi = 0;
      } else return;
    } else {
      const forward = (e.key === "ArrowRight") !== rtl;
      const to = ci + (forward ? 1 : -1);
      if (to < 0 || to >= cells.length) return;
      ci = to;
      mi = Math.min(mi, cells[ci]!.length);
    }
    this.moveModule(found.module.id, this.page.bands[bi]!.id, ci, mi);
  }
}

/**
 * Asks for one picture file. Settles at nothing when the picker is closed with
 * Escape, which fires nothing at all in some browsers: it also listens for the
 * window coming back.
 */
function pickImageFile(): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "image/*";
    input.addEventListener("change", () => resolve(input.files?.[0] ?? null), { once: true });
    window.addEventListener("focus", () => setTimeout(() => resolve(input.files?.[0] ?? null), 400), { once: true });
    input.click();
  });
}
