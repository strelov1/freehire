package facetidx

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func build(t *testing.T, docs ...string) *Index {
	t.Helper()
	b := NewBuilder()
	for _, d := range docs {
		if err := b.Add([]byte(d)); err != nil {
			t.Fatalf("add %s: %v", d, err)
		}
	}
	return b.Build()
}

// ids returns the job ids matching filter, ascending.
func ids(t *testing.T, ix *Index, filter any) []int64 {
	t.Helper()
	f, err := ix.match(filter)
	if err != nil {
		t.Fatalf("match %v: %v", filter, err)
	}
	var out []int64
	for _, d := range f.ToArray() {
		out = append(out, ix.jobIDs[d])
	}
	slices.Sort(out)
	return out
}

func TestFlatten(t *testing.T) {
	fl, err := flatten([]byte(`{"id":7,"title":"x","_vectors":{"skills":[1,2]},"skills":["go","go"],"perks":[],
		"work_mode":"","is_tech":null,"requires_clearance":true,"enrichment":{"seniority":"senior","salary_min":1500.5},
		"posted_ts":1700000000,"reality":{"class":"fresh"},"cities":[["a"],"b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if fl.id != 7 {
		t.Fatalf("id = %d", fl.id)
	}
	if _, ok := fl.attrs["title"]; ok {
		t.Error("free text must not be indexed")
	}
	cases := []struct {
		attr  string
		strs  []string
		nums  []float64
		empty bool
	}{
		{"skills", []string{"go", "go"}, nil, false},
		{"perks", nil, nil, true},
		{"work_mode", nil, nil, true},
		{"is_tech", nil, nil, false},
		{"requires_clearance", []string{"true"}, nil, false},
		{"enrichment.seniority", []string{"senior"}, nil, false},
		{"enrichment.salary_min", []string{"1500.5"}, []float64{1500.5}, false},
		{"posted_ts", nil, []float64{1.7e9}, false}, // range-only: no string postings
		{"reality.class", []string{"fresh"}, nil, false},
		{"cities", []string{"a", "b"}, nil, false},
	}
	for _, c := range cases {
		p, ok := fl.attrs[c.attr]
		if !ok {
			t.Errorf("%s: absent", c.attr)
			continue
		}
		if !reflect.DeepEqual(p.strs, c.strs) || !reflect.DeepEqual(p.nums, c.nums) || p.empty != c.empty {
			t.Errorf("%s = %+v, want strs=%v nums=%v empty=%v", c.attr, *p, c.strs, c.nums, c.empty)
		}
	}
	if _, ok := fl.attrs["equity"]; ok {
		t.Error("an absent key must stay absent")
	}
}

const fixture1 = `{"id":1,"skills":["go","sql"],"work_mode":"remote","enrichment":{"seniority":"senior","salary_min":100},"posted_ts":10,"is_tech":"true","perks":[]}`
const fixture2 = `{"id":2,"skills":["go"],"work_mode":"onsite","enrichment":{"salary_min":300},"posted_ts":20,"perks":["gym"]}`
const fixture3 = `{"id":3,"skills":[],"work_mode":"Remote","enrichment":{"seniority":"junior"},"posted_ts":30}`
const fixtureClosed = `{"id":4,"is_closed":true,"skills":["go"],"work_mode":"remote"}`
const fixtureDup = `{"id":5,"is_duplicate":true,"skills":["go"],"work_mode":"remote"}`

func TestFilterShapes(t *testing.T) {
	ix := build(t, fixture1, fixture2, fixture3, fixtureClosed, fixtureDup)
	ok := []struct {
		filter any
		want   []int64
	}{
		{nil, []int64{1, 2, 3}},
		{`skills = "go"`, []int64{1, 2}},
		{`work_mode = "remote"`, []int64{1, 3}}, // case-insensitive, closed rows never answer
		{`is_tech = true`, []int64{1}},
		{`is_tech = "true"`, []int64{1}},
		{`enrichment.salary_min = 100`, []int64{1}},
		{`enrichment.salary_min >= 300`, []int64{2}},
		{`enrichment.salary_min <= 100`, []int64{1}},
		{`enrichment.salary_min > 100`, []int64{2}},
		{`enrichment.salary_min < 300`, []int64{1}},
		{`posted_ts >= 15`, []int64{2, 3}},
		{`posted_ts <= 25`, []int64{1, 2}},
		{`skills IN ["sql", "nope"]`, []int64{1}},
		{`skills IN []`, nil},
		{`id IN [1, 3, 99]`, []int64{1, 3}},
		{`id NOT IN [1, 3]`, []int64{2}},
		{`skills EXISTS`, []int64{1, 2, 3}},
		{`perks EXISTS`, []int64{1, 2}},
		{`perks NOT EXISTS`, []int64{3}},
		{`perks IS EMPTY`, []int64{1}},
		{`skills IS EMPTY`, []int64{3}},
		{`perks IS NOT EMPTY`, []int64{2, 3}}, // NOT of IS EMPTY: a missing attribute passes
		{`NOT skills = "go"`, []int64{3}},
		{`NOT work_mode IN ["remote"]`, []int64{2}},
		{[]string{`skills = "go"`, `work_mode = "onsite"`}, []int64{2}},
		{[][]string{{`skills = "sql"`, `work_mode = "onsite"`}, {`posted_ts >= 15`}}, []int64{2}},
		{[][]string{{`skills = "go"`}, {}}, []int64{1, 2}},
		{`(work_mode = "remote" AND (skills = "sql" OR skills = "x")) OR id = 2`, []int64{1, 2}},
		{`enrichment.seniority = "se\"nior"`, nil},
	}
	for _, c := range ok {
		if got := ids(t, ix, c.filter); !slices.Equal(got, c.want) {
			t.Errorf("%v = %v, want %v", c.filter, got, c.want)
		}
	}
}

func TestNotEqualRules(t *testing.T) {
	ix := build(t, fixture1, fixture2, fixture3)
	// (a) `!=` matches documents without the attribute...
	if got := ids(t, ix, `enrichment.seniority != "senior"`); !slices.Equal(got, []int64{2, 3}) {
		t.Errorf("!= with absent attribute = %v", got)
	}
	// ...but when no document carries the attribute at all it matches nothing.
	if got := ids(t, ix, `equity != "salary"`); got != nil {
		t.Errorf("!= on an attribute nobody carries = %v, want none", got)
	}
	// (b) NOT IN matches documents without the attribute, even then.
	if got := ids(t, ix, `equity NOT IN ["salary"]`); !slices.Equal(got, []int64{1, 2, 3}) {
		t.Errorf("NOT IN on an attribute nobody carries = %v", got)
	}
	if got := ids(t, ix, `enrichment.seniority NOT IN ["senior", "x"]`); !slices.Equal(got, []int64{2, 3}) {
		t.Errorf("NOT IN = %v", got)
	}
}

func TestUnsupported(t *testing.T) {
	ix := build(t, fixture1)
	for _, f := range []any{
		"", `skills`, `skills = `, `skills ~ "go"`, `nope = "x"`, `skills IN "go"`, `skills = "go" skills = "x"`,
		`skills = "go`, `(skills = "go"`, `posted_ts >= abc`,
		`posted_ts = 10`, `posted_ts IN [10]`, `posted_ts != 10`, `created_ts = 10`, `skills IS NULL`, `skills = "go" XOR x`,
		42, map[string]string{}, []any{1},
	} {
		if _, err := ix.match(f); !errors.Is(err, ErrUnsupportedFilter) {
			t.Errorf("%#v: err = %v, want ErrUnsupportedFilter", f, err)
		}
	}
}

func TestRangeOnlyNotFaceted(t *testing.T) {
	ix := build(t, fixture1)
	if _, err := ix.Facets(nil, []string{"posted_ts"}); !errors.Is(err, ErrUnknownFacet) {
		t.Errorf("Facets(posted_ts) err = %v, want ErrUnknownFacet", err)
	}
	if _, err := ix.Distinct(nil, "created_ts"); !errors.Is(err, ErrUnknownFacet) {
		t.Errorf("Distinct(created_ts) err = %v, want ErrUnknownFacet", err)
	}
}

func TestFacetsAndStats(t *testing.T) {
	ix := build(t, fixture1, fixture2, fixture3, fixtureClosed)
	ans, err := ix.Facets(`work_mode != "onsite"`, []string{"skills", "enrichment.salary_min", "equity"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Total != 2 {
		t.Errorf("total = %d", ans.Total)
	}
	if want := map[string]int64{"go": 1, "sql": 1}; !reflect.DeepEqual(ans.Distribution["skills"], want) {
		t.Errorf("skills = %v", ans.Distribution["skills"])
	}
	if want := map[string]int64{"100": 1}; !reflect.DeepEqual(ans.Distribution["enrichment.salary_min"], want) {
		t.Errorf("salary = %v", ans.Distribution["enrichment.salary_min"])
	}
	if st := ans.Stats["enrichment.salary_min"]; st != (Stat{100, 100}) {
		t.Errorf("stat = %v", st)
	}
	if _, ok := ans.Stats["skills"]; ok {
		t.Error("a non-numeric facet has no stats")
	}
	if len(ans.Distribution["equity"]) != 0 {
		t.Error("equity should be empty")
	}
	if _, err := ix.Facets(nil, []string{"title"}); !errors.Is(err, ErrUnknownFacet) {
		t.Errorf("unknown facet err = %v", err)
	}
	if n, _ := ix.Distinct(nil, "skills"); n != 3 { // go, sql, and the document with no value
		t.Errorf("distinct skills = %d (2 values + 1 valueless document)", n)
	}
}

// manyDocs builds docs with a high-cardinality company_slug (so a forward array exists)
// and skills multi-valued over 20 values.
func manyDocs(n int) []string {
	var docs []string
	for i := 1; i <= n; i++ {
		docs = append(docs, fmt.Sprintf(
			`{"id":%d,"company_slug":"co%04d","skills":["s%d","s%d"],"work_mode":"%s"}`,
			i, i%700, i%20, (i*7)%20, []string{"remote", "onsite", "hybrid"}[i%3]))
	}
	return docs
}

func TestStrategiesAgree(t *testing.T) {
	ix := build(t, manyDocs(3000)...)
	for _, name := range []string{"company_slug", "skills"} {
		fld := ix.fields[name]
		if fld.off == nil {
			t.Fatalf("%s: expected a forward array", name)
		}
		for _, filter := range []any{nil, `work_mode = "remote"`, `company_slug = "co0007"`, `id IN [5, 6, 7]`} {
			f, err := ix.match(filter)
			if err != nil {
				t.Fatal(err)
			}
			viaWalk := toMap(fld, fld.walk(f))
			viaAnd := map[string]int64{}
			for id, bm := range fld.bms {
				if n := bm.AndCardinality(f); n > 0 {
					viaAnd[fld.vals[id]] = int64(n)
				}
			}
			if !reflect.DeepEqual(viaWalk, viaAnd) {
				t.Errorf("%s under %v: walk and AND disagree", name, filter)
			}
		}
	}
	if fld := ix.fields["work_mode"]; fld.off != nil {
		t.Error("a 3-value single-valued facet needs no forward array")
	}
}

func toMap(fld *field, vcs []valueCount) map[string]int64 {
	m := map[string]int64{}
	for _, vc := range vcs {
		m[fld.vals[vc.id]] = int64(vc.n)
	}
	return m
}

func TestDistributionCap(t *testing.T) {
	var docs []string
	// 1500 slugs: slug i appears (i%7)+1 times; ties are common.
	id := 0
	for i := 0; i < 1500; i++ {
		for k := 0; k <= i%7; k++ {
			id++
			docs = append(docs, fmt.Sprintf(`{"id":%d,"company_slug":"c%04d"}`, id, i))
		}
	}
	ix := build(t, docs...)
	ans, err := ix.Facets(nil, []string{"company_slug"})
	if err != nil {
		t.Fatal(err)
	}
	dist := ans.Distribution["company_slug"]
	if len(dist) != MaxValuesPerFacet {
		t.Fatalf("kept %d values", len(dist))
	}
	// Everything kept is at least as large as everything dropped.
	minKept := int64(1 << 30)
	for _, n := range dist {
		minKept = min(minKept, n)
	}
	full := toMap(ix.fields["company_slug"], ix.fields["company_slug"].counts(ix.live))
	for v, n := range full {
		if _, kept := dist[v]; !kept && n > minKept {
			t.Fatalf("dropped %s (%d) while keeping a count of %d", v, n, minKept)
		}
	}
	if n, _ := ix.Distinct(nil, "company_slug"); n != 1500 {
		t.Errorf("distinct = %d", n)
	}
}

func TestDuplicateID(t *testing.T) {
	b := NewBuilder()
	if err := b.Add([]byte(`{"id":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := b.Add([]byte(`{"id":1}`)); err == nil {
		t.Error("a repeated id must be refused")
	}
	if err := b.Add([]byte(`{"title":"no id"}`)); err == nil {
		t.Error("a document without an id must be refused")
	}
}
