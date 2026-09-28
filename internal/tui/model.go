// Package tui implements the interactive browser. Everything except Run is
// pure state and string rendering so it can be tested without a terminal.
package tui

import (
	"fmt"
	"strings"

	"github.com/useless-husband/macdust/internal/humanize"
	"github.com/useless-husband/macdust/internal/scan"
)

// Mode is the input mode.
type Mode int

const (
	ModeBrowse Mode = iota
	ModeSearch
	ModeConfirm
)

// Model is the whole UI state.
type Model struct {
	Root, Cur *scan.Node
	Sort      scan.SortMode
	MinSize   int64
	Query     string
	Cursor    int
	Offset    int
	W, H      int
	Mode      Mode
	Input     string
	Status    string
	Home      string

	AllowDelete bool
	confirm     *scan.Node

	Scanning  bool
	ScanFiles int64
	ScanDirs  int64
	ScanBytes int64

	DiskTotal, DiskFree uint64

	View []*scan.Node // children of Cur after filter and sort

	// Injected side effects (nil-safe for tests that do not use them).
	Guard func(path string) error
	Trash func(path string) (string, error)
	Open  func(path string) error
}

// NewModel creates a model showing root.
func NewModel(root *scan.Node, w, h int) *Model {
	m := &Model{Root: root, Cur: root, W: w, H: h}
	m.Refresh()
	return m
}

// SetRoot swaps in a finished scan.
func (m *Model) SetRoot(root *scan.Node) {
	m.Root, m.Cur, m.Scanning = root, root, false
	m.Query, m.Cursor, m.Offset = "", 0, 0
	m.Refresh()
}

// Selected returns the highlighted node or nil.
func (m *Model) Selected() *scan.Node {
	if m.Cursor >= 0 && m.Cursor < len(m.View) {
		return m.View[m.Cursor]
	}
	return nil
}

// Refresh rebuilds View after any change to Cur, Sort, Query or MinSize,
// trying to keep the same node selected.
func (m *Model) Refresh() {
	sel := m.Selected()
	if m.Cur == nil {
		m.View = nil
		return
	}
	v := scan.Filter(m.Cur.Children, m.Query, m.MinSize)
	scan.SortNodes(v, m.Sort)
	m.View = v
	if sel != nil {
		for i, n := range v {
			if n == sel {
				m.Cursor = i
			}
		}
	}
	m.clamp()
}

func (m *Model) rows() int {
	r := m.H - 5
	if r < 1 {
		r = 1
	}
	return r
}

