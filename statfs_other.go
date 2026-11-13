//go:build !linux && !darwin

package main

// filesystemInfo is a stub on systems without a statfs
// implementation in this tool; the report marks the section as
// skipped and the tree scan still runs.
func filesystemInfo(path string) FSInfo {
	return FSInfo{Available: false, Path: path}
}
