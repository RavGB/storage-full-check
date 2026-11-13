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

```text
Storage full check
RavGB - hello@ravgb.com - https://ravgb.com
Scanned: /home/hatch/workspace/ravgb-coding
----------------------------------------------------------------
Filesystem (from the operating system):
  Total: 249.98 GiB (268417609728 bytes)
  Used:  7.86 GiB (8443174912 bytes) (3.1%)
  Free:  242.12 GiB (259974434816 bytes)

Scan totals (files counted under the scanned folder):
  Files:       63
  Directories: 30
  Total size:  13.73 MiB (14394740 bytes)
  Read errors: 0

Largest files (top 20):
  9.63 MiB (10092901 bytes) /home/hatch/workspace/ravgb-coding/network-slow-check/network-slow-check
  3.98 MiB (4175858 bytes) /home/hatch/workspace/ravgb-coding/storage-full-check/storage-full-check
  11.99 KiB (12282 bytes) /home/hatch/workspace/ravgb-coding/storage-full-check/scan.go
  ...

Largest folders (top 20, subtree totals):
  13.73 MiB (14394740 bytes)  100.0%  /home/hatch/workspace/ravgb-coding
  9.66 MiB (10133329 bytes)   70.4%  /home/hatch/workspace/ravgb-coding/network-slow-check
  4.02 MiB (4216479 bytes)   29.3%  /home/hatch/workspace/ravgb-coding/storage-full-check
  ...

Folders directly under the scanned folder:
  9.66 MiB (10133329 bytes)   70.4%  network-slow-check
  4.02 MiB (4216479 bytes)   29.3%  storage-full-check
  43.88 KiB (44932 bytes)    0.3%  local-issues-check

By extension (top 15 by size):
  (no extension)       25 file(s)  13.62 MiB (14280263 bytes)
  .go                  15 file(s)  67.01 KiB (68622 bytes)
  ...

Review candidates (at least 95.37 MiB (100000000 bytes) and not modified for at least 180 days):
  (none)

Size histogram:
  under 1KB          33 file(s)  9.57 KiB (9800 bytes)
  1KB - 1MB          28 file(s)  113.46 KiB (116181 bytes)
  1MB - 100MB         2 file(s)  13.61 MiB (14268759 bytes)
  100MB - 1GB         0 file(s)  0 B
  over 1GB            0 file(s)  0 B

Summary: the biggest folders are network-slow-check (9.66 MiB (10133329 bytes), 70.4%), storage-full-check (4.02 MiB (4216479 bytes), 29.3%), local-issues-check (43.88 KiB (44932 bytes), 0.3%). Together they hold 13.73 MiB (14394740 bytes), which is 100.0% of the scanned total.
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
