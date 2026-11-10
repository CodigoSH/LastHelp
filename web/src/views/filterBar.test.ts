import { describe, expect, test } from "bun:test";
import { activeFilters, choiceMenu, chosenValues, filterChips, filterMenu, segmented, type FilterGroup } from "./filterBar";

const text = { t: (k: string) => `[${k}]`, number: (n: number) => String(n) } as never;

const group = (over: Partial<FilterGroup> = {}): FilterGroup => ({
  name: "status",
  label: "Status",
  value: "",
  options: [
    { value: "", label: "All" },
    { value: "new", label: "New", count: 3 },
    { value: "done", label: "Completed", count: 12 },
  ],
  ...over,
});

describe("a group that takes several", () => {
  test("reads what is chosen from the address's commas, and nothing as everything", () => {
    expect(chosenValues(group({ multi: true, value: "" }))).toEqual([]);
    expect(chosenValues(group({ multi: true, value: "new,done" }))).toEqual(["new", "done"]);
    expect(chosenValues(group({ value: "new" }))).toEqual(["new"]);
  });

  test("is drawn as boxes, with each chosen one ticked and 'everything' ticked only when nothing is", () => {
    const html = filterMenu([group({ multi: true, value: "new,done" })], "Filters", text);
    expect(html).toContain('role="menuitemcheckbox"');
    expect(html).toMatch(/aria-checked="true"[^>]*data-fb-value="new"/);
    expect(html).toMatch(/aria-checked="true"[^>]*data-fb-value="done"/);
    expect(html).toMatch(/aria-checked="false"[^>]*data-fb-value=""/);
    expect(filterMenu([group({ multi: true, value: "" })], "Filters", text)).toMatch(/aria-checked="true"[^>]*data-fb-value=""/);
  });

  test("makes its menu staged: it has the button that applies, and a menu of single choices does not", () => {
    expect(filterMenu([group({ multi: true }), group({ name: "kind", label: "Type" })], "Filters", text)).toContain("data-fb-apply");
    expect(filterMenu([group({ multi: true })], "Filters", text)).toContain("data-fb-staged");
    const single = filterMenu([group(), group({ name: "kind", label: "Type" })], "Filters", text);
    expect(single).not.toContain("data-fb-apply");
    expect(single).not.toContain("data-fb-staged");
  });

  test("counts as one filter however many of its boxes are ticked", () => {
    expect(activeFilters([group({ multi: true, value: "new,done" })])).toBe(1);
    expect(activeFilters([group({ multi: true, value: "" })])).toBe(0);
    expect(activeFilters([group({ multi: true, value: "new" }), group({ name: "kind", value: "bug", options: [{ value: "", label: "All" }, { value: "bug", label: "Bug" }] })])).toBe(2);
    expect(filterMenu([group({ multi: true, value: "new,done" })], "Filters", text)).toContain('<b class="fb-count">1</b>');
  });
});

describe("the other controls", () => {
  test("a choice of a few answers is in sight, with the one that is on pressed", () => {
    const html = segmented(group({ name: "kind", value: "bug", options: [{ value: "", label: "All" }, { value: "bug", label: "Bugs" }, { value: "idea", label: "Ideas" }] }));
    expect(html).toContain('class="fb-seg"');
    expect(html).toMatch(/data-fb-value="bug" aria-pressed="true"/);
    expect(html).toMatch(/data-fb-value="" aria-pressed="false"/);
    expect(segmented(group({ options: [{ value: "", label: "All" }] }))).toBe("");
  });

  test("a choice with many answers is a button that names the answer, with a box to find one from ten", () => {
    const many = group({
      name: "board",
      label: "Repository",
      value: "a/b",
      options: [{ value: "", label: "All" }, ...Array.from({ length: 12 }, (_, i) => ({ value: i === 3 ? "a/b" : `x/${i}`, label: i === 3 ? "Acme / Billing" : `Acme / R${i}` }))],
    });
    const html = choiceMenu(many, text, "<i></i>");
    expect(html).toContain("Acme / Billing</span>"); // the button says which one
    expect(html).toContain("data-fb-find");
    expect(choiceMenu(group({ options: [{ value: "", label: "All" }] }), text, "")).toBe("");
    expect(choiceMenu({ ...many, options: many.options.slice(0, 4) }, text, "")).not.toContain("data-fb-find");
  });

  test("chips come off one by one, and nothing narrowing is no chips", () => {
    expect(filterChips([], "Clear")).toBe("");
    const html = filterChips([{ name: "status:new", label: "New" }, { name: "status:done", label: "Completed" }], "Clear");
    expect(html).toContain('data-fb-undo="status:new"');
    expect(html).toContain('data-fb-undo="status:done"');
    expect(html).toContain("data-fb-clear");
  });
});
