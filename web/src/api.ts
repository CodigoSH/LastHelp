/**
 * The one place that talks to the server.
 *
 * Every write carries the CSRF token the server put in a readable cookie. That
 * cookie proves nothing on its own — the session cookie is what carries
 * authority — but only a page on this origin can read it and echo it back,
 * which is what a form hosted somewhere else cannot do.
 */

export class ApiError extends Error {
  constructor(
    readonly status: number,
    /** A translation key, never a sentence: the server does not know what
     *  language the reader chose, and a message composed there would be the one
     *  string on the screen that stayed English. */
    readonly key: string,
  ) {
    super(key);
  }
}

export function csrfToken(): string {
  for (const part of document.cookie.split(";")) {
    const [name, ...rest] = part.trim().split("=");
    if (name === "lasthelp_csrf") return rest.join("=");
  }
  return "";
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && method !== "HEAD") headers["X-CSRF-Token"] = csrfToken();

  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    // Same-origin only. The portal never talks to anywhere else, which is the
    // promise the security page makes.
    credentials: "same-origin",
  });

  if (res.status === 204) return undefined as T;

  const text = await res.text();

  // Parsed defensively rather than assumed. A refusal that arrives as anything
  // but JSON used to throw a SyntaxError here, which is not an ApiError, so
  // the screen fell back to "something went wrong here" — the message for a
  // failure with no known cause — and hid the one the server had sent.
  //
  // The middleware answers JSON now. This stays because a proxy in front of
  // this server answers with whatever it likes, and a gateway's HTML error
  // page should not read as an internal fault of ours.
  let payload: Record<string, unknown> = {};
  if (text) {
    try {
      payload = JSON.parse(text) as Record<string, unknown>;
    } catch {
      // A bare key is what the middleware used to send, and what something in
      // front of us might still forward.
      if (/^error\.[a-z_]+$/.test(text.trim())) payload = { error: text.trim() };
    }
  }

  if (!res.ok) {
    const key = typeof payload.error === "string" ? payload.error : "error.internal";
    throw new ApiError(res.status, key);
  }
  return payload as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
  /** A multipart upload. Not JSON, so it does not go through request(). */
  upload: async <T>(path: string, form: FormData): Promise<T> => {
    const response = await fetch(path, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: form,
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new ApiError(response.status, body.error ?? "error.internal");
    return body as T;
  },
  /**
   * The bytes of one file, as the body.
   *
   * Not multipart: the server wants one picture and nothing else, and a
   * multipart envelope around a single value is a parser and a boundary and a
   * field name to agree on for no gain.
   */
  postBytes: async <T>(path: string, blob: Blob): Promise<T> => {
    const response = await fetch(path, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken(), "Content-Type": blob.type },
      body: blob,
      credentials: "same-origin",
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new ApiError(response.status, body.error ?? "error.internal");
    return body as T;
  },
  del: <T>(path: string, body?: unknown) => request<T>("DELETE", path, body),
};

// --- what the server answers -------------------------------------------------

export interface Status {
  needs_setup: boolean;
}

export interface Me {
  id: number;
  username?: string;
  /** The language this account reads in, or empty when it has not chosen. */
  language?: string;
  avatar?: string;
  admin: boolean;
  /**
   * What is waiting for this account, already narrowed to the boards it may
   * open. Counted by the server for that reason: the page has no way to know
   * which boards those are, and a number it worked out itself would be a second
   * opinion on a permission.
   */
  pending?: { reports: number; messages: number };
  memberships: Array<{
    scope: string;
    role: string;
    /** The slug of the organisation, and of the repository when it is one. */
    organization?: string;
    repository?: string;
    name?: string;
  }>;
}

export interface OrganizationSummary {
  slug: string;
  name: string;
  /** Present when a single organisation is fetched. */
  forge?: string;
  /** Only for somebody who may manage this organisation. */
  webhook_path?: string;
}

export interface RepositorySummary {
  slug: string;
  name: string;
  accepts_bugs: boolean;
  accepts_ideas: boolean;
  /** Whether a mail relay exists. Without one the form does not ask for an
   *  address, rather than collecting one nothing can use. */
  can_notify: boolean;
  /** Present when a single board is fetched. The page shows the organisation's
   *  name, and the address is not its name. */
  organization?: OrganizationSummary;
}

export interface ItemSummary {
  public_id: string;
  kind: "bug" | "idea";
  title: string;
  status: string;
  votes: number;
  voted: boolean;
  created_at: number;
}

/** One answer to one question of the report form, with the label it was asked
 *  under: a form reworded later must not reword what somebody already sent. */
export interface ReportField {
  key: string;
  label: string;
  value: string;
}

export interface ItemDetail extends ItemSummary {
  body: string;
  mine: boolean;
  /** Where it was reported, so a link somebody was sent says what it is about
   *  and leads back to the board rather than nowhere. */
  repository?: { slug: string; name: string; organization?: { slug: string; name: string } };
  /** Whether it is on the board yet. Only the reporter is ever here before it
   *  is, and only they are told so. */
  moderation?: string;
  /** The answers to the questions this kind was asked, in the order asked. */
  fields?: ReportField[];
  merged_into?: string;
  issue?: { number: number; url: string };
  comments: Array<{ id?: number; author: string; name: string; body: string; created_at: number }>;
}

export interface Submitted {
  public_id: string;
  follow_token: string;
  moderation: string;
}
