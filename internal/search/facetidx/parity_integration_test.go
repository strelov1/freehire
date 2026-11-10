//go:build integration

package facetidx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"math/rand"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/facetidx"
	"github.com/strelov1/freehire/internal/search/search"
)

func startMeili(t *testing.T) *search.Client {
	t.Helper()
	ctx := context.Background()
	const key = "test-master-key"
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "getmeili/meilisearch:v1.49.0", // the version prod runs
			ExposedPorts: []string{"7700/tcp"},
			Env:          map[string]string{"MEILI_MASTER_KEY": key, "MEILI_ENV": "development"},
			WaitingFor:   wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start meilisearch: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := container.MappedPort(ctx, "7700")
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	return search.NewClient("http://"+host+":"+port.Port(), key)
}

// pools are the values the generator draws from, per query param; they double as the
// values the queries ask for.
var pools = map[string][]string{
	"source":          {"greenhouse", "lever", "ashby", "workable", "whatjobs", "whatjobs_us", "hn", "telegram", "linkedin"},
	"source_type":     {"ats", "company", "aggregator", "social"},
	"regions":         {"eu", "uk", "us", "global", "latam", "apac"},
	"countries":       {"us", "de", "gb", "br", "pl", "in", "fr", "ca"},
	"cities":          {"Berlin", "London", "Munich", "MUNICH", "munich", "Sao Paulo", "Warsaw", "Paris", "Toronto"},
	"metros":          {"Berlin (DE)", "London (GB)", "Warsaw (PL)"},
	"work_mode":       vocab.WorkModeValues,
	"employment_type": vocab.EmploymentTypeValues,
	"education_level": vocab.EducationLevelValues,
	"seniority":       vocab.SeniorityValues,
	"category":        {"backend", "frontend", "data", "devops", "design", "sales", "support", "qa"},
	"domains":         {"fintech", "health", "ecommerce", "ai", "gaming", "edtech"},
	"company_type":    vocab.CompanyTypeValues,
	"company_size":    vocab.CompanySizeValues,
	"salary_currency": {"USD", "EUR", "GBP", "BRL"},
	"salary_period":   vocab.SalaryPeriodValues,
	"skills":          {"go", "python", "java", "rust", "react", "sql", "docker", "kubernetes", "aws", "typescript"},
	"collections":     {"yc", "unicorn", "faang", "remote-first"},
	"perks":           {"gym", "equity", "insurance", "books", "pto"},
	"engagement":      {"ir35", "w2", "b2b", "clt"},
	"is_tech":         {"tech", "non_tech"},
	"equity":          vocab.EquityValues,
	"clearance_levels": {
		"secret", "top_secret", "public_trust",
	},
	"ai_archetype":     {"agent_builder", "llm_app", "ml_engineer"},
	"role_type":        vocab.RoleTypeValues,
	"reality":          {"fresh", "stale", "likely_evergreen"},
	"relocation":       vocab.RelocationValues,
	"english_level":    vocab.EnglishLevelValues,
	"posting_language": {"en", "de", "pt", "pl", "uk"},
}

// extraValues are asked for on every param: sentinels, true/false forms, and a value no
// document carries.
var extraValues = []string{"none", "unspecified", "not_stated", "true", "false", "zzz-absent"}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func some(r *rand.Rand, xs []string, maxN int) []string {
	n := r.Intn(maxN + 1)
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, pick(r, xs))
	}
	return out
}

func maybe(r *rand.Rand, p float64) bool { return r.Float64() < p }

func i4(v int) pgtype.Int4 { return pgtype.Int4{Int32: int32(v), Valid: true} }

