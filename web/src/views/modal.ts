/**
 * The modal, built on <dialog>.
 *
 * On the browser's own element rather than a div with a z-index, because
 * everything a dialog has to get right is already in it: the focus goes inside
 * and stays there, Escape closes it, the rest of the page is inert, and the
 * backdrop is a pseudo-element rather than an extra node to keep in sync. A
 * hand-rolled one is three of those bugs waiting.
 *
 * Forms used to sit at the bottom of whatever screen was nearest: adding a
 * person under the list of people, adding an organisation under the panel. A
 * page that is half list and half form reads as unfinished, and the form is
 * always the part somebody scrolls past.
 */
import { escapeAttr, escapeHTML } from "../escape";
import { icons } from "./icons";
import type { Text } from "./templates";

export interface ModalSpec {
  id: string;
  title: string;
  /** A sentence under the title, when the title alone would be an order. */
  lead?: string;
  body: string;
  submit: string;
  /** Marks the action as the destructive one, which reads and colours apart. */
  danger?: boolean;
  formClass: string;
}

export function modal(spec: ModalSpec, text: Text): string {
  return `<dialog class="modal" id="${escapeAttr(spec.id)}">
    <form class="${escapeAttr(spec.formClass)}" method="dialog">
      <header class="modal-head">
        <h2>${escapeHTML(spec.title)}</h2>
        <button type="button" class="icon-button" data-close
          aria-label="${escapeAttr(text.t("action.close"))}">${icons.close()}</button>
      </header>
      ${spec.lead ? `<p class="lead">${escapeHTML(spec.lead)}</p>` : ""}
      <div class="modal-body">${spec.body}</div>
      <p class="form-error" role="alert" hidden></p>
      <footer class="modal-foot">
        <button type="button" class="button ghost" data-close>${escapeHTML(text.t("action.cancel"))}</button>
        <button type="submit" class="button ${spec.danger ? "danger" : "primary"}">${escapeHTML(spec.submit)}</button>
      </footer>
    </form>
  </dialog>`;
}

/**
 * Asking before something that cannot be taken back: a title over a rule, the
 * sentence, and one danger button at the end of the line — the cross is the
 * way out. Drawn once
 * and filled for each question; the browser's own confirm() it replaces is a
 * grey box in another product's voice.
 */
export function confirmDialog(words: { close: string; confirm: string }): string {
  return `<dialog class="modal confirm" id="confirm-dialog" aria-labelledby="confirm-title">
    <div class="confirm-head">
      <h2 class="confirm-title" id="confirm-title"></h2>
      <button type="button" class="picker-close" data-confirm="no"
        aria-label="${escapeAttr(words.close)}">${icons.close(20)}</button>
    </div>
    <p class="confirm-message"></p>
    <div class="confirm-actions">
      <button type="button" class="button danger" data-confirm="yes">${escapeHTML(words.confirm)}</button>
    </div>
  </dialog>`;
}

/**
 * A button that carries an icon and a label.
 *
 * The label is not optional. An icon alone is a guess for anybody who has not
 * used this screen before, and the first time somebody guesses wrong on a
 * delete is the last time they trust the row of little pictures.
 */
export function actionButton(
  action: "add" | "edit" | "remove" | "download" | "upload" | "update" | "copy" | "check",
  label: string,
  attrs = "",
  kind: "primary" | "ghost" | "danger" = "ghost",
): string {
  return `<button type="button" class="button ${kind} with-icon" ${attrs}>${icons[action]()}<span>${escapeHTML(label)}</span></button>`;
}

/** The same, reduced to the icon, for a row where the label is the row itself. */
export function iconButton(
  action: "edit" | "remove" | "open" | "person",
  label: string,
  attrs = "",
): string {
  // A class the caller passes replaces this one rather than following it: a
  // second class attribute on one element is dropped by the browser, which
  // silently took away every variant — the red of a delete — for as long as
  // the two were written side by side.
  const own = /\bclass="([^"]*)"/.exec(attrs);
  const rest = own ? attrs.replace(own[0], "") : attrs;
  return `<button type="button" class="${own ? own[1] : "icon-button"}" title="${escapeAttr(label)}"
    aria-label="${escapeAttr(label)}" ${rest}>${icons[action]()}</button>`;
}
