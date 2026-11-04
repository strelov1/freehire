//go:build integration

// Integration tests for the candidate-geography SQL semantics. Three of the four
// properties this change relies on are pure SQL and cannot be observed from a unit test:
// the NULL-versus-'{}' distinction (Go's nil slice and empty slice both have to survive
// the round trip as different values), the monotonic guard on both write paths, and the
// staleness rule that keeps the reconciler off superseded structures. Run with:
// go test -tags=integration ./internal/platform/db/
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
