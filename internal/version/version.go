// Package version carries what this binary knows about its own provenance.
package version

// Current is stamped at build time with -ldflags. The fallback deliberately
// says "dev": a binary that cannot say where it came from should not claim a
// version number, because that number ends up in bug reports.
var (
	Current = "dev"
	Commit  = "unknown"
)
