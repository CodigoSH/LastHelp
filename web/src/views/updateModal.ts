/**
 * The update dialog.
 *
 * A badge that says where the update stands, the title, the two versions with
 * an arrow between them, one sentence, the timeline of steps, a failure block
 * and the actions. Pure, like the other templates: every string arrives
 * translated, so the markup can be checked without the i18n singleton.
 */
import { escapeAttr, escapeHTML } from "../escape";

export type StepStatus = "pending" | "running" | "ok" | "skipped" | "error";
export type UpdatePhase = "idle" | "available" | "running" | "restarting" | "done" | "error";

export interface TimelineStep {
  id: string;
  label: string;
  status: StepStatus;
  /** Extra text beside the label, e.g. while the server is coming back. */
  note?: string;
}

export interface UpdateModalText {
  title: string;
  close: string;
  /** Headline label, already resolved for the current phase. */
  badge: string;
  install: string;
  changelog: string;
  intro: string;
  runningHint: string;
  restarting: string;
  done: string;
  retry: string;
  dockerTitle: string;
  dockerBody: string;
}

export interface UpdateModalData {
  phase: UpdatePhase;
  currentVersion: string;
  latestVersion: string;
  isDocker: boolean;
  steps: TimelineStep[];
  /** Translated failure message, empty when there is none. */
  errorText: string;
  text: UpdateModalText;
  changelogUrl: string;
}

const ICON_CHECK = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>`;
const ICON_CROSS = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>`;
const ICON_DASH = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><line x1="5" y1="12" x2="19" y2="12"/></svg>`;

const dotContent = (status: StepStatus): string =>
  status === "ok" ? ICON_CHECK : status === "error" ? ICON_CROSS : status === "skipped" ? ICON_DASH : "";

/** One timeline entry. The status lives in a class so it can be swapped without
 *  re-rendering the list — a re-render would replay every dot's animation. */
export const stepItem = (step: TimelineStep): string => `
  <li class="tl-step is-${step.status}" data-step="${escapeAttr(step.id)}">
    <span class="tl-dot" aria-hidden="true">${dotContent(step.status)}</span>
    <span class="tl-body">
      <span class="tl-label">${escapeHTML(step.label)}</span>
      <span class="tl-note">${step.note ? escapeHTML(step.note) : ""}</span>
    </span>
  </li>`;

/** The dialog element itself, drawn once; only its body changes afterwards. */
export const updateDialogShell = (): string =>
  `<dialog class="update-dialog" id="update-dialog" aria-labelledby="um-title"><div id="update-dialog-body"></div></dialog>`;

export function updateModalBody(data: UpdateModalData): string {
  const busy = data.phase === "running" || data.phase === "restarting";

  // While the update runs there is no way back: there is no cancel, and half a
  // binary swap cannot be undone on request. A close button would be a lie.
  const closeButton = busy
    ? ""
    : `<button type="button" class="um-close" data-um="close" aria-label="${escapeAttr(data.text.close)}">
        <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
      </button>`;

  const message =
    data.phase === "available"
      ? data.text.intro
      : data.phase === "running"
        ? data.text.runningHint
        : data.phase === "restarting"
          ? data.text.restarting
          : data.phase === "done"
            ? data.text.done
            : "";

  const changelog = `<a class="button ghost" href="${escapeAttr(data.changelogUrl)}" target="_blank" rel="noopener noreferrer">${escapeHTML(data.text.changelog)}</a>`;
  const actions = busy || data.phase === "done"
    ? ""
    : data.phase === "error"
      ? `<button type="button" class="button ghost" data-um="close">${escapeHTML(data.text.close)}</button>
         <button type="button" class="button primary" data-um="retry">${escapeHTML(data.text.retry)}</button>`
      : data.isDocker
        ? changelog
        : `${changelog}<button type="button" class="button primary" data-um="install">${escapeHTML(data.text.install)}</button>`;

  return `
    <div class="um-header">
      <div class="um-heading">
        <span class="um-badge${data.phase === "error" ? " is-error" : ""}">${escapeHTML(data.text.badge)}</span>
        <h3 class="um-title" id="um-title">${escapeHTML(data.text.title)}</h3>
      </div>
      ${closeButton}
    </div>

    <div class="um-versions">
      <span class="um-from">${escapeHTML(data.currentVersion)}</span>
      <span class="um-arrow" aria-hidden="true">→</span>
      <span class="um-to">${escapeHTML(data.latestVersion)}</span>
    </div>

    ${message ? `<p class="um-message" id="um-message">${escapeHTML(message)}</p>` : ""}

    ${
      data.isDocker
        ? `<div class="um-notice">
            <h4 class="um-notice-title">${escapeHTML(data.text.dockerTitle)}</h4>
            <p class="um-notice-text">${escapeHTML(data.text.dockerBody)}</p>
          </div>`
        : `<ol class="um-timeline" role="list" aria-live="polite" aria-busy="${busy}">
            ${data.steps.map(stepItem).join("")}
          </ol>`
    }

    <div class="um-failure" id="um-failure" ${data.errorText ? "" : "hidden"}>
      <p class="um-failure-text" id="um-failure-text">${escapeHTML(data.errorText)}</p>
    </div>

    ${actions ? `<div class="um-actions">${actions}</div>` : ""}`;
}
