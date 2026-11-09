package facetidx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

// jobState is the test's model of one job: what a fresh build would be given.
type jobState struct {
	doc              map[string]any
	closed, dup, del bool
}

var slugSpace = 120

func genDoc(r *rand.Rand, id int64) map[string]any {
	d := map[string]any{
		"id":           id,
		"source":       []string{"greenhouse", "lever", "ashby"}[r.Intn(3)],
		"company_slug": fmt.Sprintf("co%03d", r.Intn(slugSpace)),
		"work_mode":    []string{"remote", "onsite", "hybrid"}[r.Intn(3)],
		"is_tech":      r.Intn(2) == 0,
	}
	if r.Intn(4) > 0 {
		var sk []string
		for i := r.Intn(4); i > 0; i-- {
			sk = append(sk, fmt.Sprintf("s%d", r.Intn(25)))
		}
		d["skills"] = sk
	}
	if r.Intn(3) == 0 {
		d["cities"] = []string{fmt.Sprintf("city%d", r.Intn(30))}
	}
	if r.Intn(5) == 0 {
		d["perks"] = []string{}
	}
	if r.Intn(2) == 0 {
		d["enrichment"] = map[string]any{"salary_min": 20000 + r.Intn(10)*10000, "seniority": []string{"junior", "senior"}[r.Intn(2)]}
	}
	if r.Intn(6) == 0 {
		d["equity"] = r.Intn(2) == 0
	}
	return d
}

