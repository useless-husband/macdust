package hogs

import (
	"fmt"
	"io"
	"strings"

	"github.com/useless-husband/macdust/internal/humanize"
)

// ShortPath replaces the home directory prefix with "~".
func ShortPath(home, p string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
)

func riskColor(r Risk) string {
	switch r {
	case Safe:
		return green
	case Careful:
		return yellow
	}
	return red
}

// Write prints findings as a human-readable report. brief prints one line
// per finding without explanations.
func Write(w io.Writer, env Env, fs []Finding, brief, color bool) {
	c := func(code, s string) string {
		if !color {
			return s
		}
		return code + s + reset
	}
	var safe int64
	for _, f := range fs {
		if f.Rule.Risk == Safe {
			safe += f.Size
		}
		size := humanize.Bytes(f.Size)
		if f.Rule.Kind == KindSnapshots {
			size = fmt.Sprintf("%d 個", f.Count)
		}
		tag := "[" + f.Rule.Risk.String() + "]"
		fmt.Fprintf(w, "%s %s  %s\n", c(riskColor(f.Rule.Risk)+bold, padRight(tag, 8)),
			c(bold, size), f.Rule.Title)
		if brief {
			continue
		}
		fmt.Fprintf(w, "         這是什麼：%s\n", f.Rule.What)
		fmt.Fprintf(w, "         能不能刪：%s\n", f.Rule.CanDelete)
		fmt.Fprintf(w, "         怎麼清理：%s\n", f.Rule.HowClean)
		if f.Note != "" {
			fmt.Fprintf(w, "         備註：%s\n", f.Note)
		}
		for _, it := range f.Items {
			sz := ""
			if it.Size > 0 {
				sz = humanize.Bytes(it.Size) + "  "
			}
			fmt.Fprintf(w, "           %s%s\n", c(dim, sz), ShortPath(env.Home, it.Path))
		}
		if len(f.Rule.Paths) > 0 && f.Rule.Kind == KindDir && len(f.Items) == 0 {
			fmt.Fprintf(w, "           %s\n", c(dim, ShortPath(env.Home, env.Expand(f.Rule.Paths[0]))))
		}
		fmt.Fprintln(w)
	}
	if brief && len(fs) > 0 {
		fmt.Fprintln(w)
	}
	if len(fs) == 0 {
		fmt.Fprintln(w, "沒有發現已知的大型項目。")
		return
	}
	fmt.Fprintf(w, "標示「安全」的項目合計約 %s（可重新產生，清掉不會遺失資料）。\n", humanize.Bytes(safe))
	fmt.Fprintln(w, "macdust hogs 只讀取、不會刪除任何東西；「使用者快取」內含 Homebrew、pip 等子項，數字有重疊。")
}

func padRight(s string, w int) string {
	n := 0
	for _, r := range s {
		if r > 0x2e80 {
			n += 2
		} else {
			n++
		}
	}
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}
