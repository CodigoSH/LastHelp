/**
 * What the service worker makes of a push: the pure half of it, so the rules
 * for what a message may do are in one place with a test. The worker itself
 * (`sw.ts`) is only the two event listeners.
 */
interface Payload {
  title?: string;
  body?: string;
  url?: string;
  tag?: string;
}

/** Where a tap goes. Only a path inside this instance; anything else is the home page. */
export function target(url: unknown): string {
  if (typeof url !== "string" || !url.startsWith("/") || url.startsWith("//") || url.includes("\\")) return "/";
  return url;
}

/** What to show for a message, whatever shape it arrived in. A push that carries nothing still says something. */
export function describe(raw: string | null): { title: string; options: Record<string, unknown> } {
  let data: Payload = {};
  if (raw) {
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === "object") data = parsed as Payload;
    } catch {
      data = { body: raw };
    }
  }
  const tag = typeof data.tag === "string" && data.tag ? data.tag : undefined;
  return {
    title: typeof data.title === "string" && data.title ? data.title : "LastHelp",
    options: {
      body: typeof data.body === "string" ? data.body : "",
      icon: "/icon-192.png",
      // The small mark in the status bar: the icon's own outline, which is mostly transparent.
      badge: "/icon-192.png",
      tag,
      // A newer one replaces the older of its tag, and should still be noticed doing so.
      renotify: !!tag,
      data: { url: target(data.url) },
    },
  };
}
