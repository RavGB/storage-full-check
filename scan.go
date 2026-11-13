package main

// Concurrency design
//
// The scan is a worker pool over dynamically discovered directory
// jobs:
//
//   - A jobs channel carries one job per directory. A fixed pool of
//     workers receives jobs. A worker reads exactly one directory
//     with os.ReadDir, sums the regular files stored directly in
//     that directory, and turns every subdirectory it finds into a
//     new job.
//   - An outstanding counter (sync.WaitGroup) tracks unfinished
//     jobs: Add(1) happens before a job is enqueued, Done happens
//     after the worker finishes it. A closer goroutine waits for
//     the counter to reach zero and then closes the jobs channel,
//     which ends the workers' receive loops.
//   - Enqueueing never blocks a worker: the send for a new job runs
//     in its own short-lived goroutine, so a full jobs channel
//     cannot deadlock the pool while all workers are trying to
//     hand off children at the same time.
//   - Workers never touch shared totals. Each job produces one
//     dirResult on a results channel, and a single aggregator (the
//     goroutine running Scan) merges results one at a time. That
//     keeps every total exact and free of data races by design.
//   - A second closer closes the results channel once every worker
//     has exited, which ends aggregation.
//
// Counting rules: symlinked directories are never followed. A
// symlink itself counts at its lstat size, like a small file, and
// its target is never read. Other non-regular entries (devices,
// sockets, named pipes) are not counted. Directory byte totals are
// sums of the counted files beneath them. A directory that cannot
// be read, and an entry that cannot be stat'ed, add to the error
// count and a short sample of paths; they never abort the scan.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileEntry is one counted file or symlink, with the facts the
// report needs about it.
type FileEntry struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size_bytes"`
	ModTime time.Time `json:"modified"`
}

// ExtStat aggregates file count and bytes for one extension.
type ExtStat struct {
	Ext   string `json:"extension"`
	Files int64  `json:"files"`
	Bytes int64  `json:"bytes"`
}

// DirTotal is a directory with its direct sums and, after
// computeSubtreeTotals runs, its subtree sums.
type DirTotal struct {
	Path        string `json:"path"`
	Depth       int    `json:"depth"`
	DirectBytes int64  `json:"direct_bytes"`
	DirectFiles int64  `json:"direct_files"`
	Bytes       int64  `json:"bytes"` // subtree total
	Files       int64  `json:"files"` // subtree total
}

// Bucket is one size histogram bucket.
type Bucket struct {
	Label string `json:"label"`
	Min   int64  `json:"min_bytes"`
	Max   int64  `json:"max_bytes"` // -1 means no upper bound
	Files int64  `json:"files"`
	Bytes int64  `json:"bytes"`
}

// ScanOptions controls one scan.
type ScanOptions struct {
	Path     string
	Workers  int
	MaxDepth int // 0 means unlimited
	Top      int
	MinSize  int64
	OldDays  int
	Now      time.Time // reference time for the age test; set by caller
}

// ScanResult is the full aggregated outcome of one scan.
type ScanResult struct {
	Root         string      `json:"root"`
	Files        int64       `json:"files"`
	Directories  int64       `json:"directories"`
	Bytes        int64       `json:"bytes"`
	Errors       int64       `json:"errors"`
	ErrorSample  []string    `json:"error_sample,omitempty"`
	LargestFiles []FileEntry `json:"largest_files"`
	LargestDirs  []DirTotal  `json:"largest_folders"`
	RootChildren []DirTotal  `json:"root_children"`
	ByExtension  []ExtStat   `json:"by_extension"`
	Review       []FileEntry `json:"review_candidates"`
	Histogram    []Bucket    `json:"histogram"`
	AllDirs      []DirTotal  `json:"all_folders"`
}

const maxErrorSample = 10

type dirJob struct {
	path  string
	depth int
}

type dirResult struct {
	path       string
	depth      int
	files      []FileEntry
	directSize int64
	errCount   int64
	errSample  []string
}

