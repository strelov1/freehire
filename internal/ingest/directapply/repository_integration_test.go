//go:build integration

// Integration tests for QueriesRepository: which postings count as on-site vacancies, and
// that a recorded application round-trips through the real direct_applications row.
// Run with: go test -tags=integration ./internal/ingest/directapply/
package directapply_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestQueriesRepository(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	repo := directapply.NewQueriesRepository(db.New(pool))

	var employerID, candidateID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('hr@acme.test') RETURNING id`).Scan(&employerID); err != nil {
		t.Fatalf("seed employer: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('c@example.test', true) RETURNING id`).Scan(&candidateID); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
	seedJob := func(source, externalID, slug string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
			VALUES ($1, $2, '', 'Platform Engineer', 'Acme', 'acme', $3, $4) RETURNING id`,
			source, externalID, slug, employerID).Scan(&id); err != nil {
			t.Fatalf("seed job %s: %v", slug, err)
		}
		return id
	}
	onSite := seedJob("employer", "freehire:0b6f", "on-site")
	seedJob("employer", "https://acme.test/jobs/1", "linked")

	t.Run("only an on-site vacancy is one", func(t *testing.T) {
		v, err := repo.Vacancy(ctx, "on-site")
		if err != nil {
			t.Fatalf("Vacancy(on-site): %v", err)
		}
		if v.ID != onSite || v.EmployerID != employerID || !v.Open {
			t.Errorf("vacancy = %+v", v)
		}
		for _, slug := range []string{"linked", "missing"} {
			if _, err := repo.Vacancy(ctx, slug); !errors.Is(err, directapply.ErrNotFound) {
				t.Errorf("Vacancy(%s) err = %v, want ErrNotFound", slug, err)
			}
		}
	})

	t.Run("a vacancy with no stored form asks nothing", func(t *testing.T) {
		form, err := repo.Form(ctx, onSite)
		if err != nil || len(form.Fields) != 0 {
			t.Errorf("Form = %+v, %v", form, err)
		}
	})

	t.Run("the candidate's verification is read", func(t *testing.T) {
		c, err := repo.Candidate(ctx, candidateID)
		if err != nil || !c.Verified || c.Email != "c@example.test" {
			t.Errorf("Candidate = %+v, %v", c, err)
		}
	})

	t.Run("an application is recorded, then stamped sent, then refused", func(t *testing.T) {
		var cvID uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`, candidateID).Scan(&cvID); err != nil {
			t.Fatalf("seed cv: %v", err)
		}
		rec := directapply.Record{JobID: onSite, UserID: candidateID, CVID: cvID, Note: "hi",
			Answers: []directapply.Answer{{ID: "q1", Label: "EU work permit?", Type: "boolean", Answer: "true"}}}
		id, recorded, err := repo.Record(ctx, rec, 30)
		if err != nil || !recorded {
			t.Fatalf("Record = %d, %v, %v", id, recorded, err)
		}
		if st, err := repo.State(ctx, onSite, candidateID); err != nil || st != directapply.StateSending {
			t.Fatalf("State before delivery = %v, %v; want a claimed, in-flight row", st, err)
		}
		if err := repo.MarkSent(ctx, id); err != nil {
			t.Fatalf("MarkSent: %v", err)
		}
		if st, err := repo.State(ctx, onSite, candidateID); err != nil || st != directapply.StateSent {
			t.Fatalf("State after delivery = %v, %v", st, err)
		}
		if _, recorded, err := repo.Record(ctx, rec, 30); err != nil || recorded {
			t.Errorf("re-Record of a sent application = %v, %v; want nothing written", recorded, err)
		}
		var label string
		if err := pool.QueryRow(ctx, `SELECT answers->0->>'label' FROM direct_applications WHERE id = $1`, id).Scan(&label); err != nil || label != "EU work permit?" {
			t.Errorf("stored answer snapshot label = %q, %v", label, err)
		}
	})
}
