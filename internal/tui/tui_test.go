package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/useless-husband/macdust/internal/scan"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func mk(t *testing.T) (*Model, string) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	files := map[string]int{
		"big/video.mov": 900_000, "big/sub/x.bin": 300_000,
		"docs/readme.txt": 50_000, "alpha.log": 20_000, "中文檔案.txt": 10_000,
	}
	for p, n := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(strings.Repeat("z", n)), 0o644)
	}
	n, _, err := scan.Scan(context.Background(), root, scan.Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(n, 80, 24)
	m.Home = filepath.Dir(root)
	return m, root
}

func key(r rune) KeyEvent { return KeyEvent{Key: KeyRune, Rune: r} }

func names(m *Model) []string {
	var s []string
	for _, n := range m.View {
		s = append(s, n.Name)
	}
	return s
}

func TestParseKeys(t *testing.T) {
	cases := []struct {
		in   string
		want []KeyEvent
	}{
		{"\x1b[A\x1b[B\x1b[C\x1b[D", []KeyEvent{{Key: KeyUp}, {Key: KeyDown}, {Key: KeyRight}, {Key: KeyLeft}}},
		{"\x1bOA", []KeyEvent{{Key: KeyUp}}},
		{"\x1b", []KeyEvent{{Key: KeyEsc}}},
		{"\r\n", []KeyEvent{{Key: KeyEnter}, {Key: KeyEnter}}},
		{"\x7f", []KeyEvent{{Key: KeyBackspace}}},
		{"\x03", []KeyEvent{{Key: KeyCtrlC}}},
		{"\x1b[5~\x1b[6~", []KeyEvent{{Key: KeyPgUp}, {Key: KeyPgDn}}},
		{"aé中", []KeyEvent{key('a'), key('é'), key('中')}},
		{"\x1b[1;5C", []KeyEvent{{Key: KeyRight}}}, // modifier form of an arrow
		{"\x1b[200~", nil}, // unknown sequences are swallowed
	}
	for _, c := range cases {
		got := ParseKeys([]byte(c.in))
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q[%d]: got %v want %v", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestWidthHelpers(t *testing.T) {
	if width("abc") != 3 || width("中文") != 4 || width("a中") != 3 {
		t.Error("width")
	}
	if got := truncate("abcdefgh", 5); got != "abcd…" {
		t.Errorf("truncate %q", got)
	}
	if got := truncate("中文中文", 5); width(got) > 5 || !strings.HasSuffix(got, "…") {
		t.Errorf("truncate wide %q", got)
	}
	if got := truncateLeft("/a/b/c/d", 5); got != "…/c/d" {
		t.Errorf("truncateLeft %q", got)
	}
	if sanitize("a\x1b[31mb\n") != "a?[31mb?" {
		t.Errorf("sanitize %q", sanitize("a\x1b[31mb\n"))
	}
	if clipped := clipANSI("\x1b[1mabcdef\x1b[0m", 3); plain(clipped) != "abc" {
		t.Errorf("clipANSI %q", clipped)
	}
}

func TestRenderBrowse(t *testing.T) {
	m, root := mk(t)
	out := plain(Render(m))
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("want 24 lines, got %d", len(lines))
	}
	for i, l := range lines {
		if width(l) > 80 {
			t.Errorf("line %d too wide (%d): %q", i, width(l), l)
		}
	}
	if !strings.Contains(lines[0], "macdust") || !strings.Contains(lines[0], "~/"+filepath.Base(root)) {
		t.Errorf("header: %q", lines[0])
	}
	for _, want := range []string{"big/", "docs/", "alpha.log", "中文檔案.txt", "%", "唯讀", "排序：大小"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q\n%s", want, out)
		}
	}
	// The biggest entry is first, selected, and has the longest bar.
	if !strings.HasPrefix(lines[3], "> ") || !strings.Contains(lines[3], "big/") {
		t.Errorf("first row: %q", lines[3])
	}
	if strings.Count(lines[3], "█") <= strings.Count(lines[4], "█") {
		t.Errorf("bar not proportional:\n%s\n%s", lines[3], lines[4])
	}
	if !strings.Contains(lines[23], "q 離開") || strings.Contains(lines[23], "d 刪除") {
		t.Errorf("help line: %q", lines[23])
	}
	m.AllowDelete = true
	if !strings.Contains(plain(Render(m)), "d 刪除") {
		t.Error("delete hint missing with --allow-delete")
	}
}

