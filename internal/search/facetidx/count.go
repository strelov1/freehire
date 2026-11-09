package facetidx

import (
	"container/heap"
	"fmt"

	"github.com/RoaringBitmap/roaring/v2"
)

// MaxValuesPerFacet is Meilisearch's faceting.maxValuesPerFacet for the jobs index
// (search.maxValuesPerFacet): a distribution keeps this many values, highest counts first.
const MaxValuesPerFacet = 1200

// walkCostFactor is the measured weight of one forward-array increment against one
// bitmap-AND step in the strategy estimate.
const walkCostFactor = 3

// Stat is the numeric range of a facet over the matched set (Meilisearch facetStats).
type Stat struct{ Min, Max float64 }

// Answer is the engine-neutral result of a facet request.
type Answer struct {
	Total        int64
	Distribution map[string]map[string]int64 // attribute -> value -> count
	Stats        map[string]Stat             // attribute -> range, numeric attributes only
}

// Facets counts the open documents matching filter (nil for all) and, for every requested
// attribute, the distribution of its values among them.
func (ix *Index) Facets(filter any, facets []string) (Answer, error) {
	f, err := ix.match(filter)
	if err != nil {
		return Answer{}, err
	}
	ans := Answer{
		Total:        int64(f.GetCardinality()),
		Distribution: make(map[string]map[string]int64, len(facets)),
		Stats:        map[string]Stat{},
	}
	for _, name := range facets {
		fld, ok := ix.fields[name]
		if !ok || rangeOnly[name] {
			return Answer{}, fmt.Errorf("%w: %q", ErrUnknownFacet, name)
		}
		top := fld.top(fld.counts(f), MaxValuesPerFacet)
		dist := make(map[string]int64, len(top))
		for _, vc := range top {
			dist[fld.vals[vc.id]] = int64(vc.n)
		}
		ans.Distribution[name] = dist
		if st, ok := fld.stat(f); ok {
			ans.Stats[name] = st
		}
	}
	return ans, nil
}

// Distinct is the number of distinct values of attr among the open documents matching filter.
// A matching document with no value counts as one of its own, as Meilisearch's `distinct` does.
func (ix *Index) Distinct(filter any, attr string) (int64, error) {
	f, err := ix.match(filter)
	if err != nil {
		return 0, err
	}
	fld, ok := ix.fields[attr]
	if !ok || rangeOnly[attr] {
		return 0, fmt.Errorf("%w: %q", ErrUnknownFacet, attr)
	}
	return int64(len(fld.counts(f))) + int64(roaring.AndNot(f, fld.valued).GetCardinality()), nil
}

type valueCount struct {
	id uint32
	n  int32
}

// counts returns the non-zero value counts among the documents of f, by the cheaper of two
// strategies: AND each value's bitmap with f, or walk f's documents through the forward array.
func (fld *field) counts(f *roaring.Bitmap) []valueCount {
	size := f.GetCardinality()
	if size == 0 || len(fld.vals) == 0 {
		return nil
	}
	if fld.off != nil && fld.walkCost(size) < fld.andCost(size) {
		return fld.walk(f)
	}
	out := make([]valueCount, 0, 64)
	for id, bm := range fld.bms {
		if n := bm.AndCardinality(f); n > 0 {
			out = append(out, valueCount{uint32(id), int32(n)})
		}
	}
	return out
}

// andCost is the estimated work of the AND strategy: sum over values of min(|value|, |f|).
func (fld *field) andCost(size uint64) uint64 {
	lo, hi := 0, len(fld.cards) // binary search: values with cardinality <= size
	for lo < hi {
		m := (lo + hi) / 2
		if uint64(fld.cards[m]) <= size {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return fld.cardSum[lo] + uint64(len(fld.cards)-lo)*size
}

// walkCost is the estimated work of the forward-array walk: |f| x values per document.
func (fld *field) walkCost(size uint64) uint64 {
	if fld.withVals == 0 {
		return 0
	}
	perDoc := float64(fld.postings) / float64(fld.withVals)
	return uint64(float64(size) * perDoc * walkCostFactor)
}

func (fld *field) walk(f *roaring.Bitmap) []valueCount {
	cnt := make([]int32, len(fld.vals))
	var touched []uint32
	buf := make([]uint32, 4096)
	it := f.ManyIterator()
	for {
		n := it.NextMany(buf)
		if n == 0 {
			break
		}
		for _, d := range buf[:n] {
			if int(d)+1 >= len(fld.off) {
				continue // added after this field's forward array was built, so it holds no value here
			}
			a, b := fld.off[d], fld.off[d+1]
			if fld.fwd16 != nil {
				for _, v := range fld.fwd16[a:b] {
					if cnt[v] == 0 {
						touched = append(touched, uint32(v))
					}
					cnt[v]++
				}
			} else {
				for _, v := range fld.fwd32[a:b] {
					if cnt[v] == 0 {
						touched = append(touched, v)
					}
					cnt[v]++
				}
			}
		}
	}
	out := make([]valueCount, len(touched))
	for i, v := range touched {
		out[i] = valueCount{v, cnt[v]}
	}
	return out
}

// top keeps the limit highest counts; equal counts are ordered by value, descending.
func (fld *field) top(all []valueCount, limit int) []valueCount {
	if len(all) <= limit {
		return all
	}
	h := &worstFirst{fld: fld, items: make([]valueCount, 0, limit)}
	for _, vc := range all {
		switch {
		case len(h.items) < limit:
			heap.Push(h, vc)
		case h.better(vc, h.items[0]):
			h.items[0] = vc
			heap.Fix(h, 0)
		}
	}
	return h.items
}

// worstFirst is a heap whose root is the worst of the kept values.
type worstFirst struct {
	fld   *field
	items []valueCount
}

func (h *worstFirst) better(a, b valueCount) bool {
	if a.n != b.n {
		return a.n > b.n
	}
	return h.fld.vals[a.id] > h.fld.vals[b.id]
}
func (h *worstFirst) Len() int           { return len(h.items) }
func (h *worstFirst) Less(i, j int) bool { return h.better(h.items[j], h.items[i]) }
func (h *worstFirst) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *worstFirst) Push(x any)         { h.items = append(h.items, x.(valueCount)) }
func (h *worstFirst) Pop() any {
	x := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	return x
}

// stat is the smallest and largest number the attribute holds among the documents of f.
func (fld *field) stat(f *roaring.Bitmap) (Stat, bool) {
	if !fld.numeric.Intersects(f) {
		return Stat{}, false // otherwise the scans below walk the whole column for nothing
	}
	var st Stat
	found := false
	for _, e := range fld.num {
		if f.Contains(e.doc) {
			st.Min, found = e.v, true
			break
		}
	}
	if !found {
		return Stat{}, false
	}
	for i := len(fld.num) - 1; i >= 0; i-- {
		if f.Contains(fld.num[i].doc) {
			st.Max = fld.num[i].v
			break
		}
	}
	return st, true
}
