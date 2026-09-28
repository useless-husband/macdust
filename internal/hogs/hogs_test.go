package hogs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("y", n)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byID(fs []Finding, id string) *Finding {
	for i := range fs {
		if fs[i].Rule.ID == id {
			return &fs[i]
		}
	}
	return nil
}

func fakeEnv(t *testing.T) Env {
	home, root := t.TempDir(), t.TempDir()
	put(t, filepath.Join(home, "Library/Developer/Xcode/DerivedData/App-abc/build.o"), 800_000)
	put(t, filepath.Join(home, "Library/Developer/Xcode/iOS DeviceSupport/17.0/sym"), 300_000)
	put(t, filepath.Join(home, "Library/Caches/com.big.app/blob"), 500_000)
	put(t, filepath.Join(home, "Library/Caches/com.small.app/blob"), 5_000)
	put(t, filepath.Join(home, "Library/Application Support/MobileSync/Backup/AAAA/data"), 400_000)
	put(t, filepath.Join(home, "Library/Containers/com.docker.docker/Data/vms/Docker.raw"), 600_000)
	put(t, filepath.Join(home, "Library/Containers/com.other/Data/file"), 100_000)
	put(t, filepath.Join(home, ".npm/_cacache/x"), 200_000)
	put(t, filepath.Join(home, ".Trash/old.dmg"), 150_000)
	put(t, filepath.Join(home, "Downloads/installer.dmg"), 300_000)
	put(t, filepath.Join(home, "Downloads/note.txt"), 10)
	put(t, filepath.Join(root, "Library/Application Support/com.apple.idleassetsd/Customer/video.mov"), 700_000)
	put(t, filepath.Join(home, "Developer/web/node_modules/pkg/index.js"), 250_000)
	put(t, filepath.Join(home, "Developer/web/node_modules/pkg/node_modules/inner/i.js"), 50_000)
	put(t, filepath.Join(home, "Desktop/site/node_modules/a/b.js"), 120_000)
	put(t, filepath.Join(home, "Developer/web/src/main.js"), 100)
	return Env{Home: home, Root: root, Run: func(name string, args ...string) (string, error) {
		return "Snapshots for volume group containing disk /:\ncom.apple.TimeMachine.2026-09-01-010101.local\ncom.apple.TimeMachine.2026-09-02-010101.local\n", nil
	}}
}

func TestCheckMatchesRules(t *testing.T) {
	env := fakeEnv(t)
	fs := Check(context.Background(), env, Rules)

	must := func(id string, min int64) *Finding {
		f := byID(fs, id)
		if f == nil {
			t.Fatalf("rule %s not found", id)
		}
		if f.Size < min {
			t.Errorf("%s size %d < %d", id, f.Size, min)
		}
		return f
	}
	must("xcode-derived", 800_000)
	must("xcode-devicesupport", 300_000)
	must("iphone-backup", 400_000)
	must("npm-cache", 200_000)
	must("trash", 150_000)
	aerial := must("aerial-system", 700_000) // found through the fake system root
	if aerial.Rule.Risk != Careful {
		t.Error("aerial should be Careful")
	}
	if byID(fs, "pip-cache") != nil || byID(fs, "xcode-archives") != nil {
		t.Error("rules whose paths do not exist must not be reported")
	}
	if byID(fs, "user-caches").Items[0].Path != filepath.Join(env.Home, "Library/Caches/com.big.app") {
		t.Errorf("caches breakdown: %+v", byID(fs, "user-caches").Items)
	}
	// Docker is excluded from the generic containers rule.
	docker := must("docker", 600_000)
	containers := must("app-containers", 100_000)
	if containers.Size >= docker.Size {
		t.Errorf("containers (%d) should exclude docker (%d)", containers.Size, docker.Size)
	}
	if containers.Rule.Risk != Never {
		t.Error("containers must be Never")
	}
	// Sorted largest first (snapshots have Size 0 and come last).
	for i := 1; i < len(fs); i++ {
		if fs[i].Size > fs[i-1].Size {
			t.Errorf("not sorted at %d", i)
		}
	}
}

