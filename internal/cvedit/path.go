// Package cvedit owns the only path that writes a stored CV. An edit is a batch of
// operations over typed paths; applying one yields the operations that would undo it, and
// the pair is recorded as a revision. Nothing else may write the document — see Editor.
package cvedit

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/cv"
)

// State is the CV as a revision may change it: the document plus the two columns the
// candidate edits beside it. The document is embedded rather than nested so addresses stay
// flat — `summary`, not `document.summary` — and one address space covers everything an
// edit can reach.
//
// The rest of the `cvs` row is deliberately absent. The agent session id, the autopilot
// report and the copy's own identity are not things the candidate edited, and none of them
// would read sensibly in a feed of edits.
type State struct {
	Title      string `json:"title"`
	TemplateID string `json:"template_id"`
	cv.Document
}

// Path is the canonical text address of one place in a State: `summary`,
// `experience[3].bullets[1]`, `style.font_size`. It is what a revision stores, what the
// preview highlights, and what the policy and the evidence gate are keyed on.
type Path string

// step is one hop of a parsed path: a named field, or an index into a list.
type step struct {
	name  string // json name; empty on an index step
	index int    // -1 on a field step
}

func (s step) isIndex() bool { return s.name == "" }

// ParsePath reads an address and checks it against State's own structure, so an operation
// cannot name a place the document does not define. It does NOT check that an index is in
// range — that depends on the document's contents, and is resolve's job.
//
// Validation is by reflection over the json tags rather than against a list kept beside
// them. A hand-maintained vocabulary drifts: an op once went missing from the schema a
// model reads, and a model that cannot see an operation cannot use it.
func ParsePath(s string) (Path, error) {
	steps, err := parseSteps(s)
	if err != nil {
		return "", err
	}
	if err := walkType(reflect.TypeOf(State{}), steps, s); err != nil {
		return "", err
	}
	return Path(s), nil
}

// steps re-parses a Path that ParsePath already accepted. The syntax cannot fail twice, so
// callers inside this package take the steps and ignore the error path.
func (p Path) steps() []step {
	steps, _ := parseSteps(string(p))
	return steps
}

func parseSteps(s string) ([]step, error) {
	if s == "" {
		return nil, fmt.Errorf("cvedit: %q is not a path", s)
	}
	var steps []step
	for _, segment := range strings.Split(s, ".") {
		name, brackets, err := splitSegment(segment, s)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step{name: name, index: -1})
		for _, b := range brackets {
			i, err := strconv.Atoi(b)
			if err != nil || i < 0 {
				return nil, fmt.Errorf("cvedit: %q has an index that is not a whole number: %q", s, b)
			}
			steps = append(steps, step{index: i})
		}
	}
	return steps, nil
}

// splitSegment cuts one dot-separated segment into its field name and its bracketed
// indices, refusing a name that is empty or brackets that do not close.
func splitSegment(segment, whole string) (string, []string, error) {
	open := strings.IndexByte(segment, '[')
	if open < 0 {
		if segment == "" {
			return "", nil, fmt.Errorf("cvedit: %q has an empty segment", whole)
		}
		if strings.ContainsAny(segment, "]") {
			return "", nil, fmt.Errorf("cvedit: %q has an unbalanced bracket", whole)
		}
		return segment, nil, nil
	}
	name := segment[:open]
	if name == "" {
		return "", nil, fmt.Errorf("cvedit: %q has an empty segment", whole)
	}

	var brackets []string
	for rest := segment[open:]; rest != ""; {
		if rest[0] != '[' {
			return "", nil, fmt.Errorf("cvedit: %q has an unbalanced bracket", whole)
		}
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return "", nil, fmt.Errorf("cvedit: %q has an unbalanced bracket", whole)
		}
		inner := rest[1:end]
		if inner == "" {
			return "", nil, fmt.Errorf("cvedit: %q has an empty index", whole)
		}
		brackets = append(brackets, inner)
		rest = rest[end+1:]
	}
	return name, brackets, nil
}

