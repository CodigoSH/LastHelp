/**
 * The one way this product asks before something that cannot be undone.
 *
 * Lastboard's confirmation, with the same promise it returns: true when the
 * danger button is pressed, false for the cross, the backdrop or Escape. It
 * replaces window.confirm(), which the browser draws in its own grey and its
 * own voice, on top of a product that otherwise never does that.
 */
import { i18n } from "./services/i18n";
import { confirmDialog } from "./views/modal";

let dialog: HTMLDialogElement | null = null;
let answer: ((yes: boolean) => void) | null = null;

function settle(yes: boolean): void {
  const resolve = answer;
  answer = null;
  if (dialog?.open) dialog.close();
  resolve?.(yes);
}

function ensure(): HTMLDialogElement {
  if (dialog?.isConnected) return dialog;
  document.body.insertAdjacentHTML("beforeend", confirmDialog({ close: i18n.t("action.close"), confirm: i18n.t("action.confirm") }));
  dialog = document.getElementById("confirm-dialog") as HTMLDialogElement;
  dialog.addEventListener("click", (e) => {
    const target = e.target as HTMLElement;
    // The backdrop reports the dialog itself as the target.
    if (target === dialog) return settle(false);
    const choice = target.closest<HTMLElement>("[data-confirm]")?.dataset.confirm;
    if (choice) settle(choice === "yes");
  });
  // Escape closes the dialog on its own; the question still needs its answer,
  // or whoever is waiting on it waits forever.
  dialog.addEventListener("close", () => settle(false));
  return dialog;
}

export function askConfirm(title: string, message: string): Promise<boolean> {
  settle(false);
  const box = ensure();
  box.querySelector(".confirm-title")!.textContent = title;
  box.querySelector(".confirm-message")!.textContent = message;
  // Written each time: the language may have changed since it was drawn.
  box.querySelector('[data-confirm="yes"]')!.textContent = i18n.t("action.confirm");
  box.querySelector('[data-confirm="no"]')!.setAttribute("aria-label", i18n.t("action.close"));
  return new Promise<boolean>((resolve) => {
    answer = resolve;
    box.showModal();
  });
}
