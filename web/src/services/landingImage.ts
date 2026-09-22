/**
 * A picture put into a block of words.
 *
 * It is re-encoded to WebP here, in the browser, before anything is sent. That
 * is deliberate and it is where the whole feature lives or dies:
 *
 *   - A photograph off a phone is four megabytes of JPEG. The same picture at
 *     the size a front page actually draws it, as WebP, is tens of kilobytes.
 *     The operator picks the file they have rather than being told to go and
 *     make a smaller one, which is not a reasonable thing to ask of somebody
 *     setting up a server.
 *   - A front page that loads quickly is the half of search ranking that is
 *     under our control, and the largest thing on most front pages is a
 *     picture.
 *   - Encoding WebP on the server would mean a C library and a binary that is
 *     no longer one static file. The browser already has the encoder.
 *
 * The server accepts WebP and nothing else, and checks the bytes rather than
 * what the request claims — see internal/core/landing/image.go. So a browser
 * too old to encode it is told plainly rather than having its JPEG quietly
 * stored under a name that promises otherwise.
 */

import { api } from "../api";

/**
 * The widest a picture is stored at.
 *
 * Twice the widest a module is ever drawn, so it still looks right on a screen
 * that draws two pixels for every one. Past that is weight nobody sees.
 */
const MAX_SIDE = 1600;

/** Thrown when this browser has no WebP encoder, which is the only hard stop. */
export class NoWebP extends Error {}

async function decode(file: File): Promise<CanvasImageSource & { width: number; height: number }> {
  // createImageBitmap applies the EXIF rotation, which is why a photograph
  // taken in portrait does not arrive on its side.
  if (typeof createImageBitmap === "function") {
    try {
      return (await createImageBitmap(file, {
        imageOrientation: "from-image",
      })) as ImageBitmap & { width: number; height: number };
    } catch {
      // Some browsers refuse the option rather than ignore it.
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
    return Object.assign(img, { width: img.naturalWidth, height: img.naturalHeight });
  } finally {
    URL.revokeObjectURL(url);
  }
}

/**
 * Scale, encode as WebP, send, and give back the path it is served from.
 *
 * The quality ladder walks down rather than picking one number: a screenshot of
 * flat colour is tiny at 0.9 and there is no reason to spend quality it does
 * not need, while a photograph needs the lower rungs to fit. Stopping at the
 * first that fits is what makes both cases come out right.
 */
export async function putImage(file: File): Promise<string> {
  const source = await decode(file);
  const scale = Math.min(1, MAX_SIDE / Math.max(source.width, source.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(source.width * scale));
  canvas.height = Math.max(1, Math.round(source.height * scale));
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new NoWebP("this browser has no 2d canvas");
  ctx.drawImage(source, 0, 0, canvas.width, canvas.height);

  let encoded: Blob | null = null;
  for (const quality of [0.9, 0.8, 0.7, 0.6]) {
    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, "image/webp", quality),
    );
    // A browser with no WebP encoder returns a PNG rather than failing, so the
    // answer is checked and not the request.
    if (!blob || blob.type !== "image/webp") throw new NoWebP("this browser cannot write WebP");
    encoded = blob;
    if (blob.size <= 400 * 1024) break;
  }
  if (!encoded) throw new NoWebP("this browser cannot write WebP");

  const { url } = await api.postBytes<{ url: string }>("/api/landing/images", encoded);
  return url;
}