func TestPercentAndBarMath(t *testing.T) {
	if bar(0.5, 10) != "█████░░░░░" || bar(0, 10) != "░░░░░░░░░░" || bar(1, 4) != "████" {
		t.Error("bar")
	}
	if bar(0.001, 10)[:3] != "█" {
		t.Error("tiny non-zero fraction should still show one block")
	}
	if barWidth(40) != 8 || barWidth(300) != 24 || barWidth(10) != 6 {
		t.Error("barWidth")
	}
}

func TestNavigation(t *testing.T) {
	m, root := mk(t)
	if names(m)[0] != "big" {
		t.Fatalf("order %v", names(m))
	}
	m.HandleKey(KeyEvent{Key: KeyEnter})
	if m.Cur.Name != "big" || names(m)[0] != "video.mov" {
		t.Fatalf("after enter: %s %v", m.Cur.Name, names(m))
	}
	m.HandleKey(key('j'))
	m.HandleKey(KeyEvent{Key: KeyRight}) // sub
	if m.Cur.Name != "sub" {
		t.Fatalf("cur %s", m.Cur.Name)
	}
	m.HandleKey(KeyEvent{Key: KeyLeft})
	if m.Cur.Name != "big" || m.Selected().Name != "sub" {
		t.Errorf("going back should reselect sub: %s %v", m.Cur.Name, m.Selected())
	}
	m.HandleKey(KeyEvent{Key: KeyBackspace})
	if m.Cur.Path != root {
		t.Error("did not return to root")
	}
	m.HandleKey(KeyEvent{Key: KeyLeft}) // at root: stays
	if m.Cur.Path != root {
		t.Error("went above root")
	}
	// Entering a file does nothing but explain.
	m.Cursor = indexOf(m, "alpha.log")
	m.HandleKey(KeyEvent{Key: KeyEnter})
	if m.Cur.Path != root || m.Status == "" {
		t.Error("file enter")
	}
	// Cursor limits.
	m.HandleKey(key('G'))
	if m.Cursor != len(m.View)-1 {
		t.Error("G")
	}
	m.HandleKey(KeyEvent{Key: KeyDown})
	if m.Cursor != len(m.View)-1 {
		t.Error("down past end")
	}
	m.HandleKey(key('g'))
	m.HandleKey(key('k'))
	if m.Cursor != 0 {
		t.Error("up past start")
	}
	if !m.HandleKey(key('q')) || !m.HandleKey(KeyEvent{Key: KeyCtrlC}) {
		t.Error("q / ctrl-c must quit")
	}
}

func indexOf(m *Model, name string) int {
	for i, n := range m.View {
		if n.Name == name {
			return i
		}
	}
	return -1
}

func TestSortToggle(t *testing.T) {
	m, _ := mk(t)
	m.HandleKey(key('s'))
	if got := names(m); got[0] != "alpha.log" || !strings.Contains(m.Status, "名稱") {
		t.Errorf("name sort: %v %q", got, m.Status)
	}
	m.HandleKey(key('s'))
	if got := names(m); got[0] != "big" { // big has the most descendants
		t.Errorf("items sort: %v", got)
	}
	m.HandleKey(key('s'))
	if !strings.Contains(plain(Render(m)), "排序：大小") {
		t.Error("sort label")
	}
}

