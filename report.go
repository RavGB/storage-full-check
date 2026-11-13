package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// ReportData bundles everything one report prints, so the text
// renderer and the JSON renderer share a single source.
type ReportData struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Options     ReportOpts  `json:"options"`
	Filesystem  FSInfo      `json:"filesystem"`
	Scan        *ScanResult `json:"scan"`
}

// ReportOpts echoes the settings a scan ran with.
type ReportOpts struct {
	Top     int   `json:"top"`
	Workers int   `json:"workers"`
	MinSize int64 `json:"min_size_bytes"`
	OldDays int   `json:"old_days"`
}

// writeTextReport renders the human-readable report.
func writeTextReport(w io.Writer, data ReportData) {
	s := data.Scan
	fmt.Fprintln(w, "Storage full check")
	fmt.Fprintln(w, "RavGB - hello@ravgb.com - https://ravgb.com")
	fmt.Fprintf(w, "Generated: %s\n", data.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Scanned: %s\n", s.Root)
	fmt.Fprintln(w, strings.Repeat("-", 64))

	fmt.Fprintln(w, "Filesystem (from the operating system):")
	if data.Filesystem.Available {
		fs := data.Filesystem
		fmt.Fprintf(w, "  Total: %s\n", formatBytes(int64(fs.Total)))
		fmt.Fprintf(w, "  Used:  %s (%.1f%%)\n", formatBytes(int64(fs.Used)), fs.UsedPct)
		fmt.Fprintf(w, "  Free:  %s\n", formatBytes(int64(fs.Free)))
	} else {
		fmt.Fprintln(w, "  (skipped: not available on this system)")
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Scan totals (files counted under the scanned folder):")
	fmt.Fprintf(w, "  Files:       %d\n", s.Files)
	fmt.Fprintf(w, "  Directories: %d\n", s.Directories)
	fmt.Fprintf(w, "  Total size:  %s\n", formatBytes(s.Bytes))
	fmt.Fprintf(w, "  Read errors: %d\n", s.Errors)
	for _, p := range s.ErrorSample {
		fmt.Fprintf(w, "    error at: %s\n", p)
	}
	// Files stored directly in the root are part of no child
	// folder, so name their share explicitly.
	for _, d := range s.AllDirs {
		if d.Depth == 0 && d.DirectFiles > 0 {
			fmt.Fprintf(w, "  Stored directly in the scanned folder itself: %s in %d file(s) (%.1f%% of the scanned total)\n",
				formatBytes(d.DirectBytes), d.DirectFiles, sharePercent(d.DirectBytes, s.Bytes))
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Largest files (top %d):\n", len(s.LargestFiles))
	if len(s.LargestFiles) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, f := range s.LargestFiles {
		fmt.Fprintf(w, "  %-14s %s\n", formatBytes(f.Size), f.Path)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Largest folders (top %d, subtree totals):\n", len(s.LargestDirs))
	if len(s.LargestDirs) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, d := range s.LargestDirs {
		fmt.Fprintf(w, "  %-14s %6.1f%%  %s\n", formatBytes(d.Bytes), sharePercent(d.Bytes, s.Bytes), d.Path)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Folders directly under the scanned folder:")
	if len(s.RootChildren) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, d := range s.RootChildren {
		fmt.Fprintf(w, "  %-14s %6.1f%%  %s\n", formatBytes(d.Bytes), sharePercent(d.Bytes, s.Bytes), displayPath(s.Root, d.Path))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "By extension (top 15 by size):")
	if len(s.ByExtension) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, e := range s.ByExtension {
		fmt.Fprintf(w, "  %-16s %6d file(s)  %s\n", e.Ext, e.Files, formatBytes(e.Bytes))
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Review candidates (at least %s and not modified for at least %d days):\n",
		formatBytes(data.Options.MinSize), data.Options.OldDays)
	if len(s.Review) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, f := range s.Review {
		fmt.Fprintf(w, "  %-14s last modified %s  %s\n",
			formatBytes(f.Size), f.ModTime.Format("2006-01-02"), f.Path)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Size histogram:")
	for _, b := range s.Histogram {
		fmt.Fprintf(w, "  %-14s %6d file(s)  %s\n", b.Label, b.Files, formatBytes(b.Bytes))
	}
	fmt.Fprintln(w)

	// Closing summary: the top three folders and their combined
	// share. The scan root itself is excluded, because it is
	// always 100% and says nothing.
	var top []DirTotal
	for _, d := range s.LargestDirs {
		if d.Path == s.Root {
			continue
		}
		top = append(top, d)
		if len(top) == 3 {
			break
		}
	}
	if len(top) == 0 {
		fmt.Fprintln(w, "Summary: no folders under the scanned folder hold any counted files.")
		return
	}
	var combined int64
	names := make([]string, 0, len(top))
	for _, d := range top {
		combined += d.Bytes
		names = append(names, fmt.Sprintf("%s (%s, %.1f%%)", displayPath(s.Root, d.Path), formatBytes(d.Bytes), sharePercent(d.Bytes, s.Bytes)))
	}
	fmt.Fprintf(w, "Summary: the biggest folders are %s. Together they hold %s, which is %.1f%% of the scanned total.\n",
		strings.Join(names, ", "), formatBytes(combined), sharePercent(combined, s.Bytes))
}

// displayPath shows a path relative to the scan root when it is
// under it, which keeps report lines short.
func displayPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}
