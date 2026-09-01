package search

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/modroot"
)

// A writer that builds a job document and pushes it WITHOUT widening its geography does not
// merely fail to widen — it NARROWS. The push is a field-level document update and the three
// geography facets are always present in the payload, so the row's own values replace whatever
// union the last rebuild wrote. The posting stays in the index, still looks right, and simply
// stops being findable by the cities of the rows it represents.
//
// That failure is invisible from inside the writer: no error, no missing document, nothing a
// unit test of that writer would think to assert. The rule therefore lives here, structurally,
// the way internal/ai/llmkey pins its own can't-see-it-afterwards invariant — a new writer that
// forgets the call fails at this test rather than in a facet count nobody audits.
//
// This is the guard for the two incremental paths in particular. The full rebuild is easy to
// verify by hand (its output is the whole index); the drain and the link import each touch a
// handful of documents at a time, which is exactly how a narrowing goes unnoticed.
func TestEveryJobDocumentWriterWidensGeography(t *testing.T) {
	const (
		builder = "FromJob"
		widener = "MergeClosureGeography"
	)

	root, err := modroot.Find()
	if err != nil {
		t.Fatal(err)
	}

	var checked []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Vendored and generated trees hold no writers and would only slow the walk.
			if name := d.Name(); name == "node_modules" || name == ".git" || name == "web" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return nil // not our business to fail on a file the compiler will reject anyway
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			builds, widens := false, false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					switch fun.Sel.Name {
					case builder:
						builds = true
					case widener:
						widens = true
					}
				case *ast.Ident:
					if fun.Name == builder {
						builds = true
					}
				}
				return true
			})
			if !builds {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			checked = append(checked, filepath.ToSlash(rel)+":"+fn.Name.Name)
			if !widens {
				t.Errorf("%s: %s builds a job document but never calls %s — the push would "+
					"REPLACE the widened geography with this row's own, not just skip widening",
					filepath.ToSlash(rel), fn.Name.Name, widener)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}

	// Without this the test passes by finding nothing — a renamed builder, a moved package or
	// a bad root would all read as "every writer complies". Three writers exist today: the full
	// rebuild, the incremental drain, and the link import.
	if len(checked) < 3 {
		t.Fatalf("found %d job-document writers (%v), want at least 3 — the guard is not "+
			"looking at what it thinks it is", len(checked), checked)
	}
}
