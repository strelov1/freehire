//go:build integration

// Integration tests for the similar-members ranking.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

// joinWith joins u and then pins the ranking keys, so a test controls the score exactly.
func joinWith(t *testing.T, pool *pgxpool.Pool, svc *buddy.Service, u int64, category string, countries, regions []string) string {
	t.Helper()
	m, err := svc.Join(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if countries == nil {
		countries = []string{}
	}
	if regions == nil {
		regions = []string{}
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE buddy_members SET category = NULLIF($2, ''), countries = $3, regions = $4 WHERE user_id = $1`,
		u, category, countries, regions); err != nil {
		t.Fatal(err)
	}
	return m.Handle
}

// applied records an application and its 'applied' ledger event and returns the
// application id the event carries.
func applied(t *testing.T, pool *pgxpool.Pool, u int64, company string, daysAgo int) int64 {
	t.Helper()
	ctx := context.Background()
	var appID int64
	if err := pool.QueryRow(ctx, `INSERT INTO applications (user_id, company_slug, applied_at)
		VALUES ($1, $2, now() - make_interval(days => $3)) RETURNING id`, u, company, daysAgo).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO application_events (user_id, application_id, company_slug, kind, occurred_at, source)
		VALUES ($1, $2, $3, 'applied', now() - make_interval(days => $4), 'user')`, u, appID, company, daysAgo); err != nil {
		t.Fatal(err)
	}
	return appID
}

func handles(list []buddy.SimilarMember) []string {
	out := make([]string, len(list))
	for i, m := range list {
		out[i] = m.Handle
	}
	return out
}

func TestSimilarRanksCloserFirstAndKeepsZeroOverlap(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme Corp'), ('globex', 'Globex')`); err != nil {
		t.Fatal(err)
	}
	viewer, a, b, none := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test"), seedUser(t, pool, "n@example.test")
	joinWith(t, pool, svc, viewer, "backend", []string{"de"}, []string{"eu"})
	hA := joinWith(t, pool, svc, a, "backend", []string{"de"}, []string{"eu"})
	hB := joinWith(t, pool, svc, b, "backend", []string{"fr"}, nil)
	hN := joinWith(t, pool, svc, none, "", nil, nil)
	for _, u := range []int64{viewer, a} {
		applied(t, pool, u, "acme", 5)
		applied(t, pool, u, "globex", 5)
	}

	got, err := svc.Similar(ctx, viewer, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{hA, hB, hN}; !slices.Equal(handles(got), want) {
		t.Fatalf("order = %v, want %v", handles(got), want)
	}
	// A: 3 category + 2 countries + 1 regions + 2*2 companies; B: category only; none: 0.
	if got[0].Score != 10 || got[1].Score != 3 || got[2].Score != 0 {
		t.Fatalf("scores = %d %d %d", got[0].Score, got[1].Score, got[2].Score)
	}
	if !slices.Equal(got[0].Shared, []buddy.Company{{Slug: "acme", Name: "Acme Corp"}, {Slug: "globex", Name: "Globex"}}) {
		t.Fatalf("shared = %+v", got[0].Shared)
	}
	if got[0].Card.Country != "de" || len(got[2].Shared) != 0 {
		t.Fatalf("card/shared = %+v / %+v", got[0].Card, got[2].Shared)
	}
}

// A viewer with no resolved category must still be ranked: a NULL comparison must not
// turn the whole score NULL (it would sort first and fail the int scan).
func TestSimilarViewerWithoutCategory(t *testing.T) {
	pool, svc := setup(t)
	viewer, a, b := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test")
	joinWith(t, pool, svc, viewer, "", []string{"de"}, nil)
	hA := joinWith(t, pool, svc, a, "backend", []string{"de"}, nil)
	hB := joinWith(t, pool, svc, b, "backend", []string{"fr"}, nil)
	got, err := svc.Similar(context.Background(), viewer, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{hA, hB}; !slices.Equal(handles(got), want) || got[0].Score != 2 || got[1].Score != 0 {
		t.Fatalf("list = %v scores %d %d", handles(got), got[0].Score, got[1].Score)
	}
}

func TestSimilarTieBreaksOnNewestJoinAndPages(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, old, fresh := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "o@example.test"), seedUser(t, pool, "f@example.test")
	joinWith(t, pool, svc, viewer, "backend", nil, nil)
	hOld := joinWith(t, pool, svc, old, "backend", nil, nil)
	hFresh := joinWith(t, pool, svc, fresh, "backend", nil, nil)
	if _, err := pool.Exec(ctx, `UPDATE buddy_members SET joined_at = now() - interval '2 days' WHERE user_id = $1`, old); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Similar(ctx, viewer, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{hFresh, hOld}; !slices.Equal(handles(got), want) {
		t.Fatalf("order = %v, want %v", handles(got), want)
	}
	page2, err := svc.Similar(ctx, viewer, 1, 1)
	if err != nil || len(page2) != 1 || page2[0].Handle != hOld {
		t.Fatalf("offset page = %+v, %v", page2, err)
	}
}

func TestSimilarExcludesLeftAndBlockedBothWays(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, left, blockedByMe, blockedMe, ok := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "l@example.test"),
		seedUser(t, pool, "b1@example.test"), seedUser(t, pool, "b2@example.test"), seedUser(t, pool, "ok@example.test")
	hViewer := joinWith(t, pool, svc, viewer, "backend", nil, nil)
	for _, u := range []int64{left, blockedByMe, blockedMe} {
		joinWith(t, pool, svc, u, "backend", nil, nil)
	}
	hOK := joinWith(t, pool, svc, ok, "backend", nil, nil)
	if err := svc.Leave(ctx, left); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2), ($3, $1)`, viewer, blockedByMe, blockedMe); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Similar(ctx, viewer, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Also proves the viewer is never listed to themselves.
	if want := []string{hOK}; !slices.Equal(handles(got), want) {
		t.Fatalf("list = %v, want %v", handles(got), want)
	}
	for _, blocked := range []int64{blockedByMe, blockedMe} {
		list, err := svc.Similar(ctx, blocked, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(handles(list), hViewer) {
			t.Fatalf("user %d still sees the viewer across a block", blocked)
		}
	}
}

func TestSimilarSharedCompanyWindowHidingAndRetraction(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	viewer, other := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "o@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	joinWith(t, pool, svc, other, "", nil, nil)

	applied(t, pool, viewer, "stale-co", 5)
	applied(t, pool, other, "stale-co", 120) // older than 90 days on one side: no points, not shown
	viewerHidden := applied(t, pool, viewer, "hidden-by-me", 5)
	applied(t, pool, other, "hidden-by-me", 5)
	applied(t, pool, viewer, "hidden-by-them", 5)
	otherHidden := applied(t, pool, other, "hidden-by-them", 5)
	applied(t, pool, viewer, "counts", 5)
	applied(t, pool, other, "counts", 5)
	if _, err := pool.Exec(ctx, `INSERT INTO buddy_hidden_applications (user_id, application_id) VALUES ($1, $2), ($3, $4)`,
		viewer, viewerHidden, other, otherHidden); err != nil {
		t.Fatal(err)
	}
	retracted := applied(t, pool, other, "retracted-co", 5)
	applied(t, pool, viewer, "retracted-co", 5)
	if _, err := pool.Exec(ctx, `UPDATE application_events SET retracted_at = now() WHERE application_id = $1`, retracted); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Similar(ctx, viewer, 10, 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	// No companies row for the slug: the name falls back to the slug.
	if got[0].Score != 2 || !slices.Equal(got[0].Shared, []buddy.Company{{Slug: "counts", Name: "counts"}}) {
		t.Fatalf("entry = %+v", got[0])
	}
}

func TestSimilarCapsSharedCompanies(t *testing.T) {
	pool, svc := setup(t)
	viewer, other := seedUser(t, pool, "v@example.test"), seedUser(t, pool, "o@example.test")
	joinWith(t, pool, svc, viewer, "", nil, nil)
	joinWith(t, pool, svc, other, "", nil, nil)
	for i := range buddy.SharedCompanyCap + 2 {
		slug := fmt.Sprintf("co-%d", i)
		applied(t, pool, viewer, slug, 3)
		applied(t, pool, other, slug, 3)
	}
	got, err := svc.Similar(context.Background(), viewer, 10, 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	if len(got[0].Shared) != buddy.SharedCompanyCap || got[0].Score != 2*buddy.SharedCompanyCap {
		t.Fatalf("shared = %d, score = %d", len(got[0].Shared), got[0].Score)
	}
}

func TestSimilarRefusesNonMembers(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	u := seedUser(t, pool, "u@example.test")
	if _, err := svc.Similar(ctx, u, 10, 0); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("never joined: %v", err)
	}
	if _, err := svc.Join(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Similar(ctx, u, 10, 0); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("left: %v", err)
	}
}
