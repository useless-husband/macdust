// Package trash decides whether a path may be deleted and moves it to the
// trash. It never removes data permanently.
package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrRefused wraps every refusal made by the guard.
var ErrRefused = errors.New("拒絕刪除")

func refuse(format string, a ...interface{}) error {
	return fmt.Errorf("%w：%s", ErrRefused, fmt.Sprintf(format, a...))
}

// protectedHome lists directories directly inside the home directory that
// must never be trashed as a whole (their contents may be).
var protectedHome = map[string]bool{
	"Desktop": true, "Documents": true, "Downloads": true, "Library": true,
	"Movies": true, "Music": true, "Pictures": true, "Public": true,
	"Applications": true, ".Trash": true, ".ssh": true, ".gnupg": true,
	".config": true, ".local": true,
}

// Guard applies the deletion safety rules for one home directory.
type Guard struct{ Home string }

// Check returns nil only if path may be moved to the trash:
//   - it must be an absolute path that exists,
//   - strictly inside the home directory (after resolving symlinks in its
//     parent, so a symlinked parent cannot smuggle it out),
//   - not the home directory itself, not a standard top-level folder such as
//     ~/Library or ~/Documents, not a direct child of ~/Library,
//   - not already inside the trash.
func (g Guard) Check(path string) error {
	if path == "" || strings.ContainsRune(path, 0) {
		return refuse("路徑無效")
	}
	if !filepath.IsAbs(path) {
		return refuse("必須是絕對路徑")
	}
	if g.Home == "" || !filepath.IsAbs(g.Home) {
		return refuse("找不到家目錄")
	}
	clean := filepath.Clean(path)
	home := filepath.Clean(g.Home)
	if clean == "/" || clean == home {
		return refuse("不能刪除家目錄或根目錄")
	}
	if !within(home, clean) {
		return refuse("只能處理家目錄以內的項目")
	}
	if _, err := os.Lstat(clean); err != nil {
		return refuse("項目不存在或無法存取")
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return refuse("無法解析家目錄")
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return refuse("無法解析上層目錄")
	}
	target := filepath.Join(realParent, filepath.Base(clean))
	if target == realHome || !within(realHome, target) {
		return refuse("實際位置在家目錄以外")
	}
	rel, _ := filepath.Rel(realHome, target)
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 1 && protectedHome[parts[0]] {
		return refuse("~/%s 是系統資料夾，只能刪除裡面的項目", parts[0])
	}
	if parts[0] == ".Trash" {
		return refuse("項目已經在垃圾桶內")
	}
	if len(parts) == 2 && parts[0] == "Library" {
		return refuse("~/Library/%s 是系統資料夾，只能刪除裡面的項目", parts[1])
	}
	return nil
}

// within reports whether p is strictly below dir (both cleaned).
func within(dir, p string) bool {
	if p == dir {
		return false
	}
	if dir == "/" {
		return true
	}
	return strings.HasPrefix(p, dir+string(filepath.Separator))
}
