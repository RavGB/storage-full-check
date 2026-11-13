package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSizedFile creates a file of exactly size bytes.
func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, size)
	for i := range data {
		data[i] = 'x'
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildFixture builds this tree under a temp dir:
//
//	root/a.txt        100 bytes
//	root/plain         50 bytes (no extension)
//	root/sub/b.txt    300 bytes
//	root/sub/deep/c.bin 200 bytes
//	root/empty/         (empty directory)
//
// Exact totals: 4 files, 650 bytes.
func buildFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "a.txt"), 100)
	writeSizedFile(t, filepath.Join(root, "plain"), 50)
	writeSizedFile(t, filepath.Join(root, "sub", "b.txt"), 300)
	writeSizedFile(t, filepath.Join(root, "sub", "deep", "c.bin"), 200)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func scanFixture(t *testing.T, root string) *ScanResult {
	t.Helper()
	res, err := Scan(ScanOptions{
		Path:    root,
		Workers: 4,
		Top:     20,
		MinSize: 100,
		OldDays: 180,
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return res
}

func TestScanExactTotals(t *testing.T) {
	root := buildFixture(t)
	res := scanFixture(t, root)

	if res.Bytes != 650 {
		t.Errorf("total bytes = %d, want 650", res.Bytes)
	}
	if res.Files != 4 {
		t.Errorf("file count = %d, want 4", res.Files)
	}
	// Directories visited: root, sub, sub/deep, empty = 4.
	if res.Directories != 4 {
		t.Errorf("directory count = %d, want 4", res.Directories)
	}
	if res.Errors != 0 {
		t.Errorf("errors = %d, want 0", res.Errors)
	}
}

func TestScanLargestFileOrder(t *testing.T) {
	root := buildFixture(t)
	res := scanFixture(t, root)

	wantSizes := []int64{300, 200, 100, 50}
	if len(res.LargestFiles) != len(wantSizes) {
		t.Fatalf("largest files = %d entries, want %d", len(res.LargestFiles), len(wantSizes))
	}
	for i, want := range wantSizes {
		if res.LargestFiles[i].Size != want {
			t.Errorf("largest[%d] size = %d, want %d (%s)", i, res.LargestFiles[i].Size, want, res.LargestFiles[i].Path)
		}
	}
	if filepath.Base(res.LargestFiles[0].Path) != "b.txt" {
		t.Errorf("largest file = %s, want b.txt", res.LargestFiles[0].Path)
	}
}

func TestScanSubtreeTotals(t *testing.T) {
	root := buildFixture(t)
	res := scanFixture(t, root)

	byPath := make(map[string]DirTotal)
	for _, d := range res.AllDirs {
		byPath[d.Path] = d
	}
	checks := map[string]int64{
		root:                               650,
		filepath.Join(root, "sub"):         500,
		filepath.Join(root, "sub", "deep"): 200,
		filepath.Join(root, "empty"):       0,
	}
	for path, want := range checks {
		got, ok := byPath[path]
		if !ok {
			t.Errorf("directory %s missing from results", path)
			continue
		}
		if got.Bytes != want {
			t.Errorf("subtree bytes for %s = %d, want %d", path, got.Bytes, want)
		}
	}
	// The root must be the largest folder, then sub.
	if len(res.LargestDirs) == 0 || res.LargestDirs[0].Path != root {
		t.Errorf("largest folder is not the scan root: %+v", res.LargestDirs)
	}
	// Immediate children of the root: sub (500) and empty (0).
	if len(res.RootChildren) != 2 {
		t.Fatalf("root children = %d, want 2 (%v)", len(res.RootChildren), res.RootChildren)
	}
	if res.RootChildren[0].Path != filepath.Join(root, "sub") || res.RootChildren[0].Bytes != 500 {
		t.Errorf("first root child = %+v, want sub with 500 bytes", res.RootChildren[0])
	}
}

func TestScanByExtension(t *testing.T) {
	root := buildFixture(t)
	res := scanFixture(t, root)

	byExt := make(map[string]ExtStat)
	for _, e := range res.ByExtension {
		byExt[e.Ext] = e
	}
	if got := byExt[".txt"]; got.Files != 2 || got.Bytes != 400 {
		t.Errorf(".txt = %+v, want 2 files / 400 bytes", got)
	}
	if got := byExt[".bin"]; got.Files != 1 || got.Bytes != 200 {
		t.Errorf(".bin = %+v, want 1 file / 200 bytes", got)
	}
	if got := byExt["(no extension)"]; got.Files != 1 || got.Bytes != 50 {
		t.Errorf("(no extension) = %+v, want 1 file / 50 bytes", got)
	}
	// Sorted by bytes descending: .txt first.
	if res.ByExtension[0].Ext != ".txt" {
		t.Errorf("first extension = %s, want .txt", res.ByExtension[0].Ext)
	}
}

func TestLargestTieBreakByPath(t *testing.T) {
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "m.txt"), 100)
	writeSizedFile(t, filepath.Join(root, "a.txt"), 100)
	writeSizedFile(t, filepath.Join(root, "z.txt"), 100)

	res := scanFixture(t, root)
	if len(res.LargestFiles) != 3 {
		t.Fatalf("largest files = %d, want 3", len(res.LargestFiles))
	}
	wantOrder := []string{"a.txt", "m.txt", "z.txt"}
	for i, name := range wantOrder {
		if filepath.Base(res.LargestFiles[i].Path) != name {
			t.Errorf("largest[%d] = %s, want %s (equal sizes must sort by path)",
				i, filepath.Base(res.LargestFiles[i].Path), name)
		}
	}
}

