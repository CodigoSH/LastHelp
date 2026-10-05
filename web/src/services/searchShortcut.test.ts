import { describe, expect, test } from "bun:test";
import { isSearchShortcut, modKey, ownsCtrlK } from "./searchShortcut";

const key = (o: Partial<Parameters<typeof isSearchShortcut>[0]>) =>
  ({ key: "k", ctrlKey: true, metaKey: false, altKey: false, shiftKey: false, ...o });

describe("the search shortcut", () => {
  test("is K with Ctrl, or with Cmd, in either case", () => {
    expect(isSearchShortcut(key({}))).toBe(true);
    expect(isSearchShortcut(key({ ctrlKey: false, metaKey: true }))).toBe(true);
    expect(isSearchShortcut(key({ key: "K" }))).toBe(true);
  });
  test("is nothing else", () => {
    expect(isSearchShortcut(key({ ctrlKey: false }))).toBe(false);
    expect(isSearchShortcut(key({ key: "j" }))).toBe(false);
    expect(isSearchShortcut(key({ altKey: true }))).toBe(false);
    expect(isSearchShortcut(key({ shiftKey: true }))).toBe(false);
  });
  test("keeps out of the way where Ctrl+K means something of its own", () => {
    expect(ownsCtrlK({ tagName: "TEXTAREA" } as Element)).toBe(true);
    expect(ownsCtrlK({ tagName: "SELECT" } as Element)).toBe(true);
    expect(ownsCtrlK({ tagName: "DIV", isContentEditable: true } as unknown as Element)).toBe(true);
    expect(ownsCtrlK({ tagName: "INPUT" } as Element)).toBe(false);
    expect(ownsCtrlK(null)).toBe(false);
  });
  test("names the modifier of the machine", () => {
    expect(modKey("MacIntel")).toBe("⌘");
    expect(modKey("Linux x86_64")).toBe("Ctrl");
    expect(modKey("Win32")).toBe("Ctrl");
  });
});
