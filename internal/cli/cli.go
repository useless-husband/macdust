// Package cli implements the macdust command line.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/useless-husband/macdust/internal/hogs"
	"github.com/useless-husband/macdust/internal/humanize"
	"github.com/useless-husband/macdust/internal/scan"
	"github.com/useless-husband/macdust/internal/tui"
)

// Version is set at build time with -ldflags "-X .../cli.Version=v1.0.0".
var Version = "dev"

// Deps are the things the command line needs from its environment.
type Deps struct {
	Home      string
	Stdout    io.Writer
	Stderr    io.Writer
	StdoutTTY bool // stdout is a terminal (enables colour)
	Interact  bool // stdin and stdout are terminals (the TUI can run)
	RunTUI    func(tui.Config) error
	// Run overrides command execution for `hogs` (tests).
	Run func(name string, args ...string) (string, error)
}

const usage = `macdust - 找出 Mac 空間被誰吃掉

用法：
  macdust [選項] [路徑]      互動式瀏覽（預設路徑為目前目錄）
  macdust hogs [選項]        檢查 macOS 常見吃空間的地方（唯讀）
  macdust version

選項：
  --allow-delete    允許在 TUI 按 d 把項目移到垃圾桶（預設唯讀）
  --json            輸出 JSON，不進入 TUI
  --top N           列出最大的 N 個檔案，不進入 TUI
  --min-size SIZE   只顯示不小於 SIZE 的項目（例如 100MB、1.5G）
  --depth N         --json 時輸出的層數（預設 2）
  --cross-fs        允許跨越檔案系統（預設不跨越）
  --workers N       平行讀取目錄的數量

hogs 選項：
  --brief           每項只印一行
  --projects DIRS   以逗號分隔，指定要找 node_modules 的資料夾
  --json            輸出 JSON
`

// Run executes the command line and returns the process exit code.
func Run(args []string, d Deps) int {
	if len(args) > 0 {
		switch args[0] {
		case "hogs":
			return runHogs(args[1:], d)
		case "version":
			fmt.Fprintln(d.Stdout, "macdust", Version)
			return 0
		case "help":
			fmt.Fprint(d.Stdout, usage)
			return 0
		}
	}
	return runScan(args, d)
}

func newFlags(name string, d Deps) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseInterspersed lets flags appear before or after positional arguments.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func runScan(args []string, d Deps) int {
	fs := newFlags("macdust", d)
	var (
		allowDelete = fs.Bool("allow-delete", false, "")
		asJSON      = fs.Bool("json", false, "")
		top         = fs.Int("top", 0, "")
		minSize     = fs.String("min-size", "0", "")
		depth       = fs.Int("depth", 2, "")
		crossFS     = fs.Bool("cross-fs", false, "")
		workers     = fs.Int("workers", 0, "")
		version     = fs.Bool("version", false, "")
		help        = fs.Bool("help", false, "")
		h           = fs.Bool("h", false, "")
	)
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		fmt.Fprintf(d.Stderr, "macdust: %v\n\n%s", err, usage)
		return 2
	}
	if *help || *h {
		fmt.Fprint(d.Stdout, usage)
		return 0
	}
	if *version {
		fmt.Fprintln(d.Stdout, "macdust", Version)
		return 0
	}
	if len(pos) > 1 {
		fmt.Fprintf(d.Stderr, "macdust: 只能指定一個路徑\n")
		return 2
	}
	min, err := humanize.ParseSize(*minSize)
	if err != nil {
		fmt.Fprintf(d.Stderr, "macdust: --min-size：%v\n", err)
		return 2
	}
	if *top < 0 || *depth < 0 {
		fmt.Fprintln(d.Stderr, "macdust: --top 與 --depth 不能是負數")
		return 2
	}
	path := "."
	if len(pos) == 1 {
		path = pos[0]
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		path = d.Home + strings.TrimPrefix(path, "~")
	}
	if *allowDelete && (*asJSON || *top > 0) {
		fmt.Fprintln(d.Stderr, "macdust: --allow-delete 只能用在互動模式")
		return 2
	}

	if !*asJSON && *top == 0 {
		if !d.Interact || d.RunTUI == nil {
			fmt.Fprintln(d.Stderr, "macdust: 需要在終端機中執行才能使用互動模式；非互動請加 --json 或 --top N")
			return 2
		}
		if err := d.RunTUI(tui.Config{Path: path, Home: d.Home, MinSize: min, CrossFS: *crossFS, AllowDelete: *allowDelete}); err != nil {
			fmt.Fprintf(d.Stderr, "macdust: %v\n", err)
			return 1
		}
		return 0
	}

	start := time.Now()
	root, prog, err := scan.Scan(context.Background(), path, scan.Options{Workers: *workers, CrossFS: *crossFS}, nil)
	if err != nil {
		fmt.Fprintf(d.Stderr, "macdust: %v\n", err)
		return 1
	}
	elapsed := time.Since(start)
	fmt.Fprintf(d.Stderr, "掃描 %d 個檔案、%d 個資料夾，共 %s，耗時 %.2f 秒；%d 個項目無法讀取\n",
		prog.Files.Load(), prog.Dirs.Load(), humanize.Bytes(root.Size), elapsed.Seconds(), prog.Errors.Load())

	switch {
	case *top > 0 && *asJSON:
		var out []scan.JSONNode
		for _, n := range scan.TopFiles(root, *top, min) {
			out = append(out, scan.ToJSON(n, 0, 0))
		}
		return writeJSON(d, out)
	case *top > 0:
		for _, n := range scan.TopFiles(root, *top, min) {
			fmt.Fprintf(d.Stdout, "%10s  %s\n", humanize.Bytes(n.Size), hogs.ShortPath(d.Home, n.Path))
		}
		return 0
	}
	return writeJSON(d, scan.ToJSON(root, *depth, min))
}