func (m *Model) clamp() {
	if m.Cursor >= len(m.View) {
		m.Cursor = len(m.View) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	r := m.rows()
	if m.Cursor < m.Offset {
		m.Offset = m.Cursor
	}
	if m.Cursor >= m.Offset+r {
		m.Offset = m.Cursor - r + 1
	}
	if max := len(m.View) - r; m.Offset > max {
		m.Offset = max
	}
	if m.Offset < 0 {
		m.Offset = 0
	}
}

// Resize records a new terminal size.
func (m *Model) Resize(w, h int) {
	m.W, m.H = w, h
	m.clamp()
}

func (m *Model) enter() {
	n := m.Selected()
	if n == nil {
		return
	}
	if !n.IsDir {
		m.Status = "這是檔案，按 o 可在 Finder 顯示"
		return
	}
	if n.OtherFS {
		m.Status = "這是其他磁碟（掛載點），未掃描；加 --cross-fs 可納入"
		return
	}
	m.Cur, m.Query, m.Cursor, m.Offset, m.Status = n, "", 0, 0, ""
	m.Refresh()
}

func (m *Model) up() {
	if m.Cur == nil || m.Cur == m.Root || m.Cur.Parent == nil {
		return
	}
	from := m.Cur
	m.Cur, m.Query, m.Status = m.Cur.Parent, "", ""
	m.Cursor, m.Offset = 0, 0
	m.Refresh()
	for i, n := range m.View {
		if n == from {
			m.Cursor = i
		}
	}
	m.clamp()
}

func (m *Model) move(d int) {
	m.Cursor += d
	m.clamp()
}

// HandleKey applies one key press and reports whether to quit.
func (m *Model) HandleKey(k KeyEvent) (quit bool) {
	if k.Key == KeyCtrlC {
		return true
	}
	switch m.Mode {
	case ModeSearch:
		m.searchKey(k)
		return false
	case ModeConfirm:
		m.confirmKey(k)
		return false
	}
	if m.Scanning {
		if k.Key == KeyRune && k.Rune == 'q' {
			return true
		}
		return false
	}
	switch k.Key {
	case KeyUp:
		m.move(-1)
	case KeyDown:
		m.move(1)
	case KeyPgUp:
		m.move(-m.rows())
	case KeyPgDn:
		m.move(m.rows())
	case KeyHome:
		m.Cursor = 0
		m.clamp()
	case KeyEnd:
		m.Cursor = len(m.View) - 1
		m.clamp()
	case KeyEnter, KeyRight:
		m.enter()
	case KeyLeft, KeyBackspace:
		m.up()
	case KeyEsc:
		if m.Query != "" {
			m.Query = ""
			m.Refresh()
		}
	case KeyRune:
		switch k.Rune {
		case 'q':
			return true
		case 'j':
			m.move(1)
		case 'k':
			m.move(-1)
		case 'l':
			m.enter()
		case 'h':
			m.up()
		case 'g':
			m.Cursor = 0
			m.clamp()
		case 'G':
			m.Cursor = len(m.View) - 1
			m.clamp()
		case 's':
			m.Sort = m.Sort.Next()
			m.Status = "排序：" + m.Sort.String()
			m.Refresh()
		case '/':
			m.Mode, m.Input = ModeSearch, m.Query
		case 'o':
			m.open()
		case 'd':
			m.startDelete()
		}
	}
	return false
}

func (m *Model) searchKey(k KeyEvent) {
	switch k.Key {
	case KeyEnter:
		m.Mode = ModeBrowse
	case KeyEsc:
		m.Mode, m.Query, m.Input = ModeBrowse, "", ""
		m.Cursor = 0
		m.Refresh()
	case KeyBackspace:
		if r := []rune(m.Input); len(r) > 0 {
			m.Input = string(r[:len(r)-1])
		}
		m.Query = m.Input
		m.Refresh()
	case KeyUp:
		m.move(-1)
	case KeyDown:
		m.move(1)
	case KeyRune:
		m.Input += string(k.Rune)
		m.Query = m.Input
		m.Cursor = 0
		m.Refresh()
	}
}

func (m *Model) open() {
	n := m.Selected()
	if n == nil {
		return
	}
	if m.Open == nil {
		m.Status = "此環境無法開啟檔案管理員"
		return
	}
	if err := m.Open(n.Path); err != nil {
		m.Status = "開啟失敗：" + err.Error()
		return
	}
	m.Status = "已在 Finder 顯示"
}

func (m *Model) startDelete() {
	n := m.Selected()
	if n == nil {
		return
	}
	if !m.AllowDelete {
		m.Status = "唯讀模式：要刪除請用 macdust --allow-delete 重新啟動"
		return
	}
	if m.Guard != nil {
		if err := m.Guard(n.Path); err != nil {
			m.Status = err.Error()
			return
		}
	}
	m.confirm, m.Mode, m.Input, m.Status = n, ModeConfirm, "", ""
}

func (m *Model) confirmKey(k KeyEvent) {
	switch k.Key {
	case KeyEsc:
		m.Mode, m.confirm, m.Input, m.Status = ModeBrowse, nil, "", "已取消"
	case KeyBackspace:
		if r := []rune(m.Input); len(r) > 0 {
			m.Input = string(r[:len(r)-1])
		}
	case KeyRune:
		m.Input += string(k.Rune)
	case KeyEnter:
		n, ans := m.confirm, strings.TrimSpace(m.Input)
		m.Mode, m.confirm, m.Input = ModeBrowse, nil, ""
		if ans != "yes" {
			m.Status = "已取消（必須輸入 yes）"
			return
		}
		if m.Trash == nil {
			m.Status = "此環境無法使用垃圾桶"
			return
		}
		if _, err := m.Trash(n.Path); err != nil {
			m.Status = err.Error()
			return
		}
		size := n.Size
		n.Remove()
		m.Refresh()
		m.Status = fmt.Sprintf("已移到垃圾桶：%s（%s）", sanitize(n.Name), humanize.Bytes(size))
	}
}
