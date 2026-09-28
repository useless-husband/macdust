package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/useless-husband/macdust/internal/tui"
)

func setup(t *testing.T) (Deps, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	for p, n := range map[string]int{"proj/a.bin": 400_000, "proj/b.bin": 300_000, "c.txt": 10_000, "Library/Developer/Xcode/DerivedData/x": 200_000} {
		full := filepath.Join(home, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(strings.Repeat("k", n)), 0o644)
	}
	var out, errb bytes.Buffer
	return Deps{Home: home, Stdout: &out, Stderr: &errb,
		Run: func(string, ...string) (string, error) { return "", os.ErrNotExist }}, &out, &errb, home
}

func TestTopText(t *testing.T) {
	d, out, errb, home := setup(t)
	if code := Run([]string{"--top", "2", home}, d); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "~/proj/a.bin") || !strings.HasSuffix(lines[1], "~/proj/b.bin") {
		t.Errorf("top output:\n%s", out)
	}
	if !strings.Contains(errb.String(), "掃描") || strings.Contains(out.String(), "掃描") {
		t.Error("summary must go to stderr")
	}
	// Flags may follow the path.
	out.Reset()
	if code := Run([]string{home, "--top", "1", "--min-size", "350KB"}, d); code != 0 || strings.Count(out.String(), "\n") != 1 {
		t.Errorf("interspersed flags: %d %q", code, out)
	}
	out.Reset()
	Run([]string{"--top", "5", "--min-size", "1GB", home}, d)
	if out.Len() != 0 {
		t.Error("min-size filter")
	}
}

func TestJSON(t *testing.T) {
	d, out, _, home := setup(t)
	if code := Run([]string{"--json", "--depth", "1", "--min-size", "100KB", home}, d); code != 0 {
		t.Fatal("exit")
	}
	var v struct {
		Type     string
		Size     int64
		Children []struct {
			Name     string
			Type     string
			Children []interface{}
		}
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if v.Type != "dir" || v.Size < 900_000 || len(v.Children) != 2 { // proj and Library; c.txt filtered
		t.Errorf("json: %+v", v)
	}
	for _, c := range v.Children {
		if len(c.Children) != 0 {
			t.Error("depth ignored")
		}
	}
	out.Reset()
	Run([]string{"--json", "--top", "1", home}, d)
	var arr []map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil || len(arr) != 1 || arr[0]["name"] != "a.bin" {
		t.Errorf("top json: %v %s", err, out)
	}
}

func TestErrors(t *testing.T) {
	d, _, errb, home := setup(t)
	cases := [][]string{
		{"--bogus"}, {"--min-size", "xx", home}, {"--top", "-1", home}, {"a", "b"},
		{"--allow-delete", "--json", home}, {home}, // no TTY and no --json/--top
	}
	for _, c := range cases {
		errb.Reset()
		if code := Run(c, d); code != 2 || errb.Len() == 0 {
			t.Errorf("%v: exit %d, stderr %q", c, code, errb)
		}
	}
	errb.Reset()
	if code := Run([]string{"--top", "1", filepath.Join(home, "missing")}, d); code != 1 {
		t.Errorf("missing path: %d", code)
	}
}

func TestLaunchesTUIWithConfig(t *testing.T) {
	d, _, _, home := setup(t)
	var got tui.Config
	d.Interact = true
	d.RunTUI = func(c tui.Config) error { got = c; return nil }
	if code := Run([]string{"--allow-delete", "--min-size", "1MB", "--cross-fs", "~/proj"}, d); code != 0 {
		t.Fatal("exit")
	}
	if got.Path != home+"/proj" || !got.AllowDelete || !got.CrossFS || got.MinSize != 1_000_000 || got.Home != home {
		t.Errorf("config: %+v", got)
	}
	got = tui.Config{}
	Run(nil, d)
	if got.Path != "." || got.AllowDelete {
		t.Errorf("defaults: %+v (delete must be off by default)", got)
	}
}

func TestHogsCommand(t *testing.T) {
	d, out, _, home := setup(t)
	if code := Run([]string{"hogs", "--projects", "~/nowhere"}, d); code != 0 {
		t.Fatal("exit")
	}
	s := out.String()
	if !strings.Contains(s, "Xcode DerivedData") || !strings.Contains(s, "~/Library/Developer/Xcode/DerivedData") || strings.Contains(s, home) {
		t.Errorf("hogs output:\n%s", s)
	}
	out.Reset()
	if code := Run([]string{"hogs", "--json"}, d); code != 0 {
		t.Fatal("exit json")
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil || len(arr) == 0 || arr[0]["id"] != "xcode-derived" {
		t.Errorf("hogs json: %v %s", err, out)
	}
	if code := Run([]string{"hogs", "extra"}, d); code != 2 {
		t.Error("extra args")
	}
}

func TestVersionAndHelp(t *testing.T) {
	d, out, _, _ := setup(t)
	if Run([]string{"version"}, d) != 0 || !strings.HasPrefix(out.String(), "macdust ") {
		t.Error("version")
	}
	out.Reset()
	if Run([]string{"-h"}, d) != 0 || !strings.Contains(out.String(), "--allow-delete") {
		t.Error("help")
	}
}
