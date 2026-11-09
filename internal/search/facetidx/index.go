// Package facetidx is an in-memory facet engine: roaring bitmaps per attribute value, a
// filter evaluator over the Meilisearch filter fragments our code builds, and facet counts,
// totals and distinct counts that match Meilisearch's answers. It is immutable once built.
package facetidx

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RoaringBitmap/roaring/v2"
)

// ErrUnsupportedFilter is returned for any filter shape the evaluator does not know. The
// caller falls back to Meilisearch; the evaluator never reads an unknown shape as match-all.
var ErrUnsupportedFilter = errors.New("facetidx: unsupported filter")

// ErrUnknownFacet is returned for a facet or distinct attribute the index does not hold.
var ErrUnknownFacet = errors.New("facetidx: attribute is not indexed")

// Walk-array thresholds: a forward array (document -> value ids) is built only for facets
// where walking the filter's documents can beat ANDing every value bitmap.
const (
	fwdMinValues      = 64 // high cardinality
	fwdMinMultiValues = 16 // multi-value facets (skills, perks, regions...) with fewer values
)

// norm is the key a facet string is compared under. Meilisearch filters are case-insensitive
// on string values; the distribution still reports the spelling the first document used.
func norm(s string) string { return strings.ToLower(s) }

type numEntry struct {
	v   float64
	doc uint32
}

// field is one indexed attribute.
type field struct {
	exists  *roaring.Bitmap // the key is in the document (null, [] and "" included)
	empty   *roaring.Bitmap // empty array or empty string
	numeric *roaring.Bitmap // documents with at least one numeric element

	keys map[string]uint32 // norm(value) -> value id
	vals []string          // display spelling per value id
	bms  []*roaring.Bitmap // documents per value id

	cards    []uint32        // value cardinalities, ascending
	cardSum  []uint64        // prefix sums of cards
	postings uint64          // total (document, value) pairs
	valued   *roaring.Bitmap // documents with at least one value
	withVals uint64          // its cardinality
	num      []numEntry

	// forward array in CSR form: values of document d are fwd[off[d]:off[d+1]], ascending.
	// Exactly one of fwd16 (< 65536 values) and fwd32 is set when the array exists.
	off   []uint32
	fwd16 []uint16
	fwd32 []uint32
}

// Index is an immutable snapshot of the catalogue's facets. Every answer is restricted to
// open documents: closed, duplicate and deleted ones are subtracted automatically, so the
// filter a caller passes arrives without the open-jobs clause.
type Index struct {
	n       uint32
	jobIDs  []int64 // doc id -> job id
	docOf   map[int64]uint32
	all     *roaring.Bitmap
	live    *roaring.Bitmap // all minus closed, duplicate and deleted
	closed  *roaring.Bitmap
	dup     *roaring.Bitmap
	deleted *roaring.Bitmap
	fields  map[string]*field
}

// Builder collects documents and produces an Index.
type Builder struct {
	ix *Index
}

// NewBuilder returns an empty builder.
func NewBuilder() *Builder {
	ix := &Index{docOf: map[int64]uint32{}, closed: roaring.New(), dup: roaring.New(), deleted: roaring.New(), fields: map[string]*field{}}
	for _, a := range indexedAttrs {
		ix.fields[a] = &field{exists: roaring.New(), empty: roaring.New(), numeric: roaring.New(), keys: map[string]uint32{}}
	}
	return &Builder{ix: ix}
}

