/**
 * Frontend build.
 *
 * Bun bundles the sources into web/dist, which is then mirrored into
 * internal/web/dist so the Go binary can embed it. The mirror is not a
 * convenience: go:embed cannot reach outside its own package directory, and a
 * half-finished copy would ship stale assets inside a release binary, so any
 * failure here has to be loud.
 */
import { build } from "bun";
import { cp, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { createHash } from "node:crypto";

const OUT = "./web/dist";
const EMBED = "./internal/web/dist";

// The stamp is a digest of everything this build produced, not the clock.
//
// It has one job: anything fetched with ?v=<stamp> is served immutable, so the
// stamp must be identical across every asset of a build and different on the
// next one that changes anything. A timestamp does that and one thing more —
// it makes every build unique even when the sources are identical, which means
// two builds of the same commit produce two different binaries and the promise
// that you can rebuild a release and compare hashes is not true.
//
// So the assets are assembled with a placeholder, hashed, and the placeholder
// is replaced by the hash. Same sources, same bytes; one character changed
// anywhere in the output, new stamp.
const PLACEHOLDER = "0BUILDSTAMP0";

console.log("building frontend…");

// cp() never removes files, so leftovers from an earlier build would linger and
// get embedded. Start clean.
await rm(OUT, { recursive: true, force: true });
await mkdir(OUT, { recursive: true });

const result = await build({
  entrypoints: ["./web/src/main.ts"],
  outdir: OUT,
  minify: true,
  sourcemap: "none",
  target: "browser",
  naming: "[dir]/[name].js",
  define: {
    "process.env.NODE_ENV": JSON.stringify("production"),
    __BUILD_STAMP__: JSON.stringify(PLACEHOLDER),
  },
});

if (!result.success) {
  console.error("frontend build failed:");
  for (const message of result.logs) console.error(message);
  process.exit(1);
}

const copyVersioned = async (src: string, dest: string) => {
  const content = (await readFile(src, "utf-8")).replace(/(\?v=)[0-9a-zA-Z]+/g, `$1${PLACEHOLDER}`);
  await writeFile(dest, content);
};

// Every file under a directory, sorted, so the digest does not depend on the
// order the filesystem happens to return.
const filesUnder = async (dir: string): Promise<string[]> => {
  const entries = await readdir(dir, { withFileTypes: true });
  const found: string[] = [];
  for (const entry of entries.sort((a, b) => (a.name < b.name ? -1 : 1))) {
    const full = `${dir}/${entry.name}`;
    if (entry.isDirectory()) found.push(...(await filesUnder(full)));
    else found.push(full);
  }
  return found;
};

try {
  await copyVersioned("./web/index.html", `${OUT}/index.html`);
  if (existsSync("./web/styles")) await cp("./web/styles", `${OUT}/styles`, { recursive: true });
  // The catalogues live under public/locales and are fetched at runtime, one
  // per session, so they are copied rather than bundled.
  if (existsSync("./web/public")) await cp("./web/public", OUT, { recursive: true });

  // The web manifest and the app icons are plain files in web/public, copied
  // above with everything else — the dashboard's arrangement. No service
  // worker, on purpose: the app installs but does not work offline, because a
  // portal needs the network to report or vote, and a cached shell is the one
  // that serves last week's bundle after an update.

  // Everything is now in place with the placeholder in it. Hash the lot —
  // paths included, so moving a file changes the stamp — and swap it in.
  const digest = createHash("sha256");
  const built = await filesUnder(OUT);
  for (const file of built) {
    digest.update(file.slice(OUT.length));
    digest.update(await readFile(file));
  }
  const buildStamp = digest.digest("hex").slice(0, 16);

  let stamped = 0;
  for (const file of built) {
    const content = await readFile(file);
    if (!content.includes(PLACEHOLDER)) continue;
    await writeFile(file, content.toString("utf-8").replaceAll(PLACEHOLDER, buildStamp));
    stamped++;
  }
  if (stamped === 0) {
    // index.html and the bundle both carry it. None
    // carrying it means the substitution silently stopped working and every
    // asset is about to be served immutable under one name for ever.
    throw new Error("nothing carried the build stamp: the substitution is broken");
  }

  await rm(EMBED, { recursive: true, force: true });
  await mkdir(EMBED, { recursive: true });
  await cp(OUT, EMBED, { recursive: true });
  await writeFile(`${EMBED}/.gitkeep`, "");

  console.log(`frontend built (stamp ${buildStamp}) and synced to ${EMBED}`);
} catch (e) {
  console.error("failed to assemble the frontend output:", e);
  process.exit(1);
}