func TestComputeSubtreeTotalsPure(t *testing.T) {
	dirs := []DirTotal{
		{Path: "/r/sub/deep", Depth: 2, DirectBytes: 200, DirectFiles: 1},
		{Path: "/r", Depth: 0, DirectBytes: 150, DirectFiles: 2},
		{Path: "/r/sub", Depth: 1, DirectBytes: 300, DirectFiles: 1},
	}
	out := computeSubtreeTotals(dirs)
	byPath := make(map[string]DirTotal)
	for _, d := range out {
		byPath[d.Path] = d
	}
	if byPath["/r"].Bytes != 650 || byPath["/r"].Files != 4 {
		t.Errorf("/r = %d bytes / %d files, want 650 / 4", byPath["/r"].Bytes, byPath["/r"].Files)
	}
	if byPath["/r/sub"].Bytes != 500 {
		t.Errorf("/r/sub = %d bytes, want 500", byPath["/r/sub"].Bytes)
	}
	if byPath["/r/sub/deep"].Bytes != 200 {
		t.Errorf("/r/sub/deep = %d bytes, want 200", byPath["/r/sub/deep"].Bytes)
	}
	// Purity: the input slice must be unchanged.
	if dirs[1].Bytes != 0 {
		t.Errorf("input was modified: %+v", dirs[1])
	}
}

func TestScanReviewCandidates(t *testing.T) {
	root := t.TempDir()
	big := filepath.Join(root, "big.bin")
	writeSizedFile(t, big, 500)
	small := filepath.Join(root, "small.bin")
	writeSizedFile(t, small, 10)

	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(big, old, old); err != nil {
		t.Fatal(err)
	}
	// small is old too but below the fixture min size of 100.
	if err := os.Chtimes(small, old, old); err != nil {
		t.Fatal(err)
	}

	res := scanFixture(t, root)
	if len(res.Review) != 1 || res.Review[0].Path != big {
		t.Errorf("review candidates = %v, want only big.bin", res.Review)
	}
}

func TestScanMaxDepth(t *testing.T) {
	root := buildFixture(t)
	res, err := Scan(ScanOptions{
		Path: root, Workers: 2, Top: 20, MinSize: 1, OldDays: 0, Now: time.Now(), MaxDepth: 1,
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// At depth 1 the scan reads root and sub, but not sub/deep,
	// so c.bin (200 bytes) is excluded: 100 + 50 + 300 = 450.
	if res.Bytes != 450 {
		t.Errorf("bytes at max-depth 1 = %d, want 450", res.Bytes)
	}
}

func TestScanRootUnusable(t *testing.T) {
	if _, err := Scan(ScanOptions{Path: filepath.Join(t.TempDir(), "does-not-exist"), Workers: 2, Now: time.Now()}); err == nil {
		t.Error("Scan of a missing path returned no error")
	}
	file := filepath.Join(t.TempDir(), "a-file")
	writeSizedFile(t, file, 10)
	if _, err := Scan(ScanOptions{Path: file, Workers: 2, Now: time.Now()}); err == nil {
		t.Error("Scan of a regular file returned no error")
	}
}

func TestScanSymlinkedDirNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeSizedFile(t, filepath.Join(outside, "secret.txt"), 999)
	writeSizedFile(t, filepath.Join(root, "real.txt"), 10)
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	res := scanFixture(t, root)
	// real.txt (10) plus the symlink itself at its lstat size.
	// The 999-byte file behind the link must not appear.
	for _, f := range res.LargestFiles {
		if f.Size == 999 {
			t.Errorf("symlinked directory was followed: found the 999-byte file")
		}
	}
	if res.Bytes >= 999 {
		t.Errorf("total bytes = %d, the linked tree leaked into the scan", res.Bytes)
	}
}
