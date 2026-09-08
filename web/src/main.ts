/**
 * The application shell: routing, fetching, and putting the templates on screen.
 *
 * Rendering is deliberately per-view rather than one big redraw. Reassigning the
 * whole container on every interaction destroys and recreates every child — the
 * entrance animations replay, a focused field loses focus, and a list that was
 * scrolled jumps back to the top. This house has measured that three times.
 */
import { api, ApiError } from "./api";
import type {
  ItemDetail,
  ItemSummary,
  Me,
  OrganizationSummary,
  ReportField,
  RepositorySummary,
  Status,
  Submitted,
} from "./api";
import { i18n } from "./services/i18n";
import { availableLocales } from "./locales/metadata";
import { applyTheme, currentTheme, restoreTheme, type Theme } from "./services/theme";
import { escapeHTML } from "./escape";
import { toast, toastError } from "./services/toast";
import { shrink, TooLarge } from "./services/image";
import { attachPasswordToggles } from "./services/password";
import * as views from "./views/templates";
import { icons } from "./views/icons";

const text: views.Text = {
  t: (key, params) => i18n.t(key, params),
  date: (value) => i18n.date(value),
  number: (value) => i18n.number(value),
  bytes: (value) => i18n.bytes(value),
  since: (seconds) => i18n.since(seconds),
};

const root = document.getElementById("app");
const nav = document.getElementById("nav");

let me: Me | null = null;

/**
 * What this installation calls itself.
 *
 * Read once when a panel page is drawn and kept: it is on every screen, and
 * asking the server for it on each navigation would be a request for a string
 * that changes about once in the life of an install.
 */
let instanceName = "";

/** The logo beside it, if the operator chose one. */
let instanceLogo = "";

/** The accent this installation chose, applied to the document root. */
let instanceAccent = "";

// --- routing ---------------------------------------------------------------

/** Navigate without a reload, so the language and the scroll survive. */
function go(path: string): void {
  history.pushState({}, "", path);
  void render();
}

document.addEventListener("click", (event) => {
  const link = (event.target as HTMLElement | null)?.closest?.("a");
  if (!link) return;
  const href = link.getAttribute("href");
  // The feed is served by the server, not by this router: intercepting it
  // would show a blank page where a reader expects XML.
  // The feed and the export are served by the server, not by this router:
  // intercepting either shows a blank page where a file was expected.
  if (
    !href?.startsWith("/") ||
    href.startsWith("/feed/") ||
    href.startsWith("/api/") ||
    link.hasAttribute("target")
  ) {
    return;
  }
  event.preventDefault();
  go(href);
});

window.addEventListener("popstate", () => void render());

// --- the shell -------------------------------------------------------------

function renderNav(): void {
  if (!nav) return;

  // No language picker here. A visitor who has not chosen one is already
  // reading their browser's language, and somebody who wants a different one
  // sets it once — in the wizard on the first run, or under Appearance — rather
  // than being offered a list of twenty on every page of the product.
  const account = me
    ? `<a href="/panel">${escapeHTML(i18n.t("nav.panel"))}</a>
       <button class="link" data-signout>${escapeHTML(i18n.t("nav.signOut"))}</button>`
    : // Not on the sign-in page itself: a link to where you already are is the
      // third thing on that screen saying the same word.
      location.pathname === "/sign-in"
      ? ""
      : `<a class="button small" href="/sign-in">${escapeHTML(i18n.t("nav.signIn"))}</a>`;

  // The name the operator chose, on the public side too. Personalising an
  // installation and then seeing "LastHelp" on the page everybody else opens is
  // a setting that did not take.
  // The bar's contents share the measure of the page under it. Pinned to the
  // window's own edges, the brand floated a long way from the first word of the
  // page it names, which is most of what made these screens read as unaligned.
  nav.innerHTML = `<div class="nav-inner">
      <a class="brand" href="/">
        ${views.brandMark(instanceLogo)}
        <span>${escapeHTML(instanceName || i18n.t("app.name"))}</span>
      </a>
      <div class="nav-right">${account}</div>
    </div>`;

  nav.querySelector<HTMLButtonElement>("[data-signout]")?.addEventListener("click", async () => {
    await api.del("/api/session").catch(() => undefined);
    me = null;
    go("/");
  });
}

function show(html: string): void {
  document.body.classList.remove("admin-view");
  if (root) root.innerHTML = html;
  // Every password field on whatever was just drawn, rather than a toggle
  // written into the one form somebody asked about and missing from the rest.
  attachPasswordToggles(root);
}

/**
 * Draw a panel page inside the administration shell.
 *
 * The sidebar needs the organisations and, when you are inside one, its boards,
 * so this fetches them once for every panel page rather than each screen
 * building its own navigation and drifting from the others.
 */
async function showInShell(content: string, path: string): Promise<void> {
  const { organizations } = await api
    .get<{ organizations: OrganizationSummary[] }>("/api/orgs")
    .catch(() => ({ organizations: [] as OrganizationSummary[] }));

  const orgs: views.SideOrganization[] = organizations ?? [];
  const inside = orgs.find((o) => path.startsWith(`/panel/${o.slug}`) || path.startsWith(`/${o.slug}/`));
  if (inside) {
    const body = await api
      .get<{ repositories: RepositorySummary[] }>(`/api/orgs/${encodeURIComponent(inside.slug)}/repos`)
      .catch(() => ({ repositories: [] as RepositorySummary[] }));
    inside.repos = (body.repositories ?? []).map((r) => ({ slug: r.slug, name: r.name }));
  }

  if (root) {
    root.innerHTML = views.adminShell(
      {
        path,
        organizations: orgs,
        who: me?.username ?? "",
        isAdmin: me?.admin === true,
        mayAddOrganization: me?.admin === true,
        instance: instanceName || i18n.t("app.name"),
        logo: instanceLogo,
        avatar: me?.avatar,
        content,
      },
      text,
    );
  }
  document.body.classList.add("admin-view");
  attachPasswordToggles(root);
  wireShell();
}

/**
 * The name, the mark and the colour this installation wears.
 *
 * Read once and kept: it is on every screen, public and private, and asking on
 * every navigation would be a request for three strings that change about once
 * in the life of an install.
 */
let instanceLoaded = false;

async function loadInstance(): Promise<void> {
  if (instanceLoaded) return;
  instanceLoaded = true;
  const instance = await api
    .get<{ name: string; logo: string; accent: string }>("/api/instance")
    .catch(() => null);
  if (!instance) return;
  instanceName = instance.name;
  instanceLogo = instance.logo;
  instanceAccent = instance.accent;
  applyAccent(instanceAccent);
}

/** The controls that live in the shell rather than in any one screen. */
function wireShell(): void {
  root?.querySelector<HTMLButtonElement>(".side [data-signout]")?.addEventListener("click", async () => {
    await api.del("/api/session").catch(() => undefined);
    me = null;
    instanceName = "";
    go("/");
  });
}

/**
 * A chosen image, scaled and encoded into something that fits in a row.
 *
 * It used to measure the file and refuse it, which failed twice over: no
 * photograph a phone takes is under the limit, and the number it compared was
 * the file's while the server bounds the data URL — about a third longer — so a
 * file that passed here could still be refused there.
 */
async function readAsDataURL(file: File | undefined): Promise<string | null> {
  if (!file) return null;
  try {
    // Scaled and re-encoded rather than measured and refused. Every photograph
    // a phone takes is over the limit, and telling the person setting up a
    // server to go and find a smaller picture is not a feature.
    return await shrink(file);
  } catch (err) {
    // A file this browser cannot decode — an HEIC from an iPhone, say — is not
    // a file that is too large, and saying so sends somebody off to shrink an
    // image that was never the problem.
    showFormError(new ApiError(400, err instanceof TooLarge ? "error.image_too_big" : "error.image_unreadable"));
    return null;
  }
}

