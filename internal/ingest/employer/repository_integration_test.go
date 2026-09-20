//go:build integration

// Integration tests for what a fake repository cannot model: the real unique-constraint
// conflicts InsertPending must map (company_accounts_pkey vs
// company_accounts_company_slug_key), SeedCompanyAccountWebsite's fill-only-if-blank guard
// and UpdateCompanyProfile's nil-means-unchanged COALESCE guard, both of which live in the
// SQL, not in Go.
// Run with: go test -tags=integration ./internal/ingest/employer/
package employer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func TestInsertPending_SecondClaimBySameUser_IsAlreadyHasAccount(t *testing.T) {
	pool := testdb.Pool(t)
	repo := employer.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()
	userID := seedUser(t, pool, "founder@acme.test")

	if _, err := repo.InsertPending(ctx, userID, "acme", "Acme", "founder@acme.test"); err != nil {
		t.Fatalf("first InsertPending: %v", err)
	}
	if _, err := repo.InsertPending(ctx, userID, "other-co", "Other Co", "founder@acme.test"); !errors.Is(err, employer.ErrAlreadyHasAccount) {
		t.Fatalf("err = %v, want ErrAlreadyHasAccount", err)
	}
}

func TestInsertPending_SecondClaimOnSameSlug_IsCompanyAlreadyClaimed(t *testing.T) {
	pool := testdb.Pool(t)
	repo := employer.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()
	userA := seedUser(t, pool, "founder@acme.test")
	userB := seedUser(t, pool, "other@acme.test")

	if _, err := repo.InsertPending(ctx, userA, "acme", "Acme", "founder@acme.test"); err != nil {
		t.Fatalf("first InsertPending: %v", err)
	}
	if _, err := repo.InsertPending(ctx, userB, "acme", "Acme", "other@acme.test"); !errors.Is(err, employer.ErrCompanyAlreadyClaimed) {
		t.Fatalf("err = %v, want ErrCompanyAlreadyClaimed", err)
	}
}

func TestSeedCompanyWebsite_FillsABlankWebsiteOnANewCompany(t *testing.T) {
	pool := testdb.Pool(t)
	repo := employer.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()

	if err := repo.SeedCompanyWebsite(ctx, "brand-new", "Brand New", "brandnew.test"); err != nil {
		t.Fatalf("SeedCompanyWebsite: %v", err)
	}
	_, website, found, err := repo.ExistingCompany(ctx, "brand-new")
	if err != nil {
		t.Fatalf("ExistingCompany: %v", err)
	}
	if !found || website != "brandnew.test" {
		t.Fatalf("website = %q (found=%v), want brandnew.test", website, found)
	}
}

func TestSeedCompanyWebsite_NeverOverwritesAnExistingWebsite(t *testing.T) {
	pool := testdb.Pool(t)
	repo := employer.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()

	if err := repo.SeedCompanyWebsite(ctx, "acme", "Acme", "acme.test"); err != nil {
		t.Fatalf("first SeedCompanyWebsite: %v", err)
	}
	if err := repo.SeedCompanyWebsite(ctx, "acme", "Acme", "impostor.test"); err != nil {
		t.Fatalf("second SeedCompanyWebsite: %v", err)
	}
	_, website, _, err := repo.ExistingCompany(ctx, "acme")
	if err != nil {
		t.Fatalf("ExistingCompany: %v", err)
	}
	if website != "acme.test" {
		t.Errorf("website = %q, must never move once set", website)
	}
}

func TestUpdateCompanyProfile_AppliesOnlyTheSuppliedFields(t *testing.T) {
	pool := testdb.Pool(t)
	repo := employer.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, hq_country, company_info)
		 VALUES ('acme', 'Acme', 'de', '{"funding":"Series B"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	tagline := "We build things"
	website := "acme.test"
	yearFounded := 2015
	if err := repo.UpdateCompanyProfile(ctx, "acme", employer.CompanyProfilePatch{
		Tagline:     &tagline,
		Website:     &website,
		YearFounded: &yearFounded,
		// HqCountry and Description deliberately omitted — must stay unchanged.
	}); err != nil {
		t.Fatalf("UpdateCompanyProfile: %v", err)
	}

	var storedTagline, hqCountry string
	var yearFoundedGot int
	var infoJSON []byte
	if err := pool.QueryRow(ctx,
		`SELECT tagline, hq_country, year_founded, company_info FROM companies WHERE slug = 'acme'`,
	).Scan(&storedTagline, &hqCountry, &yearFoundedGot, &infoJSON); err != nil {
		t.Fatalf("read company: %v", err)
	}
	if storedTagline != tagline {
		t.Errorf("tagline = %q, want %q", storedTagline, tagline)
	}
	if hqCountry != "de" {
		t.Errorf("hq_country = %q, want de (unsupplied field left unchanged)", hqCountry)
	}
	if yearFoundedGot != 2015 {
		t.Errorf("year_founded = %d, want 2015", yearFoundedGot)
	}
	var info map[string]string
	if err := json.Unmarshal(infoJSON, &info); err != nil {
		t.Fatalf("company_info not valid JSON: %v", err)
	}
	if info["website"] != website {
		t.Errorf("company_info.website = %q, want %q", info["website"], website)
	}
	if info["funding"] != "Series B" {
		t.Errorf("company_info.funding = %q, want the untouched existing key preserved", info["funding"])
	}
}

func TestResolveCanonicalSlug_FollowsARealAlias(t *testing.T) {
	pool := testdb.Pool(t)
	repo := employer.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		 VALUES ('acme-corp', 'acme', 'acme', 'spelling')`); err != nil {
		t.Fatalf("seed alias: %v", err)
	}

	canonical, err := repo.ResolveCanonicalSlug(ctx, "acme-corp")
	if err != nil {
		t.Fatalf("ResolveCanonicalSlug: %v", err)
	}
	if canonical != "acme" {
		t.Errorf("canonical = %q, want acme", canonical)
	}

	// A slug that names no alias resolves to itself.
	same, err := repo.ResolveCanonicalSlug(ctx, "never-merged")
	if err != nil {
		t.Fatalf("ResolveCanonicalSlug: %v", err)
	}
	if same != "never-merged" {
		t.Errorf("canonical = %q, want the candidate unchanged", same)
	}
}
