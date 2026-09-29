/**
 * The pieces every browser pass needs: a page over CDP, a way to run code in
 * it, waits on conditions rather than on time, and a line per check.
 *
 * The same code the main journey (drive-check.ts) carries inline, for the
 * passes that run against a seeded installation. Kept apart rather than moved
 * out of the main journey, which works and is not the thing being changed.
 */
export const PORT = Number(process.env.PORT ?? 8099);
export const CDP = Number(process.env.CDP ?? 9222);
export const BASE = `http://127.0.0.1:${PORT}`;

export const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

type Page = { send: (m: string, p?: unknown) => Promise<any>; close: () => void };

export async function connect() {
  const consoleErrors: string[] = [];
  // What the content policy refused: it arrives as a Log entry from the
  // security source, never as a console call.
  const refusedByPolicy: string[] = [];
  // Every request the page sends, as the browser saw it leave: counted here
  // rather than read from the page's own timings, which measure something else.
  const requests: { url: string; at: number }[] = [];

  const res = await fetch(`http://127.0.0.1:${CDP}/json/new?${encodeURIComponent(BASE + "/")}`, { method: "PUT" });
  const target = (await res.json()) as { id: string; webSocketDebuggerUrl: string };

  const page = await new Promise<Page>((resolve, reject) => {
    const ws = new WebSocket(target.webSocketDebuggerUrl);
    let next = 1;
    const pending = new Map<number, (v: any) => void>();
    ws.onerror = reject;
    ws.onmessage = (event) => {
      const msg = JSON.parse(String(event.data));
      if (msg.id && pending.has(msg.id)) {
        pending.get(msg.id)!(msg.result);
        pending.delete(msg.id);
      }
      if (msg.method === "Runtime.exceptionThrown") {
        consoleErrors.push(msg.params?.exceptionDetails?.exception?.description ?? "exception");
      }
      if (msg.method === "Runtime.consoleAPICalled" && msg.params?.type === "error") {
        consoleErrors.push(msg.params.args?.map((a: any) => a.value ?? a.description).join(" ") ?? "console.error");
      }
      if (msg.method === "Network.requestWillBeSent") {
        requests.push({ url: String(msg.params?.request?.url ?? ""), at: Date.now() });
      }
      if (msg.method === "Log.entryAdded") {
        const entry = msg.params?.entry ?? {};
        if (entry.source === "security" || /Content Security Policy/i.test(entry.text ?? "")) {
          refusedByPolicy.push(String(entry.text ?? "").slice(0, 160));
        }
      }
    };
    ws.onopen = () =>
      resolve({
        send: (method, params) =>
          new Promise((ok) => {
            const id = next++;
            pending.set(id, ok);
            ws.send(JSON.stringify({ id, method, params }));
          }),
        close: () => ws.close(),
      });
  });

  await page.send("Page.enable");
  await page.send("Runtime.enable");
  await page.send("Log.enable");
  await page.send("Network.enable");

  const evaluate = async (expression: string) => {
    const result = await page.send("Runtime.evaluate", {
      // Top-level await throws, so everything is wrapped.
      expression: `(async () => { ${expression} })()`,
      awaitPromise: true,
      returnByValue: true,
    });
    if (result?.exceptionDetails) {
      throw new Error(result.exceptionDetails.exception?.description ?? "evaluate failed");
    }
    return result?.result?.value;
  };

  /** Until something is on the page, and loud when it gives up. */
  const waitFor = async (selector: string, within = 10_000) => {
    const until = Date.now() + within;
    for (;;) {
      if (await evaluate(`return !!document.querySelector(${JSON.stringify(selector)})`)) return;
      if (Date.now() > until) {
        const said = consoleErrors.length ? `; the page said: ${consoleErrors.slice(-3).join(" | ")}` : "";
        const where = await evaluate(`return location.pathname + location.search`);
        throw new Error(`nothing matched ${selector} after ${within}ms at ${where}${said}`);
      }
      await sleep(80);
    }
  };

  /**
   * Go somewhere and wait for the thing that says you have arrived — and for
   * the screen being left to be gone, so the marker cannot match it.
   */
  const goTo = async (path: string, marker: string) => {
    await evaluate(`
      const app = document.getElementById("app");
      app?.firstElementChild?.setAttribute("data-leaving", "1");
      history.pushState({}, "", ${JSON.stringify(path)});
      dispatchEvent(new PopStateEvent("popstate"));
    `);
    await waitFor("#app > :not([data-leaving])");
    await waitFor(marker);
  };

  const check = (label: string, ok: boolean, detail = "") => {
    console.log(`  ${ok ? "✓" : "✗"} ${label}${detail ? ` — ${detail}` : ""}`);
    if (!ok) process.exitCode = 1;
  };

  /** A real key, through the protocol, not a synthetic event. */
  const key = async (name: string, code: string, vk: number, text = "") => {
    await page.send("Input.dispatchKeyEvent", { type: "keyDown", key: name, code, windowsVirtualKeyCode: vk, ...(text ? { text } : {}) });
    await page.send("Input.dispatchKeyEvent", { type: "keyUp", key: name, code, windowsVirtualKeyCode: vk });
  };

  const close = async () => {
    await fetch(`http://127.0.0.1:${CDP}/json/close/${target.id}`);
    page.close();
  };

  return { page, evaluate, waitFor, goTo, check, key, close, consoleErrors, refusedByPolicy, requests };
}