// Add indexes one document: its JSON as the search drain sends it to Meilisearch (a
// []byte or json.RawMessage), or any value that marshals to it (search.JobDocument). A job
// id seen twice is an error: the builder is for a fresh generation, writes come later.
func (b *Builder) Add(doc any) error {
	var raw []byte
	switch d := doc.(type) {
	case []byte:
		raw = d
	case json.RawMessage:
		raw = d
	default:
		var err error
		if raw, err = json.Marshal(doc); err != nil {
			return fmt.Errorf("facetidx: marshal document: %w", err)
		}
	}
	fl, err := flatten(raw)
	if err != nil {
		return err
	}
	ix := b.ix
	if _, dup := ix.docOf[fl.id]; dup {
		return fmt.Errorf("facetidx: duplicate job id %d", fl.id)
	}
	d := ix.n
	ix.n++
	ix.jobIDs = append(ix.jobIDs, fl.id)
	ix.docOf[fl.id] = d
	if fl.closed {
		ix.closed.Add(d)
	}
	if fl.duplicate {
		ix.dup.Add(d)
	}
	for name, p := range fl.attrs {
		f := ix.fields[name]
		f.exists.Add(d)
		if p.empty {
			f.empty.Add(d)
		}
		for _, s := range p.strs {
			k := norm(s)
			id, ok := f.keys[k]
			if !ok {
				id = uint32(len(f.vals))
				f.keys[k] = id
				f.vals = append(f.vals, s)
				f.bms = append(f.bms, roaring.New())
			} else if s < f.vals[id] {
				f.vals[id] = s // the spelling Meilisearch reports for case variants: the smallest
			}
			f.bms[id].Add(d) // idempotent: a repeated value in one array counts the document once
		}
		for _, v := range p.nums {
			f.num = append(f.num, numEntry{v, d})
			f.numeric.Add(d)
		}
	}
	return nil
}

// Build finishes the index. The builder must not be used afterwards.
func (b *Builder) Build() *Index {
	ix := b.ix
	ix.all = roaring.New()
	if ix.n > 0 {
		ix.all.AddRange(0, uint64(ix.n))
	}
	ix.live = roaring.AndNot(roaring.AndNot(roaring.AndNot(ix.all, ix.closed), ix.dup), ix.deleted)
	for _, f := range ix.fields {
		f.finish(ix.n)
	}
	return ix
}

func (f *field) finish(n uint32) {
	slices.SortFunc(f.num, func(a, b numEntry) int {
		switch {
		case a.v < b.v:
			return -1
		case a.v > b.v:
			return 1
		}
		return int(a.doc) - int(b.doc)
	})
	f.exists.RunOptimize()
	f.empty.RunOptimize()
	f.numeric.RunOptimize()
	f.cards = make([]uint32, len(f.bms))
	f.valued = roaring.New()
	for i, bm := range f.bms {
		bm.RunOptimize()
		f.cards[i] = uint32(bm.GetCardinality())
		f.postings += uint64(f.cards[i])
		f.valued.Or(bm)
	}
	f.valued.RunOptimize()
	f.withVals = f.valued.GetCardinality()
	slices.Sort(f.cards)
	f.cardSum = make([]uint64, len(f.cards)+1)
	for i, c := range f.cards {
		f.cardSum[i+1] = f.cardSum[i] + uint64(c)
	}
	multi := f.postings > f.withVals
	if len(f.vals) >= fwdMinValues || (multi && len(f.vals) >= fwdMinMultiValues) {
		f.buildForward(n)
	}
}

// buildForward turns the value bitmaps into the CSR document -> values array. Values are
// visited in id order, so each document's list comes out ascending and duplicate-free.
func (f *field) buildForward(n uint32) {
	f.off = make([]uint32, n+1)
	for _, bm := range f.bms {
		it := bm.Iterator()
		for it.HasNext() {
			f.off[it.Next()+1]++
		}
	}
	for d := uint32(0); d < n; d++ {
		f.off[d+1] += f.off[d]
	}
	pos := slices.Clone(f.off[:n])
	small := len(f.vals) < 1<<16
	if small {
		f.fwd16 = make([]uint16, f.postings)
	} else {
		f.fwd32 = make([]uint32, f.postings)
	}
	for v, bm := range f.bms {
		it := bm.Iterator()
		for it.HasNext() {
			d := it.Next()
			if small {
				f.fwd16[pos[d]] = uint16(v)
			} else {
				f.fwd32[pos[d]] = uint32(v)
			}
			pos[d]++
		}
	}
}
