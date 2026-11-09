package facetidx

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring/v2"
)

// match evaluates a caller's filter (nil, string, []string, [][]string, []any: outer AND,
// inner OR) and restricts it to the live documents.
func (ix *Index) match(filter any) (*roaring.Bitmap, error) {
	if filter == nil {
		return ix.live, nil
	}
	got, err := ix.evalTop(filter)
	if err != nil {
		return nil, err
	}
	return roaring.And(got, ix.live), nil
}

// evalTop ANDs the elements of a top-level filter; a nested list inside is an OR group.
func (ix *Index) evalTop(filter any) (*roaring.Bitmap, error) {
	switch f := filter.(type) {
	case string:
		return ix.evalFragment(f)
	case []string:
		return ix.evalAll(len(f), func(i int) (*roaring.Bitmap, error) { return ix.evalFragment(f[i]) })
	case [][]string:
		return ix.evalAll(len(f), func(i int) (*roaring.Bitmap, error) { return ix.evalGroup(f[i]) })
	case []any:
		return ix.evalAll(len(f), func(i int) (*roaring.Bitmap, error) {
			switch g := f[i].(type) {
			case string:
				return ix.evalFragment(g)
			case []string:
				return ix.evalGroup(g)
			}
			return nil, fmt.Errorf("%w: element %T", ErrUnsupportedFilter, f[i])
		})
	}
	return nil, fmt.Errorf("%w: %T", ErrUnsupportedFilter, filter)
}

func (ix *Index) evalAll(n int, one func(int) (*roaring.Bitmap, error)) (*roaring.Bitmap, error) {
	acc := ix.all
	for i := 0; i < n; i++ {
		b, err := one(i)
		if err != nil {
			return nil, err
		}
		acc = roaring.And(acc, b)
	}
	return acc, nil
}

// evalGroup ORs the fragments of one group. Empty fragments are skipped, as the callers'
// builders skip them; a group with nothing in it constrains nothing, as Meilisearch ignores it.
func (ix *Index) evalGroup(group []string) (*roaring.Bitmap, error) {
	acc := roaring.New()
	if !slices.ContainsFunc(group, func(f string) bool { return strings.TrimSpace(f) != "" }) {
		return ix.all, nil
	}
	for _, frag := range group {
		if strings.TrimSpace(frag) == "" {
			continue
		}
		b, err := ix.evalFragment(frag)
		if err != nil {
			return nil, err
		}
		acc = roaring.Or(acc, b)
	}
	return acc, nil
}

func (ix *Index) evalFragment(frag string) (*roaring.Bitmap, error) {
	toks, err := tokenize(frag)
	if err != nil {
		return nil, err
	}
	p := &parser{ix: ix, toks: toks}
	b, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("%w: trailing %q in %q", ErrUnsupportedFilter, p.toks[p.pos].text, frag)
	}
	return b, nil
}

// --- tokens ---

type tokKind int

const (
	tWord tokKind = iota
	tStr
	tOp
	tLParen
	tRParen
	tLBrack
	tRBrack
	tComma
)

type token struct {
	kind tokKind
	text string
}

func tokenize(s string) ([]token, error) {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			i++
		case '(':
			toks, i = append(toks, token{tLParen, "("}), i+1
		case ')':
			toks, i = append(toks, token{tRParen, ")"}), i+1
		case '[':
			toks, i = append(toks, token{tLBrack, "["}), i+1
		case ']':
			toks, i = append(toks, token{tRBrack, "]"}), i+1
		case ',':
			toks, i = append(toks, token{tComma, ","}), i+1
		case '"', '\'':
			var b strings.Builder
			j := i + 1
			for ; j < len(s) && s[j] != c; j++ {
				if s[j] == '\\' && j+1 < len(s) {
					j++
				}
				b.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, fmt.Errorf("%w: unterminated string in %q", ErrUnsupportedFilter, s)
			}
			toks, i = append(toks, token{tStr, b.String()}), j+1
		case '=', '!', '<', '>':
			j := i + 1
			if j < len(s) && s[j] == '=' {
				j++
			}
			op := s[i:j]
			if op == "!" {
				return nil, fmt.Errorf("%w: bare ! in %q", ErrUnsupportedFilter, s)
			}
			toks, i = append(toks, token{tOp, op}), j
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\n\r()[],\"'=!<>", rune(s[j])) {
				j++
			}
			toks, i = append(toks, token{tWord, s[i:j]}), j
		}
	}
	return toks, nil
}