/**
 * The modal plumbing, in one place.
 *
 * <dialog> already does the hard parts — focus, Escape, inert background — so
 * this is only opening, closing and the submit. Every screen wiring its own
 * would be four copies of the same three listeners, and the fourth would be the
 * one that forgot to close on cancel.
 */
/**
 * The accent an operator chose, as a variable on the root.
 *
 * A style attribute rather than a stylesheet, because the value comes from the
 * database and a stylesheet with a hole in it needs a nonce, a request and a
 * cache story. The server only ever stores six hex digits, which is why this
 * can be written into a property at all.
 */
function applyAccent(accent: string): void {
  const root = document.documentElement;
  if (!accent) {
    root.style.removeProperty("--accent");
    root.style.removeProperty("--accent-rgb");
    root.style.removeProperty("--accent-alt");
    return;
  }
  const [r, g, b] = [1, 3, 5].map((at) => parseInt(accent.slice(at, at + 2), 16));
  root.style.setProperty("--accent", accent);
  root.style.setProperty("--accent-rgb", `${r}, ${g}, ${b}`);
  root.style.setProperty("--accent-alt", accent);
}

function openModal(id: string): void {
  root?.querySelector<HTMLDialogElement>("#" + id)?.showModal();
}

function wireModalOpeners(): void {
  // One attribute opens any of them. Screens used to wire their own opener and
  // one of them forgot, which is a button that looks right and does nothing —
  // the kind of bug that only a walk through the browser finds.
  root?.querySelectorAll<HTMLElement>("[data-open-modal]").forEach((button) =>
    button.addEventListener("click", () => openModal(button.dataset.openModal ?? "")),
  );
  root?.querySelectorAll<HTMLButtonElement>("[data-close]").forEach((button) =>
    button.addEventListener("click", () => button.closest("dialog")?.close()),
  );
}

/**
 * Submit handling for a form, whether it is in a modal or on the page.
 *
 * A form inside <dialog method="dialog"> closes the dialog on submit, which is
 * why the work happens first and the close is left to the browser: closing by
 * hand as well means closing twice, and the second one throws.
 */
function onSubmit(selector: string, handler: (data: FormData) => Promise<void>): void {
  root?.querySelector<HTMLFormElement>(selector)?.addEventListener("submit", async (event) => {
    const form = event.currentTarget as HTMLFormElement;
    const data = new FormData(form);
    const dialog = form.closest("dialog");
    if (!dialog) event.preventDefault();
    try {
      await handler(data);
    } catch (err) {
      // The dialog has already closed itself by the time this runs, so the
      // message goes where the screen can show it.
      showFormError(err);
    }
  });
}

/**
 * What this account may do here, asked of its memberships rather than of a flag.
 *
 * The screens used to check `admin`, which is stricter than the rules: an owner
 * may create repositories in their organisation, rename it and store its token,
 * and the panel offered none of it. A screen that hides what the server would
 * allow is as wrong as one that offers what it would refuse — the difference is
 * only which of the two you find out about.
 */
function ownsOrganization(slug: string): boolean {
  if (me?.admin === true) return true;
  return (me?.memberships ?? []).some(
    (m) => m.role === "owner" && m.scope === "org" && m.organization === slug,
  );
}

/** Ending an organisation is the server's, not its owner's. */
function isAdministrator(): boolean {
  return me?.admin === true;
}

/** Signed in, or sent to the door. Every panel screen starts here. */
async function requirePanel(): Promise<boolean> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) {
    go("/sign-in");
    return false;
  }
  return true;
}

async function showProfile(): Promise<void> {
  if (!(await requirePanel())) return;
  await showInShell(
    views.profileScreen(
      { username: me?.username ?? "", name: me?.name, avatar: me?.avatar, admin: me?.admin === true },
      text,
    ),
    "/panel/account",
  );

  // The picture, which is read in the browser and sent as a data URL. A file
  // upload would need a store, a route to serve it from and a second thing to
  // back up; one small image belongs in the row beside the name.
  const avatarFile = root?.querySelector<HTMLInputElement>("#avatar-file");
  root?.querySelector<HTMLButtonElement>("[data-pick-avatar]")?.addEventListener("click", () => avatarFile?.click());
  avatarFile?.addEventListener("change", async () => {
    const picture = await readAsDataURL(avatarFile.files?.[0]);
    if (picture === null) return;
    try {
      await api.post("/api/me/avatar", { avatar: picture });
      me = null;
      await render();
    } catch (err) {
      showFormError(err);
    }
  });
  root?.querySelector<HTMLButtonElement>("[data-clear-avatar]")?.addEventListener("click", async () => {
    await api.post("/api/me/avatar", { avatar: "" }).catch(() => undefined);
    me = null;
    await render();
  });

  root?.querySelector<HTMLFormElement>("form.profile-name")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    try {
      await api.post("/api/me/profile", { display_name: String(data.get("display_name") ?? "") });
      await render();
    } catch (err) {
      showFormError(err);
    }
  });

  root?.querySelector<HTMLFormElement>("form.profile-password")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    const next = String(data.get("new") ?? "");
    if (next !== String(data.get("again") ?? "")) {
      showFormError(new ApiError(400, "error.password_mismatch"));
      return;
    }
    try {
      await api.post("/api/me/password", { current: String(data.get("current") ?? ""), new: next });
      await render();
    } catch (err) {
      showFormError(err);
    }
  });
}

async function showAppearance(): Promise<void> {
  if (!(await requirePanel())) return;
  await showInShell(
    views.appearanceScreen(
      availableLocales.map((l) => ({ code: l.code, name: l.name })),
      i18n.current,
      currentTheme(),
      text,
    ),
    "/panel/appearance",
  );

  root?.querySelectorAll<HTMLButtonElement>("[data-theme-choice]").forEach((button) =>
    button.addEventListener("click", () => {
      applyTheme((button.dataset.themeChoice ?? "system") as Theme);
      void render();
    }),
  );
  root?.querySelector<HTMLSelectElement>("#appearance-language")?.addEventListener("change", (e) => {
    const code = (e.target as HTMLSelectElement).value;
    void i18n.setLanguage(code).then(async () => {
      // On the account as well as in this browser, so it is there on the next
      // machine. Failing quietly: the language has already changed on screen,
      // and a toast about a preference nobody asked to save would be noise.
      if (me) await api.post("/api/me/language", { language: code }).catch(() => undefined);
      await render();
    });
  });
}

