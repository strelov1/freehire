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

// TestMarkersAnswer calls every marker on every registered adapter. The markers are empty
// methods read only by type assertion, so nothing else runs them; this holds each one callable
// and keeps the value-returning ones (SweepGrace, ValidateBoard) from panicking on a zero adapter.
func TestMarkersAnswer(t *testing.T) {
	for name, s := range All(nil) {
		if m, ok := s.(boardless); ok {
			m.Boardless()
		}
		if m, ok := s.(boardValidator); ok {
			_ = m.ValidateBoard("x")
		}
		if m, ok := s.(aggregator); ok {
			m.Aggregator()
		}
		if m, ok := s.(social); ok {
			m.Social()
		}
		if m, ok := s.(unknownOrigin); ok {
			m.UnknownOrigin()
		}
		if m, ok := s.(selfClosing); ok {
			m.SelfClosing()
		}
		if m, ok := s.(probeClosing); ok {
			m.ProbeClosing()
		}
		if m, ok := s.(fullCatalog); ok {
			m.FullCatalog()
		}
		if m, ok := s.(sliceBoardListing); ok {
			m.SliceBoardListing()
		}
		if m, ok := s.(sweepGrace); ok && m.SweepGrace() <= 0 {
			t.Errorf("%s: SweepGrace() = %v, want a positive window", name, m.SweepGrace())
		}
		if m, ok := s.(livenessProbed); ok {
			m.LivenessProbed()
		}
		if m, ok := s.(fullBoardListing); ok {
			m.FullBoardListing()
		}
		if m, ok := s.(emptyBoardListing); ok {
			m.EmptyBoardListing()
		}
	}
}
