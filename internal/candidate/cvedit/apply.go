package cvedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// OpKind is the closed set of things an edit can do. The four structural operations reach
// every field of the document.
type OpKind string

const (
	OpSet    OpKind = "set"
	OpInsert OpKind = "insert"
	OpRemove OpKind = "remove"
	OpMove   OpKind = "move"
)

// OpKinds is the vocabulary as a caller meets it, for the schema a model reads.
var OpKinds = []string{string(OpSet), string(OpInsert), string(OpRemove), string(OpMove)}

// Op is one edit. Path addresses the place; Value carries the new content for set and
// insert; To is the destination index for move. EvidenceID accompanies an operation that
// writes a claim about the candidate, and is checked by the gate rather than here — Apply
// is a pure transform and knows nothing about the experience bank.
type Op struct {
	Kind       OpKind `json:"kind"`
	Path       Path   `json:"path"`
	Value      any    `json:"value,omitempty"`
	To         *int   `json:"to,omitempty"`
	EvidenceID string `json:"evidence_id,omitempty"`
}

// ErrInvalidOp marks an operation that cannot be carried out against this state: an index
// past the end of its list, a value of the wrong shape, a list operation on something that
// is not a list. Handlers render it as a client error.
var ErrInvalidOp = errors.New("cvedit: invalid operation")

// Apply carries out a batch and returns the state it produced together with the operations
// that would undo it.
//
// Inverses are computed here because this is the only moment the previous value is still in
// hand. They come back in reverse order, so applying them in sequence walks the state backwards.
//
// The batch is all-or-nothing: Apply works on a deep copy and hands it back only on success,
// so a refused batch leaves the caller's state untouched.
func Apply(state State, ops []Op) (State, []Op, error) {
	working, err := clone(state)
	if err != nil {
		return state, nil, err
	}

	inverse := make([]Op, 0, len(ops))
	for i, op := range ops {
		undo, err := applyOne(&working, op)
		if err != nil {
			return state, nil, fmt.Errorf("operation %d: %w", i+1, err)
		}
		inverse = append([]Op{undo}, inverse...)
	}
	return working, inverse, nil
}

// OrderAgainstOriginal rewrites a batch written against the document as the author SAW it into
// one that means the same thing applied in sequence, by running each list's consecutive
// removals from the highest index down.
//
// It must not move into Apply: Apply's indices are sequential (each operation addresses the
// list as changed so far), and both Diff and undo's replayed inverses depend on that. Only a
// model's batch, written against positions it read once, needs this conversion.
func OrderAgainstOriginal(ops []Op) []Op {
	ordered := make([]Op, len(ops))
	copy(ordered, ops)

	for start := 0; start < len(ordered); {
		list, _, ok := listAddress(ordered[start])
		if !ok {
			start++
			continue
		}
		end := start + 1
		for end < len(ordered) {
			next, _, ok := listAddress(ordered[end])
			if !ok || next != list {
				break
			}
			end++
		}
		run := ordered[start:end]
		sort.SliceStable(run, func(a, b int) bool {
			_, x, _ := listAddress(run[a])
			_, y, _ := listAddress(run[b])
			return x > y
		})
		start = end
	}
	return ordered
}

// listAddress splits a removal's path into the list it addresses and the position within it —
// `experience[0].bullets[3]` becomes `experience[0].bullets` and 3. It reports false for
// anything that is not a removal from a position, which is what excludes every other operation
// from reordering.
func listAddress(op Op) (string, int, bool) {
	if op.Kind != OpRemove {
		return "", 0, false
	}
	s := string(op.Path)
	open := strings.LastIndexByte(s, '[')
	if open < 0 || !strings.HasSuffix(s, "]") {
		return "", 0, false
	}
	at, err := strconv.Atoi(s[open+1 : len(s)-1])
	if err != nil {
		return "", 0, false
	}
	return s[:open], at, true
}

func applyOne(state *State, op Op) (Op, error) {
	switch op.Kind {
	case OpSet:
		return applySet(state, op)
	case OpInsert:
		return applyInsert(state, op)
	case OpRemove:
		return applyRemove(state, op)
	case OpMove:
		return applyMove(state, op)
	default:
		return Op{}, fmt.Errorf("%w: %q is not one of %v", ErrInvalidOp, op.Kind, OpKinds)
	}
}

func applySet(state *State, op Op) (Op, error) {
	target, err := resolve(state, op.Path)
	if err != nil {
		return Op{}, fmt.Errorf("%w: %s", ErrInvalidOp, err)
	}
	next, err := coerce(op.Value, target.Type(), op.Path)
	if err != nil {
		return Op{}, err
	}
	previous, err := detach(target)
	if err != nil {
		return Op{}, err
	}
	target.Set(next)
	return Op{Kind: OpSet, Path: op.Path, Value: previous}, nil
}

func applyInsert(state *State, op Op) (Op, error) {
	list, at, err := resolveList(state, op.Path)
	if err != nil {
		return Op{}, err
	}
	// One past the end appends.
	if at > list.Len() {
		return Op{}, fmt.Errorf("%w: %q inserts at position %d of a list holding %d%s",
			ErrInvalidOp, op.Path, at, list.Len(), holds(list))
	}
	value, err := coerce(op.Value, list.Type().Elem(), op.Path)
	if err != nil {
		return Op{}, err
	}

	list.Set(sliceInsertAt(list, at, value))
	return Op{Kind: OpRemove, Path: op.Path}, nil
}