func genDocs(t *testing.T, r *rand.Rand, n int, withEquity bool) []search.JobDocument {
	t.Helper()
	now := time.Now()
	docs := make([]search.JobDocument, 0, n)
	for i := 1; i <= n; i++ {
		j := db.Job{ID: int64(i), Title: fmt.Sprintf("Job %d", i), Description: "A role.", PublicSlug: fmt.Sprintf("job-%d", i)}
		j.Source = pick(r, pools["source"])
		// ~2600 distinct slugs with a heavy head, so company_slug passes the 1200 cap and has ties.
		j.CompanySlug = fmt.Sprintf("co%04d", int(float64(2600)*r.Float64()*r.Float64()))
		j.Company = j.CompanySlug
		if maybe(r, 0.05) {
			j.CompanySlug = ""
		}
		j.Countries = some(r, pools["countries"], 3)
		j.Regions = some(r, pools["regions"], 2)
		j.WorkMode = pick(r, append([]string{""}, pools["work_mode"]...))
		j.Skills = some(r, pools["skills"], 5)
		if maybe(r, 0.5) {
			j.Seniority = pick(r, pools["seniority"])
		}
		j.Category = pick(r, pools["category"])
		j.PostingLanguage = pick(r, append([]string{""}, pools["posting_language"]...))
		j.EmploymentType = pick(r, append([]string{""}, pools["employment_type"]...))
		j.EducationLevel = pick(r, append([]string{""}, pools["education_level"]...))
		if maybe(r, 0.4) {
			j.ExperienceYearsMin = i4(r.Intn(12))
		}
		j.Collections = some(r, pools["collections"], 2)
		j.EnglishLevel = pick(r, append([]string{""}, pools["english_level"]...))
		// A long tail of cities beyond the cap, plus case variants of one name.
		if maybe(r, 0.7) {
			j.Cities = append(some(r, pools["cities"], 2), fmt.Sprintf("Town%04d", r.Intn(1500)))
		}
		j.Metros = some(r, pools["metros"], 2)
		if maybe(r, 0.6) {
			j.IsTech = pgtype.Bool{Bool: maybe(r, 0.7), Valid: true}
		}
		if maybe(r, 0.3) {
			j.SalaryMinManual = i4(1000 * (1 + r.Intn(15)))
			j.SalaryMaxManual = i4(int(j.SalaryMinManual.Int32) + 1000*r.Intn(8))
			j.SalaryCurrencyManual = pick(r, pools["salary_currency"])
			j.SalaryPeriodManual = pick(r, pools["salary_period"])
		}
		if maybe(r, 0.1) {
			j.RequiresClearance = pgtype.Bool{Bool: true, Valid: true}
			j.ClearanceLevels = some(r, pools["clearance_levels"], 2)
		}
		if maybe(r, 0.15) {
			j.ViaRecruiter = pgtype.Bool{Bool: true, Valid: true}
		}
		if withEquity && maybe(r, 0.1) {
			j.Equity = pgtype.Text{String: pick(r, pools["equity"]), Valid: true}
		}
		j.Perks = some(r, pools["perks"], 3)
		j.Engagement = some(r, pools["engagement"], 2)
		j.CreatedAt = pgtype.Timestamptz{Time: now.Add(-time.Duration(r.Intn(300*24)) * time.Hour), Valid: true}
		if maybe(r, 0.8) {
			j.PostedAt = pgtype.Timestamptz{Time: now.Add(-time.Duration(r.Intn(200*24)) * time.Hour), Valid: true}
		}
		if maybe(r, 0.05) {
			j.ClosedAt = pgtype.Timestamptz{Time: now, Valid: true}
		}
		if maybe(r, 0.05) {
			j.DuplicateOf = pgtype.Int8{Int64: 1, Valid: true}
		}
		e := enrich.Enrichment{}
		if maybe(r, 0.6) {
			e.CompanyType = pick(r, pools["company_type"])
			e.CompanySize = pick(r, pools["company_size"])
			e.Domains = some(r, pools["domains"], 3)
			e.Relocation = pick(r, pools["relocation"])
		}
		if maybe(r, 0.3) {
			v := maybe(r, 0.5)
			e.VisaSponsorship = &v
		}
		raw, err := jsonRaw(e)
		if err != nil {
			t.Fatal(err)
		}
		j.Enrichment = raw
		doc, err := search.FromJob(j, pick(r, append([]string{""}, pools["source_type"]...)))
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		if maybe(r, 0.2) {
			doc.AIInterview = true
		}
		if maybe(r, 0.3) {
			doc.AIArchetype = pick(r, pools["ai_archetype"])
		} else {
			doc.AIArchetype = ""
		}
		if maybe(r, 0.1) {
			doc.RoleType = pools["role_type"][0]
		} else {
			doc.RoleType = ""
		}
		if maybe(r, 0.7) {
			doc.Reality = &jobview.Reality{Class: pick(r, pools["reality"])}
		}
		if maybe(r, 0.3) {
			doc.AttachApplyQuestions(map[int64]int{doc.ID: r.Intn(9)})
		}
		docs = append(docs, doc)
	}
	return docs
}

