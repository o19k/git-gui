// Command git-gui is a terminal git client.
package main

import (
	"context"
	"errors"
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
	// Before anything that can fail: a panic with the terminal on the alternate
	// screen leaves nothing to read the failure on.
	defer restoreOnPanic()

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

	// Preferences live in git config, so a run starts where the last one left
	// off. Reading them cannot fail in a way worth stopping for: an
	// unconfigured repository and an unreadable one both mean the defaults.
	settings := repo.LoadSettings(ctx)

	// Opt-in: capture costs the terminal's own text selection, and every mouse
	// action here already has a key. The flag turns it on for one run, the
	// setting for every one.
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if *mouse || settings.Mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}

	program := tea.NewProgram(tui.New(ctx, repo, tui.WithSettings(settings)), opts...)
	_, err = program.Run()

	// A panic has already printed the stack bubbletea caught, and clearing
	// would erase the one record of it.
	panicked := errors.Is(err, tea.ErrProgramPanic)

	// Before any error goes out, or it would be the thing that gets erased.
	if isTerminal(os.Stdout) && !panicked {
		clearScreen(os.Stdout)
	}
	if err != nil {
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
