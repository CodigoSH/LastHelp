/**
 * A picture, made small enough to store, in the browser.
 *
 * An avatar is scaled to a couple of hundred pixels
 * and re-encoded before it is sent, so nobody is ever told their photo is too
 * large. Every photograph a phone takes is too large, and "pick a smaller one"
 * is not something to ask of the person setting up a server.
 *
 * Two things that were bugs before they were rules:
 *
 *   - The cap is measured on the data URL, not on the file. The server stores
 *     the string and bounds the string; base64 is about a third longer than the
 *     bytes it carries, so a file comfortably under the limit produced a value
 *     over it and the upload was refused after passing the check meant to
 *     prevent exactly that.
 *
 *   - Transparency survives where it can. A logo re-encoded as JPEG comes back
 *     with a black square behind it, which is worse than the size problem it
 *     was solving. WebP and PNG keep the alpha channel; JPEG is the last
 *     resort, and only then is a white background painted in.
 */

/** Matches the bound the server enforces on the stored value. */
const MAX_BYTES = 96 * 1024;

/** Enough for an avatar at twice the size it is ever drawn. */
const MAX_SIDE = 256;

export async function decode(file: File): Promise<{ width: number; height: number; draw: CanvasImageSource }> {
  // createImageBitmap applies the EXIF rotation, which is why a photograph
  // taken in portrait does not arrive on its side.
  if (typeof createImageBitmap === "function") {
    try {
      const bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
      return { width: bitmap.width, height: bitmap.height, draw: bitmap };
    } catch {
      // Fall through: some browsers refuse the option rather than ignore it.
    }
  }

  const url = URL.createObjectURL(file);
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image();
      el.onload = () => resolve(el);
      el.onerror = () => reject(new Error("that file is not an image this browser can read"));
      el.src = url;
    });
    return { width: img.naturalWidth, height: img.naturalHeight, draw: img };
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** How big a picked image may come out: its longest side, and its encoded size. */
export interface Fit {
  side: number;
  bytes: number;
}

/** An avatar or a logo: twice the size it is ever drawn, within the inline cap. */
export const PICTURE: Fit = { side: MAX_SIDE, bytes: MAX_BYTES };

/**
 * A repository's icon: drawn at 40px, and carried in every list of
 * repositories, so small — the server refuses more than 24 KB.
 */
export const ICON: Fit = { side: 128, bytes: 22 * 1024 };

function render(source: { width: number; height: number; draw: CanvasImageSource }, opaque: boolean, side: number): HTMLCanvasElement {
  const scale = Math.min(1, side / Math.max(source.width, source.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(source.width * scale));
  canvas.height = Math.max(1, Math.round(source.height * scale));

  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("this browser has no 2d canvas");
  if (opaque) {
    // Only for the format that cannot carry alpha. Painting this in every time
    // would put a white square behind every transparent logo.
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
  }
  ctx.drawImage(source.draw, 0, 0, canvas.width, canvas.height);
  return canvas;
}

/**
 * Scale and re-encode a picked file into a data URL under the cap.
 *
 * Throws only when no encoding of a 256-pixel image fits, which does not
 * happen with a real picture — the caller still handles it, because a thrown
 * error is better than an upload that is refused with no explanation.
 */
export async function shrink(file: File, fit: Fit = PICTURE): Promise<string> {
  const source = await decode(file);

  // Lossless first, and only then lossy. A logo is flat colour: PNG keeps it
  // exact and fits easily, while WebP at 0.9 came back with the house orange
  // one value out — invisible, and still the wrong answer for a brand mark
  // when the right one costs nothing. A photograph does not fit as PNG, falls
  // through, and gets the compression it needs.
  //
  // Transparency is kept for as long as possible: the white background belongs
  // only to the format that cannot carry alpha.
  const attempts: Array<{ type: string; quality?: number; opaque: boolean }> = [
    { type: "image/png", opaque: false },
    { type: "image/webp", quality: 0.9, opaque: false },
    { type: "image/webp", quality: 0.7, opaque: false },
    { type: "image/webp", quality: 0.5, opaque: false },
    { type: "image/jpeg", quality: 0.8, opaque: true },
    { type: "image/jpeg", quality: 0.6, opaque: true },
    { type: "image/jpeg", quality: 0.4, opaque: true },
  ];

  let smallest = "";
  for (const attempt of attempts) {
    const url = render(source, attempt.opaque, fit.side).toDataURL(attempt.type, attempt.quality);
    // A browser that cannot encode the type returns a PNG instead of failing,
    // so the answer is checked rather than the request.
    if (!url.startsWith(`data:${attempt.type}`) && attempt.type !== "image/png") continue;
    if (url.length <= fit.bytes) return url;
    if (!smallest || url.length < smallest.length) smallest = url;
  }

  throw new TooLarge("that image cannot be made small enough");
}

/** Distinguishable from a file the browser could not decode at all. */
export class TooLarge extends Error {}