// processDir reads one directory and builds its result. Subdirectory
// jobs are handed to enqueue.
func processDir(job dirJob, opts ScanOptions, enqueue func(dirJob)) dirResult {
	res := dirResult{path: job.path, depth: job.depth}
	entries, err := os.ReadDir(job.path)
	if err != nil {
		res.errCount = 1
		res.errSample = append(res.errSample, job.path)
		return res
	}
	for _, entry := range entries {
		full := filepath.Join(job.path, entry.Name())
		info, err := os.Lstat(full)
		if err != nil {
			res.errCount++
			if len(res.errSample) < maxErrorSample {
				res.errSample = append(res.errSample, full)
			}
			continue
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			// A real directory (Lstat, so symlinked directories
			// never reach this branch).
			if opts.MaxDepth == 0 || job.depth < opts.MaxDepth {
				enqueue(dirJob{path: full, depth: job.depth + 1})
			}
		case mode.IsRegular() || mode&os.ModeSymlink != 0:
			// A regular file counts at its size. A symlink counts
			// at its own lstat size and is never followed.
			fe := FileEntry{Path: full, Size: info.Size(), ModTime: info.ModTime()}
			res.files = append(res.files, fe)
			res.directSize += info.Size()
		default:
			// Devices, sockets and named pipes hold no file data
			// for this report and are skipped.
		}
	}
	return res
}

// Scan walks opts.Path with a worker pool and aggregates the exact
// totals. A root that does not exist, is not a directory, or cannot
// be read at all is a fatal error; problems below the root are
// counted in the result instead.
func Scan(opts ScanOptions) (*ScanResult, error) {
	root, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, err
	}
	opts.Path = root
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "scan", Path: root, Err: errNotDir}
	}
	if _, err := os.ReadDir(root); err != nil {
		return nil, err
	}

	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan dirJob, 256)
	results := make(chan dirResult, 256)

	var outstanding sync.WaitGroup
	enqueue := func(job dirJob) {
		outstanding.Add(1)
		// The send runs in its own goroutine so a worker handing
		// off children can never block the pool (see design note).
		go func() { jobs <- job }()
	}

	var workerWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for job := range jobs {
				res := processDir(job, opts, enqueue)
				results <- res
				outstanding.Done()
			}
		}()
	}

	enqueue(dirJob{path: root, depth: 0})

	go func() {
		outstanding.Wait()
		close(jobs)
	}()
	go func() {
		workerWG.Wait()
		close(results)
	}()

	agg := newAggregator(opts)
	for res := range results {
		agg.add(res)
	}
	return agg.finish(), nil
}

var errNotDir = errString("not a directory")

type errString string

func (e errString) Error() string { return string(e) }

// aggregator merges dirResults one at a time. It is only ever used
// from the single goroutine that ranges over the results channel,
// so it needs no locks.
type aggregator struct {
	opts     ScanOptions
	files    int64
	dirs     int64
	bytes    int64
	errors   int64
	errSeen  []string
	allFiles []FileEntry
	review   []FileEntry
	exts     map[string]*ExtStat
	hist     []Bucket
	dirMap   map[string]*DirTotal
	dirOrder []string
}

func newAggregator(opts ScanOptions) *aggregator {
	return &aggregator{
		opts:   opts,
		exts:   make(map[string]*ExtStat),
		hist:   newHistogram(),
		dirMap: make(map[string]*DirTotal),
	}
}

func newHistogram() []Bucket {
	const kb = 1024
	const mb = 1024 * kb
	return []Bucket{
		{Label: "under 1KB", Min: 0, Max: kb - 1},
		{Label: "1KB - 1MB", Min: kb, Max: mb - 1},
		{Label: "1MB - 100MB", Min: mb, Max: 100*mb - 1},
		{Label: "100MB - 1GB", Min: 100 * mb, Max: 1024*mb - 1},
		{Label: "over 1GB", Min: 1024 * mb, Max: -1},
	}
}

func (a *aggregator) add(res dirResult) {
	a.dirs++
	dt := &DirTotal{
		Path:        res.path,
		Depth:       res.depth,
		DirectBytes: res.directSize,
		DirectFiles: int64(len(res.files)),
	}
	a.dirMap[res.path] = dt
	a.dirOrder = append(a.dirOrder, res.path)

	a.errors += res.errCount
	for _, p := range res.errSample {
		if len(a.errSeen) < maxErrorSample {
			a.errSeen = append(a.errSeen, p)
		}
	}

	cutoff := a.opts.Now.Add(-time.Duration(a.opts.OldDays) * 24 * time.Hour)
	for _, fe := range res.files {
		a.files++
		a.bytes += fe.Size
		a.allFiles = append(a.allFiles, fe)

		ext := extensionOf(fe.Path)
		st, ok := a.exts[ext]
		if !ok {
			st = &ExtStat{Ext: ext}
			a.exts[ext] = st
		}
		st.Files++
		st.Bytes += fe.Size

		for i := range a.hist {
			b := &a.hist[i]
			if fe.Size >= b.Min && (b.Max < 0 || fe.Size <= b.Max) {
				b.Files++
				b.Bytes += fe.Size
				break
			}
		}

		if fe.Size >= a.opts.MinSize && !fe.ModTime.After(cutoff) {
			a.review = append(a.review, fe)
		}
	}
}

