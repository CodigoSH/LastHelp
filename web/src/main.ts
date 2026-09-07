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
  RepositorySummary,
  Status,
  Submitted,
} from "./api";
import { i18n } from "./services/i18n";
import { availableLocales } from "./locales/metadata";
import { escapeAttr, escapeHTML } from "./escape";
import * as views from "./views/templates";

const text: views.Text = {
  t: (key, params) => i18n.t(key, params),
  date: (value) => i18n.date(value),
  number: (value) => i18n.number(value),
};

const root = document.getElementById("app");
const nav = document.getElementById("nav");

let me: Me | null = null;

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

  const picker = `<select class="lang" aria-label="Language">${availableLocales
    .map(
      (l) =>
        `<option value="${escapeAttr(l.code)}"${l.code === i18n.current ? " selected" : ""}>${escapeHTML(l.name)}</option>`,
    )
    .join("")}</select>`;

  const account = me
    ? `<a href="/panel">${escapeHTML(i18n.t("nav.panel"))}</a>
       <button class="link" data-signout>${escapeHTML(i18n.t("nav.signOut"))}</button>`
    : `<a href="/sign-in">${escapeHTML(i18n.t("nav.signIn"))}</a>`;

  nav.innerHTML = `<a class="brand" href="/">${escapeHTML(i18n.t("app.name"))}</a>
    <div class="nav-right">${account}${picker}</div>`;

  nav.querySelector<HTMLSelectElement>(".lang")?.addEventListener("change", (e) => {
    void i18n.setLanguage((e.target as HTMLSelectElement).value).then(() => void render());
  });
  nav.querySelector<HTMLButtonElement>("[data-signout]")?.addEventListener("click", async () => {
    await api.del("/api/session").catch(() => undefined);
    me = null;
    go("/");
  });
}

function show(html: string): void {
  if (root) root.innerHTML = html;
}

function fail(err: unknown): void {
  const key = err instanceof ApiError ? err.key : "error.internal";
  show(`<p class="empty">${escapeHTML(i18n.t(key))}</p>`);
}

// --- the views -------------------------------------------------------------

async function render(): Promise<void> {
  await i18n.ready;
  renderNav();

  const path = location.pathname.replace(/\/+$/, "") || "/";
  const parts = path.split("/").filter(Boolean);

  try {
    if (path === "/") return await showOrganizations();
    if (path === "/sign-in") return await showSignIn();
    if (path === "/setup/organization") return showSetupStep(2);
    if (path === "/setup/board") return showSetupStep(3);
    if (path === "/setup/done") return showSetupDone();
    if (path === "/panel") return await showPanel();
    // Before the generic /panel/{org}: "new" is a screen, not an organisation
    // anybody would be able to open.
    if (path === "/panel/new") return showOrganizationWizard(1);
    if (path === "/panel/new/connection") return showOrganizationWizard(2);
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
    show(`<p class="empty">${escapeHTML(i18n.t("error.not_found"))}</p>`);
  } catch (err) {
    fail(err);
  }
}