func TestSearch(t *testing.T) {
	m, _ := mk(t)
	m.HandleKey(key('/'))
	if m.Mode != ModeSearch {
		t.Fatal("not in search mode")
	}
	for _, r := range "ALP" {
		m.HandleKey(key(r))
	}
	if got := names(m); len(got) != 1 || got[0] != "alpha.log" {
		t.Errorf("filter: %v", got)
	}
	out := plain(Render(m))
	if !strings.Contains(out, "搜尋：ALP") || !strings.Contains(out, "篩選「ALP」1/") {
		t.Errorf("search render:\n%s", out)
	}
	m.HandleKey(KeyEvent{Key: KeyBackspace})
	m.HandleKey(KeyEvent{Key: KeyBackspace})
	m.HandleKey(KeyEvent{Key: KeyBackspace})
	if len(m.View) != 4 {
		t.Errorf("empty query should show all: %v", names(m))
	}
	for _, r := range "中文" {
		m.HandleKey(key(r))
	}
	m.HandleKey(KeyEvent{Key: KeyEnter})
	if m.Mode != ModeBrowse || m.Query != "中文" || len(m.View) != 1 {
		t.Errorf("enter keeps filter: %v %q", m.Mode, m.Query)
	}
	m.HandleKey(KeyEvent{Key: KeyEsc})
	if m.Query != "" || len(m.View) != 4 {
		t.Error("esc clears filter")
	}
	m.HandleKey(key('/'))
	m.HandleKey(key('z'))
	m.HandleKey(key('z'))
	if !strings.Contains(plain(Render(m)), "沒有符合的項目") {
		t.Error("no match message")
	}
	m.HandleKey(KeyEvent{Key: KeyEsc})
	if m.Mode != ModeBrowse || m.Query != "" {
		t.Error("esc in search")
	}
}

func TestMinSizeFilter(t *testing.T) {
	m, _ := mk(t)
	m.MinSize = 100_000
	m.Refresh()
	if got := names(m); len(got) != 1 || got[0] != "big" {
		t.Errorf("min size: %v", got)
	}
}

func TestDeleteFlow(t *testing.T) {
	m, root := mk(t)
	var trashed []string
	m.Guard = func(p string) error {
		if strings.HasSuffix(p, "docs") {
			return errors.New("拒絕刪除：測試")
		}
		return nil
	}
	m.Trash = func(p string) (string, error) { trashed = append(trashed, p); return "", os.Remove(p) }

	// Read-only mode.
	m.HandleKey(key('d'))
	if m.Mode != ModeBrowse || !strings.Contains(m.Status, "--allow-delete") {
		t.Fatalf("read-only: %v %q", m.Mode, m.Status)
	}
	m.AllowDelete = true

	// Guard refusal never reaches the confirm prompt.
	m.Cursor = indexOf(m, "docs")
	m.HandleKey(key('d'))
	if m.Mode != ModeBrowse || !strings.Contains(m.Status, "拒絕") || len(trashed) != 0 {
		t.Fatalf("guard: %v %q", m.Mode, m.Status)
	}

	m.Cursor = indexOf(m, "alpha.log")
	victim := m.Selected()
	before := m.Root.Size
	m.HandleKey(key('d'))
	if m.Mode != ModeConfirm {
		t.Fatal("expected confirm mode")
	}
	if s := plain(Render(m)); !strings.Contains(s, "alpha.log") || !strings.Contains(s, "輸入 yes") {
		t.Errorf("confirm prompt:\n%s", s)
	}
	// Anything other than yes cancels and touches nothing.
	for _, r := range "no" {
		m.HandleKey(key(r))
	}
	m.HandleKey(KeyEvent{Key: KeyEnter})
	if len(trashed) != 0 || m.Mode != ModeBrowse || indexOf(m, "alpha.log") < 0 {
		t.Fatal("'no' must cancel")
	}
	// Esc cancels.
	m.Cursor = indexOf(m, "alpha.log")
	m.HandleKey(key('d'))
	m.HandleKey(key('y'))
	m.HandleKey(KeyEvent{Key: KeyEsc})
	if len(trashed) != 0 || m.Mode != ModeBrowse {
		t.Fatal("esc must cancel")
	}
	// "yes" deletes and updates totals.
	m.Cursor = indexOf(m, "alpha.log")
	m.HandleKey(key('d'))
	for _, r := range "yes" {
		m.HandleKey(key(r))
	}
	m.HandleKey(KeyEvent{Key: KeyEnter})
	if len(trashed) != 1 || trashed[0] != filepath.Join(root, "alpha.log") {
		t.Fatalf("trashed = %v", trashed)
	}
	if indexOf(m, "alpha.log") >= 0 || m.Root.Size != before-victim.Size || !strings.Contains(m.Status, "已移到垃圾桶") {
		t.Errorf("state after delete: size %d before %d status %q", m.Root.Size, before, m.Status)
	}
	// A failing trash keeps the node.
	m.Trash = func(string) (string, error) { return "", errors.New("boom") }
	m.Cursor = 0
	first := m.Selected().Name
	m.HandleKey(key('d'))
	for _, r := range "yes" {
		m.HandleKey(key(r))
	}
	m.HandleKey(KeyEvent{Key: KeyEnter})
	if indexOf(m, first) < 0 || m.Status != "boom" {
		t.Errorf("failed delete: %q", m.Status)
	}
}

