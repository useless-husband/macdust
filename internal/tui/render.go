package tui

import (
	"fmt"
	"strings"

	"github.com/useless-husband/macdust/internal/humanize"
)

const (
	rst  = "\x1b[0m"
	bld  = "\x1b[1m"
	dm   = "\x1b[2m"
	rev  = "\x1b[7m"
	blue = "\x1b[34m"
	ylw  = "\x1b[33m"
	red  = "\x1b[31m"
	grn  = "\x1b[32m"
)

// Render draws the whole screen as one string of exactly m.H lines (each
// clipped to m.W cells) separated by "\n". Colours are ANSI escape codes.
func Render(m *Model) string {
	if m.W < 24 || m.H < 6 {
		return "視窗太小，請放大\n"
	}
	lines := make([]string, 0, m.H)
	lines = append(lines, header(m), summary(m), dm+strings.Repeat("─", m.W)+rst)

	rows := m.rows()
	switch {
	case m.Scanning:
		lines = append(lines, fmt.Sprintf("  掃描中… 已看過 %d 個檔案、%d 個資料夾，共 %s",
			m.ScanFiles, m.ScanDirs, humanize.Bytes(m.ScanBytes)))
	case len(m.View) == 0:
		msg := "  （空的）"
		if m.Query != "" || m.MinSize > 0 {
			msg = "  （沒有符合的項目）"
		}
		lines = append(lines, dm+msg+rst)
	default:
		end := m.Offset + rows
		if end > len(m.View) {
			end = len(m.View)
		}
		for i := m.Offset; i < end; i++ {
			lines = append(lines, row(m, i))
		}
	}
	for len(lines) < m.H-2 {
		lines = append(lines, "")
	}
	lines = lines[:m.H-2]
	lines = append(lines, statusLine(m), helpLine(m))

	for i, l := range lines {
		lines[i] = clipANSI(l, m.W)
	}
	return strings.Join(lines, "\n")
}

func header(m *Model) string {
	path := ""
	if m.Cur != nil {
		path = shortHome(m.Home, m.Cur.Path)
	}
	title := " macdust "
	return bld + title + rst + dm + "│ " + rst + truncateLeft(sanitize(path), m.W-width(title)-2)
}

func summary(m *Model) string {
	if m.Cur == nil || m.Scanning {
		return dm + " 掃描進行中" + rst
	}
	parts := []string{
		"合計 " + humanize.Bytes(m.Cur.Size),
		fmt.Sprintf("%d 項", m.Cur.Items),
		"排序：" + m.Sort.String(),
	}
	if m.Query != "" {
		parts = append(parts, fmt.Sprintf("篩選「%s」%d/%d", sanitize(m.Query), len(m.View), len(m.Cur.Children)))
	}
	if !m.AllowDelete {
		parts = append(parts, "唯讀")
	}
	if m.DiskTotal > 0 {
		parts = append(parts, fmt.Sprintf("磁碟可用 %s / %s", humanize.Bytes(int64(m.DiskFree)), humanize.Bytes(int64(m.DiskTotal))))
	}
	return " " + strings.Join(parts, " · ")
}

func barWidth(w int) int {
	b := w / 5
	if b < 6 {
		b = 6
	}
	if b > 24 {
		b = 24
	}
	return b
}

func bar(frac float64, w int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac*float64(w) + 0.5)
	if n == 0 && frac > 0 {
		n = 1
	}
	return strings.Repeat("█", n) + strings.Repeat("░", w-n)
}

func row(m *Model, i int) string {
	n := m.View[i]
	var frac float64
	if m.Cur.Size > 0 {
		frac = float64(n.Size) / float64(m.Cur.Size)
	}
	bw := barWidth(m.W)
	// marker(2) size(9) sp bar sp pct(6) sp
	fixed := 2 + 9 + 1 + bw + 1 + 6 + 1
	name := sanitize(n.Name)
	switch {
	case n.IsDir && n.OtherFS:
		name += "/ [其他磁碟]"
	case n.IsDir:
		name += "/"
	case n.IsSymlink:
		name += "@"
	}
	if n.Err != "" {
		name += " [!" + n.Err + "]"
	}
	if n.Linked {
		name += " [硬連結]"
	}
	name = truncate(name, m.W-fixed)

	marker := "  "
	if i == m.Cursor {
		marker = "> "
	}
	plain := marker + padLeft(humanize.Bytes(n.Size), 9) + " " + bar(frac, bw) + " " +
		padLeft(fmt.Sprintf("%.1f%%", frac*100), 6) + " " + name
	if i == m.Cursor {
		return rev + padRight(plain, m.W) + rst
	}
	colour := ""
	if n.IsDir {
		colour = blue + bld
	} else if n.Err != "" {
		colour = red
	}
	return marker + padLeft(humanize.Bytes(n.Size), 9) + " " + ylw + bar(frac, bw) + rst + " " +
		padLeft(fmt.Sprintf("%.1f%%", frac*100), 6) + " " + colour + name + rst
}

func statusLine(m *Model) string {
	switch m.Mode {
	case ModeSearch:
		return " 搜尋：" + sanitize(m.Input) + "█"
	case ModeConfirm:
		n := m.confirm
		if n == nil {
			return ""
		}
		return red + bld + " 將「" + sanitize(n.Name) + "」（" + humanize.Bytes(n.Size) +
			"）移到垃圾桶？輸入 yes 後按 Enter，Esc 取消：" + rst + sanitize(m.Input) + "█"
	}
	if m.Status != "" {
		return grn + " " + m.Status + rst
	}
	return ""
}

func helpLine(m *Model) string {
	switch m.Mode {
	case ModeSearch:
		return dm + " 輸入關鍵字即時篩選  Enter 確定  Esc 清除" + rst
	case ModeConfirm:
		return dm + " 只有輸入 yes 才會執行；其他一律取消" + rst
	}
	s := " ↑↓/jk 移動  Enter/→ 進入  ←/⌫ 返回  s 排序  / 搜尋  o Finder"
	if m.AllowDelete {
		s += "  d 刪除"
	}
	return dm + s + "  q 離開" + rst
}

func shortHome(home, p string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// clipANSI cuts s so that its visible width does not exceed w. Escape
// sequences (CSI ... final byte) are copied through without counting.
func clipANSI(s string, w int) string {
	var b strings.Builder
	used := 0
	rs := []rune(s)
	clipped := false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b && i+1 < len(rs) && rs[i+1] == '[' {
			j := i + 2
			for j < len(rs) && !(rs[j] >= 0x40 && rs[j] <= 0x7e) {
				j++
			}
			if j < len(rs) {
				b.WriteString(string(rs[i : j+1]))
				i = j
				continue
			}
		}
		rw := runeWidth(r)
		if used+rw > w {
			clipped = true
			break
		}
		b.WriteRune(r)
		used += rw
	}
	if clipped {
		b.WriteString(rst)
	}
	return b.String()
}
