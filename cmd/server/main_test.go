package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/handler"
)

// TestEveryConfigFieldTheHandlerAlsoNamesIsWiredThrough guards a whole class of silent failure:
// config.Settings and handler.Config are two structs with overlapping field names, copied across by
// hand in the literal below. A field added to both and forgotten in the copy compiles, passes vet,
// passes every test that builds a handler directly — and reaches production as a zero value.
//
// That happened: TRACER_LINK_SALT was read into config.Settings and read out of handler.Config, with
// nothing in between, so CV link tracing reported "not available on this deployment" on a host
// where the salt was set.
//
// The literal is inspected as source rather than exercised, because building it needs a database,
// an LLM client and a Meilisearch — none of which this invariant depends on.
func TestEveryConfigFieldTheHandlerAlsoNamesIsWiredThrough(t *testing.T) {
	shared := sharedFieldNames(reflect.TypeOf(config.Settings{}), reflect.TypeOf(handler.Config{}))
	if len(shared) == 0 {
		t.Fatal("the two configs share no field names — this test is looking at the wrong types")
	}
	wired := fieldsSetInHandlerConfigLiteral(t)

	var missing []string
	for _, name := range shared {
		if !wired[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("handler.Config{...} in main.go never sets %s — the field exists on both configs, "+
			"so it reads as its zero value at runtime however the environment is set",
			strings.Join(missing, ", "))
	}
}

// sharedFieldNames returns the exported field names both structs declare.
func sharedFieldNames(a, b reflect.Type) []string {
	inB := make(map[string]bool, b.NumField())
	for i := range b.NumField() {
		inB[b.Field(i).Name] = true
	}
	var out []string
	for i := range a.NumField() {
		if name := a.Field(i).Name; inB[name] {
			out = append(out, name)
		}
	}
	return out
}

// fieldsSetInHandlerConfigLiteral reads main.go and reports which fields the handler.Config
// composite literal names.
func fieldsSetInHandlerConfigLiteral(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	set := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Config" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "handler" {
			return true
		}
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					set[key.Name] = true
				}
			}
		}
		return true
	})
	if len(set) == 0 {
		t.Fatal("found no handler.Config literal in main.go — the test cannot see what it checks")
	}
	return set
}
