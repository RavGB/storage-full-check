# Storage Full Check

A small tool from RavGB that answers one question: **why is this
device out of storage, and where did the space actually go?**

Contact: hello@ravgb.com
Site: https://ravgb.com

It scans a folder tree and reports the filesystem totals, the
largest files, the largest folders, a breakdown by extension, a
size histogram, and a review list of big files that have not been
touched in a long time - the usual first place to look when a disk
fills up.

## How it scans

The scan runs as a worker pool, using goroutines:

- Every directory is one job. A pool of workers (default: one per
  CPU, capped at 16) reads one directory at a time, sums the files
  stored directly in it, and turns subdirectories into new jobs.
- A counter tracks outstanding jobs, so the scan ends exactly when
  the last directory is done - totals are exact, and no file is
  counted twice. Each directory is visited once, and a file is
  counted only by the worker that reads its own directory.
- Workers hand their per-directory results to a single aggregator,
  so shared totals never need guessing or repairing afterwards.
- Folder totals are worked out from the per-directory direct sums
  by adding each folder into its parent, deepest folders first.

The tool uses only the Go standard library, builds to one binary,
and never modifies anything it scans.

## Build

```bash
go build -o storage-full-check .
```

## Run

Scan the current folder:

```bash
./storage-full-check
```

Scan a whole drive or home folder, save the report, and widen the
review list:

```bash
./storage-full-check --path /home --output report.txt
./storage-full-check --path / --min-size 500MB --old-days 90 --top 30
```

Print the report as JSON (a text report file is still written when
--output is given):

```bash
./storage-full-check --path /home --json
```

## Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--path` | `.` | Folder to scan |
| `--top` | `20` | Entries in the largest files and folders lists |
| `--workers` | one per CPU, max 16 | Scan workers |
| `--min-size` | `100MB` | Smallest size for review candidates; accepts plain bytes, `KB/MB/GB/TB` and `KiB/MiB/GiB/TiB` |
| `--old-days` | `180` | Review candidates must be unmodified for at least this many days |
| `--max-depth` | `0` | Deepest folder level to scan (0 = unlimited) |
| `--output` | | Also save the text report to this file |
| `--json` | off | Print the report as JSON |

Exit codes: 0 = scan completed, 1 = the scan root could not be
used, 2 = a flag or argument was wrong.

## Example output

This is a real run on 13 Nov 2026 inside a clean Linux container
(Docker, `golang:1.25-alpine`), scanning the Go installation that
comes with the image, copied as printed. `--top 5` keeps the lists
short here; the default is 20.

