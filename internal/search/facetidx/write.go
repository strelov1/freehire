package facetidx

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/RoaringBitmap/roaring/v2"
)

// overlayMax is how many job ids the per-generation overlay may hold before it is merged into
// the shared id map (a full copy of that map, once per overlayMax new jobs).
const overlayMax = 1 << 16

// Batch is one unit of writes, applied in this order: upserts, closes, duplicate marks,
// deletes. Applying a batch twice gives the same index.
type Batch struct {
	// Upserts are full documents exactly as the search drain sends them to Meilisearch; each
	// replaces everything the index knew about that job, the closed and duplicate marks too.
	Upserts [][]byte
	// Close and Duplicate are job ids whose is_closed / is_duplicate becomes true.
	Close, Duplicate []int64
	// Delete are job ids removed from the index. Unknown ids are ignored everywhere.
	Delete []int64
}

func (b Batch) empty() bool {
	return len(b.Upserts)+len(b.Close)+len(b.Duplicate)+len(b.Delete) == 0
}

type fieldAdd struct {
	doc uint32
	p   *posting
}

// apply builds the next generation. Nothing the receiver reaches is modified: a bitmap, slice
// or map the batch touches is cloned first, everything else is shared. A batch with an
// undecodable document fails whole, before anything is built.
func (ix *Index) apply(b Batch) (*Index, error) {
	fls := make([]flattened, 0, len(b.Upserts))
	last := make(map[int64]int, len(b.Upserts))
	for _, raw := range b.Upserts {
		fl, err := flatten(raw)
		if err != nil {
			return nil, err
		}
		last[fl.id] = len(fls) // the last upsert of a job in the batch wins
		fls = append(fls, fl)
	}

	n := ix.n
	added := map[int64]uint32{}
	var newIDs []int64
	resolve := func(id int64) (uint32, bool) {
		if d, ok := added[id]; ok {
			return d, true
		}
		return ix.lookup(id)
	}
	type upsert struct {
		d  uint32
		fl *flattened
	}
	var ups []upsert
	gone := roaring.New() // existing documents whose old postings must go
	for i := range fls {
		fl := &fls[i]
		if last[fl.id] != i {
			continue
		}
		d, ok := resolve(fl.id)
		if !ok {
			d = n
			n++
			added[fl.id] = d
			newIDs = append(newIDs, fl.id)
		} else if d < ix.n {
			gone.Add(d)
		}
		ups = append(ups, upsert{d, fl})
	}
	deleting := roaring.New()
	for _, id := range b.Delete {
		if d, ok := resolve(id); ok {
			deleting.Add(d)
			if d < ix.n {
				gone.Add(d)
			}
		}
	}

	next := *ix
	next.gen = ix.gen + 1
	next.n = n
	next.closed, next.dup, next.deleted = ix.closed.Clone(), ix.dup.Clone(), ix.deleted.Clone()
	affected := roaring.New()
	perField := map[string][]fieldAdd{}
	for _, u := range ups {
		affected.Add(u.d)
		setBit(next.closed, u.d, u.fl.closed)
		setBit(next.dup, u.d, u.fl.duplicate)
		next.deleted.Remove(u.d) // an upsert of a deleted job revives its document
		if deleting.Contains(u.d) {
			continue // deleted later in this same batch: no postings
		}
		for name, p := range u.fl.attrs {
			perField[name] = append(perField[name], fieldAdd{u.d, p})
		}
	}
	mark := func(ids []int64, to *roaring.Bitmap) {
		for _, id := range ids {
			if d, ok := resolve(id); ok {
				to.Add(d)
				affected.Add(d)
			}
		}
	}
	mark(b.Close, next.closed)
	mark(b.Duplicate, next.dup)
	mark(b.Delete, next.deleted)

	if n > ix.n {
		next.all = ix.all.Clone()
		next.all.AddRange(uint64(ix.n), uint64(n))
		next.jobIDs = append(ix.jobIDs, newIDs...) // appends past every older generation's length
		next.overlay = maps.Clone(ix.overlay)
		if next.overlay == nil {
			next.overlay = map[int64]uint32{}
		}
		maps.Copy(next.overlay, added)
		if len(next.overlay) > overlayMax {
			merged := maps.Clone(ix.docOf)
			maps.Copy(merged, next.overlay)
			next.docOf, next.overlay = merged, nil
		}
	}
	next.live = ix.live.Clone()
	it := affected.Iterator()
	for it.HasNext() {
		d := it.Next()
		if next.closed.Contains(d) || next.dup.Contains(d) || next.deleted.Contains(d) {
			next.live.Remove(d)
		} else {
			next.live.Add(d)
		}
	}

	next.fields = maps.Clone(ix.fields)
	for name, f := range ix.fields {
		adds := perField[name]
		if len(adds) > 0 || (!gone.IsEmpty() && f.exists.Intersects(gone)) {
			next.fields[name] = f.patch(gone, adds)
		}
	}
	return &next, nil
}

