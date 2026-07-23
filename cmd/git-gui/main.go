// Command git-gui is a terminal git client.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/o19k/git-gui/internal/git"
	"github.com/o19k/git-gui/internal/tui"
)

// version is stamped at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	// git runs this program as its rebase editors (see internal/git/rebase.go).
	// Handled before flag parsing: those arguments are git's, not ours.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case git.TodoSubcommand:
			exitOn(git.RunTodoEditor(os.Args[2:]))
			return
		case git.MessageSubcommand:
			exitOn(git.RunMessageEditor(os.Args[2:]))
			return
		}
	}

	showVersion := flag.Bool("version", false, "print the version and exit")
	mouse := flag.Bool("mouse", false, "capture the mouse for scrolling and click-to-select")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: git-gui [path]\n\n  path  repository to open (default: current directory)\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("git-gui", version)
		return
	}

	path := "."
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	ctx := context.Background()
	repo, err := git.Open(ctx, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-gui:", err)
		os.Exit(1)
	}

	// Opt-in: capture costs the terminal's own text selection, and every mouse
	// action here already has a key.
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if *mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}

	program := tea.NewProgram(tui.New(ctx, repo), opts...)
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "git-gui:", err)
		os.Exit(1)
	}
}

// exitOn reports an error from a hidden subcommand and stops. git reads the
// exit status to decide whether the rebase can go ahead.
func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-gui:", err)
		os.Exit(1)
	}
}
