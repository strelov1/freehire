//go:build integration

package companyrelation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func seed(t *testing.T, pool *pgxpool.Pool) (userID int64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('proposer@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, c := range []struct {
		slug, name string
		jobs       int
	}{
		{"krka", "Krka", 3},
		{"krka-tovarna-zdravil-d-d-novo-mesto", "Krka, tovarna zdravil, d. d., Novo mesto", 2},
		{"novartis", "Novartis", 1},
		{"novartis-slovenija", "Novartis Slovenija", 1},
		{"sandoz", "Sandoz", 1},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, job_count) VALUES ($1, $2, $3)`, c.slug, c.name, c.jobs); err != nil {
			t.Fatalf("seed company %s: %v", c.slug, err)
		}
		for i := range c.jobs {
			ext := fmt.Sprintf("%s-%d", c.slug, i)
			if _, err := pool.Exec(ctx,
				`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
				 VALUES ('test', $1, 'https://example.test/'||$1, 'Engineer', $2, $3, $1)`,
				ext, c.name, c.slug); err != nil {
				t.Fatalf("seed job %s: %v", ext, err)
			}
		}
	}
	return userID
}

func TestApproveSameMergesAndMovesTheJobs(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	user := seed(t, pool)
	s := New(pool)

	p, err := s.Propose(ctx, user, Proposal{Kind: KindSame, CompanySlug: "krka-tovarna-zdravil-d-d-novo-mesto", TargetSlug: "krka"})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := s.Propose(ctx, user, Proposal{Kind: KindSame, CompanySlug: "krka-tovarna-zdravil-d-d-novo-mesto", TargetSlug: "krka"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second identical proposal: err = %v, want ErrDuplicate", err)
	}

	d, err := s.Approve(ctx, p.ID, user)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if d.MovedJobs != 2 {
		t.Errorf("moved %d jobs, want 2", d.MovedJobs)
	}

	var canon, folded, reason string
	if err := pool.QueryRow(ctx,
		`SELECT canonical_slug, folded_key, reason FROM company_slug_aliases WHERE alias_slug = 'krka-tovarna-zdravil-d-d-novo-mesto'`).
		Scan(&canon, &folded, &reason); err != nil {
		t.Fatalf("read alias: %v", err)
	}
	// The key is the fold of the NAME the source sends, which is what ingest looks up.
	if canon != "krka" || folded != "krkatovarnazdravil" || reason != "moderator" {
		t.Errorf("alias = (%q, %q, %q), want (krka, krkatovarnazdravil, moderator)", canon, folded, reason)
	}
	var left int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE company_slug = 'krka-tovarna-zdravil-d-d-novo-mesto'`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d jobs left on the retired slug", left)
	}
	var retiredRow int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM companies WHERE slug = 'krka-tovarna-zdravil-d-d-novo-mesto'`).Scan(&retiredRow); err != nil {
		t.Fatal(err)
	}
	if retiredRow != 0 {
		t.Error("the retired company row is still there, so its page answers 200 instead of the 301")
	}

	// Approving again resumes the move rather than refusing, and a rejection is refused.
	if _, err := s.Approve(ctx, p.ID, user); err != nil {
		t.Errorf("re-approve: %v", err)
	}
	if err := s.Reject(ctx, p.ID, user); !errors.Is(err, ErrDecided) {
		t.Errorf("reject after approval: err = %v, want ErrDecided", err)
	}
}

func TestSameRefusesARetiringCanon(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	user := seed(t, pool)
	if _, err := pool.Exec(ctx,
		`INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		 VALUES ('sandoz-ag', 'sandoz', 'sandozag', 'legal_form')`); err != nil {
		t.Fatal(err)
	}
	_, err := New(pool).Propose(ctx, user, Proposal{Kind: KindSame, CompanySlug: "sandoz", TargetSlug: "novartis"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict: sandoz is a canon, and retiring it strands its aliases", err)
	}
}

func TestSubsidiaryLinksAndStaysOneLevel(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	user := seed(t, pool)
	s := New(pool)

	p, err := s.Propose(ctx, user, Proposal{Kind: KindSubsidiary, CompanySlug: "novartis-slovenija", TargetSlug: "novartis", Note: "local entity"})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	d, err := s.Approve(ctx, p.ID, user)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if d.MovedJobs != 0 {
		t.Errorf("a subsidiary moved %d jobs, want 0", d.MovedJobs)
	}
	var parent string
	if err := pool.QueryRow(ctx,
		`SELECT parent_slug FROM company_parents WHERE child_slug = 'novartis-slovenija'`).Scan(&parent); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if parent != "novartis" {
		t.Errorf("parent = %q, want novartis", parent)
	}

	// novartis-slovenija is now a subsidiary, so nothing may sit under it.
	if _, err := s.Propose(ctx, user, Proposal{Kind: KindSubsidiary, CompanySlug: "sandoz", TargetSlug: "novartis-slovenija"}); !errors.Is(err, ErrConflict) {
		t.Errorf("subsidiary of a subsidiary: err = %v, want ErrConflict", err)
	}
	// novartis has a subsidiary, so it cannot become one.
	if _, err := s.Propose(ctx, user, Proposal{Kind: KindSubsidiary, CompanySlug: "novartis", TargetSlug: "sandoz"}); !errors.Is(err, ErrConflict) {
		t.Errorf("parent becoming a subsidiary: err = %v, want ErrConflict", err)
	}
}

func TestMergeCarriesSubsidiariesToTheCanon(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	user := seed(t, pool)
	s := New(pool)
	if _, err := pool.Exec(ctx,
		`INSERT INTO company_parents (child_slug, parent_slug) VALUES ('novartis-slovenija', 'sandoz')`); err != nil {
		t.Fatal(err)
	}
	p, err := s.Propose(ctx, user, Proposal{Kind: KindSame, CompanySlug: "sandoz", TargetSlug: "novartis"})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := s.Approve(ctx, p.ID, user); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var parent string
	if err := pool.QueryRow(ctx,
		`SELECT parent_slug FROM company_parents WHERE child_slug = 'novartis-slovenija'`).Scan(&parent); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if parent != "novartis" {
		t.Errorf("parent = %q, want novartis: a merge keeps the retiring slug's subsidiaries", parent)
	}
}

func TestProposeRefusesUnknownCompanyAndBadKind(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	user := seed(t, pool)
	s := New(pool)
	if _, err := s.Propose(ctx, user, Proposal{Kind: KindSame, CompanySlug: "krka", TargetSlug: "nobody"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown target: err = %v, want ErrNotFound", err)
	}
	if _, err := s.Propose(ctx, user, Proposal{Kind: "merge", CompanySlug: "krka", TargetSlug: "sandoz"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad kind: err = %v, want ErrInvalid", err)
	}
}
