/**
 * Reports which keys are missing from a catalogue and which it has that English
 * does not.
 *
 * It reports. It does not fill anything in: a missing key filled with the
 * English sentence is a locale that looks complete and reads as neglected, and
 * every audit after that stops noticing. Translating by hand into all twenty is
 * the rule, and this is the tool that says what still needs it.
 */
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const dir = "web/public/locales";
const master = JSON.parse(readFileSync(join(dir, "en.json"), "utf-8")) as Record<string, string>;
const masterKeys = new Set(Object.keys(master));

let problems = 0;

for (const file of readdirSync(dir).sort()) {
  if (!file.endsWith(".json") || file === "en.json") continue;
  const code = file.replace(".json", "");
  const table = JSON.parse(readFileSync(join(dir, file), "utf-8")) as Record<string, string>;
  const keys = new Set(Object.keys(table));

  const missing = [...masterKeys].filter((k) => !keys.has(k));
  const extra = [...keys].filter((k) => !masterKeys.has(k));
  const empty = [...keys].filter((k) => !table[k]?.trim());

  if (missing.length || extra.length || empty.length) {
    problems++;
    console.log(`\n${code}`);
    if (missing.length) console.log(`  missing (${missing.length}): ${missing.join(", ")}`);
    if (extra.length) console.log(`  not in en (${extra.length}): ${extra.join(", ")}`);
    if (empty.length) console.log(`  empty (${empty.length}): ${empty.join(", ")}`);
  }
}

if (problems === 0) {
  console.log(`all catalogues match en.json (${masterKeys.size} keys)`);
} else {
  process.exitCode = 1;
}
