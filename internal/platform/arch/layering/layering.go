// Package layering checks that the packages under internal/ respect the block layering:
// every package lives in exactly one block, and a block imports only blocks strictly below
// its own layer.
//
// The check is a pure function over a graph and a table; nothing here reads the filesystem.
package layering

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Kind classifies a violation. The membership kinds describe a package that is in the
// wrong place; the edge kinds describe an import that crosses a layer it may not.
type Kind string

const (
	// KindUnassigned is a package sitting directly under internal/, in no block.
	KindUnassigned Kind = "unassigned"
	// KindUnknownBlock is a package under a block name that is not in the layer table.
	KindUnknownBlock Kind = "unknown-block"
	// KindUpward is an import into a block on a higher layer.
	KindUpward Kind = "upward"
	// KindSameLayer is an import into a different block on the same layer. Forbidden in
	// both directions: two blocks that can see each other are one block wearing two names.
	KindSameLayer Kind = "same-layer"
)

// Violation is one problem found in the graph. From is always set. To, FromLayer and
// ToLayer are set for the edge kinds; Block is set for KindUnknownBlock.
type Violation struct {
	Kind      Kind
	From      string
	To        string
	FromLayer int
	ToLayer   int
	Block     string
}

func (v Violation) String() string {
	switch v.Kind {
	case KindUnassigned:
		return fmt.Sprintf("%s: %s is directly under internal/, not in a block", v.Kind, v.From)
	case KindUnknownBlock:
		return fmt.Sprintf("%s: %s is under unknown block %q", v.Kind, v.From, v.Block)
	default:
		return fmt.Sprintf("%s: %s (layer %d) imports %s (layer %d)", v.Kind, v.From, v.FromLayer, v.To, v.ToLayer)
	}
}

// Check reports every way graph violates layers. graph maps a package's import path to the
// import paths it depends on, production and test alike; imports outside internal/ are
// ignored. layers maps a block name to its layer, counting up from 1 at the bottom.
//
// Violations are sorted by importing package, then by imported package, so a failure reads
// the same on every run.
func Check(graph map[string][]string, layers map[string]int) []Violation {
	var out []Violation

	for _, pkg := range slices.Sorted(maps.Keys(graph)) {
		block, ok := blockOf(pkg)
		if !ok {
			// Not under internal/ at all — cmd/, services/, anything else. Not governed.
			continue
		}
		if block == "" {
			out = append(out, Violation{Kind: KindUnassigned, From: pkg})
			continue
		}
		fromLayer, known := layers[block]
		if !known {
			out = append(out, Violation{Kind: KindUnknownBlock, From: pkg, Block: block})
			continue
		}

		for _, imp := range slices.Compact(slices.Sorted(slices.Values(graph[pkg]))) {
			toBlock, ok := blockOf(imp)
			if !ok || toBlock == "" || toBlock == block {
				continue
			}
			toLayer, known := layers[toBlock]
			if !known {
				// The target's own membership is already reported where it is the source.
				continue
			}
			if toLayer < fromLayer {
				continue
			}
			kind := KindUpward
			if toLayer == fromLayer {
				kind = KindSameLayer
			}
			out = append(out, Violation{
				Kind: kind, From: pkg, To: imp, FromLayer: fromLayer, ToLayer: toLayer,
			})
		}
	}
	return out
}

const marker = "/internal/"

// blockOf returns the block segment of an internal package path. The second return is
// false when the path is not under internal/ at all; the block is empty when the path is a
// package directly under internal/, which is a violation rather than a non-answer.
//
// It matches any /internal/: Go forbids importing another module's internal package, so a
// foreign one cannot appear in the graph.
func blockOf(pkg string) (string, bool) {
	i := strings.LastIndex(pkg, marker)
	if i < 0 {
		return "", false
	}
	rest := pkg[i+len(marker):]
	block, _, found := strings.Cut(rest, "/")
	if !found {
		return "", true
	}
	return block, true
}
