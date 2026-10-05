import { describe, expect, test } from "bun:test";
import { SECRET_MASK, secretChange } from "./secretField";

describe("secretChange", () => {
  test("leaves a saved secret alone when the dots are untouched", () => {
    expect(secretChange(SECRET_MASK, true)).toEqual({ action: "keep" });
  });

  test("treats half-erased dots as the sign, not as an instruction", () => {
    expect(secretChange("•••", true)).toEqual({ action: "keep" });
  });

  test("replaces the secret with what is typed, over or beside the dots", () => {
    expect(secretChange("tk_new", true)).toEqual({ action: "set", token: "tk_new" });
    expect(secretChange(SECRET_MASK + "tk_new", true)).toEqual({ action: "set", token: "tk_new" });
    expect(secretChange("  tk_new ", false)).toEqual({ action: "set", token: "tk_new" });
  });

  test("removes the secret when the box is emptied", () => {
    expect(secretChange("", true)).toEqual({ action: "clear" });
  });

  test("has nothing to do when there was none and none is typed", () => {
    expect(secretChange("", false)).toEqual({ action: "keep" });
  });
});
