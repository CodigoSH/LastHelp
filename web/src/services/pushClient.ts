import { api } from "../api";
import { decodeKey, deviceRef, isIOS, pushSupport, type PushEnv, type PushSupport } from "./pushSupport";

/**
 * Web Push from the browser's side: asking this device for permission, subscribing it, and keeping
 * the account's list of devices. The rules that can be tested without a browser are in `pushSupport`.
 *
 * Nothing here runs until somebody turns the switch on in their profile: no worker is registered and
 * no permission is asked for on load, so an installation that does not use push never sees any of it.
 */

/** A device in the account's list. The address of its subscription is not part of it: the server keeps that. */
export interface PushDevice {
  id: number;
  ref: string;
  label: string;
  created_at: number;
  last_ok_at: number | null;
}

/** What the profile draws: whether this device can take part, and who does. */
export interface PushView {
  support: PushSupport;
  devices: PushDevice[];
  /** The row that is this browser, if it is subscribed here and the server knows it. */
  thisDevice: number | null;
}

export function currentEnv(): PushEnv {
  const nav = navigator as Navigator & { standalone?: boolean };
  return {
    serviceWorker: "serviceWorker" in navigator,
    pushManager: "PushManager" in window,
    notification: "Notification" in window,
    permission: "Notification" in window ? Notification.permission : "default",
    ios: isIOS(nav.userAgent, nav.maxTouchPoints ?? 0),
    standalone: window.matchMedia?.("(display-mode: standalone)").matches === true || nav.standalone === true,
  };
}

/** This browser's own subscription, if it has one. Does not register the worker: asking is not enabling. */
async function ownSubscription(): Promise<PushSubscription | null> {
  if (!("serviceWorker" in navigator)) return null;
  const registration = await navigator.serviceWorker.getRegistration("/");
  return registration ? registration.pushManager.getSubscription() : null;
}

/** The current picture: support, the account's devices and which one is this browser. */
export async function loadPush(): Promise<PushView> {
  const support = pushSupport(currentEnv());
  const { devices } = await api.get<{ devices: PushDevice[] }>("/api/push/subscriptions");
  let thisDevice: number | null = null;
  if (support === "ready" || support === "denied") {
    const own = await ownSubscription();
    if (own) {
      const ref = await deviceRef(own.endpoint);
      thisDevice = devices.find((d) => d.ref === ref)?.id ?? null;
    }
  }
  return { support, devices, thisDevice };
}

/** The name this browser is listed under, from what it says about itself. */
export function deviceLabel(): string {
  const data = (navigator as Navigator & { userAgentData?: { brands?: { brand: string }[]; platform?: string } }).userAgentData;
  const brand = data?.brands?.find((b) => !/not.?a.?brand/i.test(b.brand))?.brand;
  if (brand && data?.platform) return `${brand} · ${data.platform}`;
  return navigator.userAgent.slice(0, 60);
}

/**
 * Turns notifications on for this browser. Must be called from a click: the permission prompt is
 * refused otherwise, and on an iPhone it is refused for anything but a tap.
 */
export async function enablePush(): Promise<void> {
  // First, before any await that could outlast the click: the prompt needs the gesture.
  const permission = await Notification.requestPermission();
  if (permission !== "granted") throw new Error("push.permission_denied");

  await navigator.serviceWorker.register("/sw.js", { scope: "/" });
  const registration = await navigator.serviceWorker.ready;
  const { public_key } = await api.get<{ public_key: string }>("/api/push/key");
  const key = decodeKey(public_key);

  let subscription = await registration.pushManager.getSubscription();
  // A subscription made for another key (the installation was restored onto a new one) cannot be used.
  if (subscription && !sameKey(subscription.options.applicationServerKey, key)) {
    await subscription.unsubscribe();
    subscription = null;
  }
  subscription ??= await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });

  const json = subscription.toJSON();
  await api.post("/api/push/subscriptions", {
    endpoint: json.endpoint,
    keys: { p256dh: json.keys?.p256dh, auth: json.keys?.auth },
    label: deviceLabel(),
  });
}

/** Turns notifications off for this browser: the server forgets it and the browser lets go of the subscription. */
export async function disablePush(deviceId: number | null): Promise<void> {
  if (deviceId !== null) await api.del(`/api/push/subscriptions/${deviceId}`);
  const own = await ownSubscription();
  if (own) await own.unsubscribe();
}

/** Removes a device of the account, which may be another one. */
export async function removePushDevice(id: number): Promise<void> {
  await api.del(`/api/push/subscriptions/${id}`);
}

/** Sends a notification to every device of the account, so a person sees it arrive. */
export async function testPush(): Promise<void> {
  await api.post("/api/push/test");
}

function sameKey(have: ArrayBuffer | null, want: Uint8Array): boolean {
  if (!have) return false;
  const a = new Uint8Array(have);
  return a.length === want.length && a.every((b, i) => b === want[i]);
}