// walkType checks the steps against the type, without needing a document to walk.
func walkType(t reflect.Type, steps []step, whole string) error {
	for _, s := range steps {
		if s.isIndex() {
			if t.Kind() != reflect.Slice {
				return fmt.Errorf("cvedit: %q indexes something that is not a list", whole)
			}
			t = t.Elem()
			continue
		}
		if t.Kind() != reflect.Struct {
			return fmt.Errorf("cvedit: %q reaches into a value that has no fields", whole)
		}
		idx, ok := fieldIndexByJSONName(t, s.name)
		if !ok {
			return fmt.Errorf("cvedit: %q names a field the CV does not have: %q", whole, s.name)
		}
		t = t.FieldByIndex(idx).Type
	}
	return nil
}

// resolve walks a parsed path down a live State and returns the addressed value, refusing
// an index past the end of its list. The value is addressable, so callers may set through
// it — the whole point of holding a State pointer.
func resolve(state *State, p Path) (reflect.Value, error) {
	return resolveSteps(state, p.steps(), p)
}

// resolveSteps walks a prefix of a path. Callers that change a list rather than an element —
// insert, remove, move — stop one step short, and pass the whole path for the message.
func resolveSteps(state *State, steps []step, p Path) (reflect.Value, error) {
	v := reflect.ValueOf(state).Elem()
	for _, s := range steps {
		if s.isIndex() {
			if s.index >= v.Len() {
				return reflect.Value{}, fmt.Errorf("cvedit: %q addresses position %d of a list holding %d",
					p, s.index, v.Len())
			}
			v = v.Index(s.index)
			continue
		}
		idx, ok := fieldIndexByJSONName(v.Type(), s.name)
		if !ok {
			return reflect.Value{}, fmt.Errorf("cvedit: %q names a field the CV does not have: %q", p, s.name)
		}
		v = v.FieldByIndex(idx)
	}
	return v, nil
}

// jsonName reads a struct field's wire name, or reports that the field is not addressable
// (unexported, or explicitly excluded from the wire shape).
func jsonName(f reflect.StructField) (string, bool) {
	if f.PkgPath != "" {
		return "", false // unexported
	}
	tag := f.Tag.Get("json")
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return "", false
	}
	if name == "" {
		return f.Name, true
	}
	return name, true
}

// fieldIndexByJSONName finds a field by its wire name and returns the index chain reaching
// it, descending into embedded structs so the document's own fields sit flat beside title
// and template_id. Type and value walks share it, so the two can never disagree about which
// field an address names.
func fieldIndexByJSONName(t reflect.Type, name string) ([]int, bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
			if inner, ok := fieldIndexByJSONName(f.Type, name); ok {
				return append([]int{i}, inner...), true
			}
			continue
		}
		if n, ok := jsonName(f); ok && n == name {
			return []int{i}, true
		}
	}
	return nil, false
}

// indexLetters name the list positions in an enumerated path shape, outermost first.
const indexLetters = "ijkl"

// Paths enumerates every shape an operation may address, with list positions written as
// `[i]`, `[j]`. It is generated from State by the same reflection ParsePath validates
// against, so the schema a model reads cannot disagree with the struct: a field added to
// the document becomes addressable and appears here without anyone editing a list.
func Paths() []string {
	var out []string
	enumerate(reflect.TypeOf(State{}), "", 0, &out)
	return out
}

func enumerate(t reflect.Type, prefix string, depth int, out *[]string) {
	switch t.Kind() {
	case reflect.Struct:
		for i := range t.NumField() {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
				enumerate(f.Type, prefix, depth, out)
				continue
			}
			name, ok := jsonName(f)
			if !ok {
				continue
			}
			child := name
			if prefix != "" {
				child = prefix + "." + name
			}
			*out = append(*out, child)
			enumerate(f.Type, child, depth, out)
		}
	case reflect.Slice:
		if depth >= len(indexLetters) {
			return
		}
		indexed := fmt.Sprintf("%s[%c]", prefix, indexLetters[depth])
		*out = append(*out, indexed)
		enumerate(t.Elem(), indexed, depth+1, out)
	}
}
