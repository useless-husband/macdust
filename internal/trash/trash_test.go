package trash

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeHome(t *testing.T) string {
	t.Helper()
	h, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"Library/Caches/app", "Documents/x", "Downloads", "work/proj", ".Trash"} {
		if err := os.MkdirAll(filepath.Join(h, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(h, "work", "proj", "big.bin"), []byte("data"), 0o644)
	return h
}

func TestGuardRefusals(t *testing.T) {
	home := fakeHome(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "victim"), []byte("x"), 0o644)
	// Symlink inside home pointing outside: its target must not be deletable
	// via the symlinked directory.
	os.Symlink(outside, filepath.Join(home, "escape"))

	g := Guard{Home: home}
	bad := map[string]string{
		"empty":           "",
		"relative":        "work/proj",
		"home itself":     home,
		"home with slash": home + "/",
		"root":            "/",
		"outside home":    filepath.Join(outside, "victim"),
		"system dir":      "/System/Library",
		"etc":             "/etc/hosts",
		"dotdot escape":   filepath.Join(home, "..", filepath.Base(outside), "victim"),
		"symlink parent":  filepath.Join(home, "escape", "victim"),
		"Library":         filepath.Join(home, "Library"),
		"Documents":       filepath.Join(home, "Documents"),
		"Downloads":       filepath.Join(home, "Downloads"),
		"Library child":   filepath.Join(home, "Library", "Caches"),
		"trash itself":    filepath.Join(home, ".Trash"),
		"nonexistent":     filepath.Join(home, "nope"),
		"NUL":             home + "/a\x00b",
		"parent of home":  filepath.Dir(home),
	}
	for name, p := range bad {
		err := g.Check(p)
		if err == nil {
			t.Errorf("%s: %q should be refused", name, p)
		} else if !errors.Is(err, ErrRefused) {
			t.Errorf("%s: error does not wrap ErrRefused: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "victim")); err != nil {
		t.Fatal("outside file was touched")
	}
}

func TestGuardAllows(t *testing.T) {
	home := fakeHome(t)
	g := Guard{Home: home}
	for _, p := range []string{
		filepath.Join(home, "work", "proj"),
		filepath.Join(home, "work", "proj", "big.bin"),
		filepath.Join(home, "Library", "Caches", "app"),
		filepath.Join(home, "Documents", "x"),
	} {
		if err := g.Check(p); err != nil {
			t.Errorf("%s should be allowed: %v", p, err)
		}
	}
	// A symlink inside home is trashed as a link, never through it.
	outside := t.TempDir()
	link := filepath.Join(home, "work", "lnk")
	os.Symlink(outside, link)
	if err := g.Check(link); err != nil {
		t.Errorf("symlink itself should be allowed: %v", err)
	}
}

func TestMoveMac(t *testing.T) {
	home := fakeHome(t)
	tr := Trasher{Home: home, Style: StyleMac, Dir: filepath.Join(home, "fake-trash")}
	src := filepath.Join(home, "work", "proj")
	dest, err := tr.Move(src)
	if err != nil {
		t.Fatal(err)
	}
	if exists(src) || !exists(filepath.Join(dest, "big.bin")) {
		t.Error("directory not moved")
	}
	// Name collision gets a suffix.
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(home, "a.txt"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(home, "work", "a.txt"), []byte("2"), 0o644)
	d1, err := tr.Move(filepath.Join(home, "a.txt"))
	d2, err2 := tr.Move(filepath.Join(home, "work", "a.txt"))
	if err != nil || err2 != nil || d1 == d2 || !strings.HasSuffix(d2, "a 2.txt") {
		t.Errorf("collision handling: %q %q %v %v", d1, d2, err, err2)
	}
}

func TestMoveXDG(t *testing.T) {
	home := fakeHome(t)
	tr := Trasher{Home: home, Style: StyleXDG, Dir: filepath.Join(home, "xdg-trash")}
	src := filepath.Join(home, "work", "my file.txt")
	os.WriteFile(src, []byte("x"), 0o644)
	dest, err := tr.Move(src)
	if err != nil {
		t.Fatal(err)
	}
	if exists(src) || !exists(dest) {
		t.Fatal("file not moved")
	}
	b, err := os.ReadFile(filepath.Join(home, "xdg-trash", "info", "my file.txt.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "[Trash Info]\n") || !strings.Contains(s, "Path="+home+"/work/my%20file.txt") || !strings.Contains(s, "DeletionDate=") {
		t.Errorf("bad trashinfo:\n%s", s)
	}
}

func TestMoveRefusedDoesNotTouch(t *testing.T) {
	home := fakeHome(t)
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	os.WriteFile(victim, []byte("x"), 0o644)
	tr := Trasher{Home: home, Style: StyleMac, Dir: filepath.Join(home, "fake-trash")}
	for _, p := range []string{victim, home, filepath.Join(home, "Library")} {
		if _, err := tr.Move(p); !errors.Is(err, ErrRefused) {
			t.Errorf("Move(%q) err=%v", p, err)
		}
	}
	if !exists(victim) || !exists(filepath.Join(home, "Library")) {
		t.Error("refused path was modified")
	}
}