async function showPeople(): Promise<void> {
  if (!(await requirePanel())) return;
  const body = await api
    .get<{ people: Parameters<typeof views.peopleScreen>[0] }>("/api/people")
    .catch(() => null);
  if (!body) {
    await showInShell(views.emptyState(i18n.t("error.forbidden"), icons.warning(28)), "/panel/people");
    return;
  }

  // Every place a role can be granted in, named the way the rest of the API
  // names them. Fetched once for the screen rather than per person.
  const { organizations } = await api
    .get<{ organizations: OrganizationSummary[] }>("/api/orgs")
    .catch(() => ({ organizations: [] as OrganizationSummary[] }));
  const places: { organization: string; repository?: string; label: string }[] = [];
  for (const org of organizations ?? []) {
    places.push({ organization: org.slug, label: org.name });
    const repos = await api
      .get<{ repositories: RepositorySummary[] }>(`/api/orgs/${encodeURIComponent(org.slug)}/repos`)
      .catch(() => ({ repositories: [] as RepositorySummary[] }));
    for (const repo of repos.repositories ?? []) {
      places.push({ organization: org.slug, repository: repo.slug, label: `${org.name} / ${repo.name}` });
    }
  }

  await showInShell(
    views.peopleScreen(body.people ?? [], text) +
      views.modal(
        {
          id: "person-modal",
          title: text.t("people.add"),
          lead: text.t("people.addHelp"),
          body:
            views.field({ id: "person-username", name: "username", label: text.t("auth.username"), required: true, example: "ana" }, text) +
            views.field({ id: "person-name", name: "display_name", label: text.t("profile.name"), example: "Ana" }, text) +
            views.field({ id: "person-password", name: "password", label: text.t("auth.password"), type: "password", required: true, hint: text.t("auth.passwordHelp") }, text) +
            // Chosen here rather than left to whatever their browser asks for:
            // otherwise somebody's first sign-in lands in a language they did
            // not pick, and the setting to change it is in that language.
            views.choiceField({
              id: "person-language",
              name: "language",
              label: text.t("people.language"),
              hint: text.t("people.languageHelp"),
              options: availableLocales.map((l) => [l.code, l.name] as [string, string]),
              selected: "en",
            }),
          submit: text.t("auth.create"),
          formClass: "person-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "person-roles",
          title: text.t("people.roles"),
          lead: text.t("people.rolesHelp"),
          body: "",
          submit: text.t("people.grant"),
          formClass: "person-roles-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "person-edit",
          title: text.t("people.edit"),
          lead: text.t("people.editHelp"),
          body:
            views.field({ id: "edit-name", name: "display_name", label: text.t("profile.name") }, text) +
            views.field({ id: "edit-password", name: "password", label: text.t("people.newPassword"), type: "password", hint: text.t("people.newPasswordHelp") }, text),
          submit: text.t("action.save"),
          formClass: "person-edit-form",
        },
        text,
      ),
    "/panel/people",
  );

  wireModalOpeners();
  onSubmit("form.person-form", async (data) => {
    await api.post("/api/people", {
      username: String(data.get("username") ?? ""),
      display_name: String(data.get("display_name") ?? ""),
      password: String(data.get("password") ?? ""),
      language: String(data.get("language") ?? ""),
    });
    toast(i18n.t("toast.personAdded"));
    await render();
  });

  let editing = 0;
  root?.querySelectorAll<HTMLButtonElement>("[data-edit-person]").forEach((button) =>
    button.addEventListener("click", () => {
      editing = Number(button.dataset.editPerson ?? 0);
      const name = root?.querySelector<HTMLInputElement>("#edit-name");
      if (name) name.value = button.dataset.name ?? "";
      const pass = root?.querySelector<HTMLInputElement>("#edit-password");
      if (pass) pass.value = "";
      openModal("person-edit");
    }),
  );
  onSubmit("form.person-edit-form", async (data) => {
    const payload: Record<string, string> = {};
    const name = String(data.get("display_name") ?? "");
    const pass = String(data.get("password") ?? "");
    if (name) payload.display_name = name;
    if (pass) payload.password = pass;
    await api.patch(`/api/people/${editing}`, payload);
    await render();
  });

  // Roles, in a modal of their own. Cramming them into the edit form would put
  // "change this person's name" and "let this person into an organisation" one
  // above the other, which is two different weights of decision on one button.
  let granting = 0;
  root?.querySelectorAll<HTMLButtonElement>("[data-roles-person]").forEach((button) =>
    button.addEventListener("click", () => {
      granting = Number(button.dataset.rolesPerson ?? 0);
      const person = (body.people ?? []).find((p) => p.id === granting);
      const holder = root?.querySelector<HTMLDivElement>("#person-roles .modal-body");
      if (holder && person) holder.innerHTML = views.rolesModalBody(person.grants, places, text);
      wireRevoke();
      openModal("person-roles");
    }),
  );

  const wireRevoke = () => {
    root?.querySelectorAll<HTMLButtonElement>("[data-revoke]").forEach((button) =>
      button.addEventListener("click", async () => {
        try {
          await api.post(`/api/people/${granting}/grants`, {
            organization: button.dataset.organization ?? "",
            repository: button.dataset.repository ?? "",
            role: "",
          });
          await render();
        } catch (err) {
          showFormError(err);
        }
      }),
    );
  };

  onSubmit("form.person-roles-form", async (data) => {
    const [organization, repository] = String(data.get("where") ?? "").split("/");
    await api.post(`/api/people/${granting}/grants`, {
      organization: organization ?? "",
      repository: repository ?? "",
      role: String(data.get("role") ?? ""),
    });
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-person]").forEach((button) =>
    button.addEventListener("click", async () => {
      if (!confirm(i18n.t("people.confirmDelete", { name: button.dataset.name ?? "" }))) return;
      try {
        await api.del(`/api/people/${button.dataset.deletePerson}`);
        await render();
      } catch (err) {
        showFormError(err);
      }
    }),
  );
}

async function showBackups(): Promise<void> {
  if (!(await requirePanel())) return;
  await showInShell(views.backupScreen(text), "/panel/backups");

  const file = root?.querySelector<HTMLInputElement>("#restore-file");
  const note = root?.querySelector<HTMLParagraphElement>(".restore-note");
  root?.querySelector<HTMLButtonElement>("[data-pick-restore]")?.addEventListener("click", () => file?.click());

  file?.addEventListener("change", async () => {
    const chosen = file.files?.[0];
    if (!chosen) return;
    // Asked in words rather than with a second button: this replaces every
    // report anybody has written since the backup was taken, and there is no
    // undo that is not another restore.
    if (!confirm(i18n.t("backups.confirmRestore", { file: chosen.name }))) {
      file.value = "";
      return;
    }

    const form = new FormData();
    form.set("backup", chosen);
    if (note) {
      note.textContent = i18n.t("backups.restoring");
      note.hidden = false;
    }
    try {
      const done = await api.upload<{ kept: string }>("/api/restore", form);
      // This one keeps its line as well as toasting: it ends with "restart the
      // server", which is an instruction, and an instruction that flies away
      // after three seconds is one somebody will ask for again.
      if (note) note.textContent = i18n.t("backups.restored", { kept: done.kept });
      toast(i18n.t("toast.restored"));
    } catch (err) {
      if (note) note.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
      report(err);
    }
    file.value = "";
  });
}

async function showAbout(): Promise<void> {
  if (!(await requirePanel())) return;
  // Asked of the server rather than assembled from what the page happens to
  // know: uptime, disk and memory are facts about the machine, and the browser
  // has none of them.
  const facts = await api.get<views.ServerFacts>("/api/server").catch(() => ({}) as views.ServerFacts);
  await showInShell(
    views.aboutScreen(facts, text),
    "/panel/about",
  );
}

/** A page that could not be loaded. The screen has nothing else to show. */
function fail(err: unknown): void {
  const key = err instanceof ApiError ? err.key : "error.internal";
  show(views.emptyState(i18n.t(key), icons.warning(28)));
}

/**
 * An action that did not work, said without taking the page away.
 *
 * This used to be `fail`, which replaces the whole screen: a status change that
 * hit a network error left somebody looking at one grey sentence where their
 * report had been, with everything they were reading gone.
 */
function report(err: unknown): void {
  toastError(i18n.t(err instanceof ApiError ? err.key : "error.internal"));
}

// --- the views -------------------------------------------------------------

