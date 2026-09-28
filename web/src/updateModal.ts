/**
 * The one place an update happens, as in Lastboard.
 *
 * Lastboard's updateStore and UpdateModal in one: it owns the whole flow —
 * confirmation, progress, restart, outcome — and the page only opens it. The
 * dialog is drawn once and kept; its body is redrawn when the layout changes
 * (a new phase), and only the dots change while the steps advance, so no
 * animation replays under the reader's eyes.
 */
import { csrfToken } from "./api";
import { i18n } from "./services/i18n";
import {
  stepItem,
  updateDialogShell,
  updateModalBody,
  type StepStatus,
  type TimelineStep,
  type UpdatePhase,
} from "./views/updateModal";

/** What the last check found — the server's /api/update answer. */
export interface UpdateInfo {
  current: string;
  latest?: string;
  available?: boolean;
  container?: boolean;
}

/** The steps the server reports, in the order they run. Mirrors UpdateSteps in update.go. */
export const UPDATE_STEPS = ["signature", "download", "checksum", "prove", "backup", "install", "restart"] as const;
type StepId = (typeof UPDATE_STEPS)[number];

const CHANGELOG_URL = "https://git.codigosh.com/CodigoSH/LastHelp/releases";

/** How long the finished timeline stays on screen before the page reloads. */
const RELOAD_DELAY_MS = 1400;
/** How long to wait for the new binary to answer before giving up. */
const RESTART_POLL_INTERVAL_MS = 1000;
const RESTART_TIMEOUT_MS = 90_000;

const freshSteps = (): Record<StepId, StepStatus> =>
  Object.fromEntries(UPDATE_STEPS.map((id) => [id, "pending"])) as Record<StepId, StepStatus>;

let dialog: HTMLDialogElement | null = null;
let info: UpdateInfo | null = null;
let phase: UpdatePhase = "idle";
let steps = freshSteps();
let errorKey = "";
let renderedPhase: UpdatePhase | null = null;

const busy = () => phase === "running" || phase === "restarting";

/** Opens the dialog on what the last check found. */
export function openUpdateDialog(found: UpdateInfo): void {
  if (busy()) return;
  info = found;
  phase = found.available || found.container ? "available" : "idle";
  steps = freshSteps();
  errorKey = "";
  ensureDialog();
  renderBody();
  if (!dialog?.open) dialog?.showModal();
}

function ensureDialog(): void {
  if (dialog?.isConnected) return;
  document.body.insertAdjacentHTML("beforeend", updateDialogShell());
  dialog = document.getElementById("update-dialog") as HTMLDialogElement;
  dialog.addEventListener("click", onClick);
  // Escape must not abandon a running update: there is nothing to return to,
  // and the process would carry on unobserved.
  dialog.addEventListener("cancel", (e) => {
    if (busy()) e.preventDefault();
  });
}

function close(): void {
  if (busy()) return;
  dialog?.close();
}

function onClick(e: Event): void {
  const target = e.target as HTMLElement | null;
  if (!target) return;
  // Clicking the backdrop reports the dialog itself as the target.
  if (target === dialog) return close();
  const action = target.closest<HTMLElement>("[data-um]")?.dataset.um;
  if (action === "close") close();
  else if (action === "install") void start();
  else if (action === "retry") {
    steps = freshSteps();
    errorKey = "";
    void start();
  }
}

// --- state → DOM ------------------------------------------------------------

function buildSteps(): TimelineStep[] {
  return UPDATE_STEPS.map((id) => ({
    id,
    label: i18n.t(`update.step.${id}`),
    status: steps[id],
    note: id === "restart" && steps[id] === "running" ? i18n.t("update.waitingForServer") : "",
  }));
}

function badgeFor(p: UpdatePhase): string {
  if (p === "error") return i18n.t("update.errorTitle");
  if (p === "running" || p === "restarting") return i18n.t("update.runningBadge");
  if (p === "done") return i18n.t("update.doneBadge");
  return i18n.t("update.badge");
}

function renderBody(): void {
  const host = document.getElementById("update-dialog-body");
  if (!host) return;
  host.innerHTML = updateModalBody({
    phase,
    currentVersion: info?.current ?? "",
    latestVersion: info?.latest ?? "",
    isDocker: info?.container === true,
    steps: buildSteps(),
    errorText: errorKey ? i18n.t(errorKey) : "",
    changelogUrl: CHANGELOG_URL,
    text: {
      title: i18n.t("update.modalTitle"),
      close: i18n.t("action.close"),
      badge: badgeFor(phase),
      install: i18n.t("update.install"),
      changelog: i18n.t("update.changelog"),
      intro: i18n.t("update.intro"),
      runningHint: i18n.t("update.runningHint"),
      restarting: i18n.t("update.restarting"),
      done: i18n.t("update.done", { version: info?.latest ?? "" }),
      retry: i18n.t("update.retry"),
      dockerTitle: i18n.t("update.dockerMode"),
      dockerBody: i18n.t("update.dockerDesc"),
    },
  });
  renderedPhase = phase;
}