func TestOpen(t *testing.T) {
	m, root := mk(t)
	var got string
	m.Open = func(p string) error { got = p; return nil }
	m.HandleKey(key('o'))
	if got != filepath.Join(root, "big") {
		t.Errorf("open %q", got)
	}
	m.Open = func(string) error { return errors.New("nope") }
	m.HandleKey(key('o'))
	if !strings.Contains(m.Status, "nope") {
		t.Error("open error")
	}
}

func TestResizeAndScroll(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 40; i++ {
		os.WriteFile(filepath.Join(root, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))), []byte(strings.Repeat("q", 1000*(i+1))), 0o644)
	}
	n, _, _ := scan.Scan(context.Background(), root, scan.Options{}, nil)
	m := NewModel(n, 80, 12) // 7 visible rows
	for i := 0; i < 20; i++ {
		m.HandleKey(KeyEvent{Key: KeyDown})
	}
	out := strings.Split(plain(Render(m)), "\n")
	if len(out) != 12 {
		t.Fatalf("lines %d", len(out))
	}
	sel := 0
	for _, l := range out {
		if strings.HasPrefix(l, "> ") {
			sel++
		}
	}
	if sel != 1 {
		t.Errorf("selected row must stay visible (found %d)\n%s", sel, strings.Join(out, "\n"))
	}
	m.Resize(50, 30)
	out = strings.Split(plain(Render(m)), "\n")
	if len(out) != 30 {
		t.Errorf("resize to 30 rows gave %d", len(out))
	}
	for _, l := range out {
		if width(l) > 50 {
			t.Errorf("line wider than 50: %q", l)
		}
	}
	m.Resize(10, 3)
	if !strings.Contains(Render(m), "視窗太小") {
		t.Error("tiny window message")
	}
}

func TestRenderScanningAndBadNames(t *testing.T) {
	m := NewModel(nil, 80, 24)
	m.Scanning = true
	m.ScanFiles, m.ScanDirs, m.ScanBytes = 1234, 56, 7_800_000
	out := plain(Render(m))
	if !strings.Contains(out, "掃描中") || !strings.Contains(out, "1234") || !strings.Contains(out, "7.8 MB") {
		t.Errorf("scanning screen:\n%s", out)
	}
	if m.HandleKey(key('j')) || !m.HandleKey(key('q')) {
		t.Error("only q quits during scan")
	}

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "evil\x1b[2Jname"), []byte("x"), 0o644)
	n, _, _ := scan.Scan(context.Background(), root, scan.Options{}, nil)
	m2 := NewModel(n, 80, 24)
	if strings.Contains(Render(m2), "\x1b[2J") {
		t.Error("escape sequence in file name reached the terminal")
	}
}

func TestErrAndLinkedMarkers(t *testing.T) {
	root := &scan.Node{Name: "r", Path: "/r", IsDir: true, Size: 100}
	root.Children = []*scan.Node{
		{Name: "locked", Path: "/r/locked", IsDir: true, Err: "權限不足", Parent: root},
		{Name: "vol", Path: "/r/vol", IsDir: true, OtherFS: true, Parent: root},
		{Name: "ln", Path: "/r/ln", IsSymlink: true, Parent: root},
	}
	m := NewModel(root, 80, 24)
	out := plain(Render(m))
	for _, w := range []string{"locked/ [!權限不足]", "vol/ [其他磁碟]", "ln@"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q", w)
		}
	}
}
