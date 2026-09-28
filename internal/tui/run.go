package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/useless-husband/macdust/internal/scan"
	"github.com/useless-husband/macdust/internal/trash"
)

// Config is everything Run needs from the command line.
type Config struct {
	Path        string
	Home        string
	MinSize     int64
	CrossFS     bool
	AllowDelete bool
}

type scanResult struct {
	root *scan.Node
	err  error
}

// Run starts the interactive browser on the controlling terminal.
func Run(cfg Config) error {
	in, out := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	old, err := term.MakeRaw(in)
	if err != nil {
		return err
	}
	restore := func() {
		fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[?1049l")
		term.Restore(in, old)
	}
	defer restore()
	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l")

	w, h, err := term.GetSize(out)
	if err != nil {
		w, h = 80, 24
	}
	m := NewModel(nil, w, h)
	m.Home, m.MinSize, m.AllowDelete, m.Scanning = cfg.Home, cfg.MinSize, cfg.AllowDelete, true
	m.DiskTotal, m.DiskFree = diskUsage(cfg.Path)
	tr := trash.Default(cfg.Home)
	m.Guard = trash.Guard{Home: cfg.Home}.Check
	m.Trash = tr.Move
	m.Open = openInFileManager

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	prog := &scan.Progress{}
	done := make(chan scanResult, 1)
	go func() {
		root, _, err := scan.Scan(ctx, cfg.Path, scan.Options{CrossFS: cfg.CrossFS}, prog)
		done <- scanResult{root, err}
	}()

	keys := make(chan []KeyEvent, 16)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				keys <- ParseKeys(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				close(keys)
				return
			}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	quitSig := make(chan os.Signal, 1)
	signal.Notify(quitSig, syscall.SIGTERM, syscall.SIGHUP)
	tick := time.NewTicker(120 * time.Millisecond)
	defer tick.Stop()

	draw := func() {
		fmt.Fprint(os.Stdout, "\x1b[H"+strings.ReplaceAll(Render(m), "\n", "\x1b[K\r\n")+"\x1b[K\x1b[J")
	}
	draw()
	for {
		select {
		case evs, ok := <-keys:
			if !ok {
				return nil
			}
			for _, ev := range evs {
				if m.HandleKey(ev) {
					return nil
				}
			}
		case <-winch:
			if w, h, err := term.GetSize(out); err == nil {
				m.Resize(w, h)
			}
		case <-quitSig:
			return nil
		case r := <-done:
			if r.err != nil {
				return r.err
			}
			m.SetRoot(r.root)
			if n := prog.Errors.Load(); n > 0 {
				m.Status = fmt.Sprintf("有 %d 個項目無法讀取（權限不足等），已略過", n)
			}
		case <-tick.C:
			if m.Scanning {
				m.ScanFiles, m.ScanDirs, m.ScanBytes = prog.Files.Load(), prog.Dirs.Load(), prog.Bytes.Load()
			} else {
				continue
			}
		}
		draw()
	}
}

func diskUsage(path string) (total, free uint64) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0
	}
	return uint64(st.Blocks) * uint64(st.Bsize), uint64(st.Bavail) * uint64(st.Bsize)
}

func openInFileManager(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path).Run()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Run()
}