func (j jobState) raw(t testing.TB) []byte {
	d := map[string]any{}
	for k, v := range j.doc {
		d[k] = v
	}
	d["is_closed"], d["is_duplicate"] = j.closed, j.dup
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var testFilters = []any{
	nil,
	`work_mode = "remote"`,
	[][]string{{`skills = "s1"`, `skills = "s2"`}, {`work_mode != "onsite"`}},
	`enrichment.salary_min >= 50000`,
	`perks IS EMPTY`,
	`cities NOT EXISTS`,
	`company_slug != "co001"`,
	`id = 5`,
	`equity EXISTS`,
	`equity != "true"`,
	`NOT skills IN ["s3", "s4"]`,
}

// sameAnswers fails unless a and b answer every test filter alike.
func sameAnswers(t *testing.T, label string, a, b *Index) {
	t.Helper()
	for _, f := range testFilters {
		x, err := a.Facets(f, facetable())
		if err != nil {
			t.Fatalf("%s %v: %v", label, f, err)
		}
		y, err := b.Facets(f, facetable())
		if err != nil {
			t.Fatalf("%s %v: %v", label, f, err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Fatalf("%s: filter %v answers differ\n got %+v\nwant %+v", label, f, x, y)
		}
		for _, attr := range []string{"company_slug", "skills", "cities", "perks"} {
			dx, _ := a.Distinct(f, attr)
			dy, _ := b.Distinct(f, attr)
			if dx != dy {
				t.Fatalf("%s: distinct %s under %v: %d, want %d", label, attr, f, dx, dy)
			}
		}
	}
}

// waitForward blocks until the background forward-array rebuild has settled.
func waitForward(t *testing.T, s *Store) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		s.mu.Lock()
		busy := s.refreshing
		s.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("forward arrays never settled")
}

func freshBuild(t testing.TB, model map[int64]*jobState) *Index {
	b := NewBuilder()
	for id := int64(1); id < 100000; id++ {
		if j, ok := model[id]; ok && !j.del {
			if err := b.Add(j.raw(t)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return b.Build()
}

// TestBatchesEqualFreshBuild: after every random batch the store answers exactly as a
// Builder over the resulting documents, with and without the forward arrays, and a second
// application of the same batch changes nothing.
func TestBatchesEqualFreshBuild(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	model := map[int64]*jobState{}
	st := NewStore(NewBuilder().Build())
	const idSpace = 700
	for round := 0; round < 25; round++ {
		var b Batch
		for i := r.Intn(250); i > 0; i-- {
			id := int64(1 + r.Intn(idSpace))
			j := &jobState{doc: genDoc(r, id)}
			if r.Intn(8) == 0 {
				j.closed = true
			}
			b.Upserts = append(b.Upserts, j.raw(t))
			model[id] = j // a later upsert of the same job in the batch wins
		}
		pick := func() []int64 {
			var ids []int64
			for i := r.Intn(30); i > 0; i-- {
				ids = append(ids, int64(1+r.Intn(idSpace)))
			}
			return ids
		}
		b.Close, b.Duplicate, b.Delete = pick(), pick(), pick()
		for _, id := range b.Close {
			if j, ok := model[id]; ok {
				j.closed = true
			}
		}
		for _, id := range b.Duplicate {
			if j, ok := model[id]; ok {
				j.dup = true
			}
		}
		for _, id := range b.Delete {
			if j, ok := model[id]; ok {
				j.del = true
			}
		}
		if _, err := st.Apply(b); err != nil {
			t.Fatal(err)
		}
		want := freshBuild(t, model)
		sameAnswers(t, fmt.Sprintf("round %d (AND)", round), st.Current(), want)
		gen := st.Current().Generation()
		if _, err := st.Apply(b); err != nil { // idempotent, except for the reopen the first pass already did
			t.Fatal(err)
		}
		sameAnswers(t, fmt.Sprintf("round %d (replayed)", round), st.Current(), want)
		waitForward(t, st)
		sameAnswers(t, fmt.Sprintf("round %d (forward)", round), st.Current(), want)
		if st.Current().Generation() <= gen {
			t.Fatal("generation did not advance")
		}
	}
}

func TestReopenAndRevive(t *testing.T) {
	st := NewStore(build(t, `{"id":1,"work_mode":"remote","is_closed":true}`, `{"id":2,"work_mode":"remote"}`))
	count := func() int64 {
		a, _ := st.Current().Facets(nil, nil)
		return a.Total
	}
	if count() != 1 {
		t.Fatal("closed job counted")
	}
	mustApply(t, st, Batch{Upserts: [][]byte{[]byte(`{"id":1,"work_mode":"remote","is_closed":false}`)}})
	if count() != 2 {
		t.Fatal("reopen did not clear the closed mark")
	}
	mustApply(t, st, Batch{Delete: []int64{1, 99}})
	if count() != 1 {
		t.Fatal("delete")
	}
	mustApply(t, st, Batch{Upserts: [][]byte{[]byte(`{"id":1,"work_mode":"hybrid"}`)}})
	if count() != 2 {
		t.Fatal("re-upsert of a deleted job did not revive it")
	}
	if _, err := st.Apply(Batch{Upserts: [][]byte{[]byte(`{"id":3}`), []byte(`not json`)}}); err == nil {
		t.Fatal("bad document accepted")
	}
	if count() != 2 || st.Current().Generation() != 3 {
		t.Fatal("a failed batch changed the index")
	}
	if g, _ := st.Apply(Batch{}); g != 3 {
		t.Fatal("empty batch published a generation")
	}
	st.Replace(build(t, `{"id":9,"work_mode":"remote"}`))
	if count() != 1 || st.Current().Generation() != 4 {
		t.Fatal("replace")
	}
}

func TestOverlayMerges(t *testing.T) {
	st := NewStore(NewBuilder().Build())
	var b Batch
	for i := 1; i <= overlayMax+10; i++ {
		b.Upserts = append(b.Upserts, []byte(fmt.Sprintf(`{"id":%d,"work_mode":"remote"}`, i)))
	}
	mustApply(t, st, b)
	ix := st.Current()
	if ix.overlay != nil || len(ix.docOf) != overlayMax+10 {
		t.Fatalf("overlay not merged: %d/%d", len(ix.overlay), len(ix.docOf))
	}
	if got := ids(t, ix, `id = 65546`); len(got) != 1 {
		t.Fatalf("lookup after merge: %v", got)
	}
}

func syntheticBatch(r *rand.Rand, from, n int) Batch {
	var b Batch
	for i := 0; i < n; i++ {
		b.Upserts = append(b.Upserts, jobState{doc: genDoc(r, int64(from+i))}.raw(nil))
	}
	return b
}

// TestReaderKeepsItsGeneration: a reader that took Current() before a 15,000-document batch
// answers the same during and after the Apply, and concurrent readers always see a whole
// generation (single-valued work_mode counts add up to the total).
func TestReaderKeepsItsGeneration(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	st := NewStore(NewBuilder().Build())
	mustApply(t, st, syntheticBatch(r, 1, 20000))
	old := st.Current()
	before, _ := old.Facets(`work_mode = "remote"`, facetable())

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				a, err := st.Current().Facets(nil, []string{"work_mode"})
				if err != nil {
					t.Error(err)
					return
				}
				var sum int64
				for _, c := range a.Distribution["work_mode"] {
					sum += c
				}
				if sum != a.Total {
					t.Errorf("torn generation: %d vs %d", sum, a.Total)
					return
				}
				if mid, _ := old.Facets(`work_mode = "remote"`, facetable()); !reflect.DeepEqual(mid, before) {
					t.Error("old generation changed under a reader")
					return
				}
			}
		}()
	}
	// 15,000 documents: 10,000 replaced with new values, 5,000 new, plus closes
	b := syntheticBatch(r, 10001, 15000)
	b.Close = []int64{1, 2, 3}
	if _, err := st.Apply(b); err != nil {
		t.Fatal(err)
	}
	waitForward(t, st)
	for i := 0; i < 3; i++ {
		mustApply(t, st, syntheticBatch(r, 1+i*7000, 3000))
	}
	close(stop)
	wg.Wait()
	after, _ := old.Facets(`work_mode = "remote"`, facetable())
	if !reflect.DeepEqual(before, after) {
		t.Fatal("old generation changed after Apply")
	}
	if cur, _ := st.Current().Facets(`work_mode = "remote"`, facetable()); reflect.DeepEqual(cur, before) {
		t.Fatal("new generation did not change")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	st := NewStore(NewBuilder().Build())
	mustApply(t, st, syntheticBatch(r, 1, 3000))
	b := syntheticBatch(r, 2000, 1500)
	b.Close, b.Duplicate, b.Delete = []int64{5, 6}, []int64{7}, []int64{8, 9}
	mustApply(t, st, b)
	waitForward(t, st)
	ix := st.Current()
	var buf bytes.Buffer
	if err := ix.WriteSnapshot(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSnapshot(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	sameAnswers(t, "snapshot", got, ix)
	if len(got.docOf) != len(ix.jobIDs) {
		t.Fatal("id map not restored")
	}
	// the restored index takes writes like a built one
	s2 := NewStore(got)
	mustApply(t, s2, Batch{Close: []int64{10}})
	mustApply(t, s2, Batch{Upserts: [][]byte{[]byte(`{"id":8,"work_mode":"remote"}`)}})
	if n := ids(t, s2.Current(), `id = 8`); len(n) != 1 {
		t.Fatal("revive after restore")
	}

	whole := buf.Bytes()
	for _, cut := range []int{0, 5, 12, len(whole) / 3, len(whole) / 2, len(whole) - 5, len(whole) - 1} {
		if ix, err := ReadSnapshot(bytes.NewReader(whole[:cut])); err == nil || ix != nil {
			t.Errorf("truncated at %d of %d: no error", cut, len(whole))
		}
	}
	for _, at := range []int{3, 14, len(whole) / 2, len(whole) - 2} {
		bad := bytes.Clone(whole)
		bad[at] ^= 0x40
		if ix, err := ReadSnapshot(bytes.NewReader(bad)); err == nil || ix != nil {
			t.Errorf("flipped byte %d: no error", at)
		}
	}
}

// TestScale measures Apply and the snapshot on 200k documents; FACETD_SCALE=1 to run it.
func TestScale(t *testing.T) {
	if os.Getenv("FACETD_SCALE") == "" {
		t.Skip("set FACETD_SCALE=1")
	}
	slugSpace = 20000
	defer func() { slugSpace = 120 }()
	r := rand.New(rand.NewSource(5))
	t0 := time.Now()
	bld := NewBuilder()
	for _, raw := range syntheticBatch(r, 1, 200000).Upserts {
		if err := bld.Add(raw); err != nil {
			t.Fatal(err)
		}
	}
	ix := bld.Build()
	t.Logf("build 200k: %v", time.Since(t0))
	st := NewStore(ix)
	// 15k: 10k replaced, 5k new
	b := syntheticBatch(r, 190001, 15000)
	b.Close = []int64{1, 2, 3, 4, 5}
	t0 = time.Now()
	if _, err := st.Apply(b); err != nil {
		t.Fatal(err)
	}
	t.Logf("Apply 15k on 200k: %v", time.Since(t0))
	t0 = time.Now()
	waitForward(t, st)
	t.Logf("forward arrays back after: %v", time.Since(t0))
	t0 = time.Now()
	_, _ = st.Current().Facets(`work_mode = "remote"`, facetable())
	t.Logf("Facets all attrs, 1/3 filter: %v", time.Since(t0))

	var buf bytes.Buffer
	t0 = time.Now()
	if err := st.Current().WriteSnapshot(&buf); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot write: %v, %d bytes", time.Since(t0), buf.Len())
	t0 = time.Now()
	if _, err := ReadSnapshot(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot load: %v", time.Since(t0))
}

func mustApply(t *testing.T, s *Store, b Batch) {
	t.Helper()
	if _, err := s.Apply(b); err != nil {
		t.Fatal(err)
	}
}

// TestWalkSkipsDocumentsNewerThanTheForwardArray: a batch that adds documents without a field
// keeps that field's forward array, which is shorter than the new document ids; walking a
// filter that selects such a document must skip it, not index past the array.
func TestWalkSkipsDocumentsNewerThanTheForwardArray(t *testing.T) {
	docs := manyDocs(3000)
	ix := build(t, docs...)
	added := `{"id":5000,"work_mode":"remote"}`
	next, err := ix.apply(Batch{Upserts: [][]byte{[]byte(added)}})
	if err != nil {
		t.Fatal(err)
	}
	fld := next.fields["company_slug"]
	if fld.off == nil || len(fld.off) > int(next.n) {
		t.Fatalf("setup: want company_slug's forward array kept from before the batch")
	}
	f, err := next.match(`id IN [5000, 7]`)
	if err != nil {
		t.Fatal(err)
	}
	if got := toMap(fld, fld.walk(f)); !reflect.DeepEqual(got, map[string]int64{"co0007": 1}) {
		t.Fatalf("walk = %v", got)
	}
	sameAnswers(t, "after adding a document without company_slug", next, build(t, append(docs, added)...))
}

// facetable is every indexed attribute a facet request may name (range-only ones are not).
func facetable() []string {
	return slices.DeleteFunc(Attributes(), func(a string) bool { return rangeOnly[a] })
}
