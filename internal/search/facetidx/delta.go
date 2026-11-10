package facetidx

import (
	"maps"
	"slices"
	"sort"
	"sync/atomic"

	"github.com/RoaringBitmap/roaring/v2"
)

// What a write costs. A batch touches a few thousand documents of a catalogue of millions, so
// nothing a field holds per document may be copied or rebuilt per batch. Two structures would
// be, and both carry a small overlay instead that is folded in later:
//
// The number column (num, sorted by value) is a base plus numAdd, a small sorted delta of the
// entries written since, and numDead, the documents whose base entries no longer count. The
// two merge back into one column once the delta outgrows numDeltaMax.
//
// The forward array (off/fwd16/fwd32, document -> value ids) stays valid for every document
// except those in fdirty, whose current value ids are in fresh. A write adds the documents it
// touches to fdirty and to dirtyLog (the same documents in write order, a slice shared by
// generations like jobIDs). The background refresh rebuilds the array from a generation G and
// installs it on whatever generation is current by then, carrying over only the documents
// dirtied after G: dirtyLog past G's length, in the same epoch (one epoch per installed
// array). So a refresh always lands, however fast writes come.

// numDeltaMin is the delta size below which the number column is never merged.
const numDeltaMin = 1024

// dirtyMax is how many dirtied documents a field's forward overlay may accumulate before the
// background refresh rebuilds the array. A var so tests can make the refresh run often.
var dirtyMax = 16384

var epochs atomic.Uint64

func nextEpoch() uint64 { return epochs.Add(1) }

// --- number column ---

// numDeltaMax is how large the delta and the dead set may grow before a merge: a share of
// the base, so merging costs O(1) amortised per written entry.
func (f *field) numDeltaMax() int { return max(numDeltaMin, len(f.num)/16) }

// numAll is the live column: base entries of documents not dead, merged with the delta.
func (f *field) numAll() []numEntry {
	if len(f.numAdd) == 0 && (f.numDead == nil || f.numDead.IsEmpty()) {
		return f.num
	}
	base := make([]numEntry, 0, len(f.num))
	for _, e := range f.num {
		if f.numDead == nil || !f.numDead.Contains(e.doc) {
			base = append(base, e)
		}
	}
	return mergeNums(base, f.numAdd)
}

// setNumBase makes num the whole column: no delta, nothing dead.
func (f *field) setNumBase(num []numEntry) {
	f.num, f.numAdd, f.numDead = num, nil, nil
	f.numBase = roaring.New()
	for _, e := range num {
		f.numBase.Add(e.doc)
	}
	f.numBase.RunOptimize()
	f.numOne = uint64(len(num)) == f.numBase.GetCardinality()
}

// numRange is the documents with a number inside both bounds: lower(v) reports that v is at
// or past the lower bound, upper(v) that v is past the upper one; nil means unbounded.
func (f *field) numRange(lower, upper func(float64) bool) *roaring.Bitmap {
	bounds := func(col []numEntry) (int, int) {
		lo, hi := 0, len(col)
		if lower != nil {
			lo = sort.Search(len(col), func(i int) bool { return lower(col[i].v) })
		}
		if upper != nil {
			hi = sort.Search(len(col), func(i int) bool { return upper(col[i].v) })
		}
		return lo, hi
	}
	docs := func(es []numEntry) *roaring.Bitmap {
		ids := make([]uint32, len(es))
		for i, e := range es {
			ids[i] = e.doc
		}
		return roaring.BitmapOf(ids...)
	}
	out := roaring.New()
	if lo, hi := bounds(f.num); hi > lo {
		// With one number per document a range over most of the column is cheaper as the
		// complement of the two ends; with several, a document can sit on both sides.
		if hi-lo > len(f.num)/2 && f.numOne {
			out = roaring.AndNot(roaring.AndNot(f.numBase, docs(f.num[:lo])), docs(f.num[hi:]))
		} else {
			out = docs(f.num[lo:hi])
		}
		if f.numDead != nil {
			out.AndNot(f.numDead)
		}
	}
	if lo, hi := bounds(f.numAdd); hi > lo {
		out.Or(docs(f.numAdd[lo:hi]))
	}
	return out
}

