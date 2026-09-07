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
  if (!href?.startsWith("/") || link.hasAttribute("target")) return;
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
    if (path === "/panel") return await showPanel();
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
  show(`<h1>${escapeHTML(i18n.t("app.name"))}</h1>
        <p class="lead">${escapeHTML(i18n.t("app.tagline"))}</p>
        ${views.organizationList(organizations ?? [], text)}`);
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
    show(views.board({ slug: orgSlug, name: orgSlug }, repo, await load(), filters, text));
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
  show(views.roadmap({ slug: orgSlug, name: orgSlug }, repo, board.columns ?? [], text));
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

async function showSignIn(): Promise<void> {
  const status = await api.get<Status>("/api/status");
  show(views.signInForm(status.needs_setup, text));

  const form = root?.querySelector<HTMLFormElement>("form.auth");
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

  const { organizations } = await api.get<{ organizations: OrganizationSummary[] }>("/api/orgs");
  show(`<h1>${escapeHTML(i18n.t("nav.panel"))}</h1>
        <h2>${escapeHTML(i18n.t("portal.boards"))}</h2>
        ${views.organizationList(organizations ?? [], text)}`);
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
