//go:build integration

// Integration test for GET /api/v1/companies/:slug/hiring, the company page's Hiring tab:
// the dense activity series, the open/growth scalar (zeros when the company has no row),
// the sample-gated posting lifetime, and the same 404 / retired-slug 301 as the company
// read. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

type hiringBody struct {
	Data struct {
		Open     int32           `json:"open"`
		OpenPrev int32           `json:"open_prev"`
		Growth   int32           `json:"growth"`
		Series   []activityPoint `json:"series"`
		Lifetime *struct {
			Closed     int32   `json:"closed"`
			MedianDays float32 `json:"median_days"`
		} `json:"lifetime"`
	} `json:"data"`
}

func TestCompanyHiring(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	today := time.Now().UTC().Format(dateLayout)
	if _, err := pool.Exec(ctx, `
		INSERT INTO companies (slug, name, job_count) VALUES ('acme', 'Acme', 42), ('tiny', 'Tiny', 1);
		INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		     VALUES ('acme-inc', 'acme', 'acme', 'spelling');
		INSERT INTO insights_company_growth (company_slug, open_count, open_count_prev) VALUES ('acme', 42, 30);
		INSERT INTO insights_company_lifetime (company_slug, closed, median_days) VALUES ('acme', 5, 21.5), ('tiny', 4, 3);
		INSERT INTO insights_company_stats (company_slug, day, added, removed, open) VALUES ('acme', (now() AT TIME ZONE 'UTC')::date, 3, 1, 42);`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies/:slug/hiring", h.CompanyHiring)

	get := func(t *testing.T, url string) (int, string, hiringBody) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var body hiringBody
		if resp.StatusCode == fiber.StatusOK {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode %s: %v", raw, err)
			}
		}
		return resp.StatusCode, resp.Header.Get("Location"), body
	}

	t.Run("serves growth, a dense series and the lifetime", func(t *testing.T) {
		status, _, body := get(t, "/api/v1/companies/acme/hiring")
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		d := body.Data
		if d.Open != 42 || d.OpenPrev != 30 || d.Growth != 12 {
			t.Errorf("growth = %d/%d/%d, want 42/30/12", d.Open, d.OpenPrev, d.Growth)
		}
		// The default daily window is 90 days back from today, inclusive.
		if len(d.Series) != 91 {
			t.Errorf("series has %d days, want 91", len(d.Series))
		}
		if last := d.Series[len(d.Series)-1]; last.Period != today || last.Added != 3 || last.Removed != 1 {
			t.Errorf("today = %+v, want {%s 3 1}", last, today)
		}
		if d.Lifetime == nil || d.Lifetime.Closed != 5 || d.Lifetime.MedianDays != 21.5 {
			t.Errorf("lifetime = %+v, want closed 5, median 21.5", d.Lifetime)
		}
	})

	t.Run("below the gate the lifetime is absent and missing growth reads zero", func(t *testing.T) {
		status, _, body := get(t, "/api/v1/companies/tiny/hiring")
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if body.Data.Lifetime != nil {
			t.Errorf("lifetime = %+v, want absent below the gate", body.Data.Lifetime)
		}
		if body.Data.Open != 0 || body.Data.Growth != 0 {
			t.Errorf("growth = %d/%d, want zeros", body.Data.Open, body.Data.Growth)
		}
	})

	t.Run("an invalid granularity is a 400", func(t *testing.T) {
		if status, _, _ := get(t, "/api/v1/companies/acme/hiring?granularity=year"); status != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", status)
		}
	})

	t.Run("an unknown slug is a 404", func(t *testing.T) {
		if status, _, _ := get(t, "/api/v1/companies/nobody/hiring"); status != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", status)
		}
	})

	t.Run("a retired slug redirects to the canonical hiring endpoint", func(t *testing.T) {
		status, loc, _ := get(t, "/api/v1/companies/acme-inc/hiring?granularity=week")
		if status != fiber.StatusMovedPermanently {
			t.Fatalf("status = %d, want 301", status)
		}
		if loc != "/api/v1/companies/acme/hiring?granularity=week" {
			t.Errorf("Location = %q, want the hiring path with its query", loc)
		}
	})
}