func applyRemove(state *State, op Op) (Op, error) {
	list, at, err := resolveList(state, op.Path)
	if err != nil {
		return Op{}, err
	}
	if at >= list.Len() {
		return Op{}, fmt.Errorf("%w: %q removes position %d of a list holding %d%s",
			ErrInvalidOp, op.Path, at, list.Len(), holds(list))
	}
	removed, err := detach(list.Index(at))
	if err != nil {
		return Op{}, err
	}

	list.Set(sliceRemoveAt(list, at))
	return Op{Kind: OpInsert, Path: op.Path, Value: removed}, nil
}

func applyMove(state *State, op Op) (Op, error) {
	if op.To == nil {
		return Op{}, fmt.Errorf("%w: %q is a move with no destination", ErrInvalidOp, op.Path)
	}
	list, from, err := resolveList(state, op.Path)
	if err != nil {
		return Op{}, err
	}
	to := *op.To
	if from >= list.Len() || to >= list.Len() || to < 0 {
		return Op{}, fmt.Errorf("%w: %q moves %d to %d in a list holding %d",
			ErrInvalidOp, op.Path, from, to, list.Len())
	}

	// Copy the element out before the removal shifts the indices.
	element := reflect.New(list.Type().Elem()).Elem()
	element.Set(list.Index(from))
	list.Set(sliceInsertAt(sliceRemoveAt(list, from), to, element))

	back := from
	return Op{Kind: OpMove, Path: replaceLastIndex(op.Path, to), To: &back}, nil
}

// sliceInsertAt and sliceRemoveAt build a new slice rather than shuffling in place, which
// could alias the backing array of an inverse detached earlier.
func sliceInsertAt(list reflect.Value, at int, v reflect.Value) reflect.Value {
	grown := reflect.MakeSlice(list.Type(), list.Len()+1, list.Len()+1)
	reflect.Copy(grown, list.Slice(0, at))
	grown.Index(at).Set(v)
	reflect.Copy(grown.Slice(at+1, grown.Len()), list.Slice(at, list.Len()))
	return grown
}

func sliceRemoveAt(list reflect.Value, at int) reflect.Value {
	if list.Len() == 1 {
		// Emptying a list yields the absent list: the json tags omit both, and keeping them
		// distinct would make the differ report operations between identical documents.
		return reflect.Zero(list.Type())
	}
	shrunk := reflect.MakeSlice(list.Type(), list.Len()-1, list.Len()-1)
	reflect.Copy(shrunk, list.Slice(0, at))
	reflect.Copy(shrunk.Slice(at, shrunk.Len()), list.Slice(at+1, list.Len()))
	return shrunk
}

// resolveList reaches the list an indexed path sits in, and the position it names. Insert,
// remove and move change the list, not the element (which does not exist yet for an append).
func resolveList(state *State, p Path) (reflect.Value, int, error) {
	steps := p.steps()
	last := steps[len(steps)-1]
	if !last.isIndex() {
		// A bare list path is a common agent mistake; spell out the append index.
		if v, err := resolveSteps(state, steps, p); err == nil && v.Kind() == reflect.Slice {
			return reflect.Value{}, 0, fmt.Errorf("%w: %q is a whole list, not a position in it: use \"%s[%d]\" to add at the end, or name an existing position%s",
				ErrInvalidOp, p, p, v.Len(), holds(v))
		}
		return reflect.Value{}, 0, fmt.Errorf("%w: %q does not name a position in a list", ErrInvalidOp, p)
	}
	list, err := resolveSteps(state, steps[:len(steps)-1], p)
	if err != nil {
		return reflect.Value{}, 0, fmt.Errorf("%w: %s", ErrInvalidOp, err)
	}
	return list, last.index, nil
}

// replaceLastIndex rewrites the trailing `[n]` of a path: a move's inverse starts from where
// the element now sits.
func replaceLastIndex(p Path, index int) Path {
	s := string(p)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '[' {
			return Path(fmt.Sprintf("%s[%d]", s[:i], index))
		}
	}
	return p
}

// coerce turns a caller's value into the addressed field's own type, going through json so a
// Go value and one read back from jsonb (stored inverses) are treated identically.
func coerce(value any, want reflect.Type, p Path) (reflect.Value, error) {
	blob, err := json.Marshal(value)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("%w: %q was given a value that is not json: %s", ErrInvalidOp, p, err)
	}
	out := reflect.New(want)
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("%w: %q was given a value it cannot hold: %s", ErrInvalidOp, p, err)
	}
	return out.Elem(), nil
}

// detach copies a value out of the state so an inverse holding it survives later edits; a
// shallow read would alias a slice the batch is about to rewrite.
func detach(v reflect.Value) (any, error) {
	blob, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func clone(s State) (State, error) {
	blob, err := json.Marshal(s)
	if err != nil {
		return State{}, err
	}
	var out State
	if err := json.Unmarshal(blob, &out); err != nil {
		return State{}, err
	}
	return out, nil
}
