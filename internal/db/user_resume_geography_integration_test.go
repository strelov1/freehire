//go:build integration

// Integration tests for the candidate-geography SQL semantics. Three of the four
// properties this change relies on are pure SQL and cannot be observed from a unit test:
// the NULL-versus-'{}' distinction (Go's nil slice and empty slice both have to survive
// the round trip as different values), the monotonic guard on both write paths, and the
// staleness rule that keeps the reconciler off superseded structures. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// seedUserWithCV creates a user with a stored CV uploaded at uploadedAt, and optionally a
// structured résumé stamped at structuredAt (zero = none stored).
func seedUserWithCV(t *testing.T, q *Queries, email string, uploadedAt, structuredAt time.Time, structuredJSON string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := q.db.QueryRow(ctx,
		`INSERT INTO users (email, email_verified, resume_object_key, resume_uploaded_at)
		 VALUES ($1, true, 'resumes/x', $2) RETURNING id`, email, uploadedAt).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	if !structuredAt.IsZero() {
		if _, err := q.db.Exec(ctx,
			`UPDATE users SET resume_structured = $2::jsonb, resume_structured_uploaded_at = $3 WHERE id = $1`,
			id, structuredJSON, structuredAt); err != nil {
			t.Fatalf("seed structure for %s: %v", email, err)
		}
	}
	return id
}

// TestResumeGeographyDistinguishesUnknownFromUnresolved is the whole reason these columns
// are nullable with no default. A candidate whose CV states nothing and a candidate whose
// CV states a place the dictionary could not resolve are different facts, and the second
// one is the dictionary's live coverage metric.
func TestResumeGeographyDistinguishesUnknownFromUnresolved(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	at := time.Now().UTC().Truncate(time.Second)
	unknown := seedUserWithCV(t, q, "geo-unknown@example.test", at, at, `{}`)
	unresolved := seedUserWithCV(t, q, "geo-unresolved@example.test", at, at, `{"location":"Greater Philadelphia"}`)

	// "not known" — nil arrays.
	if err := q.SetUserResumeGeography(ctx, SetUserResumeGeographyParams{
		ID: unknown, ResumeCountries: nil, ResumeRegions: nil, ResumeCities: nil,
		ResumeUploadedAt: stamp(at),
	}); err != nil {
		t.Fatalf("SetUserResumeGeography(unknown): %v", err)
	}
	// "stated, but the dictionary was silent" — empty non-nil arrays.
	if err := q.SetUserResumeGeography(ctx, SetUserResumeGeographyParams{
		ID: unresolved, ResumeCountries: []string{}, ResumeRegions: []string{}, ResumeCities: []string{},
		ResumeUploadedAt: stamp(at),
	}); err != nil {
		t.Fatalf("SetUserResumeGeography(unresolved): %v", err)
	}

	var isNull bool
	if err := pool.QueryRow(ctx, `SELECT resume_countries IS NULL FROM users WHERE id = $1`, unknown).Scan(&isNull); err != nil {
		t.Fatalf("read unknown: %v", err)
	}
	if !isNull {
		t.Error("a candidate whose CV states no location stored a non-NULL country array")
	}
	if err := pool.QueryRow(ctx, `SELECT resume_countries IS NULL FROM users WHERE id = $1`, unresolved).Scan(&isNull); err != nil {
		t.Fatalf("read unresolved: %v", err)
	}
	if isNull {
		t.Error("a stated-but-unresolvable location stored NULL, collapsing it with 'not known'")
	}

	// The coverage metric the distinction exists to make possible.
	var gap int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE resume_countries = '{}'`).Scan(&gap); err != nil {
		t.Fatalf("coverage query: %v", err)
	}
	if gap != 1 {
		t.Errorf("count of stated-but-unresolved = %d, want 1", gap)
	}
}

// TestListUsersForResumeGeoBackfillSkipsSupersededStructures pins the reconciler's
// selection rule. Deriving geography from a structure that no longer describes the stored
// CV would route around the staleness rule that governs the structure itself.
func TestListUsersForResumeGeoBackfillSkipsSupersededStructures(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	at := time.Now().UTC().Truncate(time.Second)
	fresh := seedUserWithCV(t, q, "geo-fresh@example.test", at, at, `{"location":"Kraków, Poland"}`)
	// A newer CV landed; the structure still carries the older stamp.
	seedUserWithCV(t, q, "geo-stale@example.test", at.Add(time.Hour), at, `{"location":"Kraków, Poland"}`)
	// A CV whose extraction never landed at all — the 37-user case on production.
	seedUserWithCV(t, q, "geo-nostruct@example.test", at, time.Time{}, "")

	rows, err := q.ListUsersForResumeGeoBackfill(ctx, 0)
	if err != nil {
		t.Fatalf("ListUsersForResumeGeoBackfill: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != fresh {
		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		t.Fatalf("selected %v, want only the user with a current structure (%d)", ids, fresh)
	}
	if rows[0].Location != "Kraków, Poland" {
		t.Errorf("location = %q, want the structure's location line", rows[0].Location)
	}

	// --user narrows to one; a user outside the eligible set stays excluded.
	one, err := q.ListUsersForResumeGeoBackfill(ctx, fresh)
	if err != nil {
		t.Fatalf("ListUsersForResumeGeoBackfill(--user): %v", err)
	}
	if len(one) != 1 || one[0].ID != fresh {
		t.Errorf("--user selection = %+v, want exactly the requested eligible user", one)
	}
}

// TestListUsersForResumeGeoBackfillReadsAnAbsentLocationAsEmpty guards the coalesce in
// the query: a structure with no location key must not scan as NULL into a string.
func TestListUsersForResumeGeoBackfillReadsAnAbsentLocationAsEmpty(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	at := time.Now().UTC().Truncate(time.Second)
	seedUserWithCV(t, q, "geo-noloc@example.test", at, at, `{"full_name":"Jane"}`)

	rows, err := q.ListUsersForResumeGeoBackfill(ctx, 0)
	if err != nil {
		t.Fatalf("ListUsersForResumeGeoBackfill: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("selected %d users, want 1", len(rows))
	}
	if rows[0].Location != "" {
		t.Errorf("location = %q, want empty for a structure with no location key", rows[0].Location)
	}
}

// TestSetUserResumeGeographyRefusesASupersededStamp: the reconciler reads a row, derives,
// and writes back. If a fresh upload lands in between, the write must be dropped rather
// than stamping the new CV with geography derived from the old one.
func TestSetUserResumeGeographyRefusesASupersededStamp(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	at := time.Now().UTC().Truncate(time.Second)
	id := seedUserWithCV(t, q, "geo-race@example.test", at, at, `{"location":"Kraków, Poland"}`)

	// A new CV lands after the reconciler read the row.
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`, id, at.Add(time.Hour)); err != nil {
		t.Fatalf("simulate re-upload: %v", err)
	}

	if err := q.SetUserResumeGeography(ctx, SetUserResumeGeographyParams{
		ID: id, ResumeCountries: []string{"pl"}, ResumeRegions: []string{"eu"}, ResumeCities: []string{"Kraków"},
		ResumeUploadedAt: stamp(at), // the stamp read before the re-upload
	}); err != nil {
		t.Fatalf("SetUserResumeGeography: %v", err)
	}

	var isNull bool
	if err := pool.QueryRow(ctx, `SELECT resume_countries IS NULL FROM users WHERE id = $1`, id).Scan(&isNull); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !isNull {
		t.Error("geography derived from a superseded CV was written; the monotonic guard did not hold")
	}
}

