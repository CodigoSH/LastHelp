/**
 * A picture put into the front page.
 *
 * Scaled and re-encoded here, in the browser, before anything is sent — see
 * picture.ts for how the format is chosen. A photograph off a phone is several
 * megabytes; at the size a front page draws it, it is tens of kilobytes, and
 * the operator picks the file they have instead of being told to make a
 * smaller one.
 */

import { api } from "../api";
import { FRONT_PAGE, preparePicture } from "./picture";

export { TooLarge } from "./picture";

/** Prepare, send, and give back the path the picture is served from. */
export async function putImage(file: File): Promise<string> {
  const prepared = await preparePicture(file, FRONT_PAGE);
  const { url } = await api.postBytes<{ url: string }>("/api/landing/images", prepared);
  return url;
}
