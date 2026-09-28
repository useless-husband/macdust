// Package humanize formats and parses byte sizes.
//
// Sizes are shown in decimal units (1 GB = 1,000,000,000 bytes) to match
// what Finder and "About This Mac" display on macOS.
package humanize

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var units = []string{"B", "KB", "MB", "GB", "TB", "PB"}

// Bytes formats n as e.g. "12.3 GB". Negative values are clamped to 0.
func Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	i := 0
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

var sizeRe = regexp.MustCompile(`^\s*([0-9]*\.?[0-9]+)\s*([a-zA-Z]*)\s*$`)

// ParseSize parses strings such as "100MB", "1.5G", "500k" or "2GiB".
// K/KB/M/MB... are decimal; KiB/MiB/... are binary. A bare number is bytes.
func ParseSize(s string) (int64, error) {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("無法解析大小 %q（範例：100MB、1.5G、500k）", s)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("無法解析大小 %q", s)
	}
	var mult float64
	switch strings.ToLower(m[2]) {
	case "", "b":
		mult = 1
	case "k", "kb":
		mult = 1e3
	case "m", "mb":
		mult = 1e6
	case "g", "gb":
		mult = 1e9
	case "t", "tb":
		mult = 1e12
	case "kib":
		mult = 1 << 10
	case "mib":
		mult = 1 << 20
	case "gib":
		mult = 1 << 30
	case "tib":
		mult = 1 << 40
	default:
		return 0, fmt.Errorf("未知的單位 %q", m[2])
	}
	r := v * mult
	if r > math.MaxInt64 {
		return 0, fmt.Errorf("數值太大：%q", s)
	}
	return int64(r), nil
}
