//go:build darwin

package main

import "syscall"

// filesystemInfo reads total and available space with statfs(2).
func filesystemInfo(path string) FSInfo {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return FSInfo{Available: false, Path: path}
	}
	bsize := uint64(st.Bsize)
	total := st.Blocks * bsize
	free := st.Bavail * bsize
	used := uint64(0)
	if total >= free {
		used = total - free
	}
	pct := 0.0
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	return FSInfo{
		Available: true,
		Path:      path,
		Total:     total,
		Free:      free,
		Used:      used,
		UsedPct:   pct,
	}
}
