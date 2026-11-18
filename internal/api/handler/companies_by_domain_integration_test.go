//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestCompanySiteHostMatchesGoNormalizer(t *testing.T) {
	pool := startPostgres(t)
	for _, c := range siteHostCases {
		var got *string
		if err := pool.QueryRow(context.Background(),
			`SELECT company_site_host(jsonb_build_object('website', $1::text))`, c.in).Scan(&got); err != nil {
			t.Fatal(err)
		}
		g := ""
		if got != nil {
			g = *got
		}
		if g != c.want {
			t.Errorf("company_site_host(%q) = %q, want %q", c.in, g, c.want)
		}
	}
}

func TestCompaniesByDomain(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	for _, c := range []struct{ slug, info string }{
		{"stripe", `{"website":"https://www.stripe.com/"}`},
		{"acme", `{"website":"","homepage":"http://ACME.io/about"}`},
		{"acme-eu", `{"website":"acme.io:443"}`},
		{"tagged", `{}`},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, job_count, company_info) VALUES ($1, $1, 0, $2::jsonb)`, c.slug, c.info); err != nil {
			t.Fatalf("seed %q: %v", c.slug, err)
		}
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountCompaniesContract(app, &companiesHandlers{queries: db.New(pool)})

	get := func(q string) (int, string) {
		resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/companies/by-domain"+q, nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	status, body := get("?domain=Stripe.com&domain=https://acme.io/x,nope.example")
	if status != 200 {
		t.Fatalf("status %d: %s", status, body)
	}
	var out struct {
		Data []struct{ Domain, Slug, Website string } `json:"data"`
		Meta struct{ Unmatched []string }             `json:"meta"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, d := range out.Data {
		slugs = append(slugs, d.Domain+"="+d.Slug)
	}
	if want := []string{"acme.io=acme", "acme.io=acme-eu", "stripe.com=stripe"}; !reflect.DeepEqual(slugs, want) {
		t.Errorf("matches = %v, want %v", slugs, want)
	}
	if !reflect.DeepEqual(out.Meta.Unmatched, []string{"nope.example"}) {
		t.Errorf("unmatched = %v", out.Meta.Unmatched)
	}
	if out.Data[0].Website != "http://ACME.io/about" {
		t.Errorf("website = %q, want the raw homepage", out.Data[0].Website)
	}

	if s, _ := get(""); s != 400 {
		t.Errorf("empty: status %d, want 400", s)
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = "d" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".com"
	}
	if s, _ := get("?domain=" + strings.Join(many, ",")); s != 400 {
		t.Errorf("101 domains: status %d, want 400", s)
	}
}
