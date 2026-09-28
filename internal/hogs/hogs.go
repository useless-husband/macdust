package hogs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/useless-husband/macdust/internal/scan"
)

// Item is one path (or file) that contributed to a finding.
type Item struct {
	Path string
	Size int64
}

// Finding is the result of checking one rule.
type Finding struct {
	Rule  *Rule
	Size  int64
	Items []Item // largest contributors, biggest first (may be empty)
	Count int    // number of node_modules folders / big files / snapshots
	Note  string // extra remark, e.g. "無法讀取"
	// Found is false when none of the rule's paths exist.
	Found bool
}

// Env carries everything that differs between machines, so tests can run
// against a fake home directory.
type Env struct {
	Home     string
	Root     string   // system root, "/" on a real machine
	Projects []string // roots searched for node_modules; nil = defaults
	// Run executes a command and returns its stdout. nil uses os/exec.
	Run func(name string, args ...string) (string, error)
	// Workers limits how many rules are measured at once (default 4).
	Workers int
}

// DefaultProjectDirs are searched for node_modules when none are given.
var DefaultProjectDirs = []string{
	"Desktop", "Documents", "Developer", "Projects", "Code", "dev", "src",
	"github-projects", "workspace", "repos", "git",
}

// Expand turns a rule path into an absolute path for this environment.
func (e Env) Expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(e.Home, p[2:])
	}
	root := e.Root
	if root == "" {
		root = "/"
	}
	return filepath.Join(root, p)
}

func (e Env) run(name string, args ...string) (string, error) {
	if e.Run != nil {
		return e.Run(name, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// Check evaluates every rule and returns findings for those that exist,
// largest first (snapshots, which have no size, come last). It only reads.
func Check(ctx context.Context, env Env, rules []Rule) []Finding {
	workers := env.Workers
	if workers <= 0 {
		workers = 4
	}
	results := make([]Finding, len(rules))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i := range rules {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = checkOne(ctx, env, &rules[i])
		}(i)
	}
	wg.Wait()
	var out []Finding
	for _, f := range results {
		if f.Found {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

func checkOne(ctx context.Context, env Env, r *Rule) Finding {
	f := Finding{Rule: r}
	switch r.Kind {
	case KindNodeModules:
		checkNodeModules(ctx, env, &f)
	case KindBigFiles:
		checkBigFiles(ctx, env, &f)
	case KindSnapshots:
		checkSnapshots(env, &f)
	default:
		checkDirs(ctx, env, &f)
	}
	return f
}

func lookup(root *scan.Node, path string) *scan.Node {
	if root.Path == path {
		return root
	}
	if !root.IsDir || !strings.HasPrefix(path, root.Path+string(filepath.Separator)) {
		return nil
	}
	for _, c := range root.Children {
		if n := lookup(c, path); n != nil {
			return n
		}
	}
	return nil
}

func checkDirs(ctx context.Context, env Env, f *Finding) {
	r := f.Rule
	for _, p := range r.Paths {
		abs := env.Expand(p)
		if _, err := os.Lstat(abs); err != nil {
			continue
		}
		f.Found = true
		root, prog, err := scan.Scan(ctx, abs, scan.Options{}, nil)
		if err != nil || root == nil {
			f.Note = "無法讀取"
			continue
		}
		size := root.Size
		for _, ex := range r.Exclude {
			rel, err := filepath.Rel(abs, env.Expand(ex))
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				continue
			}
			// The scan resolves symlinks in its root, so look up by relative path.
			if n := lookup(root, filepath.Join(root.Path, rel)); n != nil {
				size -= n.Size
			}
		}
		f.Size += size
		if root.Err != "" || prog.Errors.Load() > 0 {
			f.Note = "部分內容無法讀取（權限不足），實際可能更大"
		}
		if r.TopChildren > 0 {
			for _, c := range root.Children {
				f.Items = append(f.Items, Item{filepath.Join(abs, c.Name), c.Size})
			}
		} else {
			f.Items = append(f.Items, Item{abs, size})
		}
	}
	sort.SliceStable(f.Items, func(i, j int) bool { return f.Items[i].Size > f.Items[j].Size })
	if r.TopChildren > 0 && len(f.Items) > r.TopChildren {
		f.Items = f.Items[:r.TopChildren]
	}
	if len(r.Paths) == 1 && r.TopChildren == 0 {
		f.Items = nil // a single path needs no breakdown
	}
}

func checkBigFiles(ctx context.Context, env Env, f *Finding) {
	for _, p := range f.Rule.Paths {
		abs := env.Expand(p)
		if _, err := os.Lstat(abs); err != nil {
			continue
		}
		f.Found = true
		root, _, err := scan.Scan(ctx, abs, scan.Options{}, nil)
		if err != nil || root == nil {
			f.Note = "無法讀取"
			continue
		}
		for _, n := range scan.TopFiles(root, 1<<30, BigFileThreshold) {
			f.Size += n.Size
			f.Count++
			f.Items = append(f.Items, Item{filepath.Join(abs, strings.TrimPrefix(n.Path, root.Path)), n.Size})
		}
	}
	sort.SliceStable(f.Items, func(i, j int) bool { return f.Items[i].Size > f.Items[j].Size })
	if len(f.Items) > 8 {
		f.Items = f.Items[:8]
	}
	if f.Found && f.Count == 0 {
		f.Found = false
	}
}

func checkNodeModules(ctx context.Context, env Env, f *Finding) {
	roots := env.Projects
	if len(roots) == 0 {
		for _, d := range DefaultProjectDirs {
			roots = append(roots, filepath.Join(env.Home, d))
		}
	}
	var found []string
	for _, root := range roots {
		findNodeModules(ctx, root, 0, &found)
	}
	var items []Item
	for _, p := range found {
		n, _, err := scan.Scan(ctx, p, scan.Options{}, nil)
		if err != nil || n == nil {
			continue
		}
		items = append(items, Item{p, n.Size})
		f.Size += n.Size
	}
	if len(items) == 0 {
		return
	}
	f.Found = true
	f.Count = len(items)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Size > items[j].Size })
	if len(items) > 8 {
		items = items[:8]
	}
	f.Items = items
}

const maxProjectDepth = 6

// findNodeModules looks for node_modules directories without descending
// into them, into symlinks or into hidden directories.
func findNodeModules(ctx context.Context, dir string, depth int, out *[]string) {
	if depth > maxProjectDepth || ctx.Err() != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() { // symlinks report as non-dirs here
			continue
		}
		name := e.Name()
		if name == "node_modules" {
			*out = append(*out, filepath.Join(dir, name))
			continue
		}
		if strings.HasPrefix(name, ".") || name == "Library" {
			continue
		}
		findNodeModules(ctx, filepath.Join(dir, name), depth+1, out)
	}
}

func checkSnapshots(env Env, f *Finding) {
	out, err := env.run("tmutil", "listlocalsnapshots", "/")
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "com.apple.TimeMachine.") {
			f.Count++
			f.Items = append(f.Items, Item{Path: line})
		}
	}
	if f.Count == 0 {
		return
	}
	f.Found = true
	f.Note = "共 " + strconv.Itoa(f.Count) + " 個快照（大小無法從這裡得知）"
	if len(f.Items) > 5 {
		f.Items = f.Items[:5]
	}
}
