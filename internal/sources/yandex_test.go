package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// yandexListItem builds one list item fragment. cities and workModes are inline JSON
// fragments so a test can vary them; redirect/fastTrack are "null" unless overridden.
func yandexListItem(id int, slug, title, redirect, fastTrack, cities, workModes string) string {
	return `{"id":` + itoa(id) +
		`,"publication_slug_url":"` + slug + `"` +
		`,"title":"` + title + `"` +
		`,"redirect_url":` + redirect +
		`,"fast_track":` + fastTrack +
		`,"vacancy":{"cities":[` + cities + `],"work_modes":[` + workModes + `]}}`
}

func yandexListPage(next string, items ...string) string {
	n := "null"
	if next != "" {
		n = `"` + next + `"`
	}
	return `{"results":[` + strings.Join(items, ",") + `],"next":` + n + `,"count":1250}`
}

func yandexDetail(short, duties, keyQual, addReq, conditions, modified string) string {
	return `{"short_summary":"` + short + `","duties":"` + duties +
		`","key_qualifications":"` + keyQual + `","additional_requirements":"` + addReq +
		`","conditions":"` + conditions + `","modified":"` + modified + `"}`
}

func TestYandexProvider(t *testing.T) {
	if got := NewYandex(nil).Provider(); got != "yandex" {
		t.Errorf("Provider() = %q, want %q", got, "yandex")
	}
}

func TestYandexIsBoardless(t *testing.T) {
	if _, ok := NewYandex(nil).(boardless); !ok {
		t.Error("yandex must implement boardless: it is a single-company adapter; board selects host/language, not a different tenant")
	}
}