// --- parser/evaluator: expr := and (OR and)*; and := not (AND not)*; not := NOT not | primary ---

type parser struct {
	ix   *Index
	toks []token
	pos  int
}

func (p *parser) peekWord(w string) bool {
	return p.pos < len(p.toks) && p.toks[p.pos].kind == tWord && strings.EqualFold(p.toks[p.pos].text, w)
}

func (p *parser) eatWord(w string) bool {
	if p.peekWord(w) {
		p.pos++
		return true
	}
	return false
}

func (p *parser) bad(what string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedFilter, what)
}

func (p *parser) or() (*roaring.Bitmap, error) {
	acc, err := p.and()
	if err != nil {
		return nil, err
	}
	for p.eatWord("OR") {
		b, err := p.and()
		if err != nil {
			return nil, err
		}
		acc = roaring.Or(acc, b)
	}
	return acc, nil
}

func (p *parser) and() (*roaring.Bitmap, error) {
	acc, err := p.not()
	if err != nil {
		return nil, err
	}
	for p.eatWord("AND") {
		b, err := p.not()
		if err != nil {
			return nil, err
		}
		acc = roaring.And(acc, b)
	}
	return acc, nil
}

func (p *parser) not() (*roaring.Bitmap, error) {
	if p.eatWord("NOT") {
		b, err := p.not()
		if err != nil {
			return nil, err
		}
		return roaring.AndNot(p.ix.all, b), nil
	}
	if p.pos < len(p.toks) && p.toks[p.pos].kind == tLParen {
		p.pos++
		b, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.pos >= len(p.toks) || p.toks[p.pos].kind != tRParen {
			return nil, p.bad("missing )")
		}
		p.pos++
		return b, nil
	}
	return p.cond()
}

func (p *parser) value() (token, error) {
	if p.pos >= len(p.toks) || (p.toks[p.pos].kind != tWord && p.toks[p.pos].kind != tStr) {
		return token{}, p.bad("expected a value")
	}
	p.pos++
	return p.toks[p.pos-1], nil
}