func setBit(bm *roaring.Bitmap, d uint32, on bool) {
	if on {
		bm.Add(d)
	} else {
		bm.Remove(d)
	}
}

// patch returns a copy of the field without the documents in gone and with the adds. Only
// the value bitmaps that change are cloned; the number column and the forward array take the
// change as an overlay (see delta.go), so a batch costs what it touches, not the catalogue.
func (f *field) patch(gone *roaring.Bitmap, adds []fieldAdd) *field {
	nf := *f
	nf.exists, nf.empty, nf.numeric, nf.valued = f.exists.Clone(), f.empty.Clone(), f.numeric.Clone(), f.valued.Clone()
	nf.bms = slices.Clone(f.bms)
	owned := map[uint32]bool{}
	own := func(id uint32) {
		if !owned[id] {
			nf.bms[id] = nf.bms[id].Clone()
			owned[id] = true
		}
	}
	touched := roaring.New() // documents whose values for this field change
	if !gone.IsEmpty() && f.exists.Intersects(gone) {
		here := roaring.And(f.exists, gone)
		touched.Or(here)
		nf.exists.AndNot(gone)
		nf.empty.AndNot(gone)
		nf.numeric.AndNot(gone)
		nf.valued.AndNot(gone)
		if f.off != nil { // the old values of each document, straight from the forward array
			var scratch []uint32
			it := here.Iterator()
			for it.HasNext() {
				d := it.Next()
				for _, id := range f.fwdValues(d, &scratch) {
					own(id)
					nf.bms[id].Remove(d)
					nf.postings--
				}
			}
		} else {
			for i, bm := range f.bms {
				if bm.Intersects(here) {
					nf.postings -= bm.AndCardinality(here)
					nf.bms[i] = roaring.AndNot(bm, here)
					owned[uint32(i)] = true
				}
			}
		}
	}

	ownKeys := false
	cloneKeys := func() {
		if !ownKeys {
			nf.keys, nf.vals, ownKeys = maps.Clone(f.keys), slices.Clone(f.vals), true
		}
	}
	var newNums []numEntry
	vals := map[uint32][]uint32{} // the value ids each added document now holds
	for _, a := range adds {
		d := a.doc
		touched.Add(d)
		nf.exists.Add(d)
		if a.p.empty {
			nf.empty.Add(d)
		}
		for _, s := range a.p.strs {
			k := norm(s)
			id, ok := nf.keys[k]
			switch {
			case !ok:
				cloneKeys()
				id = uint32(len(nf.vals))
				nf.keys[k] = id
				nf.vals = append(nf.vals, s)
				nf.bms = append(nf.bms, roaring.New())
				owned[id] = true
			case s < nf.vals[id]:
				cloneKeys()
				nf.vals[id] = s
			}
			own(id)
			if nf.bms[id].CheckedAdd(d) {
				nf.postings++
				vals[d] = append(vals[d], id)
			}
			nf.valued.Add(d)
		}
		for _, v := range a.p.nums {
			newNums = append(newNums, numEntry{v, d})
			nf.numeric.Add(d)
		}
	}
	slices.SortFunc(newNums, cmpNum)
	nf.numPatch(f, gone, newNums)

	for id := range owned {
		nf.bms[id].RunOptimize()
	}
	nf.exists.RunOptimize()
	nf.empty.RunOptimize()
	nf.numeric.RunOptimize()
	nf.valued.RunOptimize()
	nf.withVals = nf.valued.GetCardinality()
	nf.cards = make([]uint32, len(nf.bms))
	for i, bm := range nf.bms {
		nf.cards[i] = uint32(bm.GetCardinality())
	}
	slices.Sort(nf.cards)
	nf.cardSum = make([]uint64, len(nf.cards)+1)
	for i, c := range nf.cards {
		nf.cardSum[i+1] = nf.cardSum[i] + uint64(c)
	}
	nf.track(f, touched, vals)
	return &nf
}