func extensionOf(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "(no extension)"
	}
	return ext
}

// sortFiles orders by size descending, then path ascending, so the
// output is fully deterministic even when sizes tie.
func sortFiles(files []FileEntry) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Size != files[j].Size {
			return files[i].Size > files[j].Size
		}
		return files[i].Path < files[j].Path
	})
}

func sortDirs(dirs []DirTotal) {
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].Bytes != dirs[j].Bytes {
			return dirs[i].Bytes > dirs[j].Bytes
		}
		return dirs[i].Path < dirs[j].Path
	})
}

func (a *aggregator) finish() *ScanResult {
	top := a.opts.Top
	if top < 0 {
		top = 0
	}

	largest := make([]FileEntry, len(a.allFiles))
	copy(largest, a.allFiles)
	sortFiles(largest)
	if len(largest) > top {
		largest = largest[:top]
	}

	sortFiles(a.review)

	allDirs := make([]DirTotal, 0, len(a.dirOrder))
	for _, p := range a.dirOrder {
		allDirs = append(allDirs, *a.dirMap[p])
	}
	allDirs = computeSubtreeTotals(allDirs)

	largestDirs := make([]DirTotal, len(allDirs))
	copy(largestDirs, allDirs)
	sortDirs(largestDirs)
	if len(largestDirs) > top {
		largestDirs = largestDirs[:top]
	}

	var rootChildren []DirTotal
	for _, d := range allDirs {
		if d.Depth == 1 {
			rootChildren = append(rootChildren, d)
		}
	}
	sortDirs(rootChildren)

	extList := make([]ExtStat, 0, len(a.exts))
	for _, st := range a.exts {
		extList = append(extList, *st)
	}
	sort.Slice(extList, func(i, j int) bool {
		if extList[i].Bytes != extList[j].Bytes {
			return extList[i].Bytes > extList[j].Bytes
		}
		return extList[i].Ext < extList[j].Ext
	})
	if len(extList) > 15 {
		extList = extList[:15]
	}

	return &ScanResult{
		Root:         a.opts.Path,
		Files:        a.files,
		Directories:  a.dirs,
		Bytes:        a.bytes,
		Errors:       a.errors,
		ErrorSample:  a.errSeen,
		LargestFiles: largest,
		LargestDirs:  largestDirs,
		RootChildren: rootChildren,
		ByExtension:  extList,
		Review:       a.review,
		Histogram:    a.hist,
		AllDirs:      allDirs,
	}
}

// computeSubtreeTotals fills Bytes and Files for every directory
// from its direct sums by accumulating into ancestors, deepest
// directories first. It is a pure function of its input: the input
// slice is not modified, and directories may arrive in any order.
func computeSubtreeTotals(dirs []DirTotal) []DirTotal {
	out := make([]DirTotal, len(dirs))
	copy(out, dirs)
	for i := range out {
		out[i].Bytes = out[i].DirectBytes
		out[i].Files = out[i].DirectFiles
	}

	index := make(map[string]int, len(out))
	for i := range out {
		index[out[i].Path] = i
	}

	order := make([]int, len(out))
	for i := range order {
		order[i] = i
	}
	// Deepest first, so a parent is only updated after its own
	// subtree is complete. Path is the tie-break for determinism.
	sort.Slice(order, func(x, y int) bool {
		if out[order[x]].Depth != out[order[y]].Depth {
			return out[order[x]].Depth > out[order[y]].Depth
		}
		return out[order[x]].Path < out[order[y]].Path
	})

	for _, i := range order {
		if out[i].Depth == 0 {
			continue
		}
		parent := filepath.Dir(out[i].Path)
		if pi, ok := index[parent]; ok {
			out[pi].Bytes += out[i].Bytes
			out[pi].Files += out[i].Files
		}
	}
	return out
}