/**
 * A new phase is usually a new layout; a step changing inside one is not.
 * Running and restarting draw the same thing — the timeline, no way out — so
 * between them only the dots and the sentence change: repainting the body at
 * that moment made every finished dot pop again at once, in Lastboard.
 */
function apply(): void {
  if (!dialog?.open) return;
  const sameLayout = (a: UpdatePhase, b: UpdatePhase | null) =>
    (a === "running" || a === "restarting") && (b === "running" || b === "restarting");
  if (phase !== renderedPhase && !sameLayout(phase, renderedPhase)) {
    renderBody();
  } else {
    for (const step of buildSteps()) {
      const li = dialog.querySelector(`[data-step="${step.id}"]`);
      if (li && li.className.trim() !== `tl-step is-${step.status}`) li.outerHTML = stepItem(step);
    }
    const message = dialog.querySelector("#um-message");
    const text = phase === "restarting" ? i18n.t("update.restarting") : i18n.t("update.runningHint");
    if (message && message.textContent !== text) message.textContent = text;
    renderedPhase = phase;
  }
  if (phase === "done") setTimeout(() => location.reload(), RELOAD_DELAY_MS);
}

// --- the update itself ------------------------------------------------------

interface StepEvent {
  step: string;
  status: "running" | "ok" | "skipped" | "error";
  error?: string;
}

function applyEvent(event: StepEvent): void {
  const id = event.step as StepId;
  if (!UPDATE_STEPS.includes(id)) return;
  steps = { ...steps, [id]: event.status };
  if (event.status === "error") {
    phase = "error";
    errorKey = event.error || "error.update_failed";
  } else if (id === "restart" && event.status === "running") {
    // The server announces the restart and then replaces itself: the last
    // event a successful run ever sends.
    phase = "restarting";
  }
  apply();
}

/**
 * A refusal that arrived before the update started, as the key to show. What
 * the session or the form token turns away is not the release's fault, and
 * the way out of it is to sign in again — so it says that.
 */
async function refusalKey(res: Response): Promise<string> {
  if (res.status === 401 || res.status === 403) return "update.sessionExpired";
  const body = (await res.json().catch(() => ({}))) as { error?: string };
  return body.error?.startsWith("error.") ? body.error : "error.update_failed";
}

async function start(): Promise<void> {
  if (busy()) return;
  const previous = info?.current ?? "";
  phase = "running";
  steps = freshSteps();
  errorKey = "";
  apply();

  try {
    const res = await fetch("/api/update", {
      method: "POST",
      headers: { Accept: "text/event-stream", "X-CSRF-Token": csrfToken() },
      cache: "no-store",
    });
    if (!res.ok) {
      phase = "error";
      errorKey = await refusalKey(res);
      apply();
      return;
    }
    if (res.body) {
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        // Events are separated by a blank line; a partial one waits for the rest.
        let sep = buffer.indexOf("\n\n");
        while (sep !== -1) {
          const frame = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          const payload = frame
            .split("\n")
            .filter((line) => line.startsWith("data:"))
            .map((line) => line.slice(5).trim())
            .join("");
          if (payload) {
            try {
              applyEvent(JSON.parse(payload) as StepEvent);
            } catch {
              // A malformed frame is skipped; the next one carries on.
            }
          }
          sep = buffer.indexOf("\n\n");
        }
      }
    }
  } catch {
    // The connection dropped. If the install had finished, the server is
    // restarting; otherwise nothing says it did, and waiting finds out.
  }

  // Read through a function: the events changed it while this was awaiting,
  // and the compiler still believes the value set above.
  const now = (): UpdatePhase => phase;
  if (now() === "error") return;
  if (now() !== "restarting") {
    phase = "restarting";
    steps = { ...steps, restart: "running" };
    apply();
  }
  await waitForRestart(previous);
}

/**
 * Waits until a DIFFERENT version answers. The old process exits a moment
 * after replying, so "the server responds" would go green while still talking
 * to the binary being replaced; a changed version is the one signal that the
 * new build is serving.
 */
async function waitForRestart(previous: string): Promise<void> {
  const deadline = Date.now() + RESTART_TIMEOUT_MS;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, RESTART_POLL_INTERVAL_MS));
    try {
      const res = await fetch("/healthz", { cache: "no-store" });
      const version = ((await res.json()) as { version?: string }).version ?? "";
      if (res.ok && version && version !== previous) {
        steps = { ...steps, restart: "ok" };
        phase = "done";
        apply();
        return;
      }
    } catch {
      // Refused while the process is being replaced: expected.
    }
  }
  steps = { ...steps, restart: "error" };
  phase = "error";
  errorKey = "update.restartTimeout";
  apply();
}
