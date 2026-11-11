//go:build integration

// Integration tests for GET /api/v1/jobs/find, the endpoint the browser extension asks
// "is the page I am on a posting you carry?". Both tiers need a real Postgres: the
// identity lookup by (source, external_id), and the fall-through that compares the page
// URL against jobs.url through normalize_job_url.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// stubPostingAPI serves one canned JSON body per URL, standing in for the ATS the
// posting-URL resolver asks. An unlisted URL is an error, so a test that expects no call
// fails loudly rather than silently resolving.
type stubPostingAPI map[string]string

func (s stubPostingAPI) GetJSON(_ context.Context, url string, v any) error {
	body, ok := s[url]
	if !ok {
		return fmt.Errorf("stubPostingAPI: unexpected GET %s", url)
	}
	return json.Unmarshal([]byte(body), v)
}

// findSlug calls the endpoint and returns the resolved slug, or "" for {"data": null}.
func findSlug(t *testing.T, app *fiber.App, url string) string {
	t.Helper()
	slug, _ := find(t, app, url)
	return slug
}

// find calls the endpoint and returns the resolved slug and, on a miss, the meta.ats_board
// verdict (nil when the response carries none).
func find(t *testing.T, app *fiber.App, url string) (string, *bool) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/find?url="+url, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET find?url=%s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET find?url=%s: status %d, want 200", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out struct {
		Data *struct {
			PublicSlug string `json:"public_slug"`
		} `json:"data"`
		Meta *struct {
			ATSBoard *bool `json:"ats_board"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	var atsBoard *bool
	if out.Meta != nil {
		atsBoard = out.Meta.ATSBoard
	}
	if out.Data == nil {
		return "", atsBoard
	}
	return out.Data.PublicSlug, atsBoard
}

func TestFindJobResolvesByIdentityAndByURL(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	// A Greenhouse posting, whose identity the URL parser can recover, and an aggregator
	// posting, whose identity it cannot — the latter is only reachable by its stored URL.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug) VALUES
		 ('greenhouse', 'vgw-eu:8617539002', 'https://job-boards.greenhouse.io/vgw-eu/jobs/8617539002', 'Engineer', 'engineer-vgw-eu'),
		 ('himalayas', ':https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer',
		  'https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer', 'Staff Java Backend Developer', 'staff-java-mindera'),
		 ('ashby', 'truelogic:c6d2719d-3935-4e59-8446-26135d01957a',
		  'https://jobs.ashbyhq.com/truelogic/c6d2719d-3935-4e59-8446-26135d01957a', 'Senior Go Engineer', 'senior-go-truelogic'),
		 ('smartrecruiters', 'blend360:744000143615340',
		  'https://jobs.smartrecruiters.com/Blend360/744000143615340-senior-ai-engineer', 'Senior AI Engineer', 'senior-ai-blend360')`); err != nil {
		t.Fatalf("seed jobs: %v", err)
	}

	h := &jobsHandlers{
		queries: db.New(pool),
		// SmartRecruiters answers which posting a publication uuid is; everything else in
		// this test is canonicalised offline and never reaches the fake.
		postings: sources.NewPostingURLResolver(stubPostingAPI{
			"https://api.smartrecruiters.com/v1/companies/Blend360/postings/59957d76-615a-4809-a282-bcee1120ca7d": `{"postingUrl":"https://jobs.smartrecruiters.com/Blend360/744000143615340-senior-ai-engineer"}`,
		}),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountJobsContract(app, h, middleware{})

	t.Run("greenhouse URL resolves by identity", func(t *testing.T) {
		if slug := findSlug(t, app, "https%3A%2F%2Fjob-boards.greenhouse.io%2Fvgw-eu%2Fjobs%2F8617539002"); slug != "engineer-vgw-eu" {
			t.Errorf("slug = %q, want engineer-vgw-eu", slug)
		}
	})

	t.Run("aggregator page resolves by its stored URL", func(t *testing.T) {
		// Carrying the tracking tag freehire stamps on its own outbound links.
		const page = "https%3A%2F%2Fhimalayas.app%2Fcompanies%2Fmindera%2Fjobs%2Fstaff-java-backend-developer%3Futm_source%3Dfreehire.me"
		if slug := findSlug(t, app, page); slug != "staff-java-mindera" {
			t.Errorf("slug = %q, want staff-java-mindera", slug)
		}
	})

	// The form is where a candidate stands when they ask about a vacancy, and it is a
	// different URL from the one the catalog stores. Without collapsing it the panel
	// tells them freehire does not have the posting it is showing on the page behind.
	t.Run("an apply form resolves to its posting", func(t *testing.T) {
		const form = "https%3A%2F%2Fjobs.ashbyhq.com%2Ftruelogic%2Fc6d2719d-3935-4e59-8446-26135d01957a%2Fapplication%3Futm_source%3Dfreehire.me"
		if slug := findSlug(t, app, form); slug != "senior-go-truelogic" {
			t.Errorf("slug = %q, want senior-go-truelogic", slug)
		}
	})

	// One in five open postings is a duplicate of another. The candidate is standing
	// on a page we know perfectly well; answering "freehire does not have this" because
	// the dedup passes preferred a twin is wrong from where they are sitting.
	t.Run("a duplicate page resolves to the posting it duplicates", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`WITH canonical AS (
			   INSERT INTO jobs (source, external_id, url, title, public_slug)
			   VALUES ('ashby', 'truelogic:ef27e902', 'https://jobs.ashbyhq.com/truelogic/ef27e902', 'Senior Full Stack', 'senior-full-stack-canonical')
			   RETURNING id
			 )
			 INSERT INTO jobs (source, external_id, url, title, public_slug, duplicate_of_role)
			 SELECT 'ashby', 'truelogic:c6d2719d', 'https://jobs.ashbyhq.com/truelogic/c6d2719d',
			        'Senior Full Stack', 'senior-full-stack-duplicate', id
			 FROM canonical`); err != nil {
			t.Fatalf("seed duplicate pair: %v", err)
		}

		const page = "https%3A%2F%2Fjobs.ashbyhq.com%2Ftruelogic%2Fc6d2719d"
		if slug := findSlug(t, app, page); slug != "senior-full-stack-canonical" {
			t.Errorf("slug = %q, want the canonical posting", slug)
		}
	})

	// SmartRecruiters' Apply button leaves the posting for a one-click form addressed by a
	// publication uuid the catalogue never stored — neither tier can match it on the URL
	// alone, so the resolver asks the platform for the posting's own URL first.
	t.Run("a smartrecruiters one-click form resolves to its posting", func(t *testing.T) {
		const form = "https%3A%2F%2Fjobs.smartrecruiters.com%2Foneclick-ui%2Fcompany%2FBlend360%2Fpublication%2F59957d76-615a-4809-a282-bcee1120ca7d%3Fdcr_ci%3DBlend360"
		if slug := findSlug(t, app, form); slug != "senior-ai-blend360" {
			t.Errorf("slug = %q, want senior-ai-blend360", slug)
		}
	})

	t.Run("unknown page is unresolved", func(t *testing.T) {
		if slug := findSlug(t, app, "https%3A%2F%2Fexample.test%2Fcareers%2Fsome-job"); slug != "" {
			t.Errorf("slug = %q, want no match", slug)
		}
	})

	t.Run("recognised identity with no posting falls through to the URL lookup", func(t *testing.T) {
		if slug := findSlug(t, app, "https%3A%2F%2Fjob-boards.greenhouse.io%2Fnobody%2Fjobs%2F1"); slug != "" {
			t.Errorf("slug = %q, want no match", slug)
		}
	})

	// The extension offers "add this job" on a miss only where the URL is a board we could
	// crawl (or the page marks itself up as a posting), so a miss has to say which it is.
	t.Run("a miss reports whether the url is a recognised ATS board", func(t *testing.T) {
		cases := []struct {
			name, url string
			want      bool
		}{
			{"ATS board we do not carry", "https%3A%2F%2Fjobs.lever.co%2Fnobody%2F1b2c3d", true},
			{"LinkedIn feed", "https%3A%2F%2Fwww.linkedin.com%2Ffeed%2F", false},
			{"chat app", "https%3A%2F%2Fchatgpt.com%2Fc%2Fabc", false},
			{"localhost", "http%3A%2F%2Flocalhost%3A5174%2Fjobs", false},
		}
		for _, tc := range cases {
			slug, atsBoard := find(t, app, tc.url)
			if slug != "" {
				t.Errorf("%s: slug = %q, want no match", tc.name, slug)
			}
			if atsBoard == nil || *atsBoard != tc.want {
				t.Errorf("%s: meta.ats_board = %v, want %v", tc.name, atsBoard, tc.want)
			}
		}
	})

	t.Run("a hit carries no meta", func(t *testing.T) {
		slug, atsBoard := find(t, app, "https%3A%2F%2Fjob-boards.greenhouse.io%2Fvgw-eu%2Fjobs%2F8617539002")
		if slug != "engineer-vgw-eu" || atsBoard != nil {
			t.Errorf("got (%q, %v), want (engineer-vgw-eu, nil)", slug, atsBoard)
		}
	})

	t.Run("an empty url matches nothing", func(t *testing.T) {
		// A posting can carry an empty url (nothing constrains the column), and an empty
		// query would otherwise normalize to the same "" and resolve to it.
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', 'blank-url', '', 'Nameless', 'nameless-job')`); err != nil {
			t.Fatalf("seed blank-url job: %v", err)
		}
		if slug := findSlug(t, app, ""); slug != "" {
			t.Errorf("slug = %q, want no match", slug)
		}
	})
}
