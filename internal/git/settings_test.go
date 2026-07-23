package git

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestSettingsDefaultWhenNothingIsConfigured(t *testing.T) {
	repo, _ := newRepo(t)

	settings := repo.LoadSettings(context.Background())

	if settings.LogLimit != 500 || settings.DiffContext != 3 {
		t.Errorf("defaults = %+v", settings)
	}
	if settings.Light || settings.Mouse || settings.Signoff || settings.IgnoreWhitespace {
		t.Errorf("a switch defaulted to on: %+v", settings)
	}
}

func TestSettingsAreReadFromGitConfig(t *testing.T) {
	repo, dir := newRepo(t)
	config(t, dir, "gitgui.theme", "light")
	config(t, dir, "gitgui.mouse", "yes")
	config(t, dir, "gitgui.loglimit", "1200")
	config(t, dir, "gitgui.signoff", "true")
	config(t, dir, "gitgui.diffcontext", "8")
	config(t, dir, "gitgui.ignorewhitespace", "off")
	// The checks live in the same namespace and are read elsewhere; an unknown
	// key must not upset the rest.
	config(t, dir, "gitgui.check", "true")

	settings := repo.LoadSettings(context.Background())

	if !settings.Light || !settings.Mouse || !settings.Signoff {
		t.Errorf("switches not read: %+v", settings)
	}
	if settings.IgnoreWhitespace {
		t.Error("off should read as false")
	}
	if settings.LogLimit != 1200 || settings.DiffContext != 8 {
		t.Errorf("numbers not read: %+v", settings)
	}
}

// A limit of zero would leave the panel permanently empty, which looks like a
// broken repository rather than a bad setting.
func TestSettingsClampNumbersToWhatIsUsable(t *testing.T) {
	repo, dir := newRepo(t)
	config(t, dir, "gitgui.loglimit", "0")
	config(t, dir, "gitgui.diffcontext", "-4")

	settings := repo.LoadSettings(context.Background())

	if settings.LogLimit != minLogLimit {
		t.Errorf("log limit = %d, want it clamped to %d", settings.LogLimit, minLogLimit)
	}
	if settings.DiffContext != 0 {
		t.Errorf("diff context = %d, want a negative one clamped to git's default", settings.DiffContext)
	}
}

func TestDiffOptsCarryTheSettings(t *testing.T) {
	opts := Settings{DiffContext: 12, IgnoreWhitespace: true}.Diff()

	args := opts.args()
	if !contains(args, "--unified=12") {
		t.Errorf("context width missing: %v", args)
	}
	if !contains(args, "--ignore-all-space") {
		t.Errorf("whitespace flag missing: %v", args)
	}

	// A patch generated this way cannot be applied back, so the staging path
	// has to strip it.
	if opts.Applicable().IgnoreWhitespace {
		t.Error("Applicable kept the flag that makes a patch unapplicable")
	}
	if opts.Applicable().Context != 12 {
		t.Error("Applicable dropped the context width, which decides the hunk numbering")
	}
}

// The width really reaches git: a wider context joins edits that were two
// hunks into one.
func TestDiffContextReachesGit(t *testing.T) {
	repo := multiHunkRepo(t)
	ctx := context.Background()

	narrow, err := repo.Diff(ctx, "f.txt", false, DiffOpts{Context: 1})
	if err != nil {
		t.Fatal(err)
	}
	wide, err := repo.Diff(ctx, "f.txt", false, DiffOpts{Context: 20})
	if err != nil {
		t.Fatal(err)
	}

	if got := len(ParseFileDiff(narrow).Hunks); got != 2 {
		t.Errorf("one line of context gave %d hunks, want 2", got)
	}
	if got := len(ParseFileDiff(wide).Hunks); got != 1 {
		t.Errorf("twenty lines of context gave %d hunks, want 1", got)
	}
}

func config(t *testing.T, dir, key, value string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "config", "--add", key, value)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config %s: %v\n%s", key, err, out)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
