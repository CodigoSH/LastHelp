/**
 * A picture somebody picked, made ready to send: scaled, and written in
 * whichever format this browser does best with.
 *
 * The recipe, and why each part is there:
 *
 *   - Several formats are tried and the lightest kept. Taking the first one
 *     that fits put a photograph in as PNG every time; asked for the smallest,
 *     the same photograph is a fraction of it.
 *   - What the browser returns is checked, not what was asked for. Safari
 *     asked for WebP hands back a PNG and says nothing — measured on Safari 27,
 *     a photograph came back as 2 MB of PNG where JPEG was 155 KB — and a
 *     front page that took WebP only refused every picture added from a Mac or
 *     an iPhone.
 *   - Transparency is kept where the picture has any. JPEG cannot carry it and
 *     puts a black square behind a logo, so JPEG is offered only to a picture
 *     with nothing to lose, on a white sheet.
 *
 * The server reads what arrives by its bytes and accepts WebP, JPEG and PNG.
 */

import { decode } from "./image";

/** Distinguishable from a file the browser could not read at all. */
export class TooLarge extends Error {}

/** How a picture is to come out. */
export interface Target {
  /** Its longest side, in pixels. */
  side: number;
  /** Stop trying lighter encodings once one is at or under this. */
  aim: number;
  /** Refuse to send anything over this: the server would refuse it anyway. */
  cap: number;
}

/** A front page picture: twice the widest a module draws it. */
export const FRONT_PAGE: Target = { side: 1600, aim: 400 * 1024, cap: 2 * 1024 * 1024 };

interface Encoding {
  type: string;
  quality?: number;
}

/**
 * The encodings worth trying, best quality first within each format.
 *
 * PNG only for a picture with transparency: for anything else it is the
 * largest of the three, and at 1600 pixels it is also the slowest to write.
 */
export function encodingsFor(transparent: boolean): Encoding[][] {
  return transparent
    ? [
        [{ type: "image/png" }, { type: "image/webp", quality: 0.85 }],
        [{ type: "image/webp", quality: 0.7 }],
      ]
    : [
        [{ type: "image/webp", quality: 0.85 }, { type: "image/jpeg", quality: 0.82 }],
        [{ type: "image/webp", quality: 0.7 }, { type: "image/jpeg", quality: 0.7 }],
        [{ type: "image/webp", quality: 0.55 }, { type: "image/jpeg", quality: 0.55 }],
      ];
}

/** Of several encodings of one picture, the lightest. Undefined when there are none. */
export function lightest<T extends { size: number }>(candidates: readonly T[]): T | undefined {
  return candidates.reduce<T | undefined>((best, c) => (!best || c.size < best.size ? c : best), undefined);
}

/** Whether any pixel is less than fully opaque. A canvas that cannot be read is assumed to have some. */
function hasTransparency(ctx: CanvasRenderingContext2D, width: number, height: number): boolean {
  let pixels: Uint8ClampedArray;
  try {
    pixels = ctx.getImageData(0, 0, width, height).data;
  } catch {
    return true;
  }
  for (let i = 3; i < pixels.length; i += 4) {
    if ((pixels[i] ?? 255) < 255) return true;
  }
  return false;
}

function toBlob(canvas: HTMLCanvasElement, encoding: Encoding): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, encoding.type, encoding.quality));
}

/**
 * Scale a picked file to fit the target and encode it as lightly as this
 * browser can. Throws TooLarge when nothing comes in under the cap, and the
 * decoder's error when the file is not a picture this browser reads.
 */
export async function preparePicture(file: File, target: Target): Promise<Blob> {
  const source = await decode(file);
  const scale = Math.min(1, target.side / Math.max(source.width, source.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(source.width * scale));
  canvas.height = Math.max(1, Math.round(source.height * scale));
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("this browser has no 2d canvas");

  ctx.drawImage(source.draw, 0, 0, canvas.width, canvas.height);
  const transparent = hasTransparency(ctx, canvas.width, canvas.height);
  if (!transparent) {
    // Painted once, under the picture, for the format that cannot carry alpha.
    // Opaque pixels cover it, so WebP of the same canvas is unchanged.
    ctx.globalCompositeOperation = "destination-over";
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.globalCompositeOperation = "source-over";
  }

  let best: Blob | undefined;
  for (const rung of encodingsFor(transparent)) {
    const made: Blob[] = [];
    for (const encoding of rung) {
      const blob = await toBlob(canvas, encoding);
      // The answer is checked, not the request: a browser that cannot write
      // the type returns a PNG instead of failing.
      if (blob && blob.type === encoding.type) made.push(blob);
    }
    best = lightest([...made, ...(best ? [best] : [])]);
    if (best && best.size <= target.aim) break;
  }

  if (!best || best.size > target.cap) throw new TooLarge("that picture cannot be made small enough");
  return best;
}
