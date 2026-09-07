/**
 * Two things no catalogue comparison can see.
 *
 * 1. Keys the code asks for that no catalogue has. Those render as the key
 *    itself — `board.report` in the middle of a page.
 * 2. A second interpolator. `t()` wraps every interpolated value in bidi
 *    isolates; a call site that takes the translated sentence and runs its own
 *    `.replace()` skips that, and the same list then renders with two different
 *    alignments in Arabic depending on how the line starts. This house has
 *    shipped that bug twice, which is why the check is a grep and why it names
 *    the file and line.
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const master = JSON.parse(readFileSync("web/public/locales/en.json", "utf-8")) as Record<string, string>;
const known = new Set(Object.keys(master));

function sources(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out.push(...sources(full));
    else if (full.endsWith(".ts") && !full.endsWith(".test.ts")) out.push(full);
  }
  return out;
}

const usedKey = /\bt\(\s*["'`]([a-zA-Z0-9_.]+)["'`]/g;
// A key built at the call site: t(`status.${item.status}`). The prefix is what
// is knowable, and every key under it counts as used.
//
// Without this the scan reports every status and every kind as unused, which is
// how a scan trains people to ignore it.
const usedPrefix = /\bt\(\s*`([a-zA-Z0-9_.]*\.)\$\{/g;
// A .replace() applied to something that came out of t().
const secondInterpolator = /t\([^)]*\)\s*\.replace\(|\bconst\s+\w+\s*=\s*t\([^)]*\)[\s\S]{0,120}?\.replace\(/;

let problems = 0;
const used = new Set<string>();
const prefixes = new Set<string>();

let blob = "";

for (const file of sources("web/src")) {
  const text = readFileSync(file, "utf-8");
  blob += text;

  for (const match of text.matchAll(usedKey)) {
    const key = match[1];
    if (key) used.add(key);
  }
  for (const match of text.matchAll(usedPrefix)) {
    const prefix = match[1];
    if (prefix) prefixes.add(prefix);
  }

  // Both forms, and the second one is why this is not a per-line test: a call
  // site that parks the translated sentence in a variable and calls .replace()
  // a few lines later skips the isolates just as thoroughly. That pattern was
  // written here from the start and nothing ever ran it — `void
  // secondInterpolator` at the foot of the file was silencing the warning that
  // said so.
  for (const match of text.matchAll(new RegExp(secondInterpolator, "g"))) {
    const line = text.slice(0, match.index).split("\n").length;
    problems++;
    console.log(
      `${file}:${line}: a second interpolator\n` +
        `  t() isolates every interpolated value so a name or a number keeps its own\n` +
        `  direction inside an Arabic sentence. A .replace() on the translated string\n` +
        `  skips that. Pass the value to t() as a parameter instead.`,
    );
  }
}

// Keys the server emits. A handler answers with a translation key rather than a
// sentence — it does not know what language the reader chose — and the page
// looks it up through a variable, which no grep can follow. Flagging these as
// unused is how a scan trains people to ignore it.
const serverEmitted = ["error."];

const coveredByPrefix = (key: string) =>
  [...prefixes, ...serverEmitted].some((p) => key.startsWith(p));

const unknown = [...used].filter((key) => !known.has(key));
if (unknown.length) {
  problems++;
  console.log(`\nkeys used in the code that no catalogue has (${unknown.length}):`);
  for (const key of unknown.sort()) console.log(`  ${key}`);
}

// The unused report is deliberately more lenient than the unknown-key one. A
// key handed to a small wrapper — `say("panel.issueQueued")`, which passes it
// to t() — is used, and reporting it as dead is how a scan trains people to
// ignore it. So for this direction the question is only whether the literal
// appears in the sources at all. The strict t() set still drives the report
// above, where precision is what catches a typo.
const mentioned = (key: string) =>
  blob.includes(`"${key}"`) || blob.includes(`'${key}'`) || blob.includes(`\`${key}\``);

const unused = [...known].filter(
  (key) => !used.has(key) && !mentioned(key) && !coveredByPrefix(key),
);
if (unused.length) {
  console.log(`\nkeys in the catalogues that nothing uses (${unused.length}):`);
  for (const key of unused.sort()) console.log(`  ${key}`);
}

if (problems === 0) console.log(`\nno unknown keys, no second interpolator`);
else process.exitCode = 1;

