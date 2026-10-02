/**
 * The two decisions in picture.ts that do not need a canvas: which formats a
 * picture is offered, and which of the results is sent.
 */
import { describe, expect, test } from "bun:test";
import { encodingsFor, lightest } from "./picture";

const types = (transparent: boolean) => encodingsFor(transparent).flat().map((e) => e.type);

describe("the formats a picture is offered", () => {
  test("a picture with transparency is never offered JPEG", () => {
    // JPEG cannot carry alpha: a logo comes back with a black square behind it.
    expect(types(true)).not.toContain("image/jpeg");
    expect(types(true)).toContain("image/png");
  });

  test("an opaque picture can always fall back to JPEG", () => {
    // Safari asked for WebP returns a PNG; JPEG is what it sends instead.
    for (const rung of encodingsFor(false)) {
      expect(rung.map((e) => e.type)).toContain("image/jpeg");
    }
  });

  test("an opaque picture is not offered PNG", () => {
    // The largest of the three for a photograph, and the slowest to write.
    expect(types(false)).not.toContain("image/png");
  });
});

describe("the one that is sent", () => {
  test("is the lightest, not the first", () => {
    // Taking the first that fitted put every photograph in as PNG.
    const chosen = lightest([
      { type: "image/png", size: 2_067_000 },
      { type: "image/jpeg", size: 155_000 },
      { type: "image/webp", size: 190_000 },
    ]);
    expect(chosen?.type).toBe("image/jpeg");
  });

  test("is nothing when nothing was made", () => {
    expect(lightest([])).toBeUndefined();
  });
});

describe("the ceilings the page keeps", () => {
  // Written twice — here, so the page stops before sending, and in the server,
  // which decides. Two copies of a number drift apart; this reads the other.
  test("match the server's", async () => {
    const { PICTURES_ON_A_MESSAGE, PICTURES_ON_A_REPORT, REPORT_PICTURE } = await import("./reportPictures");
    const go = await Bun.file(new URL("../../../internal/core/item/images.go", import.meta.url)).text();
    const constant = (name: string) => {
      const found = go.match(new RegExp(`\\b${name}\\s*=\\s*([0-9<]+(?:\\s*<<\\s*[0-9]+)?)`));
      if (!found) throw new Error(`${name} is not in images.go any more; this test is reading the wrong file`);
      const raw = found[1] ?? "";
      return raw.includes("<<") ? raw.split("<<").map(Number).reduce((a, b) => a * 2 ** b) : Number(raw);
    };
    expect(PICTURES_ON_A_REPORT).toBe(constant("MaxImagesPerReport"));
    expect(PICTURES_ON_A_MESSAGE).toBe(constant("MaxImagesPerComment"));
    expect(REPORT_PICTURE.cap).toBe(constant("MaxImageUpload"));
  });
});
