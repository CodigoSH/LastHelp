// Package arch enforces the architectural rules of this project.
//
// These are tests and not documentation on purpose. A rule written in a README
// is a rule that gets broken during a late night refactor and discovered two
// years later; a rule written as a test fails in CI the same afternoon.
//
// Every failure below prints the offending file plus a paragraph explaining why
// the rule exists, because a rule whose reason has been lost is a rule the next
// person deletes.
//
// Several of these rules find nothing today, and that is the point of writing
// them first: a rule added after the violation is a rule written with the
// violation exempted.
//
// They read the repository from disk, so they must run with -count=1: the test
// cache does not track source files it was never told about, and a cached
// "pass" here is worthless.
package arch

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const module = "git.codigosh.com/CodigoSH/LastHelp"

type source struct {
	path string // repo-relative, slash-separated
	pkg  string // repo-relative directory
	fset *token.FileSet
	file *ast.File
}

// repoRoot walks up until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the working directory")
		}
		dir = parent
	}
}

// loadSources parses every .go file in the repository.
//
// Directories are skipped by their path from the root, never by base name. An
// earlier version of this in another project skipped by base name, and "web"
// excluded internal/web along with the TypeScript frontend — so every rule
// silently stopped covering the HTTP layer, which is precisely where an
// authorisation mistake does damage. Nothing failed, because there was nothing
// left to fail on.
func loadSources(t *testing.T) []source {
	t.Helper()
	root := repoRoot(t)

	skip := map[string]bool{
		".git":         true,
		"node_modules": true,
		"web":          true, // the TypeScript frontend, not internal/web
		"bin":          true,
		"docs":         true,
	}

	var out []source
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			if rel != "." && skip[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		out = append(out, source{
			path: rel,
			pkg:  filepath.ToSlash(filepath.Dir(rel)),
			fset: fset,
			file: parsed,
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// imports returns the repo-relative package paths this file imports from this
// module. Imports of the standard library and of dependencies come back as
// their literal path.
func (s source) imports() []string {
	var out []string
	for _, spec := range s.file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		out = append(out, strings.TrimPrefix(strings.TrimPrefix(path, module), "/"))
	}
	return out
}

// render prints the file back out, which is how the rules below look for a call
// without walking every expression shape it could hide in.
func (s source) render(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	if err := printer.Fprint(&b, s.fset, s.file); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func under(pkg string, prefixes []string) bool {
	for _, p := range prefixes {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
}

// TestLayering asserts that dependencies only ever point downwards.
//
// cmd wires things together and may see everything. internal/web is the HTTP
// layer and may use core and platform. core is the domain and knows nothing
// about HTTP. platform is the machinery — configuration, database, clock — and
// knows nothing about either.
//
// The rule exists because the cheapest way to fix a problem is always to reach
// upwards for the thing that already has the answer, and a domain that imports
// the web layer cannot be tested, reused or reasoned about without a server.
func TestLayering(t *testing.T) {
	forbidden := map[string][]string{
		"internal/core":     {"internal/web", "cmd"},
		"internal/platform": {"internal/core", "internal/web", "cmd"},
	}

	for _, f := range loadSources(t) {
		for layer, banned := range forbidden {
			if !under(f.pkg, []string{layer}) {
				continue
			}
			for _, imp := range f.imports() {
				if under(imp, banned) {
					t.Errorf("\n%s imports %s\n\n"+
						"Dependencies point downwards: cmd → web → core → platform.\n"+
						"%s must not know about %s. The cheapest fix for a problem is\n"+
						"always to reach upwards for the thing that already has the answer,\n"+
						"and that is exactly what turns a domain into something that cannot\n"+
						"be tested without starting a server.", f.path, imp, layer, imp)
				}
			}
		}
	}
}

// TestOnlyConfigReadsTheEnvironment asserts there is one door to the environment.
//
// .env.example claims to list every variable this binary reads. That claim is
// worth exactly as much as the number of places that can read one, and this
// pins it at one.
func TestOnlyConfigReadsTheEnvironment(t *testing.T) {
	const gate = "internal/platform/config"

	// cmd/sign is not the server. It is a build-time tool that runs in CI, and
	// the one thing it reads is the signing secret — which is not an operator
	// setting, does not belong in .env.example, and must never be readable from
	// anything that ships.
	//
	// Named here rather than left to the rule, so that the next thing added to
	// cmd/ has to argue for itself instead of inheriting an exemption.
	exempt := []string{"cmd/sign"}

	for _, f := range loadSources(t) {
		if f.pkg == gate || strings.HasSuffix(f.path, "_test.go") || under(f.pkg, exempt) {
			continue
		}
		src := f.render(t)
		for _, call := range []string{"os.Getenv", "os.LookupEnv", "os.Environ"} {
			if strings.Contains(src, call) {
				t.Errorf("\n%s uses %s\n\n"+
					"Everything this binary reads from the environment is named in\n"+
					"%s, once, so that the list of knobs is a file somebody\n"+
					"can read rather than a grep. A variable read anywhere else is a\n"+
					"variable .env.example does not mention.", f.path, call, gate)
			}
		}
	}
}

// TestCoreDoesNotCallTimeNow asserts there is one source of time.
//
// A domain that reads the clock directly cannot be tested against a Tuesday, a
// leap second or a subscription that expired yesterday, and the tests that try
// end up sleeping.
func TestCoreDoesNotCallTimeNow(t *testing.T) {
	for _, f := range loadSources(t) {
		if !under(f.pkg, []string{"internal/core"}) || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		if strings.Contains(f.render(t), "time.Now()") {
			t.Errorf("\n%s calls time.Now()\n\n"+
				"The domain takes the time it needs as an argument or from a clock\n"+
				"it was given. Reading the wall clock in here is what makes a test\n"+
				"have to sleep, and what makes a bug that only happens at midnight\n"+
				"impossible to write a test for.", f.path)
		}
	}
}

// TestNoDBHandleInCore asserts that core stores take an Execer, not a handle.
//
// A store that holds *sql.DB opens its own transactions, which means two of
// them cannot be made atomic with each other. Taking the narrower interface is
// what lets a caller hand them all the same transaction.
func TestNoDBHandleInCore(t *testing.T) {
	for _, f := range loadSources(t) {
		if !under(f.pkg, []string{"internal/core"}) || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		if strings.Contains(f.render(t), "*sql.DB") {
			t.Errorf("\n%s holds a *sql.DB\n\n"+
				"Stores take the narrow interface (Exec/Query/QueryRow), not the\n"+
				"handle. A store holding the handle starts its own transactions, and\n"+
				"two operations that must succeed or fail together no longer can.", f.path)
		}
	}
}

// TestOnlyAuthzDecidesPermissions asserts there is one place that decides who
// may do what.
//
// SECURITY.md and the README describe what each role can reach. That promise is
// worth exactly as much as the number of places that can violate it, and this
// pins that number at one. Handlers do not get an "if administrator" branch and
// no future endpoint decides for itself who may read whose reports.
func TestOnlyAuthzDecidesPermissions(t *testing.T) {
	const gate = "internal/core/authz"

	// The role names, as they are spelled in the database and in the UI. A
	// comparison against one of these outside the gate is a decision being made
	// somewhere it cannot be reviewed.
	roles := []string{`"administrator"`, `"owner"`, `"collaborator"`}

	for _, f := range loadSources(t) {
		if under(f.pkg, []string{gate}) || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		src := f.render(t)
		for _, role := range roles {
			if strings.Contains(src, role) {
				t.Errorf("\n%s mentions the role %s\n\n"+
					"Only %s decides permissions. A handler that compares\n"+
					"against a role is a second implementation of the access rules, and\n"+
					"the two drift the first time one of them is updated. Ask authz\n"+
					"whether this actor may do this thing.", f.path, role, gate)
			}
		}
	}
}

// TestTheRulesActuallySeeTheCode is the rule about the rules.
//
// A rule that inspects no files always passes. If loadSources ever stops
// reaching a directory — a skip that matches more than it meant to, a walk that
// returns early — every rule above goes green while covering nothing, and
// nothing fails, because there is nothing left to fail on.
//
// This asserts that the packages the other rules care most about are being read.
func TestTheRulesActuallySeeTheCode(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range loadSources(t) {
		seen[f.pkg] = true
	}

	// Directories that must exist and must be parsed. The HTTP layer is on this
	// list because it is where an authorisation mistake does damage.
	required := []string{
		"cmd/lasthelp",
		"internal/web",
		"internal/platform/config",
		"internal/version",
	}
	for _, dir := range required {
		if !seen[dir] {
			t.Errorf("\nthe rules did not read any file under %s\n\n"+
				"Every rule in this package inspects the files loadSources returns.\n"+
				"A directory it does not reach is a directory none of them cover, and\n"+
				"they all keep passing. Either the directory moved and this list is\n"+
				"stale, or the walk stopped reaching it — the second is the dangerous\n"+
				"one.", dir)
		}
	}

	if len(seen) < 4 {
		t.Errorf("loadSources found only %d package directories; the walk is not reaching the repository", len(seen))
	}
}