async function showOrganizations(): Promise<void> {
  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");
  // Who is looking decides what an empty instance says. An administrator gets
  // the way out of it; a visitor gets told plainly, rather than being sent to a
  // form that would refuse them.
  if (!me) me = await api.get<Me>("/api/me").catch(() => null);
  show(views.landing(organizations ?? [], me?.admin === true, text));
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

  show(views.reportForm(repo, kind, text));

  const form = root?.querySelector<HTMLFormElement>("form.report");
  const similar = root?.querySelector<HTMLDivElement>(".similar");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

  root?.querySelectorAll<HTMLButtonElement>(".choice").forEach((button) =>
    button.addEventListener("click", () => {
      kind = button.dataset.choice ?? kind;
      root?.querySelectorAll(".choice").forEach((b) => b.classList.remove("on"));
      button.classList.add("on");
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
        token: ticket.token,
      });
      const link = `${location.origin}/i/${sent.public_id}?k=${sent.follow_token}`;
      show(views.submitted(link, text));
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

  show(views.itemView(item, text));

  root?.querySelector<HTMLButtonElement>("[data-vote]")?.addEventListener("click", async (event) => {
    const button = event.currentTarget as HTMLButtonElement;
    const voted = button.classList.contains("voted");
    const result = await api[voted ? "del" : "post"]<{ votes: number; voted: boolean }>(
      `/api/items/${encodeURIComponent(publicID)}/votes`,
    );
    // Only the button is redrawn. Replacing the page would scroll the thread
    // back to the top under whoever was reading it.
    button.classList.toggle("voted", result.voted);
    button.firstChild!.textContent = ` ${result.voted ? i18n.t("item.voted") : i18n.t("item.vote")} `;
    const count = button.querySelector(".count");
    if (count) count.textContent = i18n.number(result.votes);
  });

  // The team's control, and only if the server says this person may use it.
  // Asking is the check; drawing the select is not. A page that decides for
  // itself who may act is a second implementation of the rules.
  if (me) {
    const allowed = await api
      .get<{ current: string; statuses: string[] }>(`/api/items/${encodeURIComponent(publicID)}/statuses`)
      .catch(() => null);
    if (allowed) {
      root
        ?.querySelector("article.detail")
        ?.insertAdjacentHTML("beforeend", views.statusControl(allowed.current, allowed.statuses, text));

      root?.querySelector<HTMLSelectElement>("#status-select")?.addEventListener("change", async (e) => {
        const select = e.target as HTMLSelectElement;
        await api
          .post(`/api/items/${encodeURIComponent(publicID)}/status`, { status: select.value })
          .catch((err) => fail(err));
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
function wireCredentials(orgSlug: string, existingPath?: string): void {
  const form = root?.querySelector<HTMLFormElement>("form.credentials");
  if (!form) return;

  const path = form.querySelector<HTMLParagraphElement>(".webhook-path");
  const errorLine = form.querySelector<HTMLParagraphElement>(".form-error");

  const showPath = (value: string) => {
    if (!path) return;
    // The address is shown as text to copy, not as a link: it only answers
    // POST, so a link would go to a 404 and read as broken.
    path.textContent = `${i18n.t("panel.webhookPath")}: ${location.origin}${value}`;
    path.hidden = false;
  };
  if (existingPath) showPath(existingPath);

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      const saved = await api.post<{ webhook_path: string }>(
        `/api/orgs/${encodeURIComponent(orgSlug)}/credentials`,
        {
          token: String(data.get("token") ?? ""),
          webhook_secret: String(data.get("webhook_secret") ?? ""),
        },
      );
      form.reset();
      showPath(saved.webhook_path);
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
  const note = form.querySelector<HTMLParagraphElement>(".channel-note");
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
      if (note) {
        note.textContent = i18n.t("panel.saved");
        note.hidden = false;
      }
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
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
    ?.querySelector("article.detail")
    ?.insertAdjacentHTML("beforeend", views.issueControl(linked, text));

  const note = root?.querySelector<HTMLParagraphElement>(".issue-note");
  const say = (key: string) => {
    if (note) {
      note.textContent = i18n.t(key);
      note.hidden = false;
    }
  };

  root?.querySelector<HTMLButtonElement>("[data-create-issue]")?.addEventListener("click", async () => {
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/issue`);
      say("panel.issueQueued");
    } catch (err) {
      say(err instanceof ApiError ? err.key : "error.internal");
    }
  });

  const attach = root?.querySelector<HTMLFormElement>("form.attach");
  attach?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const value = String(new FormData(attach).get("issue") ?? "");
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/attach`, { issue: value });
      await render();
    } catch (err) {
      say(err instanceof ApiError ? err.key : "error.internal");
    }
  });
}

/** Replying as the team, and merging a duplicate away. */
function wireTriage(publicID: string): void {
  root?.insertAdjacentHTML("beforeend", views.triageControls(text));

  const note = root?.querySelector<HTMLParagraphElement>(".triage-note");
  const say = (key: string) => {
    if (note) {
      note.textContent = i18n.t(key);
      note.hidden = false;
    }
  };

  const reply = root?.querySelector<HTMLFormElement>("form.team-reply");
  reply?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(reply);
    try {
      await api.post(`/api/items/${encodeURIComponent(publicID)}/reply`, {
        body: String(data.get("body") ?? ""),
      });
      reply.reset();
      await render();
    } catch (err) {
      say(err instanceof ApiError ? err.key : "error.internal");
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
      await render();
    } catch (err) {
      say(err instanceof ApiError ? err.key : "error.internal");
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
const draft: { forge: string; orgSlug: string; repoSlug: string } = {
  forge: "none",
  orgSlug: "",
  repoSlug: "",
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

function showSetupStep(step: 2 | 3): void {
  show(views.setupWizard(step, text, draft.forge));

  if (step === 2) {
    wireForgeChoice((forge) => showSetupStep2(forge));
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
          go("/setup/board");
        } catch (err) {
          showFormError(err);
        }
      },
    );
    return;
  }

  root?.querySelector<HTMLFormElement>("form.setup-repository")?.addEventListener(
    "submit",
    async (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget as HTMLFormElement);
      draft.repoSlug = String(data.get("slug") ?? "");
      try {
        await api.post(`/api/orgs/${encodeURIComponent(draft.orgSlug)}/repositories`, {
          name: String(data.get("name") ?? ""),
          slug: draft.repoSlug,
          forge_repo: String(data.get("forge_repo") ?? ""),
          accepts_bugs: data.get("accepts_bugs") !== null,
          accepts_ideas: data.get("accepts_ideas") !== null,
        });
        go("/setup/done");
      } catch (err) {
        showFormError(err);
      }
    },
  );
}

/** Re-render step two in place when the forge changes, keeping what was typed. */
function showSetupStep2(forge: string): void {
  const kept = new FormData(root?.querySelector<HTMLFormElement>("form.setup-organization") ?? undefined);
  draft.forge = forge;
  showSetupStep(2);
  const form = root?.querySelector<HTMLFormElement>("form.setup-organization");
  for (const [name, value] of kept.entries()) {
    const input = form?.querySelector<HTMLInputElement>(`[name="${name}"]`);
    if (input && input.type !== "checkbox" && name !== "forge") input.value = String(value);
  }
}

function showSetupDone(): void {
  show(views.wizardDone(draft.orgSlug, draft.repoSlug, draft.forge !== "none", text));
}

function showOrganizationWizard(step: 1 | 2): void {
  show(views.organizationWizard(step, text, draft.forge));

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
  show(views.repositoryWizard(body.organization, step, text));

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
  if (status.needs_setup) {
    show(views.setupWizard(1, text));
  } else {
    show(views.signInForm(text));
  }

  const form = root?.querySelector<HTMLFormElement>("form.auth, form.setup-account");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const payload = {
      email: String(data.get("email") ?? ""),
      password: String(data.get("password") ?? ""),
    };
    try {
      await api.post(status.needs_setup ? "/api/setup" : "/api/session", payload);
      me = await api.get<Me>("/api/me");
      // A fresh instance goes on to the rest of the wizard rather than dropping
      // somebody on an empty panel with a form and no order.
      go(status.needs_setup ? "/setup/organization" : "/panel");
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

  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");
  show(views.panelHome(organizations ?? [], text));

  // Updates, for the administrator only. Asking is the check: the endpoint
  // refuses anybody else, and a panel that decided for itself who may see this
  // would be a second implementation of the rules.
  const status = await api
    .get<{ current: string; latest?: string; available?: boolean; notes?: string; reason?: string }>(
      "/api/update",
    )
    .catch(() => null);
  if (status) {
    root?.insertAdjacentHTML("beforeend", views.updatePanel(status, text));

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

  const form = root?.querySelector<HTMLFormElement>("form.new-org");
  const forgeFields = root?.querySelector<HTMLDivElement>(".forge-fields");
  const errorLine = root?.querySelector<HTMLParagraphElement>(".form-error");

  // The forge fields only when there is a forge. Asking for the address of
  // something you said you do not have is a form arguing with you.
  const select = root?.querySelector<HTMLSelectElement>("#org-forge");
  const syncFields = () => {
    if (forgeFields) forgeFields.hidden = (select?.value ?? "none") === "none";
  };
  select?.addEventListener("change", syncFields);
  syncFields();

  form?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      await api.post("/api/organizations", {
        name: String(data.get("name") ?? ""),
        slug: String(data.get("slug") ?? ""),
        forge: String(data.get("forge") ?? "none"),
        base_url: String(data.get("base_url") ?? ""),
        forge_owner: String(data.get("forge_owner") ?? ""),
      });
      go("/panel/" + String(data.get("slug") ?? ""));
    } catch (err) {
      if (errorLine) {
        errorLine.textContent = i18n.t(err instanceof ApiError ? err.key : "error.internal");
        errorLine.hidden = false;
      }
    }
  });
}

async function showPanelOrganization(orgSlug: string): Promise<void> {
  me = await api.get<Me>("/api/me").catch(() => null);
  if (!me) return go("/sign-in");
  renderNav();

  const body = await api.get<{
    organization: OrganizationSummary;
    repositories: RepositorySummary[];
  }>(`/api/orgs/${encodeURIComponent(orgSlug)}/repos`);

  const repos = body.repositories ?? [];
  show(views.panelOrganization(body.organization, repos, text));
  root?.insertAdjacentHTML("beforeend", views.connectionForms(body.organization, repos, text));

  wireCredentials(orgSlug, body.organization.webhook_path);
  wireChannel(orgSlug);

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
  const body = await api.get<{ items: Array<ItemSummary & { body: string }> }>(`${base}/queue`);
  const items = body.items ?? [];
  const bodies = new Map(items.map((i) => [i.public_id, i.body]));

  show(`<p class="crumb"><a href="/${escapeAttr(orgSlug)}/${escapeAttr(repoSlug)}">${escapeHTML(i18n.t("nav.back"))}</a></p>
        <h1>${escapeHTML(i18n.t("panel.queue"))}</h1>
        ${views.queueView(items, bodies, text)}`);

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
        fail(err);
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

void (async () => {
  await i18n.ready;
  me = await api.get<Me>("/api/me").catch(() => null);
  i18n.subscribe(() => renderNav());
  await render();
})();