// scalarSets are the non-facet params and modifiers a query may carry.
func scalarParams(r *rand.Rand, v url.Values) {
	opts := []func(){
		func() { v.Set("salary_min", fmt.Sprint(1000*(1+r.Intn(15)))) },
		func() { v.Set("salary_max", fmt.Sprint(1000*(1+r.Intn(20)))) },
		func() { v.Set("experience_years_min", fmt.Sprint(r.Intn(10))) },
		func() { v.Set("experience_years_max", fmt.Sprint(r.Intn(10))) },
		func() { v.Set("visa_sponsorship", pick(r, []string{"true", "false"})) },
		func() { v.Set("posted_within_days", fmt.Sprint(1+r.Intn(250))) },
		func() { v.Set("open_within_days", fmt.Sprint(1+r.Intn(300))) },
		func() { v.Set("apply_questions_max", fmt.Sprint(r.Intn(9))) },
		func() { v.Set("experience_include_unspecified", "true") },
		func() { v.Set("seniority_include_unspecified", "true") },
		func() { v.Set("experience_only_seniority_unspecified", "true") },
		func() {
			v.Set("work_mode_remote_anywhere", "true")
			v["work_mode"] = []string{"remote", pick(r, []string{"hybrid", "onsite"})}
			v["countries"] = some(r, pools["countries"], 2)
			if len(v["countries"]) == 0 {
				v.Set("countries", "de")
			}
			v["work_mode_remote_regions"] = some(r, pools["regions"], 2)
			v["work_mode_remote_countries"] = some(r, pools["countries"], 2)
		},
	}
	opts[r.Intn(len(opts))]()
}

func facetParams(r *rand.Rand, v url.Values, params []string) {
	p := pick(r, params)
	vals := append(slices.Clone(pools[p]), extraValues...)
	switch r.Intn(6) {
	case 0:
		v.Set(p, pick(r, vals))
	case 1:
		v[p] = []string{pick(r, vals), pick(r, vals)}
	case 2:
		v.Set(p+"_exclude", pick(r, vals))
	case 3:
		v[p] = []string{pick(r, vals), pick(r, vals)}
		v.Set(p+"_mode", "and")
	case 4:
		v.Set(p, pick(r, vals))
		v.Set(p+"_exclude", pick(r, vals))
	default:
		v.Set(p, strings.Join([]string{pick(r, vals), pick(r, vals)}, ","))
	}
}

func queries(r *rand.Rand, params []string) []url.Values {
	var qs []url.Values
	qs = append(qs, url.Values{})
	for _, p := range params {
		for _, v := range append(slices.Clone(pools[p]), extraValues...) {
			qs = append(qs, url.Values{p: {v}}, url.Values{p + "_exclude": {v}})
		}
	}
	for i := 0; i < 400; i++ {
		v := url.Values{}
		for k := 1 + r.Intn(4); k > 0; k-- {
			if maybe(r, 0.65) {
				facetParams(r, v, params)
			} else {
				scalarParams(r, v)
			}
		}
		qs = append(qs, v)
	}
	return qs
}

// comparedFacets: every facet attribute the filter vocabulary reaches, plus numeric ones.
// `source` is folded the way search.FacetCounts folds it; `regions` ignores the "eu"
// bucket, which FacetCounts re-counts through widenImpliedRegionCounts.
func comparedFacets() []string {
	set := map[string]bool{
		"enrichment.salary_min": true, "enrichment.salary_max": true, "enrichment.experience_years_min": true,
		"apply_questions": true, "enrichment.visa_sponsorship": true,
	}
	for _, attr := range search.StringFacets {
		set[attr] = true
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func foldSource(dist map[string]int64) {
	for val, n := range dist {
		if family := vocab.SourceFamilyOf(val); family != val {
			dist[family] += n
			delete(dist, val)
		}
	}
}

// diff compares one query's answers and returns human-readable differences.
func diff(want search.FacetResult, got facetidx.Answer, facets []string) []string {
	var out []string
	if want.Total != got.Total {
		out = append(out, fmt.Sprintf("total: meili %d facetidx %d", want.Total, got.Total))
	}
	gotDist := map[string]map[string]int64{}
	for a, d := range got.Distribution {
		gotDist[a] = maps(d)
	}
	foldSource(gotDist["source"])
	for _, a := range facets {
		w, g := lowerKeys(want.Facets[a]), lowerKeys(gotDist[a])
		if a == "regions" {
			w, g = without(w, "eu"), without(g, "eu")
		}
		// A distribution cut at the cap keeps every value above its last count, but WHICH of
		// the values on that last count survive is Meilisearch's level-tree traversal order,
		// not a rule on the value. Those are compared by number, not by name.
		cut := int64(-1)
		if len(w) >= facetidx.MaxValuesPerFacet {
			cut = slices.Min(slices.Collect(mapValues(w)))
			if len(g) != len(w) {
				out = append(out, fmt.Sprintf("%s: meili kept %d values facetidx %d", a, len(w), len(g)))
			}
		}
		for v, n := range w {
			if n != cut && g[v] != n {
				out = append(out, fmt.Sprintf("%s[%q]: meili %d facetidx %d", a, v, n, g[v]))
			}
		}
		for v, n := range g {
			if wn, ok := w[v]; (!ok && n != cut) || (ok && wn != n) {
				out = append(out, fmt.Sprintf("%s[%q]: meili %d facetidx %d", a, v, wn, n))
			}
		}
	}
	for _, a := range facets {
		w, wok := want.Stats[a]
		g, gok := got.Stats[a]
		if wok != gok || (wok && (w.Min != g.Min || w.Max != g.Max)) {
			out = append(out, fmt.Sprintf("stats %s: meili %v/%v facetidx %v/%v", a, w, wok, g, gok))
		}
	}
	return out
}

// lowerKeys folds the spelling of case variants: which one Meilisearch reports depends on the
// order of the indexing batches, and the counts are what must agree.
func lowerKeys(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] += v
	}
	return out
}

