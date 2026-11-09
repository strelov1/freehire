//go:build integration

package main

// End-to-end: Meilisearch v1.49 and facetd (in-process HTTP) hold the same generated
// documents; a search.Client routed to facetd must answer exactly like one that is
// Meilisearch-only. The generator and the query set are borrowed from
// internal/search/facetidx/parity_integration_test.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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
	"github.com/strelov1/freehire/internal/search/search"
)

func startMeili(t *testing.T) string {
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
	return "http://" + host + ":" + port.Port()
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
		doc, err := search.FromJob(j, nil, pick(r, append([]string{""}, pools["source_type"]...)))
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

func jsonRaw(e enrich.Enrichment) ([]byte, error) { return json.Marshal(e) }

func queryFilterWithout(q url.Values, p string) any {
	c := url.Values{}
	for k, v := range q {
		if k != p && k != p+"_exclude" && k != p+"_mode" {
			c[k] = v
		}
	}
	return search.FilterFromValues(c)
}

func post(t *testing.T, srv *httptest.Server, path, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("POST %s = %d", path, resp.StatusCode)
	}
}

func TestEndToEndFacetdMatchesMeilisearch(t *testing.T) {
	ctx := context.Background()
	r := rand.New(rand.NewSource(20261108))
	docs := genDocs(t, r, 4000, true)

	meiliURL := startMeili(t)
	meili := search.NewClient(meiliURL, "test-master-key")
	if err := meili.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(docs); i += 1000 {
		if err := meili.IndexJobs(ctx, docs[i:min(i+1000, len(docs))]); err != nil {
			t.Fatal(err)
		}
	}

	fd := NewServer("")
	srv := httptest.NewServer(fd.Handler())
	defer srv.Close()
	post(t, srv, "/rebuild/begin", "")
	for i := 0; i < len(docs); i += 1000 {
		b, _ := json.Marshal(map[string]any{"docs": docs[i:min(i+1000, len(docs))]})
		post(t, srv, "/rebuild/push", string(b))
	}
	post(t, srv, "/rebuild/commit", "")

	routed := search.NewClient(meiliURL, "test-master-key",
		search.WithFacetEngine(search.NewFacetdClient(srv.URL, nil), func() search.FacetMode { return search.FacetModeFacetd }),
		search.WithFacetEngineTimeout(10*time.Second))

	params := make([]string, 0, len(pools))
	for p := range pools {
		params = append(params, p)
	}
	sort.Strings(params)
	qs := queries(r, params)
	facets := []string{"skills", "source", "work_mode", "countries", "enrichment.category", "regions", "enrichment.seniority", "enrichment.company_type", "enrichment.salary_min"}

	var compared, identical int
	check := func(what string, q url.Values, want, got any) {
		compared++
		if reflect.DeepEqual(want, got) {
			identical++
			return
		}
		if compared-identical <= 10 {
			t.Errorf("%s differs for %v\n meili  %+v\n facetd %+v", what, q, want, got)
		}
	}
	for qi, q := range qs {
		filter := search.FilterFromValues(q)
		want, err := meili.FacetCounts(ctx, search.FacetParams{Filter: filter, Facets: facets})
		if err != nil {
			t.Fatal(err)
		}
		got, err := routed.FacetCounts(ctx, search.FacetParams{Filter: filter, Facets: facets})
		if err != nil {
			t.Fatal(err)
		}
		check("FacetCounts", q, want, got)

		if qi%4 == 0 {
			var reqs []search.FacetReq
			for p, attr := range map[string]string{"skills": "skills", "source": "source", "work_mode": "work_mode", "countries": "countries", "category": "enrichment.category"} {
				reqs = append(reqs, search.FacetReq{Attr: attr, Filter: queryFilterWithout(q, p)})
			}
			dw, err := meili.DisjunctiveFacetCounts(ctx, "", reqs, filter)
			if err != nil {
				t.Fatal(err)
			}
			dg, err := routed.DisjunctiveFacetCounts(ctx, "", reqs, filter)
			if err != nil {
				t.Fatal(err)
			}
			check("DisjunctiveFacetCounts", q, dw, dg)

			cw, err := meili.DistinctCount(ctx, "", filter, "company_slug")
			if err != nil {
				t.Fatal(err)
			}
			cg, err := routed.DistinctCount(ctx, "", filter, "company_slug")
			if err != nil {
				t.Fatal(err)
			}
			check("DistinctCount", q, cw, cg)
		}
	}
	t.Logf("e2e: %d queries, %d comparisons, %d identical", len(qs), compared, identical)
	if c := routed.RoutingCounters(); len(c.Fallbacks) > 0 {
		for reason, n := range c.Fallbacks {
			if n != 0 {
				t.Errorf("fallback %s = %d, want 0 (%+v)", reason, n, c)
			}
		}
	}

	// Writes through /write show up in the next facetd-mode answer.
	before, err := routed.FacetCounts(ctx, search.FacetParams{Filter: search.FilterFromValues(url.Values{"source": {"lever"}}), Facets: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	var victim, upserted search.JobDocument
	for _, d := range docs {
		if d.Source == "lever" && !d.IsClosed && !d.IsDuplicate {
			victim = d
			break
		}
	}
	for _, d := range docs {
		if d.Source == "ashby" && !d.IsClosed && !d.IsDuplicate {
			upserted = d
			break
		}
	}
	upserted.Source = "lever" // an existing open job changes a facet value
	ub, _ := json.Marshal(upserted)
	post(t, srv, "/write", fmt.Sprintf(`{"close":[%d],"upserts":[%s]}`, victim.ID, ub))
	after, err := routed.FacetCounts(ctx, search.FacetParams{Filter: search.FilterFromValues(url.Values{"source": {"lever"}}), Facets: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	if after.Total != before.Total { // one closed, one moved in
		t.Errorf("lever total %d -> %d, want unchanged (one closed, one moved in)", before.Total, after.Total)
	}
	one, err := routed.FacetCounts(ctx, search.FacetParams{Filter: search.FilterFromValues(url.Values{"source": {"ashby"}}), Facets: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	wantAshby, err := meili.FacetCounts(ctx, search.FacetParams{Filter: search.FilterFromValues(url.Values{"source": {"ashby"}}), Facets: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	if one.Total != wantAshby.Total-1 {
		t.Errorf("ashby total = %d, want %d (one job moved to lever)", one.Total, wantAshby.Total-1)
	}
	if c := routed.RoutingCounters(); c.Fallbacks["error"]+c.Fallbacks["unsupported"]+c.Fallbacks["not_ready"]+c.Fallbacks["timeout"] != 0 {
		t.Errorf("fallbacks after writes: %+v", c)
	}
}
