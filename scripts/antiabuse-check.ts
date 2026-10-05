/**
 * The anti-abuse screen, walked as the administrator.
 *
 * A word is added from a form, kept once whatever its capitals and accents, edited
 * in its row and removed after asking; and a report that carries a listed word is
 * refused, and the screen says so beside the word and in the refusals by check.
 *
 * Its own pass, on `scripts/seed`, because every step here reloads the screen and
 * the main journey already sits near the per-address ceiling of requests a minute.
 * Run with `make drive-antiabuse`.
 */
import { BASE, connect, sleep } from "./drive-harness";

const PASSWORD = "una contraseña larga de verdad";

async function main() {
  const { page, evaluate, waitFor, goTo, check, close, consoleErrors, refusedByPolicy } = await connect();
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 860, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: BASE + "/" });
  await sleep(1200);

  // A stranger sends a report, before anybody is signed in.
  const send = (title: string) =>
    evaluate(`
      const base = "/api/orgs/acme/repos/atlas";
      const csrf = document.cookie.split("; ").find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1] ?? "";
      const form = await fetch(base + "/form", { credentials: "same-origin" }).then(r => r.json());
      await new Promise(r => setTimeout(r, 2800));
      const r = await fetch(base + "/items", { method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ kind: "idea", token: form.token, website: "", email: "",
          title: ${JSON.stringify(title)}, body: "what happened for the word list check" }) });
      return r.status;
    `);

  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    const r = await fetch("/api/session", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ username: "alex", password: ${JSON.stringify(PASSWORD)} }) });
    if (!r.ok) throw new Error("sign-in refused: " + r.status);
  `);
  await page.send("Page.navigate", { url: BASE + "/panel" });
  await waitFor(".admin-main");

  await goTo("/panel/antiabuse", "form.word-add");
  const palabras = await evaluate(`
    const wait = async (f) => { for (let i = 0; i < 50 && !f(); i++) await new Promise(r => setTimeout(r, 100)); return f(); };
    const rows = () => [...document.querySelectorAll(".settings-list.words .si-name")].map(e => e.textContent);
    const add = async (w) => {
      const f = document.querySelector("form.word-add");
      f.querySelector("#new-word").value = w;
      f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
      await wait(() => rows().includes(w.trim()));
    };
    await add(" Casino ");
    await add("crypto");
    const added = rows();
    // The same word with other capitals and an accent is the same word.
    const f = document.querySelector("form.word-add");
    f.querySelector("#new-word").value = "CASIÑO";
    f.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await new Promise(r => setTimeout(r, 900));
    const afterRepeat = rows();
    document.querySelector('[data-edit-word="crypto"]').click();
    const edit = document.querySelector("form.word-edit input");
    edit.value = "bitcoin";
    document.querySelector("form.word-edit").dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    await wait(() => rows().includes("bitcoin"));
    const afterEdit = rows();
    // Taking one off asks first, in the product's own dialog.
    document.querySelector('[data-remove-word="bitcoin"]').click();
    await new Promise(r => setTimeout(r, 300));
    const asked = !!document.querySelector("#confirm-dialog[open], #confirm-dialog");
    document.querySelector('#confirm-dialog [data-confirm="yes"]')?.click();
    await wait(() => !rows().includes("bitcoin"));
    return { added, afterRepeat, afterEdit, afterRemove: rows(), asked };
  `);
  check(
    "a word is added from a form, kept once whatever its capitals and accents, edited in its row and removed after asking",
    JSON.stringify(palabras.added) === '["Casino","crypto"]' && palabras.afterRepeat.length === 2 &&
      JSON.stringify(palabras.afterEdit) === '["Casino","bitcoin"]' && JSON.stringify(palabras.afterRemove) === '["Casino"]' &&
      palabras.asked === true,
    JSON.stringify(palabras),
  );

  // A report with the word is refused, and the screen counts it twice over: beside
  // the word, and in the refusals by check for that repository.
  const refused = await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/session", { method: "DELETE", headers: { "X-CSRF-Token": csrf() } });
    return true;
  `);
  void refused;
  const status = await send("great CASIÑO deals for you");
  check("a stranger's report with a listed word is refused", status === 400, String(status));

  await evaluate(`
    const csrf = () => document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const r = await fetch("/api/session", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
      body: JSON.stringify({ username: "alex", password: ${JSON.stringify(PASSWORD)} }) });
    if (!r.ok) throw new Error("sign-in refused: " + r.status);
  `);
  await page.send("Page.navigate", { url: BASE + "/panel/antiabuse" });
  await waitFor("form.word-add");
  const counted = await evaluate(`
    const word = [...document.querySelectorAll(".settings-list.words .settings-item")].find(r => r.dataset.word === "Casino");
    const layer = [...document.querySelectorAll(".settings-list.layers .settings-item")].find(r => r.textContent.includes("Atlas"));
    return { word: word?.querySelector(".mono-tag")?.textContent ?? "", layer: layer?.textContent.replace(/\\s+/g, " ").trim() ?? "" };
  `);
  check(
    "the screen counts it beside the word and in the refusals by check",
    /1$/.test(counted.word.trim()) && /Atlas/.test(counted.layer) && /1\s*$/.test(counted.layer),
    JSON.stringify(counted),
  );

  // A long list is paged and searched: ten to a page, the range said, the last
  // page shorter, a search that narrows and says when nothing matches.
  await evaluate(`
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    const words = Array.from({ length: 23 }, (_, i) => "word" + String(i).padStart(2, "0"));
    words.push("Casiño");
    await fetch("/api/antiabuse/words", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ words }) });
  `);
  await page.send("Page.navigate", { url: BASE + "/panel/antiabuse" });
  await waitFor("form.word-add");
  const paged = await evaluate(`
    const names = () => [...document.querySelectorAll(".settings-list.words .si-name")].map(e => e.textContent);
    const out = { first: names().length, firstOfPrev: document.querySelector(".pager-prev")?.disabled };
    document.querySelector(".pager-next").click();
    out.second = names().length;
    document.querySelector(".pager-next").click();
    out.third = names().length; out.lastNext = document.querySelector(".pager-next").disabled;
    const q = document.querySelector(".word-search input");
    q.value = "CASINO"; q.dispatchEvent(new Event("input", { bubbles: true }));
    out.found = names(); out.pagerGone = !document.querySelector(".pager");
    q.value = "nothing-like-this"; q.dispatchEvent(new Event("input", { bubbles: true }));
    out.empty = document.querySelectorAll(".settings-list.words").length === 0 && document.querySelector(".word-results").textContent.includes("nothing-like-this");
    return out;
  `);
  check(
    "a long list is shown ten to a page, searched without caring for capitals or accents, and says when nothing matches",
    paged.first === 10 && paged.firstOfPrev === true && paged.second === 10 && paged.third === 4 && paged.lastNext === true &&
      JSON.stringify(paged.found) === '["Casiño"]' && paged.pagerGone === true && paged.empty === true,
    JSON.stringify(paged),
  );

  // The same card as every other screen of the panel, not a wider one.
  const widthOf = async (path: string, marker: string) => {
    await goTo(path, marker);
    return evaluate(`return Math.round(document.querySelector(".settings-card").getBoundingClientRect().width)`);
  };
  const reference = await widthOf("/panel/personalization", "form.instance-settings");
  const anti = await widthOf("/panel/antiabuse", "form.word-add");
  const notif = await widthOf("/panel/notifications", "form.channel");
  check("the anti-abuse and notifications cards are as wide as the rest of the panel's", anti === reference && notif === reference, JSON.stringify({ reference, anti, notif }));

  await evaluate(`
    const csrf = document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("lasthelp_csrf="))?.split("=")[1];
    await fetch("/api/antiabuse/words", { method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ words: [] }) });
  `);
  check("no console errors", consoleErrors.length === 0, consoleErrors.slice(0, 3).join(" | "));
  check("and the content policy refused nothing", refusedByPolicy.length === 0, refusedByPolicy.slice(0, 3).join(" | "));
  await close();
}

await main();

export {};