// numPatch drops the entries of the documents in gone and adds the sorted entries add,
// merging the column back into one when the overlay has outgrown numDeltaMax. nf shares
// f's column on entry and owns its own on return.
func (nf *field) numPatch(f *field, gone *roaring.Bitmap, add []numEntry) {
	if !gone.IsEmpty() && f.numeric.Intersects(gone) {
		nf.numAdd = slices.DeleteFunc(slices.Clone(f.numAdd), func(e numEntry) bool { return gone.Contains(e.doc) })
		if f.numBase != nil && f.numBase.Intersects(gone) {
			if f.numDead == nil {
				nf.numDead = roaring.And(f.numBase, gone)
			} else {
				nf.numDead = roaring.Or(f.numDead, roaring.And(f.numBase, gone))
			}
		}
	}
	if len(add) > 0 {
		nf.numAdd = mergeNums(nf.numAdd, add)
	}
	dead := 0
	if nf.numDead != nil {
		dead = int(nf.numDead.GetCardinality())
	}
	if len(nf.numAdd)+dead > nf.numDeltaMax() {
		nf.setNumBase(nf.numAll())
	}
}

// --- forward overlay ---

// fwdValues is the value ids of document d per the forward array and its overlay; off must
// be set. A document past the array that is not dirty has no value. fwd16 values are decoded
// into *scratch; the result may alias the array or the overlay, so it is read-only.
func (f *field) fwdValues(d uint32, scratch *[]uint32) []uint32 {
	if f.fdirty != nil && f.fdirty.Contains(d) {
		return f.fresh[d]
	}
	if int(d)+1 >= len(f.off) {
		return nil
	}
	a, b := f.off[d], f.off[d+1]
	if f.fwd32 != nil {
		return f.fwd32[a:b]
	}
	buf := (*scratch)[:0]
	for _, v := range f.fwd16[a:b] {
		buf = append(buf, uint32(v))
	}
	*scratch = buf
	return buf
}

// track records, on a patched field, that the documents in touched now hold the value ids in
// vals (absent: none). Fields that do not want a forward array keep no overlay and no array.
func (nf *field) track(f *field, touched *roaring.Bitmap, vals map[uint32][]uint32) {
	if !nf.wantsForward() {
		nf.off, nf.fwd16, nf.fwd32 = nil, nil, nil
		nf.tracking, nf.fdirty, nf.fresh, nf.dirtyLog = false, nil, nil, nil
		return
	}
	if !f.tracking { // a new lineage: no array yet, the refresh builds one
		nf.off, nf.fwd16, nf.fwd32 = nil, nil, nil
		nf.epoch, nf.dirtyLog, nf.fdirty, nf.fresh = nextEpoch(), nil, nil, nil
	}
	nf.tracking = true
	if touched.IsEmpty() {
		return
	}
	if nf.fdirty == nil {
		nf.fdirty, nf.fresh = roaring.New(), map[uint32][]uint32{}
	} else {
		nf.fdirty, nf.fresh = nf.fdirty.Clone(), maps.Clone(nf.fresh) // value slices are never mutated
	}
	it := touched.Iterator()
	for it.HasNext() {
		d := it.Next()
		nf.fdirty.Add(d)
		nf.dirtyLog = append(nf.dirtyLog, d) // appends past every older generation's length
		if v := vals[d]; len(v) > 0 {
			nf.fresh[d] = v
		} else {
			delete(nf.fresh, d)
		}
	}
}

// needsRefresh reports whether the background refresh has work on this field.
func (f *field) needsRefresh() bool {
	return f.tracking && (f.off == nil || len(f.dirtyLog) > dirtyMax)
}

// withForward returns cur (the field as it is now) with the forward array built from g (the
// same field at an older generation) and the overlay cut down to the documents dirtied since
// g. ok is false when cur does not descend from g's tracking lineage.
func withForward(cur, g, built *field) (*field, bool) {
	tail := []uint32(nil)
	if cur != g {
		if !g.tracking || !cur.tracking || cur.epoch != g.epoch || len(cur.dirtyLog) < len(g.dirtyLog) {
			return nil, false
		}
		tail = cur.dirtyLog[len(g.dirtyLog):]
	}
	nf := *cur
	nf.off, nf.fwd16, nf.fwd32 = built.off, built.fwd16, built.fwd32
	nf.epoch, nf.dirtyLog, nf.fdirty, nf.fresh = nextEpoch(), slices.Clone(tail), nil, nil
	if len(tail) > 0 {
		nf.fdirty, nf.fresh = roaring.BitmapOf(tail...), map[uint32][]uint32{}
		for _, d := range tail {
			if v, ok := cur.fresh[d]; ok {
				nf.fresh[d] = v
			}
		}
	}
	return &nf, true
}
