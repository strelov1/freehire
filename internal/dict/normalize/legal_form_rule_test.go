package normalize

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/modroot"
)

// canonicalFormList is the one file allowed to enumerate corporate forms.
const canonicalFormList = "internal/dict/normalize/company.go"

// formTokens are distinctive enough that a file listing several of them is defining a
// legal-form vocabulary rather than mentioning one in passing.
var formTokens = map[string]bool{
	"ltd": true, "limited": true, "gmbh": true, "inc": true, "incorporated": true,
	"llc": true, "plc": true, "llp": true, "bv": true, "nv": true, "srl": true,
	"pty": true, "corp": true, "corporation": true, "aps": true, "oy": true,
	"cic": true, "cio": true,
}

// minTokensForAList is how many distinct form tokens a file must hold before it counts as
// defining a list. Three would fire on prose; a real vocabulary carries many more.
const minTokensForAList = 5

// TestOnlyOneLegalFormListExists is the guard that three lists is how this change started.
//
// internal/dict/normalize, internal/job/collections/register.go and cmd/harvest-ats each defined the
// corporate forms separately, and they disagreed on substance — one stripped repeatedly from
// 20 tokens, another stripped once from 15 and explicitly refused "co". The failure was
// silent: Collection.Members looks a register slug up in a map keyed by the catalogue's own
// company slug, so a form only one side knew simply matched nothing, and no log said so.
//
// A comment cannot hold that. Adding a fourth list is easy, reviews miss it, and nothing
// breaks until a company quietly stops earning a credential.
func TestOnlyOneLegalFormListExists(t *testing.T) {
	root, rootErr := modroot.Find()
	if rootErr != nil {
		t.Fatal(rootErr)
	}

	var checkedCanonical bool
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// A dot- or underscore-prefixed directory is not part of this module: the go
			// toolchain never descends into one, so `./...` cannot compile what lives there.
			// This walk is a filesystem walk, not a package walk, and without the same rule it
			// reads every git worktree checked out INSIDE the repo (.worktrees, .claude/worktrees)
			// as a second copy of company.go — one violation per worktree, none of them real.
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return fs.SkipDir
			}
			// Vendored and generated trees are not ours to police.
			if name == "node_modules" || name == "web" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		n := countFormTokens(t, path)
		if rel == canonicalFormList {
			checkedCanonical = n >= minTokensForAList
			return nil
		}
		if n >= minTokensForAList {
			t.Errorf("%s enumerates %d corporate-form tokens — %s is the only place that may. "+
				"Call normalize.IsLegalForm or normalize.CompanySlug instead; a second list "+
				"disagrees with this one sooner or later, and the disagreement is silent.",
				rel, n, canonicalFormList)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	// Counting the population, not just the violations: a detector that has drifted and now
	// matches nothing looks exactly like a codebase with no duplicate lists.
	if !checkedCanonical {
		t.Errorf("the detector did not find a form list in %s — it has drifted from how the "+
			"tokens are written and would no longer catch a duplicate", canonicalFormList)
	}
}

// countFormTokens returns how many DISTINCT form tokens appear as string literals in a file.
// Parsing rather than grepping so a token inside a comment or an identifier does not count.
func countFormTokens(t *testing.T, path string) int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		// A file this package cannot parse is not one it can police; skipping is safer than
		// failing an unrelated test on someone else's syntax error.
		return 0
	}
	found := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if key := strings.Trim(strings.ToLower(s), "-. "); formTokens[key] {
			found[key] = true
		}
		return true
	})
	return len(found)
}
