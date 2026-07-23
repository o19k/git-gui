package tui

import (
	"os"
	"path/filepath"
)

// pinGitConfig gives the test binary a git configuration of its own: an
// identity, and nothing inherited from whoever is running the tests.
//
// The fixtures set GIT_AUTHOR_* on the commands they run themselves, but the
// commits under test are made by the git package's own code, which those
// variables never reach. On a machine with no identity in config — a CI runner
// — every one of them fails with "empty ident name".
func pinGitConfig() func() {
	dir, err := os.MkdirTemp("", "git-gui-config")
	if err != nil {
		panic(err)
	}
	path := filepath.Join(dir, "gitconfig")
	config := "[user]\n\tname = Test\n\temail = test@example.com\n"
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", path)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return func() { os.RemoveAll(dir) }
}
