// Command macdust finds out what is eating your Mac's disk space.
package main

import (
	"os"

	"golang.org/x/term"

	"github.com/useless-husband/macdust/internal/cli"
	"github.com/useless-husband/macdust/internal/tui"
)

func main() {
	home, _ := os.UserHomeDir()
	out := term.IsTerminal(int(os.Stdout.Fd()))
	os.Exit(cli.Run(os.Args[1:], cli.Deps{
		Home:      home,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		StdoutTTY: out,
		Interact:  out && term.IsTerminal(int(os.Stdin.Fd())),
		RunTUI:    tui.Run,
	}))
}
