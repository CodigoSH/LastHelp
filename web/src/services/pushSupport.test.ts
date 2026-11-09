import { describe, expect, it } from "bun:test";
import { createHash } from "crypto";
import { decodeKey, deviceRef, isIOS, pushSupport, type PushEnv } from "./pushSupport";

const desktop: PushEnv = { serviceWorker: true, pushManager: true, notification: true, permission: "default", ios: false, standalone: false };

describe("pushSupport", () => {
  it("is ready where the browser has everything and nobody said no", () => {
    expect(pushSupport(desktop)).toBe("ready");
    expect(pushSupport({ ...desktop, permission: "granted" })).toBe("ready");
  });

  it("says so when the person refused notifications for the site", () => {
    expect(pushSupport({ ...desktop, permission: "denied" })).toBe("denied");
  });

  it("says so when the browser has no Web Push", () => {
    expect(pushSupport({ ...desktop, pushManager: false })).toBe("unsupported");
    expect(pushSupport({ ...desktop, serviceWorker: false })).toBe("unsupported");
    expect(pushSupport({ ...desktop, notification: false })).toBe("unsupported");
  });

  // Safari in a tab has no PushManager; the answer there is "install it", not "your browser can't".
  it("asks an iPhone in a tab to add the app to the Home Screen first", () => {
    expect(pushSupport({ ...desktop, ios: true, standalone: false, pushManager: false })).toBe("needs-install");
    expect(pushSupport({ ...desktop, ios: true, standalone: false })).toBe("needs-install");
  });

  it("is ready on an iPhone once the app is installed", () => {
    expect(pushSupport({ ...desktop, ios: true, standalone: true })).toBe("ready");
    expect(pushSupport({ ...desktop, ios: true, standalone: true, permission: "denied" })).toBe("denied");
  });
});

describe("isIOS", () => {
  it("knows iPhones and iPads", () => {
    expect(isIOS("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", 5)).toBe(true);
    expect(isIOS("Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)", 5)).toBe(true);
  });

  // iPadOS asks for the desktop site by default and calls itself a Mac. A real Mac has no touch screen.
  it("knows an iPad that says it is a Mac, and not a Mac", () => {
    const mac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15";
    expect(isIOS(mac, 5)).toBe(true);
    expect(isIOS(mac, 0)).toBe(false);
  });

  it("does not take Android or Linux for it", () => {
    expect(isIOS("Mozilla/5.0 (Linux; Android 14; Pixel 8) Chrome/120", 5)).toBe(false);
    expect(isIOS("Mozilla/5.0 (X11; Linux x86_64) Firefox/120.0", 0)).toBe(false);
  });
});

describe("decodeKey", () => {
  it("turns base64url, with or without padding, into bytes", () => {
    expect([...decodeKey("AQID")]).toEqual([1, 2, 3]);
    expect([...decodeKey("AQI")]).toEqual([1, 2]);
    // '-' and '_' are the URL-safe forms of '+' and '/'.
    expect([...decodeKey("-_8")]).toEqual([251, 255]);
  });

  it("decodes a 65-byte public key to 65 bytes", () => {
    const key = Buffer.from(new Uint8Array(65).fill(7)).toString("base64url");
    expect(decodeKey(key).length).toBe(65);
  });
});

describe("deviceRef", () => {
  // The server computes this too (pushRef in push.go); both are the first eight bytes of a SHA-256, so they stay in step.
  it("is the first eight bytes of the SHA-256 of the address, in hex", async () => {
    const endpoint = "https://fcm.googleapis.com/fcm/send/abc";
    expect(await deviceRef(endpoint)).toBe(createHash("sha256").update(endpoint).digest("hex").slice(0, 16));
  });

  it("differs for different addresses and is stable for the same one", async () => {
    const a = await deviceRef("https://fcm.googleapis.com/fcm/send/a");
    const b = await deviceRef("https://fcm.googleapis.com/fcm/send/b");
    expect(a).not.toBe(b);
    expect(a).toBe(await deviceRef("https://fcm.googleapis.com/fcm/send/a"));
    expect(a).toMatch(/^[0-9a-f]{16}$/);
  });
});
