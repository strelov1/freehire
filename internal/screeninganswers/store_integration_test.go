//go:build integration

// Integration test for the screening-answers store against a real Postgres: a fake
// Repository proves Store's merge/validate logic, but only a real database proves the
// generated queries and the pgtype conversions in repository.go actually round-trip
// correctly through the screening_answers table. Run with:
// go test -tags=integration ./internal/screeninganswers/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package screeninganswers_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/screeninganswers"
	"github.com/strelov1/freehire/internal/testdb"
)

func insertScreeningAnswersIntegrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestStore_GetReturnsNotFoundForAUserWithNoRecord(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	userID := insertScreeningAnswersIntegrationUser(t, pool, "screening-notfound@example.test")
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries))

	_, err := store.Get(context.Background(), userID)
	if err != screeninganswers.ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateCreatesThenPartiallyMergesOverARealDatabase(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	userID := insertScreeningAnswersIntegrationUser(t, pool, "screening-merge@example.test")
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries))
	ctx := context.Background()

	days := 30
	relocate := true
	first, err := store.Update(ctx, userID, screeninganswers.Answers{
		NoticePeriodDays:  &days,
		WillingToRelocate: &relocate,
	})
	if err != nil {
		t.Fatalf("first Update: %v", err)
	}
	if first.NoticePeriodDays == nil || *first.NoticePeriodDays != 30 {
		t.Errorf("NoticePeriodDays = %v, want 30", first.NoticePeriodDays)
	}

	// A second, partial update must not clobber the first update's fields.
	amount := 120000
	currency := "USD"
	second, err := store.Update(ctx, userID, screeninganswers.Answers{
		DesiredSalaryAmount:   &amount,
		DesiredSalaryCurrency: &currency,
	})
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if second.NoticePeriodDays == nil || *second.NoticePeriodDays != 30 {
		t.Errorf("NoticePeriodDays after partial update = %v, want 30 (untouched)", second.NoticePeriodDays)
	}
	if second.WillingToRelocate == nil || *second.WillingToRelocate != true {
		t.Errorf("WillingToRelocate after partial update = %v, want true (untouched)", second.WillingToRelocate)
	}
	if second.DesiredSalaryAmount == nil || *second.DesiredSalaryAmount != 120000 {
		t.Errorf("DesiredSalaryAmount = %v, want 120000", second.DesiredSalaryAmount)
	}
	if second.DesiredSalaryCurrency == nil || *second.DesiredSalaryCurrency != "USD" {
		t.Errorf("DesiredSalaryCurrency = %v, want USD", second.DesiredSalaryCurrency)
	}

	// Reading back through Get must agree with what Update returned.
	got, err := store.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DesiredSalaryAmount == nil || *got.DesiredSalaryAmount != 120000 {
		t.Errorf("Get().DesiredSalaryAmount = %v, want 120000", got.DesiredSalaryAmount)
	}
}

func TestStore_AuthorizedCountriesRoundTripThroughPostgresTextArray(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	userID := insertScreeningAnswersIntegrationUser(t, pool, "screening-countries@example.test")
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries))
	ctx := context.Background()

	got, err := store.Update(ctx, userID, screeninganswers.Answers{
		AuthorizedCountries: []string{"US", "de", "  gb  "},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	want := []string{"us", "de", "gb"}
	if len(got.AuthorizedCountries) != len(want) {
		t.Fatalf("AuthorizedCountries = %v, want %v", got.AuthorizedCountries, want)
	}
	for i, c := range want {
		if got.AuthorizedCountries[i] != c {
			t.Errorf("AuthorizedCountries[%d] = %q, want %q", i, got.AuthorizedCountries[i], c)
		}
	}
}
