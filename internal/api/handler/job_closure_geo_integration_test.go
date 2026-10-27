//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A posting that collapsed its reposts is found by every country they are open in, so its
// page must name the same countries. It named only its own row's: a recruiter posting one
// role in 130 capitals showed 129 flags in search and "Eswatini" on the page (#3646).
func TestGetJob_ShowsDuplicateClosureGeography(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	seed := func(extID, slug, country string) int64 {
		t.Helper()
		p := db.UpsertJobParams{
			Source: "manatal", ExternalID: extID, URL: "https://ex.test/" + extID,
			Title: "Dynamics 365 CE Analyst Programmer", Company: "Black Pen", CompanySlug: "black-pen",
			PublicSlug: slug, Location: "x", Description: "x",
		}
		p.RoleFingerprint = pgtype.Text{String: "fp", Valid: true}
		row, err := q.UpsertJob(ctx, p)
		if err != nil {
			t.Fatalf("seed %s: %v", extID, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET countries = $2 WHERE id = $1", row.Job.ID, []string{country}); err != nil {
			t.Fatalf("set countries %s: %v", extID, err)
		}
		return row.Job.ID
	}
	canon := seed("1", "d365-black-pen-aaaaaaaa", "sz")
	for i, cc := range []string{"eg", "il"} {
		id := seed(string(rune('2'+i)), "d365-black-pen-bbbbbbb"+string(rune('a'+i)), cc)
		if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $2 WHERE id = $1", id, canon); err != nil {
			t.Fatalf("mark duplicate: %v", err)
		}
	}

	h := &jobsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug", h.GetJob)
	resp, err := app.Test(httptest.NewRequestWithContext(ctx, "GET", "/api/v1/jobs/d365-black-pen-aaaaaaaa", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Countries []string `json:"countries"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if want := []string{"eg", "il", "sz"}; !reflect.DeepEqual(out.Data.Countries, want) {
		t.Errorf("countries = %v, want %v", out.Data.Countries, want)
	}
}
