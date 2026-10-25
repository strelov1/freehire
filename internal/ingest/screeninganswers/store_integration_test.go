//go:build integration

// Integration test for the screening-answers store against a real Postgres: a fake
// Repository proves Store's merge/validate logic, but only a real database proves the
// generated queries and the pgtype conversions in repository.go actually round-trip
// correctly through the screening_answers table. Run with:
// go test -tags=integration ./internal/ingest/screeninganswers/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package screeninganswers_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
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
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries, pool))

	_, err := store.Get(context.Background(), userID)
	if err != screeninganswers.ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateCreatesThenPartiallyMergesOverARealDatabase(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	userID := insertScreeningAnswersIntegrationUser(t, pool, "screening-merge@example.test")
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries, pool))
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
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries, pool))
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

// TestStore_ConcurrentUpdatesForTheSameUserDoNotLoseEachOthersFields reproduces the two
// concurrent writers the AGENTS.md documents (the manual-edit handler and the assistant's
// screening_answers_set tool, both calling this same Store.Update): eight goroutines each
// set one distinct field of the same user's record at once, racing real overlapping
// transactions against the real database rather than a fake Repository. Before
// UpdateLocked's row lock, a Store.Update that read the "existing" record and wrote back
// its merge as two separate, unlocked statements could have the read of one goroutine's
// transaction land before another's write committed, so that write's field would be
// merged away when the reader wrote back — a lost update. With every read-merge-write
// serialized on the row lock, every goroutine's write is either fully visible to every
// later one's read or has not started yet, so none can be lost: all eight fields must
// survive regardless of scheduling.
func TestStore_ConcurrentUpdatesForTheSameUserDoNotLoseEachOthersFields(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	userID := insertScreeningAnswersIntegrationUser(t, pool, "screening-concurrent@example.test")
	store := screeninganswers.New(screeninganswers.NewQueriesRepository(queries, pool))
	ctx := context.Background()

	days := 30
	amount := 120000
	currency := "USD"
	period := "year"
	relocate := true
	visa := false
	adult := true
	consent := true
	updates := []screeninganswers.Answers{
		{AuthorizedCountries: []string{"us", "de"}},
		{VisaSponsorshipNeeded: &visa},
		{DesiredSalaryAmount: &amount},
		{DesiredSalaryCurrency: &currency},
		{DesiredSalaryPeriod: &period},
		{NoticePeriodDays: &days},
		{WillingToRelocate: &relocate},
		{Age18OrOlder: &adult},
		{PrivacyConsent: &consent},
	}

	// A closed start channel released after every goroutine is parked on it lines up their
	// first UpdateLocked calls as tightly as a real scheduler allows, maximizing the chance
	// an unserialized implementation would overlap two transactions' read and write.
	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(len(updates))
	done.Add(len(updates))
	errs := make([]error, len(updates))
	for i, u := range updates {
		go func(i int, u screeninganswers.Answers) {
			defer done.Done()
			ready.Done()
			<-start
			_, errs[i] = store.Update(ctx, userID, u)
		}(i, u)
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Update #%d: %v", i, err)
		}
	}

	got, err := store.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.AuthorizedCountries) != 2 {
		t.Errorf("AuthorizedCountries = %v, want [us de]", got.AuthorizedCountries)
	}
	if got.VisaSponsorshipNeeded == nil || *got.VisaSponsorshipNeeded != visa {
		t.Errorf("VisaSponsorshipNeeded = %v, want %v", got.VisaSponsorshipNeeded, visa)
	}
	if got.DesiredSalaryAmount == nil || *got.DesiredSalaryAmount != amount {
		t.Errorf("DesiredSalaryAmount = %v, want %v", got.DesiredSalaryAmount, amount)
	}
	if got.DesiredSalaryCurrency == nil || *got.DesiredSalaryCurrency != currency {
		t.Errorf("DesiredSalaryCurrency = %v, want %v", got.DesiredSalaryCurrency, currency)
	}
	if got.DesiredSalaryPeriod == nil || *got.DesiredSalaryPeriod != period {
		t.Errorf("DesiredSalaryPeriod = %v, want %v", got.DesiredSalaryPeriod, period)
	}
	if got.NoticePeriodDays == nil || *got.NoticePeriodDays != days {
		t.Errorf("NoticePeriodDays = %v, want %v", got.NoticePeriodDays, days)
	}
	if got.WillingToRelocate == nil || *got.WillingToRelocate != relocate {
		t.Errorf("WillingToRelocate = %v, want %v", got.WillingToRelocate, relocate)
	}
	if got.Age18OrOlder == nil || *got.Age18OrOlder != adult {
		t.Errorf("Age18OrOlder = %v, want %v", got.Age18OrOlder, adult)
	}
	if got.PrivacyConsent == nil || *got.PrivacyConsent != consent {
		t.Errorf("PrivacyConsent = %v, want %v", got.PrivacyConsent, consent)
	}
}