func writeJSON(d Deps, v interface{}) int {
	enc := json.NewEncoder(d.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(d.Stderr, "macdust: %v\n", err)
		return 1
	}
	return 0
}

func runHogs(args []string, d Deps) int {
	fs := newFlags("hogs", d)
	brief := fs.Bool("brief", false, "")
	projects := fs.String("projects", "", "")
	asJSON := fs.Bool("json", false, "")
	noColor := fs.Bool("no-color", false, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) > 0 {
		if err == nil {
			err = fmt.Errorf("不認得的參數 %q", pos[0])
		}
		fmt.Fprintf(d.Stderr, "macdust hogs: %v\n\n%s", err, usage)
		return 2
	}
	env := hogs.Env{Home: d.Home, Root: "/", Run: d.Run}
	for _, p := range strings.Split(*projects, ",") {
		if p = strings.TrimSpace(p); p != "" {
			if strings.HasPrefix(p, "~/") {
				p = d.Home + p[1:]
			}
			env.Projects = append(env.Projects, p)
		}
	}
	fmt.Fprintln(d.Stderr, "正在檢查常見的吃空間位置（只讀取，不會刪除任何東西）…")
	start := time.Now()
	fs2 := hogs.Check(context.Background(), env, hogs.Rules)
	fmt.Fprintf(d.Stderr, "完成，耗時 %.2f 秒\n\n", time.Since(start).Seconds())

	if *asJSON {
		type item struct {
			Path string `json:"path"`
			Size int64  `json:"size,omitempty"`
		}
		type out struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Risk  string `json:"risk"`
			Size  int64  `json:"size"`
			Human string `json:"size_human"`
			Count int    `json:"count,omitempty"`
			Note  string `json:"note,omitempty"`
			Items []item `json:"items,omitempty"`
		}
		var res []out
		for _, f := range fs2 {
			o := out{f.Rule.ID, f.Rule.Title, f.Rule.Risk.String(), f.Size, humanize.Bytes(f.Size), f.Count, f.Note, nil}
			for _, it := range f.Items {
				o.Items = append(o.Items, item{it.Path, it.Size})
			}
			res = append(res, o)
		}
		return writeJSON(d, res)
	}
	hogs.Write(d.Stdout, env, fs2, *brief, d.StdoutTTY && !*noColor && os.Getenv("NO_COLOR") == "")
	return 0
}
