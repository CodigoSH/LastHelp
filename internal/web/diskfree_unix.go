//go:build linux || darwin

package web

import "syscall"

// diskFree is the space left on the filesystem holding a path, and its size.
//
// syscall.Statfs rather than a library: the standard library has it on both
// systems this is built for, and a dependency added to print one number on one
// screen is a dependency to keep forever.
//
// The field types differ between them — Bsize is int64 on Linux and uint32 on
// macOS — so the conversion is explicit, which compiles on both.
func diskFree(path string) (free, total uint64, ok bool) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, 0, false
	}
	block := uint64(fs.Bsize)
	return block * fs.Bavail, block * fs.Blocks, true
}
