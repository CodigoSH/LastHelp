//go:build !linux && !darwin

package web

// diskFree has no answer on a system this is not built for. The screen says so
// rather than printing a zero, which reads as a full disk.
func diskFree(string) (free, total uint64, ok bool) { return 0, 0, false }
