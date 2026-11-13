package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// Nested folders must not be added together in the summary: pkg/tool is
// inside pkg, so counting both would report more than 100%.
func TestSummaryFoldersSkipNested(t *testing.T) {
	root := filepath.FromSlash("/scan")
	p := func(s string) string { return filepath.Join(root, filepath.FromSlash(s)) }
	dirs := []DirTotal{
		{Path: root, Bytes: 200},
		{Path: p("src"), Bytes: 110},
		{Path: p("pkg"), Bytes: 50},
		{Path: p("pkg/tool"), Bytes: 50},
		{Path: p("pkg/tool/linux"), Bytes: 50},
		{Path: p("bin"), Bytes: 17},
		{Path: p("binary"), Bytes: 10},
	}
	got := summaryFolders(root, dirs, 3)
	var names []string
	for _, d := range got {
		names = append(names, d.Path)
	}
	want := []string{p("src"), p("pkg"), p("bin")}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("summaryFolders = %v, want %v", names, want)
	}
}

// A child listed before its parent (equal sizes) must also not overlap.
func TestSummaryFoldersChildBeforeParent(t *testing.T) {
	root := filepath.FromSlash("/scan")
	p := func(s string) string { return filepath.Join(root, filepath.FromSlash(s)) }
	dirs := []DirTotal{{Path: p("a/b"), Bytes: 40}, {Path: p("a"), Bytes: 40}, {Path: p("c"), Bytes: 5}}
	got := summaryFolders(root, dirs, 3)
	if len(got) != 2 || got[0].Path != p("a/b") || got[1].Path != p("c") {
		t.Fatalf("got %+v", got)
	}
}

func TestIsWithin(t *testing.T) {
	cases := []struct {
		path, dir string
		want      bool
	}{
		{"/a/b", "/a", true},
		{"/a", "/a", true},
		{"/ab", "/a", false},
		{"/a", "/a/b", false},
		{"/a/..b", "/a", true},
	}
	for _, c := range cases {
		if got := isWithin(filepath.FromSlash(c.path), filepath.FromSlash(c.dir)); got != c.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}

// The summary line must never claim more than 100% of the scan.
func TestSummaryNeverOver100Percent(t *testing.T) {
	root := filepath.FromSlash("/scan")
	p := func(s string) string { return filepath.Join(root, filepath.FromSlash(s)) }
	data := ReportData{Scan: &ScanResult{
		Root:  root,
		Bytes: 200,
		LargestDirs: []DirTotal{
			{Path: root, Bytes: 200}, {Path: p("src"), Bytes: 110},
			{Path: p("pkg"), Bytes: 50}, {Path: p("pkg/tool"), Bytes: 50},
		},
	}}
	var buf bytes.Buffer
	writeTextReport(&buf, data)
	out := buf.String()
	if !strings.Contains(out, "which is 80.0% of the scanned total") {
		t.Fatalf("unexpected summary:\n%s", out[strings.LastIndex(out, "Summary"):])
	}
}
