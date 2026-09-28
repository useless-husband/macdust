package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Non-zero content so no filesystem can store it as holes.
	if err := os.WriteFile(path, []byte(strings.Repeat("x", n)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, root string, opt Options) (*Node, *Progress) {
	t.Helper()
	n, p, err := Scan(context.Background(), root, opt, nil)
	if err != nil {
		t.Fatal(err)
	}
	return n, p
}

func find(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestBasicSizesAndItems(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "one.bin"), 300_000)
	write(t, filepath.Join(root, "a", "b", "two.bin"), 200_000)
	write(t, filepath.Join(root, "three.bin"), 100_000)

	n, _ := run(t, root, Options{})
	a := find(n, "a")
	if a == nil || !a.IsDir {
		t.Fatal("missing dir a")
	}
	if a.Items != 3 { // one.bin, b, two.bin
		t.Errorf("a.Items=%d want 3", a.Items)
	}
	if n.Items != 5 {
		t.Errorf("root.Items=%d want 5", n.Items)
	}
	// Allocated size is at least the data and within a few blocks of it.
	if s := find(a, "one.bin").Size; s < 300_000 || s > 300_000+64*1024 {
		t.Errorf("one.bin size %d not near 300000", s)
	}
	if n.Size < 600_000 {
		t.Errorf("root size %d < 600000", n.Size)
	}
	// Directory total equals the sum of its children plus its own block(s).
	var sum int64
	for _, c := range n.Children {
		sum += c.Size
	}
	if sum != n.Size-ownSize(t, root) {
		t.Errorf("children sum %d + own != %d", sum, n.Size)
	}
	// Children are sorted largest-first.
	if n.Children[0].Name != "a" {
		t.Errorf("first child %q want a", n.Children[0].Name)
	}
}

func ownSize(t *testing.T, path string) int64 {
	t.Helper()
	n, _, err := Scan(context.Background(), path, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, c := range n.Children {
		sum += c.Size
	}
	return n.Size - sum
}

func TestHardLinkCountedOnce(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "orig.bin"), 500_000)
	for _, name := range []string{"link1.bin", "link2.bin"} {
		if err := os.Link(filepath.Join(root, "orig.bin"), filepath.Join(root, name)); err != nil {
			t.Skip("hard links unsupported:", err)
		}
	}
	n, _ := run(t, root, Options{})
	var nonzero, linked int
	for _, c := range n.Children {
		if c.Size > 0 {
			nonzero++
		}
		if c.Linked {
			linked++
		}
	}
	if nonzero != 1 || linked != 2 {
		t.Errorf("nonzero=%d linked=%d, want 1 and 2", nonzero, linked)
	}
	if n.Size > 500_000+64*1024+ownSize(t, root) {
		t.Errorf("hard links were counted more than once: %d", n.Size)
	}
}

func TestSymlinkNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, filepath.Join(outside, "big.bin"), 2_000_000)
	if err := os.Symlink(outside, filepath.Join(root, "dirlink")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(filepath.Join(outside, "big.bin"), filepath.Join(root, "filelink")); err != nil {
		t.Skip(err)
	}
	n, _ := run(t, root, Options{})
	if n.Size > 100_000 {
		t.Errorf("symlink targets leaked into size: %d", n.Size)
	}
	l := find(n, "dirlink")
	if l == nil || !l.IsSymlink || l.IsDir || len(l.Children) != 0 {
		t.Errorf("dirlink should be a childless symlink: %+v", l)
	}
}

func TestRootSymlinkIsResolved(t *testing.T) {
	real := t.TempDir()
	write(t, filepath.Join(real, "f.bin"), 100_000)
	link := filepath.Join(t.TempDir(), "ln")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	n, _ := run(t, link, Options{})
	if find(n, "f.bin") == nil {
		t.Error("root symlink was not resolved")
	}
}

func TestSparseFileUsesAllocatedBlocks(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "sparse.img")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(200 * 1000 * 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("hello"), 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	fi, _ := os.Stat(p)
	n, _ := run(t, root, Options{})
	s := find(n, "sparse.img").Size
	if fi.Size() != 200_000_000 {
		t.Fatal("test setup wrong")
	}
	if s >= 50_000_000 {
		t.Skipf("filesystem does not support sparse files (allocated %d)", s)
	}
	if s <= 0 {
		t.Errorf("sparse file allocated size %d should be > 0", s)
	}
}

func TestUnreadableDirectoryRecordedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "ok", "f.bin"), 100_000)
	locked := filepath.Join(root, "locked")
	write(t, filepath.Join(locked, "secret.bin"), 100_000)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	n, p := run(t, root, Options{})
	l := find(n, "locked")
	if l == nil || l.Err == "" {
		t.Fatalf("locked dir should carry an error: %+v", l)
	}
	if p.Errors.Load() != 1 || len(p.Errs()) != 1 || p.Errs()[0].Err != "權限不足" {
		t.Errorf("errors = %v", p.Errs())
	}
	if find(n, "ok") == nil || find(n, "ok").Size < 100_000 {
		t.Error("scan did not continue past the locked directory")
	}
}

func TestCancel(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "f"), 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Scan(ctx, root, Options{}, nil); err == nil {
		t.Error("expected context error")
	}
}

func TestMissingRoot(t *testing.T) {
	if _, _, err := Scan(context.Background(), filepath.Join(t.TempDir(), "nope"), Options{}, nil); err == nil {
		t.Error("expected error")
	}
}

func TestSortAndFilter(t *testing.T) {
	ns := []*Node{
		{Name: "beta", Size: 10, Items: 5},
		{Name: "Alpha", Size: 30, Items: 1},
		{Name: "gamma", Size: 20, Items: 9},
	}
	names := func() string {
		var s []string
		for _, n := range ns {
			s = append(s, n.Name)
		}
		return strings.Join(s, ",")
	}
	SortNodes(ns, SortSize)
	if names() != "Alpha,gamma,beta" {
		t.Errorf("size: %s", names())
	}
	SortNodes(ns, SortName)
	if names() != "Alpha,beta,gamma" {
		t.Errorf("name: %s", names())
	}
	SortNodes(ns, SortItems)
	if names() != "gamma,beta,Alpha" {
		t.Errorf("items: %s", names())
	}
	if got := Filter(ns, "AM", 0); len(got) != 1 || got[0].Name != "gamma" {
		t.Errorf("filter q: %v", got)
	}
	if got := Filter(ns, "", 15); len(got) != 2 {
		t.Errorf("filter min: %v", got)
	}
	if SortSize.Next() != SortName || SortItems.Next() != SortSize {
		t.Error("Next cycle wrong")
	}
}

func TestTopFilesAndRemove(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "d", "big.bin"), 900_000)
	write(t, filepath.Join(root, "mid.bin"), 400_000)
	write(t, filepath.Join(root, "small.bin"), 5_000)
	n, _ := run(t, root, Options{})

	top := TopFiles(n, 2, 0)
	if len(top) != 2 || top[0].Name != "big.bin" || top[1].Name != "mid.bin" {
		t.Fatalf("top = %v", top)
	}
	if got := TopFiles(n, 10, 300_000); len(got) != 2 {
		t.Errorf("min size filter: %d", len(got))
	}

	before, items := n.Size, n.Items
	big := top[0]
	sz := big.Size
	big.Remove()
	if n.Size != before-sz || n.Items != items-1 {
		t.Errorf("Remove totals: size %d items %d", n.Size, n.Items)
	}
	if find(find(n, "d"), "big.bin") != nil {
		t.Error("node still attached")
	}
}

func TestToJSON(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "d", "e", "f.bin"), 100_000)
	write(t, filepath.Join(root, "tiny"), 10)
	n, _ := run(t, root, Options{})
	j := ToJSON(n, 1, 50_000)
	if j.Type != "dir" || len(j.Children) != 1 || j.Children[0].Name != "d" {
		t.Fatalf("%+v", j)
	}
	if len(j.Children[0].Children) != 0 {
		t.Error("depth limit ignored")
	}
	if ToJSON(n, 5, 0).Children[0].Children[0].Children[0].Type != "file" {
		t.Error("deep json wrong")
	}
}
