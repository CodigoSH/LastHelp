/**
 * A per-language report: how much is translated, and how much is still the
 * English sentence sitting in a catalogue that is not English.
 *
 * The second number is the one that matters. A missing key is loud — it falls
 * back and somebody notices. A key whose value is the English string looks
 * translated to every automatic check and looks neglected to the one person who
 * reads that language.
 */
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const dir = "web/public/locales";
const master = JSON.parse(readFileSync(join(dir, "en.json"), "utf-8")) as Record<string, string>;
const total = Object.keys(master).length;

const rows: Array<{ code: string; done: number; english: number; missing: number }> = [];

for (const file of readdirSync(dir).sort()) {
  if (!file.endsWith(".json") || file === "en.json") continue;
  const code = file.replace(".json", "");
  const table = JSON.parse(readFileSync(join(dir, file), "utf-8")) as Record<string, string>;

  let english = 0;
  let missing = 0;
  for (const key of Object.keys(master)) {
    const value = table[key];
    if (!value?.trim()) missing++;
    else if (value === master[key]) english++;
  }
  rows.push({ code, done: total - missing, english, missing });
}

console.log(`${total} keys in en.json\n`);
console.log("lang   translated   same as en   missing");
for (const r of rows) {
  console.log(
    `${r.code.padEnd(6)} ${String(r.done).padStart(10)} ${String(r.english).padStart(12)} ${String(r.missing).padStart(9)}`,
  );
}

const suspicious = rows.filter((r) => r.missing > 0);
if (suspicious.length) process.exitCode = 1;