func TestNodeModules(t *testing.T) {
	env := fakeEnv(t)
	fs := Check(context.Background(), env, Rules)
	nm := byID(fs, "node-modules")
	if nm == nil || nm.Count != 2 { // nested node_modules is not a separate hit
		t.Fatalf("node_modules: %+v", nm)
	}
	if nm.Size < 250_000+50_000+120_000 {
		t.Errorf("size %d", nm.Size)
	}
	env.Projects = []string{filepath.Join(env.Home, "Desktop")}
	nm = byID(Check(context.Background(), env, Rules), "node-modules")
	if nm == nil || nm.Count != 1 {
		t.Errorf("custom projects: %+v", nm)
	}
}

func TestBigFilesAndSnapshots(t *testing.T) {
	env := fakeEnv(t)
	fs := Check(context.Background(), env, []Rule{
		{ID: "dl", Title: "dl", Kind: KindBigFiles, Paths: []string{"~/Downloads"}},
		{ID: "snap", Title: "snap", Kind: KindSnapshots},
	})
	// The real threshold is 100 MB, so the small fake files are not reported.
	if byID(fs, "dl") != nil {
		t.Error("files under the threshold must not be reported")
	}
	old := BigFileThreshold
	BigFileThreshold = 200_000
	defer func() { BigFileThreshold = old }()
	dl := byID(Check(context.Background(), env, []Rule{{ID: "dl", Title: "dl", Kind: KindBigFiles, Paths: []string{"~/Downloads"}}}), "dl")
	if dl == nil || dl.Count != 1 || dl.Items[0].Path != filepath.Join(env.Home, "Downloads/installer.dmg") {
		t.Errorf("big files: %+v", dl)
	}
	snap := byID(fs, "snap")
	if snap == nil || snap.Count != 2 || !strings.Contains(snap.Note, "2") {
		t.Errorf("snapshots: %+v", snap)
	}
}

func TestSnapshotsCommandFailure(t *testing.T) {
	env := fakeEnv(t)
	env.Run = func(string, ...string) (string, error) { return "", os.ErrNotExist }
	if byID(Check(context.Background(), env, Rules), "tm-snapshots") != nil {
		t.Error("failing tmutil should yield no finding")
	}
}

func TestRuleCatalogue(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules {
		if seen[r.ID] {
			t.Errorf("duplicate id %s", r.ID)
		}
		seen[r.ID] = true
		if r.Title == "" || r.What == "" || r.CanDelete == "" || r.HowClean == "" {
			t.Errorf("%s has empty text", r.ID)
		}
		if r.Kind == KindDir && len(r.Paths) == 0 {
			t.Errorf("%s has no paths", r.ID)
		}
		for _, p := range r.Paths {
			if !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, "/") {
				t.Errorf("%s: bad path %q", r.ID, p)
			}
		}
	}
	for _, id := range []string{"xcode-derived", "xcode-devicesupport", "coresimulator", "user-caches",
		"aerial-system", "aerial-user", "iphone-backup", "docker", "homebrew-cache", "npm-cache",
		"pip-cache", "uv-cache", "go-cache", "node-modules", "downloads-big", "trash", "tm-snapshots"} {
		if !seen[id] {
			t.Errorf("missing rule %s", id)
		}
	}
	if Safe.String() != "安全" || Careful.String() != "小心" || Never.String() != "不要刪" {
		t.Error("risk labels")
	}
}

func TestReport(t *testing.T) {
	env := fakeEnv(t)
	fs := Check(context.Background(), env, Rules)
	var b bytes.Buffer
	Write(&b, env, fs, false, false)
	out := b.String()
	for _, want := range []string{"[安全]", "[小心]", "[不要刪]", "Xcode DerivedData", "這是什麼", "怎麼清理", "~/Library/Developer/Xcode/DerivedData"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if strings.Contains(out, env.Home) {
		t.Error("home directory path leaked; expected ~")
	}
	b.Reset()
	Write(&b, env, fs, true, false)
	if strings.Contains(b.String(), "這是什麼") {
		t.Error("brief must not include explanations")
	}
	b.Reset()
	Write(&b, env, nil, false, false)
	if !strings.Contains(b.String(), "沒有發現") {
		t.Error("empty report")
	}
}

func TestShortPath(t *testing.T) {
	if ShortPath("/h/u", "/h/u/a") != "~/a" || ShortPath("/h/u", "/h/user2/a") != "/h/user2/a" || ShortPath("/h/u", "/h/u") != "~" {
		t.Error("ShortPath")
	}
}
