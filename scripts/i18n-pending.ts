/**
 * Rewrites the list of keys waiting for translation.
 *
 * While a screen is still being designed only en.json is written, because
 * translating a sentence that is about to be rewritten is work done twice. What
 * is waiting is listed rather than quietly missing, and an architecture rule
 * keeps that list exact: it refuses a key that every catalogue already has, and
 * one that no longer exists.
 *
 * Run this after adding or removing English strings. The list has to be empty
 * before a release.
 */
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const dir = "web/public/locales";
const listFile = join(dir, "UNTRANSLATED.txt");

const master = JSON.parse(readFileSync(join(dir, "en.json"), "utf-8")) as Record<string, string>;

const pending = new Set<string>();
for (const file of readdirSync(dir)) {
  if (!file.endsWith(".json") || file === "en.json") continue;
  const table = JSON.parse(readFileSync(join(dir, file), "utf-8")) as Record<string, string>;
  for (const key of Object.keys(master)) {
    if (!(table[key] ?? "").trim()) pending.add(key);
  }
}

const header = `# Keys waiting for translation.
#
# The screens they belong to are still being designed, and translating a
# sentence that is about to be rewritten is work done twice. They wait here
# rather than being quietly missing: this file is in the repository, it shows up
# in every diff, and an architecture rule refuses to let it keep a key that is
# already translated everywhere or one that no longer exists.
#
# Regenerate with: bun run i18n:pending
# Nothing here ships. The list has to be empty before a release.
`;

writeFileSync(listFile, header + [...pending].sort().join("\n") + (pending.size ? "\n" : ""));
console.log(`${pending.size} key(s) waiting for translation`);
