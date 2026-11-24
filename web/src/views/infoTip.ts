/**
 * What a setting or a field means, behind a small round "i" beside its name.
 *
 * The sentence used to sit under every field and every row, and a page of
 * settings was half explanations that somebody reads once. Here it is one
 * hover, one tap or one Tab away, and the rows are one line each.
 *
 * A button rather than a span: it takes focus from the keyboard and from a
 * tap, and its accessible name is the sentence itself, so a screen reader
 * reads the help where the eye would have found it. `services/infoTips.ts`
 * opens and places it.
 */
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";

let next = 0;

/** The "i" and its sentence. `id` names the sentence, for a field that points at it with aria-describedby. */
export function infoTip(help: string, id = `tip-${++next}`): string {
  return `<button type="button" class="tip" data-tip>${icons.info(14)}<span class="tip-text" role="tooltip" id="${escapeAttr(id)}">${escapeHTML(help)}</span></button>`;
}
