/**
 * The eye on a password field, on every password field.
 *
 * Applied to whatever has just been drawn rather than written into each form:
 * there are five of them — signing in, the wizard, the profile, adding a
 * person, editing one — and a toggle added to the form somebody complained
 * about is a toggle missing from the other four.
 *
 * Idempotent, because screens are redrawn: a field that already has one is
 * left alone rather than given a second.
 */
import { i18n } from "./i18n";

const EYE = `<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
  stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7-11-7-11-7z"/><circle cx="12" cy="12" r="3"/></svg>`;

const EYE_OFF = `<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
  stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/>
  <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/>
  <path d="M14.12 14.12a3 3 0 1 1-4.24-4.24"/><path d="M1 1l22 22"/></svg>`;

export function attachPasswordToggles(root: ParentNode | null): void {
  if (!root) return;

  for (const input of root.querySelectorAll<HTMLInputElement>('input[type="password"]')) {
    if (input.parentElement?.classList.contains("pw-wrap")) continue;

    const wrap = document.createElement("div");
    wrap.className = "pw-wrap";
    input.parentElement?.insertBefore(wrap, input);
    wrap.appendChild(input);

    const button = document.createElement("button");
    button.type = "button";
    button.className = "pw-toggle";
    button.innerHTML = EYE;
    button.setAttribute("aria-label", i18n.t("auth.showPassword"));
    button.addEventListener("click", () => {
      const hidden = input.type === "password";
      input.type = hidden ? "text" : "password";
      button.innerHTML = hidden ? EYE_OFF : EYE;
      button.setAttribute("aria-label", i18n.t(hidden ? "auth.hidePassword" : "auth.showPassword"));
      // The caret goes back where it was: retyping a password because the
      // cursor jumped to the start is the reason people distrust these.
      const at = input.value.length;
      input.focus();
      input.setSelectionRange(at, at);
    });
    wrap.appendChild(button);
  }
}
