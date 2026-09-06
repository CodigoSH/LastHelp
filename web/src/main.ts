/**
 * Frontend entry point.
 *
 * The build replaces __BUILD_STAMP__ with a digest of everything the build
 * produced, so it changes when the output changes and only then.
 */
declare const __BUILD_STAMP__: string;

// Injected by the bundler's define. Files that run outside the bundle (tests)
// have no such identifier, so the guard is what keeps them from throwing.
const buildStamp = typeof __BUILD_STAMP__ === "string" ? __BUILD_STAMP__ : "dev";

const app = document.getElementById("app");
if (app) {
  app.textContent = `LastHelp (${buildStamp})`;
}
