package cvedit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// Diff derives the operations that turn one state into another.
//
// It exists because the editor's forms bind to a single shared document and autosave ships
// it whole, so the only thing the server receives is "here is the new state" — no address,
// no intent. Deriving the operations here means a whole-document save is an input format
// rather than a second way to write the document: from Commit's point of view it is
// indistinguishable from an agent's batch.
//
// The result is not merely correct but ordered: applying it to `old` in sequence produces
// `new` exactly, indices included. That property is what every test asserts first.
func Diff(old, next State) []Op {
	var ops []Op
	diffValue(reflect.ValueOf(old), reflect.ValueOf(next), "", &ops)
	return ops
}

// Equal reports whether two states are the same document once stored. It goes through the
// differ rather than through DeepEqual so that the two representations of an empty list are
// treated as one, the same way every other comparison here does.
func Equal(a, b State) bool { return len(Diff(a, b)) == 0 }

func diffValue(old, next reflect.Value, path string, ops *[]Op) {
	switch old.Kind() {
	case reflect.Struct:
		diffStruct(old, next, path, ops)
	case reflect.Slice:
		diffSlice(old, next, path, ops)
	default:
		if old.Interface() != next.Interface() {
			*ops = append(*ops, Op{Kind: OpSet, Path: Path(path), Value: next.Interface()})
		}
	}
}

func diffStruct(old, next reflect.Value, path string, ops *[]Op) {
	t := old.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
			diffStruct(old.Field(i), next.Field(i), path, ops)
			continue
		}
		name, ok := jsonName(f)
		if !ok {
			continue
		}
		child := name
		if path != "" {
			child = path + "." + name
		}
		diffValue(old.Field(i), next.Field(i), child, ops)
	}
}

// diffSlice compares two lists. Equal lengths compare in place, which is what an editor
// almost always produces — text changed where it stood. Differing lengths go through the
// longest common subsequence, and the removes and inserts it yields are then collapsed where
// they describe a rewrite rather than a replacement.
func diffSlice(old, next reflect.Value, path string, ops *[]Op) {
	if equalValues(old, next) {
		return
	}
	if old.Len() == next.Len() {
		for i := range old.Len() {
			diffValue(old.Index(i), next.Index(i), fmt.Sprintf("%s[%d]", path, i), ops)
		}
		return
	}
	*ops = append(*ops, collapseRewrites(lcsOps(old, next, path))...)
}

// derived is an operation together with the element it came from. A remove carries no value
// on the wire, so without this the collapse below could not see what was taken out — and
// recovering it by matching text would go wrong the moment a list holds two equal entries.
type derived struct {
	op       Op
	value    reflect.Value // the element removed, or the element inserted
	position int           // where it sits in the list as the batch changes it
}

// lcsOps walks the two lists together and emits the removes and inserts that turn one into
// the other, tracking the position in the list as it is being changed so each operation's
// index is the one it will actually see.
func lcsOps(old, next reflect.Value, path string) []derived {
	table := lcsTable(old, next)

	var ops []derived
	i, j, pos := 0, 0, 0
	for i < old.Len() || j < next.Len() {
		switch {
		case i < old.Len() && j < next.Len() && equalValues(old.Index(i), next.Index(j)):
			i, j, pos = i+1, j+1, pos+1
		case i < old.Len() && (j == next.Len() || table[i+1][j] >= table[i][j+1]):
			// A tie removes before it inserts, so a rewritten element leaves an adjacent
			// remove/insert pair for collapseRewrites to recognise.
			ops = append(ops, derived{
				op:       Op{Kind: OpRemove, Path: Path(fmt.Sprintf("%s[%d]", path, pos))},
				value:    old.Index(i),
				position: pos,
			})
			i++
		default:
			ops = append(ops, derived{
				op: Op{
					Kind:  OpInsert,
					Path:  Path(fmt.Sprintf("%s[%d]", path, pos)),
					Value: next.Index(j).Interface(),
				},
				value:    next.Index(j),
				position: pos,
			})
			j, pos = j+1, pos+1
		}
	}
	return ops
}

func lcsTable(old, next reflect.Value) [][]int {
	table := make([][]int, old.Len()+1)
	for i := range table {
		table[i] = make([]int, next.Len()+1)
	}
	for i := old.Len() - 1; i >= 0; i-- {
		for j := next.Len() - 1; j >= 0; j-- {
			if equalValues(old.Index(i), next.Index(j)) {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	return table
}

// collapseRewrites turns an adjacent remove-then-insert at the same position into an edit of
// what is already there, when the two values are recognisably the same thing reworded.
//
// Without it, a candidate who rewords one bullet gets "removed a bullet, added a bullet" in
// their history — technically true and useless. For anything with fields of its own the
// collapse recurses instead of replacing wholesale, so the feed says which field moved
// rather than that an entry was swapped.
func collapseRewrites(ops []derived) []Op {
	out := make([]Op, 0, len(ops))
	for i := 0; i < len(ops); i++ {
		removed, inserted, ok := rewrittenPair(ops, i)
		if !ok {
			out = append(out, ops[i].op)
			continue
		}

		target := removed.op.Path
		if removed.value.Kind() == reflect.Struct {
			// An entry reworded rather than replaced: say which of its fields moved, not
			// that the whole entry was swapped.
			var nested []Op
			diffValue(removed.value, inserted.value, string(target), &nested)
			out = append(out, nested...)
		} else {
			out = append(out, Op{Kind: OpSet, Path: target, Value: inserted.value.Interface()})
		}
		i++ // the insert half of the pair is consumed
	}
	return out
}

// rewrittenPair recognises "this element was reworded": a remove immediately followed by an
// insert at the same position, holding values that still share most of their vocabulary.
func rewrittenPair(ops []derived, i int) (removed, inserted derived, ok bool) {
	if i+1 >= len(ops) {
		return derived{}, derived{}, false
	}
	a, b := ops[i], ops[i+1]
	if a.op.Kind != OpRemove || b.op.Kind != OpInsert || a.position != b.position {
		return derived{}, derived{}, false
	}
	if !similarEnough(textOf(a.value), textOf(b.value)) {
		return derived{}, derived{}, false
	}
	return a, b, true
}

func equalValues(a, b reflect.Value) bool {
	// An absent list and an empty one are the same stored state, since the document's json
	// tags omit both. Left to DeepEqual they would differ, and the differ would emit
	// operations between two documents that are identical once saved.
	if a.Kind() == reflect.Slice && b.Kind() == reflect.Slice && a.Len() == 0 && b.Len() == 0 {
		return true
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}

// textOf renders a value for comparison: a string as itself, anything else through json, so
// two entries that share most of their fields read as similar.
func textOf(v reflect.Value) string {
	if v.Kind() == reflect.String {
		return v.String()
	}
	blob, err := json.Marshal(v.Interface())
	if err != nil {
		return ""
	}
	return string(blob)
}

// rewriteSimilarity is the share of vocabulary two values must have before one is treated as
// a rewording of the other. Half is deliberately permissive on one side and strict on the
// other: "shipped the billing service" against the same line with three words appended
// scores well above it, while two unrelated bullets in the same entry score near zero.
const rewriteSimilarity = 0.5

func similarEnough(a, b string) bool {
	return jaccard(words(a), words(b)) >= rewriteSimilarity
}

func words(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		out[w] = struct{}{}
	}
	return out
}

// jaccard is shared words over total distinct words. Two empty sets are not similar: an
// empty signature means "nothing to compare", and calling that a perfect match would collapse
// every blank entry into its neighbour.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for w := range a {
		if _, ok := b[w]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}