async function render(): Promise<void> {
  restoreTheme();
  await i18n.ready;
  await loadInstance();
  renderNav();

  const path = location.pathname.replace(/\/+$/, "") || "/";
  const parts = path.split("/").filter(Boolean);

  try {
    if (path === "/") return await showOrganizations();
    if (path === "/sign-in") return await showSignIn();
    if (path === "/setup") {
      // English, whatever the browser asks for. The first screen of the
      // installation is where a language is chosen rather than inferred, and
      // it is the language of this account and no one else's — everybody who
      // arrives later reads English until they choose otherwise.
      await startInEnglish();
      return showSetupStep(1);
    }
    if (path === "/setup/account") return showSetupStep(2);
    if (path === "/setup/instance") return showSetupStep(3);
    if (path === "/setup/done") return showSetupDone();
    if (path === "/panel") return await showPanel();
    if (path === "/panel/reports") return await showReports();
    if (path === "/panel/updates") return await showUpdates();
    if (path === "/panel/account") return await showProfile();
    if (path === "/panel/appearance") return await showAppearance();
    if (path === "/panel/people") return await showPeople();
    if (path === "/panel/organizations") return await showOrganizationsPanel();
    if (path === "/panel/personalization") return await showPersonalization();
    if (path === "/panel/backups") return await showBackups();
    if (path === "/panel/about") return await showAbout();
    // Before the generic /panel/{org}: "new" is a screen, not an organisation
    // anybody would be able to open.
    if (path === "/panel/new") return await showOrganizationWizard(1);
    if (path === "/panel/new/connection") return await showOrganizationWizard(2);
    if (parts[0] === "panel" && parts[2] === "new" && parts[1]) {
      return await showRepositoryWizard(parts[1], 1);
    }
    if (parts[0] === "panel" && parts[2] === "accepts" && parts[1]) {
      return await showRepositoryWizard(parts[1], 2);
    }
    if (parts[0] === "panel" && parts[1] && parts.length === 2) {
      return await showPanelOrganization(parts[1]);
    }
    if (parts[0] === "i" && parts[1]) return await showItem(parts[1]);
    if (parts.length === 1 && parts[0]) return await showRepositories(parts[0]);
    if (parts.length === 2 && parts[0] && parts[1]) return await showBoard(parts[0], parts[1]);
    if (parts.length === 3 && parts[2] === "new" && parts[0] && parts[1]) {
      return await showReportForm(parts[0], parts[1]);
    }
    if (parts.length === 3 && parts[2] === "queue" && parts[0] && parts[1]) {
      return await showQueue(parts[0], parts[1]);
    }
    if (parts.length === 3 && parts[2] === "roadmap" && parts[0] && parts[1]) {
      return await showRoadmap(parts[0], parts[1]);
    }
    show(views.emptyState(i18n.t("error.not_found"), icons.warning(28)));
  } catch (err) {
    fail(err);
  }
}

async function showOrganizations(): Promise<void> {
  // A brand new installation opens on the wizard, not on a landing that hides
  // it behind "sign in". Whoever reaches an unclaimed instance first is the
  // person who is going to set it up, and asking them to find the door is how
  // the first minute goes wrong.
  const status = await api.get<Status>("/api/status").catch(() => null);
  if (status?.needs_setup) return go("/setup");

  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");
  // Who is looking decides what an empty instance says. An administrator gets
  // the way out of it; a visitor gets told plainly, rather than being sent to a
  // form that would refuse them.
  if (!me) me = await api.get<Me>("/api/me").catch(() => null);
  show(
    views.landing(organizations ?? [], me?.admin === true, text, {
      name: instanceName,
      logo: instanceLogo,
    }),
  );
}

async function showRepositories(orgSlug: string): Promise<void> {
  const body = await api.get<{
    organization: OrganizationSummary;
    repositories: RepositorySummary[];
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  show(views.repositoryList(body.organization, body.repositories ?? [], text));
}

const filters = { kind: "", sort: "new", query: "" };

async function showBoard(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);

  const load = async (): Promise<ItemSummary[]> => {
    const query = new URLSearchParams();
    if (filters.kind) query.set("kind", filters.kind);
    if (filters.sort === "votes") query.set("sort", "votes");
    if (filters.query) query.set("q", filters.query);
    const body = await api.get<{ items: ItemSummary[] }>(`${base}/items?${query}`);
    return body.items ?? [];
  };

  const draw = async () => {
    show(views.board(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, await load(), filters, text));
    wire();
  };

  const wire = () => {
    root?.querySelectorAll<HTMLButtonElement>(".tab").forEach((b) =>
      b.addEventListener("click", () => {
        filters.kind = b.dataset.kind ?? "";
        void draw();
      }),
    );
    root?.querySelectorAll<HTMLButtonElement>(".sort").forEach((b) =>
      b.addEventListener("click", () => {
        filters.sort = b.dataset.sort ?? "new";
        void draw();
      }),
    );
    const search = root?.querySelector<HTMLInputElement>(".search");
    search?.addEventListener("input", debounce(() => {
      filters.query = search.value;
      void draw().then(() => root?.querySelector<HTMLInputElement>(".search")?.focus());
    }, 250));
  };

  await draw();
}

async function showReportForm(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);

  let kind = repo.accepts_bugs ? "bug" : "idea";

  // Fetching the form is what separates somebody who opened the page from a
  // script that found the endpoint. The token it returns is bound to this
  // board, this moment and this part of the internet, and is spent once.
  const ticket = await api
    .get<{ token: string; accepting: boolean }>(`${base}/form`)
    .catch(() => ({ token: "", accepting: true }));

  if (!ticket.accepting) {
    show(`<p class="notice">${escapeHTML(i18n.t("error.board_closed"))}</p>`);
    return;
  }

  show(views.reportForm(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, kind, text));

  const form = root?.querySelector<HTMLFormElement>("form.report");
  const similar = root?.querySelector<HTMLDivElement>(".similar");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

  root?.querySelectorAll<HTMLButtonElement>(".choice-card").forEach((button) =>
    button.addEventListener("click", () => {
      kind = button.dataset.choice ?? kind;
      root?.querySelectorAll(".choice-card").forEach((b) => {
        b.classList.toggle("on", b === button);
        b.setAttribute("aria-pressed", b === button ? "true" : "false");
      });
      // The questions that belong to this kind, shown and hidden rather than
      // drawn and thrown away: rebuilding the block would lose whatever had
      // already been typed into it.
      root?.querySelectorAll<HTMLElement>(".kind-fields").forEach((block) => {
        block.hidden = block.dataset.for !== kind;
      });
    }),
  );

  const title = form?.querySelector<HTMLInputElement>('[name="title"]');
  title?.addEventListener(
    "input",
    debounce(async () => {
      const value = title.value.trim();
      if (value.length < 4 || !similar) {
        if (similar) similar.hidden = true;
        return;
      }
      const found = await api
        .get<{ similar: Array<{ public_id: string; title: string }> }>(
          `${base}/similar?title=${encodeURIComponent(value)}`,
        )
        .catch(() => ({ similar: [] }));
      similar.innerHTML = views.similarItems(found.similar ?? [], text);
      similar.hidden = (found.similar ?? []).length === 0;
    }, 300),
  );

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (errorLine) errorLine.hidden = true;

    const data = new FormData(form);
    try {
      const sent = await api.post<Submitted>(`${base}/items`, {
        kind,
        title: String(data.get("title") ?? ""),
        body: String(data.get("body") ?? ""),
        email: String(data.get("email") ?? ""),
        website: String(data.get("website") ?? ""),
        // The answers to the questions this kind was asked. Sent whatever the
        // kind is: the server keeps the ones that belong to it and drops the
        // rest, so somebody who typed steps and then changed their mind and
        // called it an idea does not send them.
        fields: {
          version: String(data.get("version") ?? ""),
          steps: String(data.get("steps") ?? ""),
        },
        token: ticket.token,
      });
      // The address to keep, and the path to walk. The first is absolute
      // because it is going into somebody's notes; the second is what this
      // page's own button follows, and the router only takes a path.
      const path = `/i/${sent.public_id}?k=${sent.follow_token}`;
      const link = `${location.origin}${path}`;
      show(
        views.submitted(
          {
            link,
            path,
            org: orgSlug,
            repo: repoSlug,
            board: repo.name,
          },
          text,
        ),
      );
      root?.querySelector<HTMLButtonElement>("[data-copy]")?.addEventListener("click", async (e) => {
        await navigator.clipboard?.writeText(link).catch(() => undefined);
        (e.currentTarget as HTMLButtonElement).textContent = i18n.t("sent.copied");
      });
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

async function showRoadmap(orgSlug: string, repoSlug: string): Promise<void> {
  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const repo = await api.get<RepositorySummary>(base);
  const board = await api.get<{ columns: Array<{ column: string; items: ItemSummary[] }> }>(
    `${base}/roadmap`,
  );
  show(views.roadmap(repo.organization ?? { slug: orgSlug, name: orgSlug }, repo, board.columns ?? [], text));
}

async function showItem(publicID: string): Promise<void> {
  const token = new URLSearchParams(location.search).get("k");
  const path = `/api/items/${encodeURIComponent(publicID)}${token ? `?k=${encodeURIComponent(token)}` : ""}`;
  const item = await api.get<ItemDetail>(path);

  // Somebody with a role keeps their panel around the report.
  //
  // The address is the public one — this is the page that travels, and a
  // stranger opening it must not be shown an application — but for the team it
  // is where the work is done, and arriving at it from the listing used to drop
  // them out of the panel entirely: no navigation, no way back to what they
  // were working through except the browser's own button.
  const staff = me !== null && (me.admin === true || (me.memberships ?? []).length > 0);
  if (staff) {
    // Under Reports in the sidebar, because that is the screen this was
    // reached from and the one to go back to.
    await showInShell(views.itemView(item, text), "/panel/reports");
  } else {
    show(views.itemView(item, text));
  }

  root?.querySelector<HTMLButtonElement>("[data-vote]")?.addEventListener("click", async (event) => {
    const button = event.currentTarget as HTMLButtonElement;
    const voted = button.classList.contains("voted");
    const result = await api[voted ? "del" : "post"]<{ votes: number; voted: boolean }>(
      `/api/items/${encodeURIComponent(publicID)}/votes`,
    );
    // Only the button is redrawn. Replacing the page would scroll the thread
    // back to the top under whoever was reading it.
    button.classList.toggle("voted", result.voted);
    button.setAttribute("aria-pressed", result.voted ? "true" : "false");
    const label = button.querySelector(".label");
    if (label) label.textContent = result.voted ? i18n.t("item.voted") : i18n.t("item.vote");
    const count = button.querySelector(".count");
    if (count) count.textContent = i18n.number(result.votes);
  });

  // The reporter's way back to their own report. The address is this page's,
  // token and all, and it is filled in here rather than in the template
  // because the template is a pure function and does not know where it is.
  const follow = root?.querySelector<HTMLInputElement>("[data-follow-link]");
  if (follow) {
    follow.value = location.href;
    root?.querySelector<HTMLButtonElement>(".yours [data-copy]")?.addEventListener("click", async (event) => {
      await navigator.clipboard?.writeText(follow.value).catch(() => undefined);
      const span = (event.currentTarget as HTMLElement).querySelector("span");
      if (span) span.textContent = i18n.t("sent.copied");
    });
  }

  // The team's control, and only if the server says this person may use it.
  // Asking is the check; drawing the select is not. A page that decides for
  // itself who may act is a second implementation of the rules.
  if (me) {
    const allowed = await api
      .get<{ current: string; statuses: string[] }>(`/api/items/${encodeURIComponent(publicID)}/statuses`)
      .catch(() => null);
    if (allowed) {
      const tools = root?.querySelector<HTMLElement>(".team-tools");
      if (tools) tools.hidden = false;
      tools
        ?.querySelector(".tools")
        ?.insertAdjacentHTML("beforeend", views.statusControl(allowed.current, allowed.statuses, text));

      root?.querySelector<HTMLSelectElement>("#status-select")?.addEventListener("change", async (e) => {
        const select = e.target as HTMLSelectElement;
        await api
          .post(`/api/items/${encodeURIComponent(publicID)}/status`, { status: select.value })
          .then(() => toast(i18n.t("toast.statusChanged")))
          .catch((err) => report(err));
      });

      wireIssueControl(publicID, item.issue ?? null);
      wireTriage(publicID);
    }
  }

  const commentForm = root?.querySelector<HTMLFormElement>("form.comment-form");
  commentForm?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(commentForm);
    await api
      .post(`/api/items/${encodeURIComponent(publicID)}/comments`, {
        body: String(data.get("body") ?? ""),
        token: token ?? "",
      })
      .catch(() => undefined);
    commentForm.reset();
    const sent = commentForm.querySelector<HTMLParagraphElement>(".comment-sent");
    if (sent) sent.hidden = false;
  });
}