func TestYandexCursorPaginatesAndMapsDetail(t *testing.T) {
	// Page 1 (the bare list URL) returns a next that points at the INTERNAL host with a
	// ?cursor=ABC query. The adapter parses that cursor and re-issues against the PUBLIC
	// host; page 2's next is null, ending the loop. The redirect_url and fast_track items
	// on page 1 are skipped (links-out / hiring events, not vacancies).
	//
	// Route order matters: detail URLs (/publications/<id>) and the cursor page (cursor=ABC)
	// are matched before the bare first-page list URL, which is routed last by the shared
	// "/jobs/api/publications" substring so it only catches the no-query request.
	moscow := `{"name":"Москва"}`
	spb := `{"name":"Санкт-Петербург"}`
	office := `{"name":"Офис","slug":"office"}`
	remote := `{"name":"Удалённый","slug":"remote"}`

	fake := (&routedHTTP{}).
		route("/jobs/api/publications/111", yandexDetail(
			"summary one ", "<b>duties</b>", "* qual", "* req", "<p>conds</p>",
			"2026-06-11T12:29:16.977494Z")).
		route("/jobs/api/publications/222", yandexDetail(
			"summary two", "d2", "k2", "a2", "c2", "2026-04-01T09:00:00Z")).
		route("cursor=ABC", yandexListPage("",
			yandexListItem(222, "backend-222", "Backend", "null", "null", spb, remote),
		)).
		route("/jobs/api/publications", yandexListPage(
			"http://femida.yandex-team.ru/_api/jobs/publications/?cursor=ABC&page_size=20",
			yandexListItem(111, "data-111", "Data Engineer", "null", "null", moscow+","+spb, office),
			yandexListItem(900, "redir-900", "Redirected", `"https://elsewhere"`, "null", moscow, office),
			yandexListItem(901, "fast-901", "Hiring Event", "null", `{"id":1}`, moscow, office),
		))

	jobs, err := NewYandex(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex", Provider: "yandex", Board: "ru"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (redirect_url and fast_track items skipped, 2 pages)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["111"]
	if !ok {
		t.Fatal("publication 111 missing")
	}
	if j.Title != "Data Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Yandex" {
		t.Errorf("Company = %q, want Yandex", j.Company)
	}
	if want := "https://yandex.ru/jobs/vacancies/data-111"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Москва, Санкт-Петербург" {
		t.Errorf("Location = %q, want cities joined", j.Location)
	}
	if !strings.Contains(j.Description, "summary one") || !strings.Contains(j.Description, "duties") ||
		!strings.Contains(j.Description, "conds") {
		t.Errorf("Description not assembled: %q", j.Description)
	}
	if j.Remote {
		t.Error("111 Remote = true, want false (office only, Москва)")
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 6 {
		t.Errorf("PostedAt = %v, want parsed 2026-06 modified", j.PostedAt)
	}

	j2, ok := byID["222"]
	if !ok {
		t.Fatal("publication 222 missing")
	}
	if !j2.Remote {
		t.Error("222 Remote = false, want true (work_modes slug 'remote')")
	}
}

func TestYandexURLUsesComBoard(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/api/publications/5", yandexDetail("s", "d", "k", "a", "c", "2026-01-01T00:00:00Z")).
		route("/jobs/api/publications", yandexListPage("",
			yandexListItem(5, "slug-5", "Eng", "null", "null", `{"name":"Belgrade"}`, ""),
		))

	jobs, err := NewYandex(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex", Provider: "yandex", Board: "com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if want := "https://yandex.com/jobs/vacancies/slug-5"; jobs[0].URL != want {
		t.Errorf("URL = %q, want com host %q", jobs[0].URL, want)
	}
}

func TestYandexSkipsFailedDetail(t *testing.T) {
	// 222's detail body is a JSON array, which fails to decode into the detail struct, so
	// its detail fetch errors and the posting is skipped while 111 still comes through.
	// (Routing it explicitly also keeps it from falling through to the list route, which is
	// a substring of the detail URL.)
	fake := (&routedHTTP{}).
		route("/jobs/api/publications/111", yandexDetail("s", "d", "k", "a", "c", "2026-01-01T00:00:00Z")).
		route("/jobs/api/publications/222", `[]`).
		route("/jobs/api/publications", yandexListPage("",
			yandexListItem(111, "kept-111", "Kept", "null", "null", `{"name":"Москва"}`, ""),
			yandexListItem(222, "broken-222", "Broken", "null", "null", `{"name":"Москва"}`, ""),
		))

	jobs, err := NewYandex(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex", Provider: "yandex", Board: "ru"})
	if err != nil {
		t.Fatalf("Fetch should not abort on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("want only 111 to survive, got %d jobs", len(jobs))
	}
}

func TestYandexEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/jobs/api/publications", yandexListPage(""))

	jobs, err := NewYandex(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex", Provider: "yandex", Board: "ru"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestYandexListBoundsRunawayCursor(t *testing.T) {
	// Yandex's API tarpits datacenter IPs: under throttling it can return a `next`
	// cursor that never empties, so the cursor loop appends pages forever. In prod this
	// hung the whole custom.yml ingest for ~4h, holding its flock (one HTTP/2 keepalive
	// connection, frozen "16/17 boards crawled"). list() must bound the pagination
	// instead of trusting the upstream to terminate. The fake always replies with the
	// same non-empty next cursor, so unbounded code loops indefinitely.
	loop := yandexListPage("https://yandex.ru/jobs/api/publications?cursor=LOOP")
	fake := (&routedHTTP{}).route("/jobs/api/publications", loop)
	y := yandex{http: fake}

	done := make(chan struct{})
	go func() {
		_, _ = y.list(context.Background(), "ru")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("yandex.list did not terminate: runaway cursor pagination is unbounded")
	}
}

func TestYandexDefaultsBoardToRU(t *testing.T) {
	// Route is keyed on the exact "yandex.ru/jobs/api/publications" substring.
	// With an empty Board the adapter currently builds "yandex./jobs/..." which matches
	// no route, causing GetJSON to return an error and Fetch to fail. After the fix the
	// board defaults to "ru" so the URL is well-formed and the route matches.
	fake := (&routedHTTP{}).route("yandex.ru/jobs/api/publications", yandexListPage(""))

	jobs, err := NewYandex(fake).Fetch(context.Background(), CompanyEntry{Company: "Yandex", Provider: "yandex"})
	if err != nil {
		t.Fatalf("Fetch with empty Board: %v — want list URL to target yandex.ru/jobs/api/publications, not yandex./jobs", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
