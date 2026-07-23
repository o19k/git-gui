package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"
)

// A panel binding is dispatched before the remote keys and before the global
// switch, so adding one silently takes the key away from every tab. Reading the
// three switches to check that is exactly the review that has already missed it
// once — the plan for this tab proposed p, g and v for the Explorer, and all
// three were taken — so the check is mechanical instead.
//
// Keys are collected from the source rather than declared in a list here: a
// list would be one more thing to forget to update, which is the failure this
// test exists to prevent.

// caseKeys returns every string literal used as a case in a switch inside the
// named function of a file.
func caseKeys(t *testing.T, path, fn string) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	var keys []string
	for _, decl := range file.Decls {
		decl, ok := decl.(*ast.FuncDecl)
		if !ok || decl.Name.Name != fn {
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if key, err := strconv.Unquote(lit.Value); err == nil && key != "" {
					keys = append(keys, key)
				}
			}
			return true
		})
	}
	if len(keys) == 0 {
		t.Fatalf("found no case keys in %s of %s — has it been renamed?", fn, path)
	}
	return keys
}

func TestExplorerKeysDoNotShadowGlobalOnes(t *testing.T) {
	global := caseKeys(t, "model.go", "handleKey")
	remote := caseKeys(t, "actions.go", "handleRemoteKey")

	explorer := slices.Concat(
		caseKeys(t, "explorer.go", "explorerKey"),
		caseKeys(t, "explorerops.go", "explorerOpKey"),
		caseKeys(t, "explorersearch.go", "explorerSearchKey"),
	)

	for _, key := range explorer {
		if slices.Contains(explorerOverrides, key) {
			continue
		}
		if slices.Contains(global, key) {
			t.Errorf("the Explorer binds %q, which the global switch already means; "+
				"a panel binding wins, so that meaning is lost in this tab", key)
		}
		if slices.Contains(remote, key) {
			t.Errorf("the Explorer binds %q, which is a remote operation in every tab", key)
		}
	}
}

// The overrides are deliberate. Growing the list is a decision, not an
// accident, and this is what makes it one.
func TestExplorerOverridesAreOnlyTheNavigationKeys(t *testing.T) {
	want := []string{
		"h", "left", "l", "right",
		"j", "down", "k", "up",
		"g", "G",
		"ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "pgdown", "pgup",
	}
	if !slices.Equal(explorerOverrides, want) {
		t.Errorf("explorerOverrides is %v, want %v — every entry costs that key its "+
			"global meaning inside the Explorer", explorerOverrides, want)
	}

	// An override only earns the name if it really is taking something over.
	global := caseKeys(t, "model.go", "handleKey")
	for _, key := range explorerOverrides {
		if !slices.Contains(global, key) {
			t.Errorf("%q is listed as an override but is not a global key", key)
		}
	}
}

// paneNavigation is how the focus moves between panes. Only the Explorer may
// take these — it declares that, and the test above holds it to the list.
var paneNavigation = []string{"h", "left", "l", "right", "tab", "shift+tab"}

// The Explorer was checked and the ordinary panels were not, which is how file
// history on h came to swallow the only letter that walks out of a commit's
// file list. A panel wins the key outright, so binding one of these strands the
// focus in the pane that took it.
func TestNoPanelBindingShadowsPaneNavigation(t *testing.T) {
	panels := map[string]string{
		"filesKey":       "actions.go",
		"branchesKey":    "actions.go",
		"commitsKey":     "actions.go",
		"commitFilesKey": "actions.go",
		"stashKey":       "actions.go",
		"stashFilesKey":  "stashfiles.go",
	}

	for fn, path := range panels {
		for _, key := range caseKeys(t, path, fn) {
			if slices.Contains(paneNavigation, key) {
				t.Errorf("%s binds %q, which moves between panes; a panel binding wins, "+
					"so that pane cannot be left by it", fn, key)
			}
		}
	}
}
