//go:build integration

// Integration test for the company-list endpoint's name search: GET
// /api/v1/companies?q= must filter the returned companies and report the
// filtered count in meta.total (so search pagination is correct). The handler
// uses a concrete *db.Queries, so the wire contract can only be exercised
// against a real Postgres. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	// Apply every migration, in name order — the same way Postgres initdb runs the
	// mounted migrations/ dir — so a new migration is never silently missing from
	// the test schema (this helper previously hardcoded a list and drifted).
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestListCompaniesSearchEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	for _, c := range []struct{ slug, name string }{
		{"acme", "Acme Corp"}, {"acme-labs", "ACME Labs"}, {"globex", "Globex"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name) VALUES ($1, $2)`, c.slug, c.name); err != nil {
			t.Fatalf("seed %q: %v", c.slug, err)
		}
	}

	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies", h.ListCompanies)

	doList := func(t *testing.T, url string) (names []string, total float64) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []struct {
				Name string `json:"name"`
			} `json:"data"`
			Meta struct {
				Total float64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, c := range body.Data {
			names = append(names, c.Name)
		}
		return names, body.Meta.Total
	}

	t.Run("q filters companies and meta.total is the filtered count", func(t *testing.T) {
		names, total := doList(t, "/api/v1/companies?q=acme")
		if len(names) != 2 {
			t.Errorf("names = %v, want 2 ACME companies", names)
		}
		for _, n := range names {
			if !strings.Contains(strings.ToLower(n), "acme") {
				t.Errorf("returned non-matching company %q for q=acme", n)
			}
		}
		if total != 2 {
			t.Errorf("meta.total = %v, want 2 (filtered count)", total)
		}
	})

	t.Run("empty q returns the full list", func(t *testing.T) {
		names, total := doList(t, "/api/v1/companies")
		if len(names) != 3 || total != 3 {
			t.Errorf("full list: names=%v total=%v, want 3/3", names, total)
		}
	})
}

func TestListCompaniesFacetFilterEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	// Seed the denormalized facet arrays directly — this exercises the endpoint's
	// array-overlap filter independently of how the arrays get derived.
	seed := func(slug, name string, collections, regions, companyTypes []string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, collections, regions, company_types)
			 VALUES ($1, $2, $3, $4, $5)`,
			slug, name, collections, regions, companyTypes); err != nil {
			t.Fatalf("seed %q: %v", slug, err)
		}
	}
	seed("euro-lab", "Euro Lab", []string{"yc"}, []string{"europe"}, []string{"startup"})
	seed("asia-co", "Asia Co", []string{}, []string{"asia"}, []string{"product"})
	seed("euro-corp", "Euro Corp", []string{"bigtech"}, []string{"europe"}, []string{"enterprise"})
	seed("global-lab", "Global Lab", []string{"yc"}, []string{"north_america"}, []string{"enterprise"})

	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies", h.ListCompanies)

	doList := func(t *testing.T, url string) (slugs []string, total float64) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []struct {
				Slug string `json:"slug"`
			} `json:"data"`
			Meta struct {
				Total float64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, c := range body.Data {
			slugs = append(slugs, c.Slug)
		}
		sort.Strings(slugs)
		return slugs, body.Meta.Total
	}

	assertSlugs := func(t *testing.T, url string, want []string) {
		t.Helper()
		got, total := doList(t, url)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s → slugs %v, want %v", url, got, want)
		}
		if int(total) != len(want) {
			t.Errorf("%s → meta.total %v, want %d", url, total, len(want))
		}
	}

	t.Run("single facet filters by array membership", func(t *testing.T) {
		assertSlugs(t, "/api/v1/companies?regions=europe", []string{"euro-corp", "euro-lab"})
	})

	t.Run("multiple values within a facet are OR-ed", func(t *testing.T) {
		assertSlugs(t, "/api/v1/companies?regions=europe&regions=asia",
			[]string{"asia-co", "euro-corp", "euro-lab"})
	})

	t.Run("different facets are AND-ed", func(t *testing.T) {
		assertSlugs(t, "/api/v1/companies?collections=yc&company_type=startup",
			[]string{"euro-lab"})
	})

	t.Run("facets compose with the name search", func(t *testing.T) {
		assertSlugs(t, "/api/v1/companies?collections=yc&q=lab",
			[]string{"euro-lab", "global-lab"})
	})

	t.Run("no facet params returns the full list", func(t *testing.T) {
		assertSlugs(t, "/api/v1/companies",
			[]string{"asia-co", "euro-corp", "euro-lab", "global-lab"})
	})
}

func TestListCompaniesRemoteRegionsFacet(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// remote_regions (curated) and regions (job-derived) are deliberately crossed,
	// so a filter that hit the wrong column would return the wrong company.
	seed := func(slug string, remote, regions []string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, remote_regions, regions) VALUES ($1, $1, $2, $3)`,
			slug, remote, regions); err != nil {
			t.Fatalf("seed %q: %v", slug, err)
		}
	}
	seed("eu-remote", []string{"eu"}, []string{"north_america"})
	seed("na-remote", []string{"north_america"}, []string{"eu"})
	seed("global-remote", []string{"global"}, []string{})

	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies", h.ListCompanies)

	list := func(url string) (slugs []string, total float64) {
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []struct {
				Slug string `json:"slug"`
			} `json:"data"`
			Meta struct {
				Total float64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, c := range body.Data {
			slugs = append(slugs, c.Slug)
		}
		sort.Strings(slugs)
		return slugs, body.Meta.Total
	}

	t.Run("filters by the remote_regions column", func(t *testing.T) {
		got, total := list("/api/v1/companies?remote_regions=eu")
		if strings.Join(got, ",") != "eu-remote" || int(total) != 1 {
			t.Errorf("?remote_regions=eu → %v total=%v, want [eu-remote]/1", got, total)
		}
	})

	t.Run("is independent of the job-derived regions facet", func(t *testing.T) {
		got, _ := list("/api/v1/companies?regions=eu")
		if strings.Join(got, ",") != "na-remote" {
			t.Errorf("?regions=eu → %v, want [na-remote] (regions column, not remote_regions)", got)
		}
	})
}

func TestListCompaniesYCFacets(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	seed := func(slug string, batch, status []string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, yc_batch, yc_status) VALUES ($1, $1, $2, $3)`,
			slug, batch, status); err != nil {
			t.Fatalf("seed %q: %v", slug, err)
		}
	}
	seed("stripe", []string{"Summer 2009"}, []string{"Public"})
	seed("airbnb", []string{"Winter 2009"}, []string{"Public"})
	seed("newco", []string{"Winter 2024"}, []string{"Active"})
	seed("nonyc", []string{}, []string{}) // no YC facets

	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies", h.ListCompanies)

	list := func(url string) (slugs []string, total float64) {
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []struct {
				Slug string `json:"slug"`
			} `json:"data"`
			Meta struct {
				Total float64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, c := range body.Data {
			slugs = append(slugs, c.Slug)
		}
		sort.Strings(slugs)
		return slugs, body.Meta.Total
	}

	t.Run("status facet", func(t *testing.T) {
		got, total := list("/api/v1/companies?yc_status=Public")
		if strings.Join(got, ",") != "airbnb,stripe" || int(total) != 2 {
			t.Errorf("yc_status=Public → %v total=%v, want [airbnb stripe]/2", got, total)
		}
	})

	t.Run("batch AND status compose", func(t *testing.T) {
		got, _ := list("/api/v1/companies?yc_status=Public&yc_batch=Summer%202009")
		if strings.Join(got, ",") != "stripe" {
			t.Errorf("→ %v, want [stripe]", got)
		}
	})
}