/** Storing the forge token, and showing where the forge should deliver. */
function wireCredentials(orgSlug: string): void {
  const form = root?.querySelector<HTMLFormElement>("form.credentials");
  if (!form) return;

  const errorLine = form.querySelector<HTMLParagraphElement>(".form-error");

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      await api.post(`/api/orgs/${encodeURIComponent(orgSlug)}/credentials`, {
        token: String(data.get("token") ?? ""),
        webhook_secret: String(data.get("webhook_secret") ?? ""),
      });
      form.reset();
      // The address has a card of its own, and re-rendering fills it in. It
      // used to be printed at the foot of this form as well, which meant the
      // same value in two places on one screen.
      await render();
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

/** Choosing where a board's team is told something arrived. */
function wireChannel(orgSlug: string): void {
  const form = root?.querySelector<HTMLFormElement>("form.channel");
  if (!form) return;

  const kind = form.querySelector<HTMLSelectElement>("#chan-kind");
  const fields = form.querySelector<HTMLDivElement>(".channel-fields");
  const mail = form.querySelector<HTMLDivElement>(".channel-mail");
  const errorLine = form.querySelector<HTMLParagraphElement>(".form-error");

  // Only the fields the chosen channel needs. Asking for a URL for an email
  // channel is a form arguing with the person filling it in.
  const sync = () => {
    const value = kind?.value ?? "";
    if (fields) fields.hidden = value === "" || value === "smtp";
    if (mail) mail.hidden = value !== "smtp";
  };
  kind?.addEventListener("change", sync);
  sync();

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const repo = String(data.get("repo") ?? "");
    try {
      await api.post(
        `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repo)}/channel`,
        {
          kind: String(data.get("kind") ?? ""),
          url: String(data.get("url") ?? ""),
          token: String(data.get("token") ?? ""),
          to: String(data.get("to") ?? ""),
        },
      );
      toast(i18n.t("panel.saved"));
    } catch (err) {
      // The failure stays beside the form as well as toasting: it is about
      // what was typed, and the fix is in the fields somebody is looking at.
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
      report(err);
    }
  });
}

/**
 * The bridge control, and the only place the panel talks to a forge.
 *
 * Creating an issue answers "queued", not "created": the request does not call
 * the forge — forges rate-limit and forges go down — so saying it was created
 * would be reporting something that has not happened yet.
 */
function wireIssueControl(publicID: string, linked: { number: number; url: string } | null): void {
  root
    ?.querySelector(".team-tools .tools")
    ?.insertAdjacentHTML("beforeend", views.issueControl(linked, text));

  root?.querySelector<HTMLButtonElement>("[data-create-issue]")?.addEventListener("click", async () => {
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/issue`);
      toast(i18n.t("panel.issueQueued"));
    } catch (err) {
      report(err);
    }
  });

  const attach = root?.querySelector<HTMLFormElement>("form.attach");
  attach?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const value = String(new FormData(attach).get("issue") ?? "");
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/attach`, { issue: value });
      toast(i18n.t("toast.issueAttached"));
      await render();
    } catch (err) {
      report(err);
    }
  });
}

/** Replying as the team, and merging a duplicate away. */
function wireTriage(publicID: string): void {
  root?.querySelector(".team-tools .tools")?.insertAdjacentHTML("beforeend", views.triageControls(text));

  const reply = root?.querySelector<HTMLFormElement>("form.team-reply");
  reply?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(reply);
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/reply`, {
        body: String(data.get("body") ?? ""),
      });
      reply.reset();
      toast(i18n.t("toast.replied"));
      await render();
    } catch (err) {
      report(err);
    }
  });

  const merge = root?.querySelector<HTMLFormElement>("form.merge");
  merge?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(merge);
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/merge`, {
        into: String(data.get("into") ?? ""),
      });
      // The merged item now redirects, so the page it lands on is the survivor.
      toast(i18n.t("toast.merged"));
      await render();
    } catch (err) {
      report(err);
    }
  });
}

/**
 * What the wizard is carrying between steps.
 *
 * In memory on purpose: a half-finished organisation is not something to leave
 * in a database, and a reload landing on step three of a form whose first two
 * steps no longer exist is worse than starting again.
 */
