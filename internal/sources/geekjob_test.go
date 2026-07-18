package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// geekjobDetailHTML is a geekjob.ru vacancy detail page: a server-rendered page whose payload we
// read is the schema.org JobPosting ld+json, plus the "jobinfo" chip block that carries the work
// arrangement (geekjob states it there, never in the ld+json). datePosted is a bare date. The
// description embeds a <script> that sanitizeHTML must strip. arrangement is the raw jobinfo text.
func geekjobDetailHTML(title, company, arrangement string) string {
	return `<html><head></head><body>
<div class="location">Бишкек, Кыргызстан</div>
<div class="jobinfo">` + arrangement + ` Опыт работы более 5 лет</div>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
"identifier":{"@type":"PropertyValue","name":"Geekjob","value":"171366"},
"datePosted":"2026-07-16",
"employmentType":"FULL_TIME",
"hiringOrganization":{"@type":"Organization","name":"` + company + `"},
"title":"` + title + `",
"description":"<p>Build things.</p><script>alert(1)<\/script>",
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"Бишкек","addressCountry":"Кыргызстан"}}}
</script>
</body></html>`
}

// geekjobListingHTML is a geekjob.ru listing page carrying the given vacancy ids. Each card, like
// the real markup, renders SEVERAL anchors to the same /vacancy/<id> (title, company, date, logo),
// exercising the job-link dedup; a non-vacancy nav anchor must be ignored by the id predicate.
func geekjobListingHTML(ids ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul class="collection">`)
	for _, id := range ids {
		b.WriteString(`<li class="collection-item">`)
		b.WriteString(`<a href="/vacancy/` + id + `" class="title">A Role</a>`)
		b.WriteString(`<a href="/vacancy/` + id + `">A Company</a>`)
		b.WriteString(`<a href="/vacancy/` + id + `">16 июля</a>`)
		b.WriteString(`</li>`)
	}
	b.WriteString(`<a href="/vacancies">All vacancies</a>`)
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// geekjobEmptyListingHTML is a listing page past the last real page: no vacancy cards, so the
// pagination loop stops when it yields no new job links.
const geekjobEmptyListingHTML = `<html><body><ul class="collection"></ul></body></html>`

func TestGeekjobProvider(t *testing.T) {
	if got := NewGeekjob(nil).Provider(); got != "geekjob" {
		t.Errorf("Provider() = %q, want %q", got, "geekjob")
	}
}

func TestGeekjobJobID(t *testing.T) {
	cases := map[string]string{
		"https://geekjob.ru/vacancy/6a5880f9186d0bddab0010e9":       "6a5880f9186d0bddab0010e9",
		"/vacancy/6a589c8bda741b31f007fe90":                         "6a589c8bda741b31f007fe90",
		"https://geekjob.ru/vacancy/6a5880f9186d0bddab0010e9?utm=1": "6a5880f9186d0bddab0010e9",
		"https://geekjob.ru/vacancies/2":                            "", // listing, not a vacancy
		"https://geekjob.ru/vacancy/short":                          "", // not a 24-hex id
		"https://geekjob.ru/":                                       "",
	}
	for loc, want := range cases {
		if got := geekjobJobID(loc); got != want {
			t.Errorf("geekjobJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestGeekjobFetchListingThenDetailAndMaps(t *testing.T) {
	id := "6a5880f9186d0bddab0010e9"
	fake := (&routedHTTP{}).
		route("/vacancies/1", geekjobListingHTML(id)).
		route("/vacancies/2", geekjobEmptyListingHTML).
		route("/vacancy/"+id, geekjobDetailHTML("Backend Engineer", "Mad Devs", "Удаленная работа"))

	jobs, err := NewGeekjob(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Geekjob", Provider: "geekjob",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != id {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, id)
	}
	if j.URL != "https://geekjob.ru/vacancy/"+id {
		t.Errorf("URL = %q, want canonical detail URL", j.URL)
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Mad Devs" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Бишкек, Кыргызстан" {
		t.Errorf("Location = %q, want %q", j.Location, "Бишкек, Кыргызстан")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build things") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want remote/true", j.WorkMode, j.Remote)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-16", j.PostedAt)
	}
}

// TestGeekjobArrangementMapping covers the work-mode mapping decision: any posting offering
// remote is remote (even when an office chip is co-listed); office-only is onsite; neither leaves
// the mode unset with the remote flag falling back to the location heuristic.
func TestGeekjobArrangementMapping(t *testing.T) {
	cases := []struct {
		arrangement string
		wantMode    string
		wantRemote  bool
	}{
		{"Удаленная работа", "remote", true},
		{"Удаленная работа • Работа в офисе", "remote", true}, // remote wins over co-listed office
		{"Релокация • Удаленная работа", "remote", true},
		{"Работа в офисе", "onsite", false},
		{"Частичная занятость", "", false}, // no arrangement chip → unset, location decides Remote
	}
	for _, tc := range cases {
		id := "6a5880f9186d0bddab0010e9"
		fake := (&routedHTTP{}).
			route("/vacancies/1", geekjobListingHTML(id)).
			route("/vacancies/2", geekjobEmptyListingHTML).
			route("/vacancy/"+id, geekjobDetailHTML("Role", "Acme", tc.arrangement))

		jobs, err := NewGeekjob(fake).Fetch(context.Background(), CompanyEntry{})
		if err != nil {
			t.Fatalf("Fetch(%q): %v", tc.arrangement, err)
		}
		if len(jobs) != 1 {
			t.Fatalf("Fetch(%q): got %d jobs, want 1", tc.arrangement, len(jobs))
		}
		if jobs[0].WorkMode != tc.wantMode || jobs[0].Remote != tc.wantRemote {
			t.Errorf("arrangement %q: WorkMode/Remote = %q/%v, want %q/%v",
				tc.arrangement, jobs[0].WorkMode, jobs[0].Remote, tc.wantMode, tc.wantRemote)
		}
	}
}

func TestGeekjobPaginatesAndDedupsAnchors(t *testing.T) {
	id1 := "6a5880f9186d0bddab0010e9"
	id2 := "6a589c8bda741b31f007fe90"
	id3 := "6a589d7ef00925a5be04b9ae"
	fake := (&routedHTTP{}).
		route("/vacancies/1", geekjobListingHTML(id1, id2)).
		route("/vacancies/2", geekjobListingHTML(id3)).
		route("/vacancies/3", geekjobEmptyListingHTML).
		route("/vacancy/"+id1, geekjobDetailHTML("Role 1", "Acme", "Удаленная работа")).
		route("/vacancy/"+id2, geekjobDetailHTML("Role 2", "Acme", "Работа в офисе")).
		route("/vacancy/"+id3, geekjobDetailHTML("Role 3", "Acme", "Удаленная работа"))

	jobs, err := NewGeekjob(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (across two listing pages, anchors deduped)", len(jobs))
	}
	ids := []string{jobs[0].ExternalID, jobs[1].ExternalID, jobs[2].ExternalID}
	for _, want := range []string{id1, id2, id3} {
		if !slices.Contains(ids, want) {
			t.Errorf("missing job %q in %v", want, ids)
		}
	}
}

func TestGeekjobDropsDetailWithoutJobPosting(t *testing.T) {
	id1 := "6a5880f9186d0bddab0010e9"
	id2 := "6a589c8bda741b31f007fe90"
	fake := (&routedHTTP{}).
		route("/vacancies/1", geekjobListingHTML(id1, id2)).
		route("/vacancies/2", geekjobEmptyListingHTML).
		route("/vacancy/"+id1, geekjobDetailHTML("Good Role", "Acme", "Удаленная работа")).
		route("/vacancy/"+id2, `<html><body>no ld+json here</body></html>`)

	jobs, err := NewGeekjob(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != id1 {
		t.Fatalf("got %v, want only the posting with a JobPosting block", jobs)
	}
}

func TestGeekjobRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["geekjob"]
	if !ok {
		t.Fatal("All() missing provider geekjob")
	}
	if s.Provider() != "geekjob" {
		t.Errorf("All()[geekjob].Provider() = %q", s.Provider())
	}
	// Aggregator (boardless but many companies) — it stays in the source facet.
	if !slices.Contains(FilterableProviders(), "geekjob") {
		t.Error("FilterableProviders() should include geekjob (aggregator)")
	}
}