func mapValues(m map[string]int64) iter.Seq[int64] {
	return func(yield func(int64) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

func maps(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func without(m map[string]int64, k string) map[string]int64 {
	out := maps(m)
	delete(out, k)
	return out
}

func load(t *testing.T, docs []search.JobDocument) (*search.Client, *facetidx.Index) {
	t.Helper()
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	for i := 0; i < len(docs); i += 1000 {
		if err := c.IndexJobs(ctx, docs[i:min(i+1000, len(docs))]); err != nil {
			t.Fatalf("IndexJobs: %v", err)
		}
	}
	b := facetidx.NewBuilder()
	for _, d := range docs {
		if err := b.Add(d); err != nil {
			t.Fatalf("facetidx add: %v", err)
		}
	}
	return c, b.Build()

}

func TestParityWithMeilisearch(t *testing.T) {
	ctx := context.Background()
	r := rand.New(rand.NewSource(20261108))
	docs := genDocs(t, r, 4000, true)

	c, ix := load(t, docs)

	params := make([]string, 0, len(pools))
	for p := range pools {
		params = append(params, p)
	}
	sort.Strings(params)
	// Params whose pools live in StringFacets but are not in `pools` (company_slug) are asked
	// for through the dedicated block below.
	qs := queries(r, params)
	for i := 0; i < 60; i++ {
		qs = append(qs, url.Values{"company_slug": some(r, []string{fmt.Sprintf("co%04d", r.Intn(300)), fmt.Sprintf("co%04d", r.Intn(2600))}, 2)})
	}
	facets := comparedFacets()

	var failed, unsupported, nonEmpty int
	var shown int
	for qi, q := range qs {
		filter := search.FilterFromValues(q)
		want, err := c.FacetCounts(ctx, search.FacetParams{Filter: filter, Facets: facets})
		if err != nil {
			t.Fatalf("meili %v: %v", q, err)
		}
		got, err := ix.Facets(filter, facets)
		if err != nil {
			unsupported++
			t.Errorf("facetidx %v: %v\nfilter: %v", q, err, filter)
			continue
		}
		if want.Total > 0 {
			nonEmpty++
		}
		if d := diff(want, got, facets); len(d) > 0 {
			failed++
			if shown < 15 {
				shown++
				t.Errorf("query %v\nfilter %v\n  %s", q, filter, strings.Join(d[:min(len(d), 6)], "\n  "))
			}
		}
		// Distinct company count, on a sample of the queries.
		if qi%3 == 0 {
			wd, werr := c.DistinctCount(ctx, "", filter, "company_slug")
			gd, gerr := ix.Distinct(filter, "company_slug")
			if werr != nil || gerr != nil || wd != gd {
				failed++
				t.Errorf("distinct %v: meili %d (%v) facetidx %d (%v)", q, wd, werr, gd, gerr)
			}
		}
	}
	t.Logf("parity: %d queries (%d with matches), %d identical, %d different, %d unsupported",
		len(qs), nonEmpty, len(qs)-failed-unsupported, failed, unsupported)
	if failed+unsupported > 0 {
		t.Fail()
	}
}

func jsonRaw(e enrich.Enrichment) ([]byte, error) { return json.Marshal(e) }

// TestParityAbsentAttribute pins the `!=` rule for an attribute no document carries at all:
// there `!=` matches nothing, while NOT IN and NOT EXISTS match everything.
func TestParityAbsentAttribute(t *testing.T) {
	ctx := context.Background()
	docs := genDocs(t, rand.New(rand.NewSource(7)), 300, false)
	c, ix := load(t, docs)
	for _, q := range []url.Values{
		{"equity_exclude": {"salary"}}, {"equity": {"not_stated"}}, {"equity_exclude": {"not_stated"}},
		{"equity": {"salary"}}, {"equity_exclude": {"salary"}, "work_mode": {"remote"}},
	} {
		filter := search.FilterFromValues(q)
		want, err := c.FacetCounts(ctx, search.FacetParams{Filter: filter, Facets: []string{"work_mode"}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := ix.Facets(filter, []string{"work_mode"})
		if err != nil {
			t.Fatal(err)
		}
		if d := diff(want, got, []string{"work_mode"}); len(d) > 0 {
			t.Errorf("%v (filter %v, meili total %d): %v", q, filter, want.Total, d)
		}
	}
}
