package main

// FSInfo describes the filesystem that holds a path, as reported by
// the operating system rather than by walking the tree. Used is
// Total-Free, so the three figures always add up; on filesystems
// with blocks reserved for the system that reserved space is part of
// Used by this definition.
type FSInfo struct {
	Available bool    `json:"available"`
	Path      string  `json:"path"`
	Total     uint64  `json:"total_bytes"`
	Free      uint64  `json:"free_bytes"`
	Used      uint64  `json:"used_bytes"`
	UsedPct   float64 `json:"used_percent"`
}
