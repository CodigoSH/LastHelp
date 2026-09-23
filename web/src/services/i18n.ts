import { availableLocales, directionOf } from "../locales/metadata";
import {
  formatBytes,
  formatDate,
  formatNumber,
  formatAge,
  formatDuration,
  resolveLanguage,
  translate,
} from "./i18n.helpers";

declare const __BUILD_STAMP__: string;

const STORAGE_KEY = "lasthelp_lang";

/**
 * The one translator.
 *
 * Nothing else interpolates: a call site that takes a translated sentence and
 * runs its own `.replace()` skips the bidi isolation, and the same list then
 * renders with two different alignments in Arabic depending on whether the line
 * starts with the value or with a word. That bug has shipped twice in this
 * house, so an architecture rule greps for a second interpolator.
 */
class I18n {
  private language = "en";
  private tables = new Map<string, Record<string, string>>();
  private listeners = new Set<() => void>();

  /** Resolved once the first catalogue is in. Screens await it before drawing
   *  so nothing paints in English and then flips. */
  readonly ready: Promise<void>;

  constructor() {
    this.ready = this.start();
  }

  private async start(): Promise<void> {
    // English first and always: it is the fallback every other table leans on,
    // so a missing key shows the English sentence rather than a raw key.
    await this.load("en");

    const codes = availableLocales.map((l) => l.code);
    const saved = safeRead(STORAGE_KEY);
    const target = resolveLanguage(saved, navigator.language, codes);

    if (target !== "en") await this.load(target);
    this.language = target;
    this.apply();
  }

  private async load(code: string): Promise<void> {
    if (this.tables.has(code)) return;
    try {
      // Versioned with the build stamp rather than the clock: a per-load
      // timestamp makes every request a cache miss and re-downloads the whole
      // catalogue on every page.
      const stamp = typeof __BUILD_STAMP__ === "string" ? __BUILD_STAMP__ : "dev";
      const res = await fetch(`/locales/${code}.json?v=${stamp}`);
      if (!res.ok) throw new Error(`locale ${code}: ${res.status}`);
      this.tables.set(code, await res.json());
    } catch (err) {
      // Not fatal: English is already loaded, so the interface still reads.
      console.error(err);
    }
  }

  async setLanguage(code: string): Promise<void> {
    if (!availableLocales.some((l) => l.code === code)) return;
    await this.load(code);
    this.language = code;
    safeWrite(STORAGE_KEY, code);
    this.apply();
  }

  get current(): string {
    return this.language;
  }

  /** Whether a language was chosen here, as opposed to inferred from the browser. */
  get chosen(): boolean {
    return Boolean(safeRead(STORAGE_KEY));
  }

  t(key: string, params?: Record<string, string | number>): string {
    return translate(key, this.tables.get(this.language), this.tables.get("en"), params);
  }

  date(value: Date | number): string {
    const date = typeof value === "number" ? new Date(value * 1000) : value;
    return formatDate(this.language, date);
  }

  number(value: number): string {
    return formatNumber(this.language, value);
  }

  bytes(value: number): string {
    return formatBytes(this.language, value);
  }

  since(seconds: number): string {
    return formatDuration(this.language, seconds);
  }

  age(seconds: number): string {
    return formatAge(this.language, seconds);
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /**
   * Tell the document what language it is in and which way it reads.
   *
   * The server already stamps both for the first paint. This is what keeps them
   * right when somebody changes language without reloading — and leaving
   * lang="en" on an Arabic page is what a screen reader announces and what the
   * browser hyphenates and spellchecks by.
   */
  private apply(): void {
    document.documentElement.lang = this.language;
    document.documentElement.dir = directionOf(this.language);
    for (const listener of this.listeners) listener();
  }
}

function safeRead(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    // A private window, or site data blocked. Not a reason to fail to start.
    return null;
  }
}

function safeWrite(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* the choice simply does not survive the session */
  }
}

export const i18n = new I18n();
export const t = (key: string, params?: Record<string, string | number>): string =>
  i18n.t(key, params);
