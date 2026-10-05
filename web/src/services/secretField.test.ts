import { describe, expect, test } from "bun:test";
import { MASK, secretIntent } from "./secretField";

describe("a stored secret in a form", () => {
  test("the mask alone, or half of it, changes nothing", () => {
    expect(secretIntent(MASK, true)).toEqual({ kind: "keep" });
    expect(secretIntent(MASK.slice(0, 3), true)).toEqual({ kind: "keep" });
  });
  test("what is typed over the mask, or beside it, is the new secret", () => {
    expect(secretIntent("tk_new", true)).toEqual({ kind: "set", secret: "tk_new" });
    expect(secretIntent(MASK + "tk_new", true)).toEqual({ kind: "set", secret: "tk_new" });
    expect(secretIntent("tk" + MASK.slice(0, 2) + "_new", true)).toEqual({ kind: "set", secret: "tk_new" });
  });
  test("an emptied field removes the secret, when there was one", () => {
    expect(secretIntent("", true)).toEqual({ kind: "clear" });
  });
  test("an empty field with nothing saved is nothing", () => {
    expect(secretIntent("", false)).toEqual({ kind: "keep" });
  });
  test("a secret is set whether or not one was saved", () => {
    expect(secretIntent("tk_first", false)).toEqual({ kind: "set", secret: "tk_first" });
  });
});