```text
$ ./storage-full-check --path /usr/local/go --top 5
Storage full check
RavGB - hello@ravgb.com - https://ravgb.com
Generated: 2026-11-13 19:27:13
Scanned: /usr/local/go
----------------------------------------------------------------
Filesystem (from the operating system):
  Total: 58.42 GiB (62725623808 bytes)
  Used:  11.75 GiB (12612657152 bytes) (20.1%)
  Free:  46.67 GiB (50112966656 bytes)

Scan totals (files counted under the scanned folder):
  Files:       14541
  Directories: 1592
  Total size:  196.85 MiB (206410843 bytes)
  Read errors: 0
  Stored directly in the scanned folder itself: 6.41 KiB (6566 bytes) in 8 file(s) (0.0% of the scanned total)

Largest files (top 5):
  20.65 MiB (21654239 bytes) /usr/local/go/pkg/tool/linux_amd64/compile
  14.29 MiB (14989108 bytes) /usr/local/go/bin/go
  7.80 MiB (8182560 bytes) /usr/local/go/pkg/tool/linux_amd64/vet
  6.17 MiB (6473062 bytes) /usr/local/go/pkg/tool/linux_amd64/link
  4.93 MiB (5172006 bytes) /usr/local/go/pkg/tool/linux_amd64/cover

Largest folders (top 5, subtree totals):
  196.85 MiB (206410843 bytes)  100.0%  /usr/local/go
  111.87 MiB (117301494 bytes)   56.8%  /usr/local/go/src
  50.56 MiB (53013679 bytes)   25.7%  /usr/local/go/pkg
  50.55 MiB (53006446 bytes)   25.7%  /usr/local/go/pkg/tool
  50.55 MiB (53006446 bytes)   25.7%  /usr/local/go/pkg/tool/linux_amd64

Folders directly under the scanned folder:
  111.87 MiB (117301494 bytes)   56.8%  src
  50.56 MiB (53013679 bytes)   25.7%  pkg
  17.16 MiB (17998461 bytes)    8.7%  bin
  8.56 MiB (8970974 bytes)    4.3%  api
  7.23 MiB (7583532 bytes)    3.7%  test
  1.04 MiB (1091859 bytes)    0.5%  lib
  377.53 KiB (386588 bytes)    0.2%  doc
  56.34 KiB (57690 bytes)    0.0%  misc

By extension (top 15 by size):
  .go               10658 file(s)  85.76 MiB (89927064 bytes)
  (no extension)      287 file(s)  70.41 MiB (73825165 bytes)
  .syso                21 file(s)  11.43 MiB (11982400 bytes)
  .txt               1351 file(s)  11.06 MiB (11592151 bytes)
  .s                  635 file(s)  5.74 MiB (6022138 bytes)
  .html                14 file(s)  2.85 MiB (2990129 bytes)
  .base64              17 file(s)  1.74 MiB (1822370 bytes)
  .zip                 33 file(s)  1.03 MiB (1078739 bytes)
  .rules               20 file(s)  817.79 KiB (837420 bytes)
  .bz2                 12 file(s)  739.47 KiB (757218 bytes)
  .json                31 file(s)  656.00 KiB (671747 bytes)
  .png                 56 file(s)  395.48 KiB (404973 bytes)
  .crt                407 file(s)  382.38 KiB (391553 bytes)
  .test                40 file(s)  380.24 KiB (389370 bytes)
  .csv                  1 file(s)  334.90 KiB (342934 bytes)

Review candidates (at least 95.37 MiB (100000000 bytes) and not modified for at least 180 days):
  (none)

Size histogram:
  under 1KB        7163 file(s)  3.17 MiB (3327032 bytes)
  1KB - 1MB        7358 file(s)  105.21 MiB (110319735 bytes)
  1MB - 100MB        20 file(s)  88.47 MiB (92764076 bytes)
  100MB - 1GB         0 file(s)  0 B
  over 1GB            0 file(s)  0 B

Summary: the biggest folders are src (111.87 MiB (117301494 bytes), 56.8%), pkg (50.56 MiB (53013679 bytes), 25.7%). Together they hold 162.43 MiB (170315173 bytes), which is 82.5% of the scanned total.
```

(Trimmed from a real run of this tool against a real folder.)

## What the report shows

- **Filesystem** totals from the operating system (total, used,
  free). Skipped on systems where this tool has no statfs
  implementation.
- **Scan totals**: files, directories, total size, and read errors.
- **Largest files** (size descending, path as the tie-break) and
  **largest folders** (subtree totals), plus every folder directly
  under the scanned folder with its share of the scanned total.
- **By extension**: file counts and bytes, top 15 by size, with a
  (no extension) bucket.
- **Review candidates**: files at least `--min-size` in size and
  unmodified for at least `--old-days`. The tool only lists them;
  deciding what to remove is always a human call.
- **Size histogram**: how many files, and how many bytes, fall in
  each of under 1KB, 1KB-1MB, 1MB-100MB, 100MB-1GB, and over 1GB.
- A closing **summary** naming the top 3 folders and their combined
  share of the scanned total.

## Symlinks and errors

- Symlinked directories are **never followed**, so a linked tree is
  never scanned twice and never counted at the place of the link.
- A symlink itself counts at its own link size (the small size the
  link entry reports), and its target is never read.
- Only regular files, and symlink entries as described above, count
  toward byte totals. Devices, sockets, and named pipes do not.
- A directory or entry that cannot be read does not stop the scan.
  It is counted in the read errors, a short sample of the paths is
  shown, and the scan continues. Totals therefore cover everything
  that could be read, and the error count says how much could not.

## Tests

```bash
go test ./...
go test -race ./...
```

The tests build a fixture tree of files with known sizes and check
the scanner's exact total bytes, file count, largest-file order
(including the path tie-break), subtree folder totals, extension
aggregation, review candidates, depth limiting, symlink handling,
and the human size parser.

## License

MIT License. See the LICENSE file in this repository.

Copyright (c) 2026 RavGB Technologies Inc.
