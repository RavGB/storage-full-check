// Command storage-full-check finds out why a device is out of
// storage: it scans a folder tree in parallel and reports where
// the space actually went.
//
// RavGB - hello@ravgb.com - https://ravgb.com
//
// Exit codes: 0 = scan completed, 1 = scan root unusable,
// 2 = usage error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

// defaultWorkers picks a sensible pool size from the CPU count,
// capped so a large machine does not open an unbounded number of
// directory reads at once.
func defaultWorkers() int {
	n := runtime.NumCPU()
	if n < 2 {
		return 2
	}
	if n > 16 {
		return 16
	}
	return n
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("storage-full-check", flag.ContinueOnError)
	fs.SetOutput(stderr)

	path := fs.String("path", ".", "folder to scan")
	top := fs.Int("top", 20, "how many entries to show in the largest lists")
	workers := fs.Int("workers", defaultWorkers(), "number of scan workers")
	minSizeText := fs.String("min-size", "100MB", "smallest size for review candidates, e.g. 100MB, 1GiB, 500000")
	oldDays := fs.Int("old-days", 180, "review candidates must not have been modified for at least this many days")
	maxDepth := fs.Int("max-depth", 0, "deepest folder level to scan (0 = unlimited)")
	output := fs.String("output", "", "also save the text report to this file")
	asJSON := fs.Bool("json", false, "print the report as JSON instead of text")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", fs.Arg(0))
		return 2
	}
	if *top < 0 {
		fmt.Fprintln(stderr, "--top must be 0 or more")
		return 2
	}
	if *workers < 1 {
		fmt.Fprintln(stderr, "--workers must be at least 1")
		return 2
	}
	if *oldDays < 0 {
		fmt.Fprintln(stderr, "--old-days must be 0 or more")
		return 2
	}
	if *maxDepth < 0 {
		fmt.Fprintln(stderr, "--max-depth must be 0 or more")
		return 2
	}
	minSize, err := ParseSize(*minSizeText)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --min-size: %v\n", err)
		return 2
	}

	opts := ScanOptions{
		Path:     *path,
		Workers:  *workers,
		MaxDepth: *maxDepth,
		Top:      *top,
		MinSize:  minSize,
		OldDays:  *oldDays,
		Now:      time.Now(),
	}
	result, err := Scan(opts)
	if err != nil {
		fmt.Fprintf(stderr, "cannot scan %s: %v\n", *path, err)
		return 1
	}

	data := ReportData{
		GeneratedAt: time.Now(),
		Options: ReportOpts{
			Top:     *top,
			Workers: *workers,
			MinSize: minSize,
			OldDays: *oldDays,
		},
		Filesystem: filesystemInfo(result.Root),
		Scan:       result,
	}

	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(stderr, "cannot write report file: %v\n", err)
			return 1
		}
		writeTextReport(f, data)
		if err := f.Close(); err != nil {
			fmt.Fprintf(stderr, "cannot write report file: %v\n", err)
			return 1
		}
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(data); err != nil {
			fmt.Fprintf(stderr, "cannot encode report: %v\n", err)
			return 1
		}
		return 0
	}

	writeTextReport(stdout, data)
	return 0
}
