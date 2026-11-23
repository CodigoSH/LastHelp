import { describe, expect, test } from "bun:test";
import { userAgentLabel } from "./userAgentLabel";

const UA = {
  firefoxLinux: "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0",
  chromeWindows: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
  edgeWindows: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
  safariMac: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
  safariIphone: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
  safariIpad: "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
  chromeIphone: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/130.0.6723.37 Mobile/15E148 Safari/604.1",
  chromeAndroid: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
  firefoxAndroid: "Mozilla/5.0 (Android 14; Mobile; rv:131.0) Gecko/131.0 Firefox/131.0",
  operaLinux: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 OPR/115.0.0.0",
  chromeOs: "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
  chromeMac: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
};

describe("userAgentLabel", () => {
  const cases: Array<[string, string, string, string]> = [
      ["Firefox on Linux", UA.firefoxLinux, "firefox", "linux"],
      ["Chrome on Windows", UA.chromeWindows, "chrome", "windows"],
      ["Edge on Windows (also says Chrome and Safari)", UA.edgeWindows, "edge", "windows"],
      ["Safari on macOS", UA.safariMac, "safari", "macos"],
      ["Safari on iPhone (also says Mac OS X)", UA.safariIphone, "safari", "ios"],
      ["Safari on iPad", UA.safariIpad, "safari", "ios"],
      ["Chrome on iPhone", UA.chromeIphone, "chrome", "ios"],
      ["Chrome on Android (also says Linux)", UA.chromeAndroid, "chrome", "android"],
      ["Firefox on Android", UA.firefoxAndroid, "firefox", "android"],
      ["Opera on Linux", UA.operaLinux, "opera", "linux"],
      ["Chrome on ChromeOS (also says Linux/X11)", UA.chromeOs, "chrome", "chromeos"],
      ["Chrome on macOS", UA.chromeMac, "chrome", "macos"],
  ];

  for (const [name, ua, browser, os] of cases) {
      test(name, () => {
      expect(userAgentLabel(ua)).toEqual({ browser: browser as any, os: os as any });
      });
  }

  test("an empty or unrecognised string is unknown, not a guess", () => {
      expect(userAgentLabel("")).toEqual({ browser: "unknown", os: "unknown" });
      expect(userAgentLabel("curl/8.5.0")).toEqual({ browser: "unknown", os: "unknown" });
  });
  test("headless Chrome is still Chrome", () => {
      expect(userAgentLabel("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/156.0.0.0 Safari/537.36"))
      .toEqual({ browser: "chrome", os: "linux" });
  });
});