// TestSetUserResumeStructuredWritesGeographyInTheSameStatement is the invariant the whole
// write-path decision rests on: structure and geography land together, or not at all.
func TestSetUserResumeStructuredWritesGeographyInTheSameStatement(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	at := time.Now().UTC().Truncate(time.Second)
	id := seedUserWithCV(t, q, "geo-together@example.test", at, time.Time{}, "")

	if rows, err := q.SetUserResumeStructured(ctx, SetUserResumeStructuredParams{
		ID:                         id,
		ResumeStructured:           []byte(`{"location":"Kraków, Poland"}`),
		ResumeStructuredModel:      pgtype.Text{String: "model-x", Valid: true},
		ResumeStructuredUploadedAt: stamp(at),
		ResumeCountries:            []string{"pl"},
		ResumeRegions:              []string{"eu"},
		ResumeCities:               []string{"Kraków"},
	}); err != nil {
		t.Fatalf("SetUserResumeStructured: %v", err)
	} else if rows != 1 {
		t.Fatalf("rows affected = %d, want 1", rows)
	}

	got, err := q.GetUserResumeGeography(ctx, id)
	if err != nil {
		t.Fatalf("GetUserResumeGeography: %v", err)
	}
	if len(got.ResumeCountries) != 1 || got.ResumeCountries[0] != "pl" {
		t.Errorf("countries = %v, want [pl] from the structure write", got.ResumeCountries)
	}
	if !got.ResumeStructuredUploadedAt.Valid || !got.ResumeStructuredUploadedAt.Time.Equal(got.ResumeUploadedAt.Time) {
		t.Error("the geography read does not report the structure as current")
	}

	// And clearing the résumé takes the geography with it.
	if err := q.ClearUserResume(ctx, id); err != nil {
		t.Fatalf("ClearUserResume: %v", err)
	}
	var isNull bool
	if err := pool.QueryRow(ctx, `SELECT resume_countries IS NULL FROM users WHERE id = $1`, id).Scan(&isNull); err != nil {
		t.Fatalf("read after clear: %v", err)
	}
	if !isNull {
		t.Error("geography survived ClearUserResume — it must not outlive the CV it describes")
	}
}