func cmpNum(a, b numEntry) int {
	switch {
	case a.v < b.v:
		return -1
	case a.v > b.v:
		return 1
	}
	return int(a.doc) - int(b.doc)
}

// mergeNums merges two sorted columns into a new one.
func mergeNums(a, b []numEntry) []numEntry {
	out := make([]numEntry, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if cmpNum(a[i], b[j]) <= 0 {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}
	out = append(out, a[i:]...)
	return append(out, b[j:]...)
}

// Store serves the current generation to any number of readers without locking and swaps in
// the next one when the single writer finishes a batch. A reader that took Current() keeps
// a consistent, unchanging view for as long as it holds it.
type Store struct {
	cur        atomic.Pointer[Index]
	mu         sync.Mutex // serialises Apply, Replace and the forward-array install
	refreshing bool       // a forward-array rebuild goroutine is running; guarded by mu
}

// NewStore starts a store at the given index (which it takes over; NewBuilder().Build() is
// the empty one).
func NewStore(ix *Index) *Store {
	s := &Store{}
	s.cur.Store(ix)
	return s
}

// Current is the latest generation. Lock-free.
func (s *Store) Current() *Index { return s.cur.Load() }

// Apply applies a batch and publishes the new generation, whose number it returns. An empty
// batch publishes nothing. On error nothing changed.
func (s *Store) Apply(b Batch) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.cur.Load()
	if b.empty() {
		return cur.gen, nil
	}
	next, err := cur.apply(b)
	if err != nil {
		return cur.gen, err
	}
	s.cur.Store(next)
	s.refreshForwardLocked()
	return next.gen, nil
}

// Replace swaps in a rebuilt index as the next generation and returns its number.
func (s *Store) Replace(ix *Index) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := *ix
	next.gen = s.cur.Load().gen + 1
	s.cur.Store(&next)
	return next.gen
}

// Generation is the number of the index: 0 for a built one, +1 per published batch or replace.
func (ix *Index) Generation() uint64 { return ix.gen }

// refreshForwardLocked starts, unless one runs, the goroutine that (re)builds the forward
// arrays: those a field does not have yet, and those whose overlay outgrew dirtyMax. Until
// then counting reads the overlay, or ANDs value bitmaps when there is no array at all;
// both are exact.
func (s *Store) refreshForwardLocked() {
	if s.refreshing || !s.cur.Load().needsRefresh() {
		return
	}
	s.refreshing = true
	go s.refreshForward()
}

func (ix *Index) needsRefresh() bool {
	for _, f := range ix.fields {
		if f.needsRefresh() {
			return true
		}
	}
	return false
}

// refreshForward builds the arrays from one generation and installs them on the current one,
// carrying the documents written in between as overlay (withForward); it repeats while any
// field still needs one.
func (s *Store) refreshForward() {
	for {
		ix := s.cur.Load()
		built := map[string]*field{}
		for name, f := range ix.fields {
			if f.needsRefresh() {
				nf := *f
				nf.buildForward(ix.n)
				built[name] = &nf
			}
		}
		s.mu.Lock()
		if len(built) == 0 {
			s.refreshing = false
			s.mu.Unlock()
			return
		}
		cur := s.cur.Load()
		next := *cur
		next.fields = maps.Clone(cur.fields)
		installed := false
		for name, b := range built {
			if nf, ok := withForward(cur.fields[name], ix.fields[name], b); ok {
				next.fields[name] = nf
				installed = true
			}
		}
		if installed {
			s.cur.Store(&next) // same generation number: the answers are identical
		}
		s.mu.Unlock()
	}
}
