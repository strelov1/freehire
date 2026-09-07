package mailpreview

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestEveryRendererIsRegistered derives the expected contents of `renderers` from the
// renderers themselves.
//
// `renderers` is a hand-written slice, and everything downstream reads it: the contact
// sheet, the Storybook gallery, and TestPreviewsAreCurrent. That last one is the trap —
// it walks the same slice, so it checks that what is listed is current and can say
// nothing about what is missing. A renderer written and not listed produces no file, no
// gallery entry and no failure; the design review simply never sees that mail.
//
// This is the repository's recurring shape, and the answer is always the same: derive
// the list from its members instead of holding a second copy by hand. A Go slice of
// funcs cannot enumerate itself, so the enumeration comes from the package's own source
// and the registration is read back through the function values' names.
//
// What it cannot see is a mail with no renderer at all. That is a human judgement about
// what the product sends, and the package doc says so rather than implying this test
// covers it.
func TestEveryRendererIsRegistered(t *testing.T) {
	declared := declaredRenderers(t)
	if len(declared) == 0 {
		t.Fatal("no renderer declarations found in the package source — this guard would pass on anything")
	}

	registered := make([]string, 0, len(renderers))
	for _, r := range renderers {
		registered = append(registered, funcName(r))
	}

	for _, name := range declared {
		if !slices.Contains(registered, name) {
			t.Errorf("%s renders a mail but is missing from `renderers`, so it reaches neither the "+
				"previews on disk nor the staleness check", name)
		}
	}
}

// funcName is the declared name of a top-level function value, e.g. "digestSample".
func funcName(f func(string) (Sample, error)) string {
	full := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	return full[strings.LastIndex(full, ".")+1:]
}

// declaredRenderers reads the package's own non-test sources and returns every top-level
// function with a renderer's signature — func(string) (Sample, error). The shared
// helpers (sample, campaignSample, onboardingSample) take more arguments and Samples
// returns a slice, so the signature alone separates the mails from the machinery.
//
// `go test` runs with the package directory as the working directory, so the sources are
// simply "." — no module root to resolve.
func declaredRenderers(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if slices.Equal(fieldTypes(fn.Type.Params), []string{"string"}) &&
				slices.Equal(fieldTypes(fn.Type.Results), []string{"Sample", "error"}) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names
}

// fieldTypes names each parameter or result in order, expanding a shared type
// declaration (`a, b string`) into one entry per name. A type this test does not need to
// tell apart — a slice, a qualified name — is reported as "?", which matches nothing.
func fieldTypes(list *ast.FieldList) []string {
	if list == nil {
		return nil
	}
	var out []string
	for _, f := range list.List {
		typ := "?"
		if ident, ok := f.Type.(*ast.Ident); ok {
			typ = ident.Name
		}
		count := max(len(f.Names), 1)
		for range count {
			out = append(out, typ)
		}
	}
	return out
}
