package trash

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Style selects the trash layout.
type Style int

const (
	// StyleMac uses ~/.Trash (and Finder via osascript when UseFinder is set).
	StyleMac Style = iota
	// StyleXDG follows the freedesktop.org trash specification.
	StyleXDG
)

// Trasher moves paths to the trash after the Guard approves them.
type Trasher struct {
	Home      string
	Style     Style
	Dir       string // trash root override (tests); default depends on Style
	UseFinder bool   // macOS: ask Finder so "Put Back" works
}

// Default returns a Trasher suited to the current OS.
func Default(home string) Trasher {
	if runtime.GOOS == "darwin" {
		return Trasher{Home: home, Style: StyleMac, UseFinder: true}
	}
	return Trasher{Home: home, Style: StyleXDG}
}

// Move sends path to the trash. It returns the new location when known.
func (t Trasher) Move(path string) (string, error) {
	if err := (Guard{Home: t.Home}).Check(path); err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	if t.UseFinder && t.Style == StyleMac {
		if err := finderTrash(path); err == nil {
			if _, err := os.Lstat(path); os.IsNotExist(err) {
				return "", nil
			}
		}
		// Finder unavailable or refused: fall back to moving into ~/.Trash.
	}
	if t.Style == StyleXDG {
		return t.moveXDG(path)
	}
	return t.moveMac(path)
}

func finderTrash(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript",
		"-e", "on run argv",
		"-e", `tell application "Finder" to delete (POSIX file (item 1 of argv))`,
		"-e", "end run", path)
	return cmd.Run()
}

func (t Trasher) macDir() string {
	if t.Dir != "" {
		return t.Dir
	}
	return filepath.Join(t.Home, ".Trash")
}

func (t Trasher) moveMac(path string) (string, error) {
	dir := t.macDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Base(path)
	dest := filepath.Join(dir, base)
	for i := 2; exists(dest); i++ {
		ext := filepath.Ext(base)
		dest = filepath.Join(dir, fmt.Sprintf("%s %d%s", strings.TrimSuffix(base, ext), i, ext))
	}
	return dest, rename(path, dest)
}

func (t Trasher) moveXDG(path string) (string, error) {
	root := t.Dir
	if root == "" {
		data := os.Getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(t.Home, ".local", "share")
		}
		root = filepath.Join(data, "Trash")
	}
	files, info := filepath.Join(root, "files"), filepath.Join(root, "info")
	for _, d := range []string{files, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
	}
	base := filepath.Base(path)
	name := base
	for i := 2; exists(filepath.Join(files, name)) || exists(filepath.Join(info, name+".trashinfo")); i++ {
		name = fmt.Sprintf("%s.%d", base, i)
	}
	body := "[Trash Info]\nPath=" + (&url.URL{Path: path}).EscapedPath() +
		"\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
	infoPath := filepath.Join(info, name+".trashinfo")
	if err := os.WriteFile(infoPath, []byte(body), 0o600); err != nil {
		return "", err
	}
	dest := filepath.Join(files, name)
	if err := rename(path, dest); err != nil {
		os.Remove(infoPath)
		return "", err
	}
	return dest, nil
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func rename(src, dst string) error {
	err := os.Rename(src, dst)
	if errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("垃圾桶與此項目在不同磁碟，無法直接移動；請改用檔案管理員刪除: %w", err)
	}
	return err
}
