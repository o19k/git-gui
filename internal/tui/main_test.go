package tui

import (
	"os"
	"testing"

	"github.com/o19k/git-gui/internal/git"
)

// TestMain answers the hidden subcommands git invokes as its rebase editors.
// Without it a rebase would run this test binary as the sequence editor, which
// would re-run the whole suite recursively.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case git.TodoSubcommand:
			if err := git.RunTodoEditor(os.Args[2:]); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		case git.MessageSubcommand:
			if err := git.RunMessageEditor(os.Args[2:]); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	cleanup := pinGitConfig()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