const draft: { forge: string; orgSlug: string; repoSlug: string; picture: string; logo: string } = {
  forge: "none",
  orgSlug: "",
  repoSlug: "",
  // Chosen before the account exists, so held here and sent the moment it does.
  picture: "",
  logo: "",
};

/** The error line under a wizard card, which every step has one of. */
function showFormError(err: unknown): void {
  const line = root?.querySelector<HTMLParagraphElement>(".form-error");
  if (!line) return;
  line.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
  line.hidden = false;
}

/**
 * The forge fields appear only when there is a forge, and their examples change
 * with it: an owner on GitHub and an owner on a self-hosted Forgejo look
 * nothing alike, and a single generic example teaches neither.
 */
function wireForgeChoice(rerender: (forge: string) => void): void {
  const select = root?.querySelector<HTMLSelectElement>("#org-forge");
  select?.addEventListener("change", () => {
    draft.forge = select.value;
    rerender(select.value);
  });
}

function showSetupStep(step: 1 | 2 | 3): void {
  show(
    views.setupWizard(step, text, {
      languages: availableLocales.map((l) => ({ code: l.code, name: l.name })),
      language: i18n.current,
      theme: currentTheme(),
      picture: draft.picture,
      logo: draft.logo,
    }),
  );

  if (step === 1) {
    // Applied as they are chosen, not on submit: a language you cannot read is
    // a poor way to ask whether you can read it.
    root?.querySelectorAll<HTMLButtonElement>("[data-theme-choice]").forEach((button) =>
      button.addEventListener("click", () => {
        applyTheme((button.dataset.themeChoice ?? "system") as Theme);
        void render();
      }),
    );
    root?.querySelector<HTMLSelectElement>("#setup-language")?.addEventListener("change", (e) => {
      void i18n.setLanguage((e.target as HTMLSelectElement).value).then(() => void render());
    });
    root?.querySelector<HTMLFormElement>("form.setup-preferences")?.addEventListener("submit", (event) => {
      event.preventDefault();
      go("/setup/account");
    });
    return;
  }

  if (step === 2) {
    const avatarFile = root?.querySelector<HTMLInputElement>("#avatar-file");
    root?.querySelector<HTMLButtonElement>("[data-pick-avatar]")?.addEventListener("click", () => avatarFile?.click());
    avatarFile?.addEventListener("change", async () => {
      const picked = await readAsDataURL(avatarFile.files?.[0]);
      if (picked === null) return;
      // Kept until the account exists: there is nobody to attach it to yet.
      draft.picture = picked;
      showSetupStep(2);
    });

    root?.querySelector<HTMLFormElement>("form.setup-account")?.addEventListener(
      "submit",
      async (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget as HTMLFormElement);
        const password = String(data.get("password") ?? "");
        if (password !== String(data.get("password_again") ?? "")) {
          // Caught here rather than at the server, which never sees the second
          // box: this is the one mistake that would lock somebody out of an
          // instance with no reset to fall back on.
          showFormError(new ApiError(400, "error.password_mismatch"));
          return;
        }
        try {
          await api.post("/api/setup", { username: String(data.get("username") ?? ""), password });
          if (draft.picture) await api.post("/api/me/avatar", { avatar: draft.picture }).catch(() => undefined);
          me = await api.get<Me>("/api/me");
          go("/setup/instance");
        } catch (err) {
          showFormError(err);
        }
      },
    );
    return;
  }

  const logoFile = root?.querySelector<HTMLInputElement>("#logo-file");
  root?.querySelector<HTMLButtonElement>("[data-pick-logo]")?.addEventListener("click", () => logoFile?.click());
  logoFile?.addEventListener("change", async () => {
    const picked = await readAsDataURL(logoFile.files?.[0]);
    if (picked === null) return;
    draft.logo = picked;
    showSetupStep(3);
  });

  root?.querySelector<HTMLFormElement>("form.setup-instance")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      try {
        await api.post("/api/instance", {
          name: String(data.get("name") ?? ""),
          logo: draft.logo,
          accent: "",
        });
        // Read again, the way every other screen that writes this does. It is
        // held for the life of the page, so without this the bar and the front
        // page went on saying LastHelp for the whole session after somebody had
        // just named their installation — a setting that did not take.
        instanceLoaded = false;
        go("/setup/done");
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

function showSetupDone(): void {
  show(views.setupDone(text));
}

async function showOrganizationWizard(step: 1 | 2): Promise<void> {
  await showInShell(views.organizationWizard(step, text, draft.forge), "/panel/new");

  if (step === 1) {
    wireForgeChoice((forge) => {
      draft.forge = forge;
      showOrganizationWizard(1);
    });
  }

  root?.querySelector<HTMLFormElement>("form.setup-organization")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      draft.forge = String(data.get("forge") ?? "none");
      draft.orgSlug = String(data.get("slug") ?? "");
      try {
        await api.post("/api/organizations", {
          name: String(data.get("name") ?? ""),
          slug: draft.orgSlug,
          forge: draft.forge,
          base_url: String(data.get("base_url") ?? ""),
          forge_owner: String(data.get("forge_owner") ?? ""),
        });
        go(draft.forge === "none" ? `/panel/${draft.orgSlug}` : "/panel/new/connection");
      } catch (err) {
        showFormError(err);
      }
    },
  );

  root?.querySelector<HTMLFormElement>("form.setup-token")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      const token = String(data.get("token") ?? "");
      try {
        if (token) {
          await api.post(`/api/orgs/${encodeURIComponent(draft.orgSlug)}/credentials`, { token });
        }
        go(`/panel/${draft.orgSlug}`);
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

async function showRepositoryWizard(orgSlug: string, step: 1 | 2): Promise<void> {
  const body = await api.get<{
    organization: OrganizationSummary & { forge?: string };
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  draft.orgSlug = orgSlug;
  await showInShell(views.repositoryWizard(body.organization, step, text), `/panel/${orgSlug}`);

  root?.querySelector<HTMLFormElement>("form.setup-repository-identity")?.addEventListener(
    "submit",
    (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      draft.repoSlug = String(data.get("slug") ?? "");
      pendingRepository = {
        name: String(data.get("name") ?? ""),
        slug: draft.repoSlug,
        forge_repo: String(data.get("forge_repo") ?? ""),
      };
      go(`/panel/${orgSlug}/accepts`);
    },
  );

  root?.querySelector<HTMLFormElement>("form.setup-repository-accepts")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      try {
        await api.post(`/api/orgs/${encodeURIComponent(orgSlug)}/repositories`, {
          ...pendingRepository,
          accepts_bugs: data.get("accepts_bugs") !== null,
          accepts_ideas: data.get("accepts_ideas") !== null,
        });
        go(`/panel/${orgSlug}`);
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

let pendingRepository: { name: string; slug: string; forge_repo: string } = {
  name: "",
  slug: "",
  forge_repo: "",
};

async function showSignIn(): Promise<void> {
  const status = await api.get<Status>("/api/status");
  // A fresh instance is claimed through the wizard, which starts by asking
  // which language to say the rest of it in.
  if (status.needs_setup) return go("/setup");
  show(views.signInForm(text, { name: instanceName, logo: instanceLogo }));

  const form = root?.querySelector<HTMLFormElement>("form.auth");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    // `username`, which is what the field is called. It read `email` — the name
    // from before the rename — so every sign-in sent an empty username and was
    // told one was needed, on a form where it was plainly filled in. The two
    // sides of a rename, and only one of them moved.
    const payload = {
      username: String(data.get("username") ?? ""),
      password: String(data.get("password") ?? ""),
    };
    try {
      // Not /api/setup: a fresh instance was sent to the wizard above, so that
      // branch could never be taken. A branch nothing can reach is a branch
      // nobody tests.
      await api.post("/api/session", payload);
      me = await api.get<Me>("/api/me");
      go("/panel");
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

async function showPanel(): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");
  // Who is signed in changed, so the nav has to be redrawn: it was rendered
  // before this ran and still says "sign in".
  renderNav();

  const data = await api
    .get<Parameters<typeof views.overviewScreen>[0]>("/api/overview")
    .catch(() => null);

  if (!data) {
    // Not an administrator. The numbers count the whole installation, which is
    // not theirs to see; the places they were given a role in are.
    await showInShell(views.memberHome(me?.memberships ?? [], text), "/panel");
    return;
  }
  await showInShell(views.overviewScreen(data, text), "/panel");
}

/**
 * The panel's own listing of reports.
 *
 * The filters live in the address rather than in a variable: a view somebody
 * has narrowed down is then something they can bookmark, reload and send to
 * whoever else looks after that board. Every control writes the address and
 * the screen redraws from it, so there is one source of truth for what is on
 * screen instead of two that drift.
 */
async function showReports(): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const query = new URLSearchParams(location.search);
  const filters: views.ReportFilters = {
    moderation: query.get("moderation") ?? "",
    kind: query.get("kind") ?? "",
    status: query.get("status") ?? "",
    board: query.get("board") ?? "",
    sort: query.get("sort") ?? "",
    q: query.get("q") ?? "",
  };

  const data = await api
    .get<views.ReportsData>(`/api/reports?${new URLSearchParams({ ...filters }).toString()}`)
    .catch(() => null);
  if (!data) {
    await showInShell(views.emptyState(i18n.t("error.forbidden"), icons.warning(28)), "/panel/reports");
    return;
  }

  await showInShell(views.reportsScreen(data, filters, text), "/panel/reports");

  // One place that turns "what is on screen" into an address, so a control
  // added later cannot forget half of it.
  const narrow = (change: Partial<views.ReportFilters>) => {
    const next = new URLSearchParams();
    for (const [key, value] of Object.entries({ ...filters, ...change })) {
      if (value) next.set(key, String(value));
    }
    const search = next.toString();
    go(`/panel/reports${search ? `?${search}` : ""}`);
  };

  for (const attr of ["moderation", "kind"] as const) {
    root?.querySelectorAll<HTMLButtonElement>(`[data-${attr}]`).forEach((button) =>
      button.addEventListener("click", () => narrow({ [attr]: button.dataset[attr] ?? "" })),
    );
  }
  root?.querySelector<HTMLSelectElement>("[data-status]")?.addEventListener("change", (event) =>
    narrow({ status: (event.currentTarget as HTMLSelectElement).value }),
  );
  root?.querySelector<HTMLSelectElement>("[data-board]")?.addEventListener("change", (event) =>
    narrow({ board: (event.currentTarget as HTMLSelectElement).value }),
  );

  const search = root?.querySelector<HTMLInputElement>("[data-q]");
  search?.addEventListener(
    "input",
    debounce(() => narrow({ q: search.value }), 350),
  );

  // Triage from the list. The screen is redrawn rather than the row removed:
  // under "everything" a published report stays, wearing its new state, and a
  // row that vanished would read as a report that was deleted.
  root?.querySelectorAll<HTMLButtonElement>(".report [data-act]").forEach((button) =>
    button.addEventListener("click", async () => {
      const id = button.closest<HTMLLIElement>("li[data-id]")?.dataset.id;
      if (!id) return;
      try {
        await api.post(`/api/items/${encodeURIComponent(id)}/moderation`, { state: button.dataset.act });
        await render();
      } catch (err) {
        report(err);
      }
    }),
  );
}

/** The organisations, as cards, with the way to add one. */
async function showOrganizationsPanel(): Promise<void> {
  if (!(await requirePanel())) return;
  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");

  await showInShell(
    views.organizationsScreen(
      organizations ?? [],
      { canAdd: isAdministrator(), canManage: ownsOrganization, canDelete: isAdministrator() },
      text,
    ) +
      views.modal(
        {
          id: "org-modal",
          title: text.t("panel.newOrganization"),
          lead: text.t("setup.orgLead"),
          body: views.organizationFields(text, draft.forge),
          submit: text.t("auth.create"),
          formClass: "org-form",
        },
        text,
      ) +
      views.modal(
        {
          id: "org-rename",
          title: text.t("organizations.rename"),
          body: views.field({ id: "org-new-name", name: "name", label: text.t("panel.name"), required: true }, text),
          submit: text.t("action.save"),
          formClass: "org-rename-form",
        },
        text,
      ),
    "/panel/organizations",
  );

  wireModalOpeners();

  onSubmit("form.org-form", async (data) => {
    await api.post("/api/organizations", {
      name: String(data.get("name") ?? ""),
      slug: String(data.get("slug") ?? ""),
      forge: String(data.get("forge") ?? "none"),
      base_url: String(data.get("base_url") ?? ""),
      forge_owner: String(data.get("forge_owner") ?? ""),
    });
    await render();
  });

  let renaming = "";
  root?.querySelectorAll<HTMLButtonElement>("[data-edit-org]").forEach((button) =>
    button.addEventListener("click", () => {
      renaming = button.dataset.editOrg ?? "";
      const input = root?.querySelector<HTMLInputElement>("#org-new-name");
      if (input) input.value = button.dataset.name ?? "";
      openModal("org-rename");
    }),
  );
  onSubmit("form.org-rename-form", async (data) => {
    await api.patch(`/api/orgs/${encodeURIComponent(renaming)}`, { name: String(data.get("name") ?? "") });
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-org]").forEach((button) =>
    button.addEventListener("click", async () => {
      const slug = button.dataset.deleteOrg ?? "";
      if (!confirm(i18n.t("organizations.confirmDelete", { name: button.dataset.name ?? slug }))) return;
      await api.del(`/api/orgs/${encodeURIComponent(slug)}`).catch(showFormError);
      await render();
    }),
  );

  // The forge fields follow the choice inside the modal too.
  const select = root?.querySelector<HTMLSelectElement>("#org-forge");
  const fields = root?.querySelector<HTMLDivElement>(".forge-fields");
  const sync = () => {
    if (fields) fields.hidden = (select?.value ?? "none") === "none";
  };
  select?.addEventListener("change", sync);
  sync();
}

/** The name, the logo and the accent of this installation. */
async function showPersonalization(): Promise<void> {
  if (!(await requirePanel())) return;
  const instance = (await api
    .get<{ name: string; logo: string; accent: string }>("/api/instance")
    .catch(() => null)) ?? { name: "", logo: "", accent: "" };

  await showInShell(views.personalizationScreen(instance, text), "/panel/personalization");

  let logo = instance.logo;
  const logoFile = root?.querySelector<HTMLInputElement>("#logo-file");
  root?.querySelector<HTMLButtonElement>("[data-pick-logo]")?.addEventListener("click", () => logoFile?.click());
  logoFile?.addEventListener("change", async () => {
    const picked = await readAsDataURL(logoFile.files?.[0]);
    if (picked === null) return;
    logo = picked;
    await api.post("/api/instance", { ...instance, logo }).catch(showFormError);
    instanceLoaded = false;
    await render();
  });
  root?.querySelector<HTMLButtonElement>("[data-clear-logo]")?.addEventListener("click", async () => {
    await api.post("/api/instance", { ...instance, logo: "" }).catch(showFormError);
    instanceLoaded = false;
    await render();
  });
  root?.querySelector<HTMLButtonElement>("[data-reset-accent]")?.addEventListener("click", async () => {
    await api.post("/api/instance", { ...instance, accent: "" }).catch(showFormError);
    await render();
  });

  onSubmit("form.instance-settings", async (data) => {
    await api.post("/api/instance", {
      name: String(data.get("name") ?? ""),
      logo,
      accent: String(data.get("accent") ?? ""),
    });
    instanceLoaded = false;
    await render();
  });
}

/**
 * Updates, on a page of their own.
 *
 * They used to hang off the bottom of the panel's front page, under the form
 * for adding an organisation, which is not where anybody looks for them. The
 * endpoint refuses anybody who may not see this, and asking it is the check: a
 * screen that decided for itself who may look would be a second implementation
 * of the rules.
 */
async function showUpdates(): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const status = await api
    .get<{
      current: string;
      latest?: string;
      available?: boolean;
      notes?: string;
      reason?: string;
      beta?: boolean;
    }>("/api/update")
    .catch(() => null);
  const beta = status?.beta === true;

  await showInShell(
    status
      ? `<h1>${escapeHTML(i18n.t("shell.updates"))}</h1>
         <ul class="bento">${views.channelSwitch(beta, text)}</ul>
         ${views.updatePanel(status, text)}`
      : views.emptyState(i18n.t("error.forbidden"), icons.warning(28)),
    "/panel/updates",
  );

  root?.querySelector<HTMLInputElement>("[data-beta]")?.addEventListener("change", async (event) => {
    await api
      .post("/api/update/channel", { beta: (event.currentTarget as HTMLInputElement).checked })
      .catch(showFormError);
    await render();
  });

  if (status) {
    root?.querySelector<HTMLButtonElement>("[data-update]")?.addEventListener("click", async (event) => {
      const button = event.currentTarget as HTMLButtonElement;
      const note = root?.querySelector<HTMLParagraphElement>(".update-note");
      button.disabled = true;
      if (note) {
        note.textContent = i18n.t("update.installing");
        note.hidden = false;
      }
      try {
        await api.post("/api/update");
        // The server restarts a moment after answering, so waiting and
        // reloading is what brings the page back on the new version.
        setTimeout(() => location.reload(), 4000);
      } catch (err) {
        button.disabled = false;
        if (note) note.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
      }
    });
  }

}

async function showPanelOrganization(orgSlug: string): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const body = await api.get<{
    organization: OrganizationSummary & { forge?: string; webhook_path?: string };
    repositories: RepositorySummary[];
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);
  const repos = body.repositories ?? [];
  const canManage = ownsOrganization(orgSlug);
  const forge = body.organization.forge ?? "none";

  await showInShell(
    views.organizationScreen(body.organization, repos, canManage, isAdministrator(), text) +
      // The token and the channel belong to whoever runs the organisation. A
      // collaborator triages without ever being able to answer "what is the
      // token", which is the whole reason the two roles exist.
      (canManage ? views.connectionForms(body.organization, repos, location.origin, text) : "") +
      views.modal(
        {
          id: "repo-modal",
          title: text.t("panel.newRepository"),
          lead: text.t("repo.lead"),
          body: views.repositoryFields(text, forge, forge !== "none") + views.acceptsFields(text),
          submit: text.t("auth.create"),
          formClass: "repo-form",
        },
        text,
      ),
    `/panel/${orgSlug}`,
  );

  wireCredentials(orgSlug);

  // The one value on this page that has to end up in another application.
  root?.querySelector<HTMLButtonElement>("[data-copy-webhook]")?.addEventListener("click", async (event) => {
    const address = root?.querySelector<HTMLElement>("[data-webhook]")?.textContent ?? "";
    try {
      await navigator.clipboard.writeText(address);
      const button = event.currentTarget as HTMLButtonElement;
      const label = button.querySelector("span");
      if (label) label.textContent = i18n.t("action.copied");
    } catch {
      // No clipboard permission, or an insecure origin. The address is on the
      // screen and selectable, which is the fallback that always worked.
    }
  });
  wireChannel(orgSlug);
  wireModalOpeners();

  root?.querySelectorAll<HTMLButtonElement>("[data-open]").forEach((button) =>
    button.addEventListener("click", () => go(button.dataset.open ?? "/")),
  );

  onSubmit("form.repo-form", async (data) => {
    await api.post(`/api/orgs/${encodeURIComponent(orgSlug)}/repositories`, {
      name: String(data.get("name") ?? ""),
      slug: String(data.get("slug") ?? ""),
      forge_repo: String(data.get("forge_repo") ?? ""),
      accepts_bugs: data.get("accepts_bugs") !== null,
      accepts_ideas: data.get("accepts_ideas") !== null,
    });
    await render();
  });

  root?.querySelectorAll<HTMLButtonElement>("[data-delete-repo]").forEach((button) =>
    button.addEventListener("click", async () => {
      const slug = button.dataset.deleteRepo ?? "";
      if (!confirm(i18n.t("organizations.confirmDeleteRepo", { name: button.dataset.name ?? slug }))) return;
      try {
        await api.del(`/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(slug)}`);
        await render();
      } catch (err) {
        showFormError(err);
      }
    }),
  );
}

/**
 * The queue is the panel's front page for one board: what is waiting, with two
 * buttons.
 *
 * Acting on an entry removes that entry and leaves the rest alone. Redrawing
 * the whole list after every decision would throw away the scroll position of
 * somebody working down a long queue.
 */
async function showQueue(orgSlug: string, repoSlug: string): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");

  const base = `/api/orgs/${encodeURIComponent(orgSlug)}/repos/${encodeURIComponent(repoSlug)}`;
  const body = await api.get<{ items: Array<ItemSummary & { body: string; fields?: ReportField[] }> }>(
    `${base}/queue`,
  );
  const items = body.items ?? [];
  const bodies = new Map(items.map((i) => [i.public_id, i.body]));
  const fields = new Map(items.map((i) => [i.public_id, i.fields ?? []]));

  // The comments waiting on this board. Failing softly rather than taking the
  // screen down with it: the reports are what somebody came here for.
  const waiting = await api
    .get<{ comments: views.WaitingComment[] }>(`${base}/comments`)
    .catch(() => ({ comments: [] as views.WaitingComment[] }));

  const repo = await api.get<RepositorySummary & { organization?: OrganizationSummary }>(base);
  await showInShell(
    views.queueScreen(
      repo.organization ?? { slug: orgSlug, name: orgSlug },
      repo,
      items,
      bodies,
      fields,
      waiting.comments ?? [],
      text,
    ),
    `/${orgSlug}/${repoSlug}/queue`,
  );

  root?.querySelectorAll<HTMLButtonElement>("[data-comment-act]").forEach((button) =>
    button.addEventListener("click", async () => {
      const entry = button.closest<HTMLLIElement>("li[data-comment]");
      const id = entry?.dataset.comment;
      if (!id) return;
      try {
        await api.post(`/api/comments/${encodeURIComponent(id)}/moderation`, {
          state: button.dataset.commentAct,
        });
        entry?.remove();
      } catch (err) {
        report(err);
      }
    }),
  );

  root?.querySelectorAll<HTMLButtonElement>("[data-act]").forEach((button) =>
    button.addEventListener("click", async () => {
      const entry = button.closest<HTMLLIElement>("li[data-id]");
      const id = entry?.dataset.id;
      if (!id) return;
      try {
        await api.post(`/api/items/${encodeURIComponent(id)}/moderation`, {
          state: button.dataset.act,
        });
        entry?.remove();
        if (!root?.querySelector("li[data-id]")) {
          const list = root?.querySelector(".queue");
          list?.replaceWith(
            Object.assign(document.createElement("p"), {
              className: "empty",
              textContent: i18n.t("panel.empty"),
            }),
          );
        }
      } catch (err) {
        report(err);
      }
    }),
  );
}

// --- odds and ends ---------------------------------------------------------

function debounce<T extends (...args: never[]) => void>(fn: T, ms: number): T {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return ((...args: never[]) => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  }) as T;
}

/**
 * The language of the account that is signed in, if it has one.
 *
 * The browser's preference decides for somebody with no account, which is
 * every visitor of a public board. For somebody signed in it is a setting of
 * theirs, so it follows them to the next machine instead of living in one
 * browser's storage.
 */
async function applyAccountLanguage(): Promise<void> {
  const chosen = me?.language;
  if (chosen && chosen !== i18n.current) await i18n.setLanguage(chosen);
}

/** The first run reads English until the administrator says otherwise. */
async function startInEnglish(): Promise<void> {
  if (i18n.chosen) return;
  await i18n.setLanguage("en");
}

void (async () => {
  await i18n.ready;
  me = await api.get<Me>("/api/me").catch(() => null);
  await applyAccountLanguage();
  i18n.subscribe(() => renderNav());
  await render();
})();
