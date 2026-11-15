import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { deviceRef } from "./pushSupport";
import { resyncPush } from "./pushClient";

// What a browser that took part in push has: permission, a worker, and a subscription it still holds.
const ENDPOINT = "https://fcm.googleapis.com/fcm/send/abc";
const KEY_BYTES = [4, 1, 2, 3];
const KEY_B64URL = "BAECAw"; // the same four bytes, base64url without padding

const NAMES = ["navigator", "window", "document", "fetch", "PushManager", "Notification"];
const before = new Map<string, PropertyDescriptor | undefined>();
let calls: { method: string; url: string }[];
let listed: { ref: string }[];

const put = (name: string, value: unknown) =>
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, enumerable: true, value });

function own(key: number[]) {
  return {
    endpoint: ENDPOINT,
    options: { applicationServerKey: new Uint8Array(key).buffer },
    toJSON: () => ({ endpoint: ENDPOINT, keys: { p256dh: "p", auth: "a" } }),
  };
}

function browser(opts: { permission: string; subscription: ReturnType<typeof own> | null }) {
  calls = [];
  put("navigator", {
    userAgent: "test",
    maxTouchPoints: 0,
    serviceWorker: { getRegistration: async () => ({ pushManager: { getSubscription: async () => opts.subscription } }) },
  });
  put("Notification", { permission: opts.permission });
  put("fetch", async (url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url });
    const body = url.endsWith("/api/push/key") ? { public_key: KEY_B64URL } : url.endsWith("/api/push/subscriptions") && method === "GET" ? { devices: listed } : {};
    return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
  });
}

beforeAll(() => {
  for (const name of NAMES) before.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
  put("window", { PushManager: class {}, Notification: true, matchMedia: () => ({ matches: false }) });
  put("PushManager", class {});
  put("document", { cookie: "" });
});

afterAll(() => {
  for (const [name, d] of before) {
    if (d) Object.defineProperty(globalThis, name, d);
    else delete (globalThis as Record<string, unknown>)[name];
  }
});

describe("putting a browser back on the server list when the panel opens", () => {
  test("a browser the server forgot is subscribed again", async () => {
    listed = [];
    browser({ permission: "granted", subscription: own(KEY_BYTES) });
    await resyncPush();
    expect(calls.filter((c) => c.method === "POST").map((c) => c.url)).toEqual(["/api/push/subscriptions"]);
  });

  test("a browser the server still knows costs one request and writes nothing", async () => {
    listed = [{ ref: await deviceRef(ENDPOINT) }];
    browser({ permission: "granted", subscription: own(KEY_BYTES) });
    await resyncPush();
    expect(calls).toEqual([{ method: "GET", url: "/api/push/subscriptions" }]);
  });

  test("a subscription made for another key is left for the person to turn on again", async () => {
    listed = [];
    browser({ permission: "granted", subscription: own([9, 9, 9, 9]) });
    await resyncPush();
    expect(calls.some((c) => c.method === "POST")).toBe(false);
  });

  test("a browser that never took part asks nothing: no permission, or no subscription", async () => {
    for (const b of [
      { permission: "default", subscription: own(KEY_BYTES) },
      { permission: "denied", subscription: own(KEY_BYTES) },
      { permission: "granted", subscription: null },
    ]) {
      browser(b);
      await resyncPush();
      expect(calls).toEqual([]);
    }
  });
});
