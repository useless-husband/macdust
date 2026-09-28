package humanize

import "testing"

func TestBytes(t *testing.T) {
	cases := map[int64]string{
		-5: "0 B", 0: "0 B", 999: "999 B", 1000: "1.0 KB", 1500: "1.5 KB",
		12_300_000_000: "12.3 GB", 2_000_000_000_000: "2.0 TB",
	}
	for in, want := range cases {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%d)=%q want %q", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	ok := map[string]int64{
		"100": 100, "1k": 1000, "1KB": 1000, "1.5G": 1_500_000_000,
		"100MB": 100_000_000, "2GiB": 2 << 30, " 3 mib ": 3 << 20, ".5k": 500,
	}
	for in, want := range ok {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q)=%d,%v want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "10XB", "-5", "1..2"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) should fail", bad)
		}
	}
}