func (p *parser) list() ([]token, error) {
	if p.pos >= len(p.toks) || p.toks[p.pos].kind != tLBrack {
		return nil, p.bad("expected [")
	}
	p.pos++
	var out []token
	for {
		if p.pos < len(p.toks) && p.toks[p.pos].kind == tRBrack {
			p.pos++
			return out, nil
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if p.pos < len(p.toks) && p.toks[p.pos].kind == tComma {
			p.pos++
		}
	}
}

func (p *parser) cond() (*roaring.Bitmap, error) {
	if p.pos >= len(p.toks) || p.toks[p.pos].kind != tWord {
		return nil, p.bad("expected an attribute")
	}
	attr := p.toks[p.pos].text
	p.pos++
	ix := p.ix
	switch {
	case p.pos < len(p.toks) && p.toks[p.pos].kind == tOp:
		op := p.toks[p.pos].text
		p.pos++
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		switch op {
		case "=":
			return ix.eq(attr, v)
		case "!=":
			return ix.neq(attr, v)
		default:
			return ix.cmp(attr, op, v)
		}
	case p.eatWord("IN"):
		return p.in(attr)
	case p.peekWord("NOT"):
		p.pos++
		switch {
		case p.eatWord("IN"):
			b, err := p.in(attr)
			if err != nil {
				return nil, err
			}
			return roaring.AndNot(ix.all, b), nil
		case p.eatWord("EXISTS"):
			f, err := ix.field(attr)
			if err != nil {
				return nil, err
			}
			return roaring.AndNot(ix.all, f.exists), nil
		}
	case p.eatWord("EXISTS"):
		f, err := ix.field(attr)
		if err != nil {
			return nil, err
		}
		return f.exists, nil
	case p.eatWord("IS"):
		neg := p.eatWord("NOT")
		if p.eatWord("EMPTY") {
			f, err := ix.field(attr)
			if err != nil {
				return nil, err
			}
			if neg {
				return roaring.AndNot(ix.all, f.empty), nil
			}
			return f.empty, nil
		}
	}
	return nil, p.bad("unknown condition on " + attr)
}

func (p *parser) in(attr string) (*roaring.Bitmap, error) {
	vs, err := p.list()
	if err != nil {
		return nil, err
	}
	acc := roaring.New()
	for _, v := range vs {
		b, err := p.ix.eq(attr, v)
		if err != nil {
			return nil, err
		}
		acc = roaring.Or(acc, b)
	}
	return acc, nil
}

// --- leaves ---

func (ix *Index) field(attr string) (*field, error) {
	f, ok := ix.fields[attr]
	if !ok {
		return nil, fmt.Errorf("%w: attribute %q", ErrUnsupportedFilter, attr)
	}
	return f, nil
}

// eq is `attr = value`. A numeric-looking literal also matches the number's canonical
// string, so `x = 5` and `x = "5.0"` agree as in Meilisearch.
func (ix *Index) eq(attr string, v token) (*roaring.Bitmap, error) {
	if attr == "id" {
		n, err := strconv.ParseInt(v.text, 10, 64)
		if err != nil {
			return roaring.New(), nil
		}
		if d, ok := ix.lookup(n); ok {
			return roaring.BitmapOf(d), nil
		}
		return roaring.New(), nil
	}
	f, err := ix.field(attr)
	if err != nil {
		return nil, err
	}
	out := roaring.New()
	keys := []string{norm(v.text)}
	if n, perr := strconv.ParseFloat(v.text, 64); perr == nil {
		if k := norm(formatNum(n)); k != keys[0] {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		if id, ok := f.keys[k]; ok {
			out = roaring.Or(out, f.bms[id])
		}
	}
	return out, nil
}

// neq is `attr != value`: every document whose attribute is not that value, those without
// the attribute included, EXCEPT that when no document carries the attribute at all it
// matches nothing (Meilisearch v1.49).
func (ix *Index) neq(attr string, v token) (*roaring.Bitmap, error) {
	if attr != "id" {
		f, err := ix.field(attr)
		if err != nil {
			return nil, err
		}
		if f.postings == 0 && len(f.num) == 0 { // no value anywhere; vals may outlive removed ones
			return roaring.New(), nil
		}
	}
	b, err := ix.eq(attr, v)
	if err != nil {
		return nil, err
	}
	return roaring.AndNot(ix.all, b), nil
}

// cmp is a numeric range: only documents with a numeric element compare.
func (ix *Index) cmp(attr, op string, v token) (*roaring.Bitmap, error) {
	f, err := ix.field(attr)
	if err != nil {
		return nil, err
	}
	x, perr := strconv.ParseFloat(v.text, 64)
	if perr != nil || math.IsNaN(x) {
		return nil, p0(ErrUnsupportedFilter, "non-numeric bound "+v.text)
	}
	// [lo, hi) of the sorted column that satisfies the comparison.
	lo, hi := 0, len(f.num)
	first := func(pred func(float64) bool) int {
		return sort.Search(len(f.num), func(i int) bool { return pred(f.num[i].v) })
	}
	switch op {
	case ">=":
		lo = first(func(e float64) bool { return e >= x })
	case ">":
		lo = first(func(e float64) bool { return e > x })
	case "<=":
		hi = first(func(e float64) bool { return e > x })
	case "<":
		hi = first(func(e float64) bool { return e >= x })
	default:
		return nil, p0(ErrUnsupportedFilter, "operator "+op)
	}
	if hi <= lo {
		return roaring.New(), nil
	}
	docs := func(es []numEntry) *roaring.Bitmap {
		ids := make([]uint32, len(es))
		for i, e := range es {
			ids[i] = e.doc
		}
		return roaring.BitmapOf(ids...)
	}
	// With one number per document a range over most of the column is cheaper as the
	// complement of the two ends; with several, a document can sit on both sides.
	if hi-lo > len(f.num)/2 && uint64(len(f.num)) == f.numeric.GetCardinality() {
		return roaring.AndNot(roaring.AndNot(f.numeric, docs(f.num[:lo])), docs(f.num[hi:])), nil
	}
	return docs(f.num[lo:hi]), nil
}

func p0(err error, msg string) error { return fmt.Errorf("%w: %s", err, msg) }
