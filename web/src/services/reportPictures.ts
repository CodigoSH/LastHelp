/**
 * Pictures on a report: choosing them before they are sent, sending them, and
 * taking one back.
 *
 * A picture is sent after the report or the message it belongs to, hung on it
 * by its id. The report goes in exactly as it did before, so one picture that
 * fails does not take the report down with it, and the server knows who is
 * sending by the reporter's follow link or the team's session — never by
 * anything a page could invent.
 */

import { ApiError, csrfToken } from "../api";
import { preparePicture, type Target, TooLarge } from "./picture";

/** A picture on a report: twice the width a report draws it, and small. */
export const REPORT_PICTURE: Target = { side: 1600, aim: 400 * 1024, cap: 2 * 1024 * 1024 };

/** The ceilings the server keeps, said here so the page stops before it. */
export const PICTURES_ON_A_REPORT = 5;
export const PICTURES_ON_A_MESSAGE = 3;

/** The header the server reads a follow link from. Matches pictures.go. */
const FOLLOW_TOKEN_HEADER = "X-Follow-Token";

async function send(method: string, path: string, body: Blob | undefined, token?: string): Promise<void> {
  const headers: Record<string, string> = { "X-CSRF-Token": csrfToken() };
  if (body) headers["Content-Type"] = body.type;
  if (token) headers[FOLLOW_TOKEN_HEADER] = token;
  const res = await fetch(path, { method, headers, body, credentials: "same-origin" });
  if (res.ok) return;
  const answer = await res.json().catch(() => ({}));
  throw new ApiError(res.status, (answer as { error?: string }).error ?? "error.internal");
}

/**
 * Send pictures one after another, onto the report or onto one message on it.
 * Returns the reason for the first that failed, or "" when all went in; the
 * rest are still tried, so one bad file does not cost the others.
 */
export async function sendPictures(
  publicID: string,
  pictures: readonly Blob[],
  on: { comment?: number; token?: string } = {},
): Promise<string> {
  const path = `/api/items/${encodeURIComponent(publicID)}/images${on.comment ? `?comment=${on.comment}` : ""}`;
  let failed = "";
  for (const picture of pictures) {
    try {
      await send("POST", path, picture, on.token);
    } catch (err) {
      if (!failed) failed = err instanceof ApiError ? err.key : "error.internal";
    }
  }
  return failed;
}

/** Take one picture out. */
export function removePicture(publicID: string, id: string, token?: string): Promise<void> {
  return send("DELETE", `/api/items/${encodeURIComponent(publicID)}/images/${encodeURIComponent(id)}`, undefined, token);
}

/** What a tray holds: a picture ready to send, and the preview drawn for it. */
interface Chosen {
  blob: Blob;
  preview: string;
}

/** A tray of chosen pictures, held until the message they belong to is sent. */
export interface Tray {
  pictures(): Blob[];
  /** Empty it, once what it held has gone with its message. */
  clear(): void;
}

function asDataURL(blob: Blob): Promise<string> {
  // A data: address, not blob:, because that is what the page's policy lets
  // an image load from besides the server itself.
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

/**
 * Wire the tray drawn by views.pictureTray inside `scope`: choose, drop or
 * paste, see small previews, take one out before sending. Returns null when
 * there is no tray, which is a board with pictures off.
 */
export function wireTray(
  scope: ParentNode | null,
  labels: { remove: string },
  failed: (key: string) => void,
): Tray | null {
  const tray = scope?.querySelector<HTMLElement>("[data-tray]");
  if (!tray) return null;
  const max = Number(tray.dataset.max ?? "1");
  const input = tray.querySelector<HTMLInputElement>("[data-tray-input]");
  const list = tray.querySelector<HTMLElement>("[data-tray-thumbs]");
  const add = tray.querySelector<HTMLElement>("[data-tray-add]");
  const chosen: Chosen[] = [];

  const draw = () => {
    if (list) {
      list.innerHTML = chosen
        .map(
          (c, i) => `<li class="tray-thumb">
            <img src="${c.preview.replace(/"/g, "&quot;")}" alt="" />
            <button type="button" class="tray-remove" data-tray-remove="${i}"
                    aria-label="${labels.remove.replace(/"/g, "&quot;")}" title="${labels.remove.replace(/"/g, "&quot;")}">×</button>
          </li>`,
        )
        .join("");
      list.hidden = chosen.length === 0;
    }
    if (add) add.hidden = chosen.length >= max;
  };

  const take = async (files: Iterable<File>) => {
    for (const file of files) {
      if (chosen.length >= max) {
        failed("error.too_many_images");
        break;
      }
      if (!file.type.startsWith("image/")) {
        failed("error.not_an_image");
        continue;
      }
      try {
        const blob = await preparePicture(file, REPORT_PICTURE);
        chosen.push({ blob, preview: await asDataURL(blob) });
      } catch (err) {
        failed(err instanceof TooLarge ? "error.image_too_large" : "error.not_an_image");
      }
    }
    draw();
  };

  input?.addEventListener("change", () => {
    void take(Array.from(input.files ?? []));
    input.value = "";
  });
  list?.addEventListener("click", (event) => {
    const button = (event.target as HTMLElement).closest<HTMLElement>("[data-tray-remove]");
    if (!button) return;
    chosen.splice(Number(button.dataset.trayRemove), 1);
    draw();
  });
  // A screenshot pasted anywhere in the form it belongs to, which is how most
  // of them arrive: the person has just taken it.
  tray.closest("form")?.addEventListener("paste", (event) => {
    const files = Array.from((event as ClipboardEvent).clipboardData?.files ?? []);
    if (files.length === 0) return;
    event.preventDefault();
    void take(files);
  });
  tray.addEventListener("dragover", (event) => {
    event.preventDefault();
    tray.classList.add("dropping");
  });
  tray.addEventListener("dragleave", () => tray.classList.remove("dropping"));
  tray.addEventListener("drop", (event) => {
    event.preventDefault();
    tray.classList.remove("dropping");
    void take(Array.from(event.dataTransfer?.files ?? []));
  });

  draw();
  return {
    pictures: () => chosen.map((c) => c.blob),
    clear: () => {
      chosen.length = 0;
      draw();
    },
  };
}
