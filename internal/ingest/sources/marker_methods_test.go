package sources

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// legacyMarkerMethods are the capability-marker methods by their old unexported names. The
// markers are exported so an adapter in a family subpackage can implement them; an adapter that
// still declares the old name compiles but silently loses the capability, so this test fails it.
var legacyMarkerMethods = map[string]bool{
	"boardless": true, "validateBoard": true, "aggregator": true, "social": true,
	"unknownOrigin": true, "selfClosing": true, "probeClosing": true, "fullCatalog": true,
	"sliceBoardListing": true, "sweepGrace": true, "livenessProbed": true,
	"fullBoardListing": true, "emptyBoardListing": true,
}

func TestNoLegacyMarkerMethods(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && legacyMarkerMethods[fn.Name.Name] {
				t.Errorf("%s: method %s is a capability marker under its old name; export it (%s)",
					fset.Position(fn.Pos()), fn.Name.Name, strings.ToUpper(fn.Name.Name[:1])+fn.Name.Name[1:])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
