package scan

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
)

// Options controls a scan.
type Options struct {
	Workers int  // concurrent directory reads; <=0 means 4*NumCPU (min 8)
	CrossFS bool // descend into other filesystems (default: stay on one)
}

// Progress holds live counters (safe to read while a scan runs) and,
// once finished, the list of directories that could not be read.
type Progress struct {
	Files  atomic.Int64
	Dirs   atomic.Int64
	Bytes  atomic.Int64
	Errors atomic.Int64

	mu   sync.Mutex
	errs []Error
}

// Error records a path that could not be read.
type Error struct {
	Path string
	Err  string
}

// Errs returns a copy of the recorded errors (capped at 1000 entries;
// Errors counts all of them).
func (p *Progress) Errs() []Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Error(nil), p.errs...)
}

func (p *Progress) addErr(path string, err error) {
	p.Errors.Add(1)
	p.mu.Lock()
	if len(p.errs) < 1000 {
		p.errs = append(p.errs, Error{path, errText(err)})
	}
	p.mu.Unlock()
}

func errText(err error) string {
	if os.IsPermission(err) {
		return "權限不足"
	}
	return err.Error()
}

type inodeKey struct{ dev, ino uint64 }

type scanner struct {
	opt    Options
	ctx    context.Context
	sem    chan struct{}
	seen   sync.Map // inodeKey -> struct{} for files with nlink > 1
	rootFS uint64
	prog   *Progress
}

// Scan measures root. The size of every entry is the space actually
// allocated on disk (st_blocks*512), so sparse files count as what they
// really use; hard-linked data is counted only once; symlinks are never
// followed; other filesystems are skipped unless opt.CrossFS is set.
// Unreadable directories are recorded in prog and do not stop the scan.
// prog may be nil.
func Scan(ctx context.Context, root string, opt Options, prog *Progress) (*Node, *Progress, error) {
	if prog == nil {
		prog = &Progress{}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, prog, err
	}
	// Only the root argument itself may be a symlink that we resolve.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return nil, prog, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, prog, os.ErrInvalid
	}
	w := opt.Workers
	if w <= 0 {
		w = 4 * numCPU()
		if w < 8 {
			w = 8
		}
	}
	s := &scanner{opt: opt, ctx: ctx, sem: make(chan struct{}, w), rootFS: uint64(st.Dev), prog: prog}
	n := s.newNode(abs, filepath.Base(abs), nil, fi)
	if n.IsDir {
		s.scanDir(n)
	}
	if err := ctx.Err(); err != nil {
		return n, prog, err
	}
	return n, prog, nil
}

// newNode builds a node from lstat info and applies hard link accounting.
func (s *scanner) newNode(path, name string, parent *Node, fi os.FileInfo) *Node {
	n := &Node{Name: name, Path: path, Parent: parent, IsDir: fi.IsDir(), IsSymlink: fi.Mode()&os.ModeSymlink != 0}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		n.Size = int64(st.Blocks) * 512
		if !n.IsDir && uint64(st.Nlink) > 1 {
			if _, dup := s.seen.LoadOrStore(inodeKey{uint64(st.Dev), uint64(st.Ino)}, struct{}{}); dup {
				n.Linked = true
				n.Size = 0
			}
		}
		if n.IsDir && !s.opt.CrossFS && uint64(st.Dev) != s.rootFS {
			n.OtherFS = true
		}
	}
	if n.IsDir {
		s.prog.Dirs.Add(1)
	} else {
		s.prog.Files.Add(1)
	}
	s.prog.Bytes.Add(n.Size)
	return n
}

func (s *scanner) scanDir(n *Node) {
	if n.OtherFS || s.ctx.Err() != nil {
		return
	}
	s.sem <- struct{}{}
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		n.Err = errText(err)
		s.prog.addErr(n.Path, err)
	}
	children := make([]*Node, 0, len(entries))
	for _, e := range entries {
		if s.ctx.Err() != nil {
			break
		}
		fi, err := e.Info()
		if err != nil { // vanished or unreadable entry
			s.prog.addErr(filepath.Join(n.Path, e.Name()), err)
			continue
		}
		children = append(children, s.newNode(filepath.Join(n.Path, e.Name()), e.Name(), n, fi))
	}
	<-s.sem // release before waiting so children can always make progress

	var wg sync.WaitGroup
	for _, c := range children {
		if c.IsDir {
			wg.Add(1)
			go func(c *Node) {
				defer wg.Done()
				s.scanDir(c)
			}(c)
		}
	}
	wg.Wait()

	for _, c := range children {
		n.Size += c.Size
		n.Items += c.Items + 1
	}
	SortNodes(children, SortSize)
	n.Children = children
}
