//go:build integration

// Integration tests for the derived-requirements overlay: SetJobEnrichment must fill
// enrichment.requirements from jobs.requirements_derived when the model's payload
// states none, and leave the payload's own list alone when it states one. This is the
// only thing that keeps a derived list alive across an enrichment run — the statement
// assigns the enrichment blob wholesale — and it is SQL behavior, verifiable only
// against a real Postgres. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertJobWithDerivedRequirements inserts an open, unenriched job whose description
// derivation already produced a requirements list.
func insertJobWithDerivedRequirements(t *testing.T, pool *pgxpool.Pool, externalID, derived string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, requirements_derived)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2::jsonb)
		 RETURNING id`,
		externalID, derived).Scan(&id)
	if err != nil {
		t.Fatalf("insert job with derived requirements: %v", err)
	}
	return id
}

// enrichedRequirements is the requirements projection read back off jobs.enrichment.
type enrichedRequirements struct {
	Summary      string `json:"summary"`
	Requirements []struct {
		Text     string `json:"text"`
		Priority string `json:"priority"`
	} `json:"requirements"`
}

func readRequirements(t *testing.T, pool *pgxpool.Pool, id int64) enrichedRequirements {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(), "SELECT enrichment FROM jobs WHERE id = $1", id).Scan(&raw); err != nil {
		t.Fatalf("read enrichment: %v", err)
	}
	var e enrichedRequirements
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("decode enrichment %s: %v", raw, err)
	}
	return e
}

func texts(e enrichedRequirements) []string {
	out := make([]string, 0, len(e.Requirements))
	for _, r := range e.Requirements {
		out = append(out, r.Text+"/"+r.Priority)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

const derivedGo = `[{"text":"5+ years of Go","priority":"required"},{"text":"Kubernetes","priority":"preferred"}]`

func TestSetJobEnrichment_DerivedRequirementsOverlay(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	set := func(t *testing.T, id int64, payload string) {
		t.Helper()
		if err := q.SetJobEnrichment(ctx, SetJobEnrichmentParams{
			Enrichment:        json.RawMessage(payload),
			EnrichedAt:        pgtype.Timestamptz{},
			EnrichmentVersion: 2,
			ID:                id,
		}); err != nil {
			t.Fatalf("SetJobEnrichment: %v", err)
		}
	}

	t.Run("a payload stating requirements keeps its own", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "modelwins", derivedGo)
		set(t, id, `{"summary":"keep me","requirements":[{"text":"Rust","priority":"required"}]}`)

		got := readRequirements(t, pool, id)
		if want := []string{"Rust/required"}; !equalStrings(texts(got), want) {
			t.Errorf("requirements = %v, want the payload's %v", texts(got), want)
		}
		if got.Summary != "keep me" {
			t.Errorf("summary = %q, want the payload's (the overlay must not disturb it)", got.Summary)
		}
	})

	t.Run("a payload stating none picks up the derivation", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "derivefills", derivedGo)
		set(t, id, `{"summary":"a synopsis"}`)

		got := readRequirements(t, pool, id)
		want := []string{"5+ years of Go/required", "Kubernetes/preferred"}
		if !equalStrings(texts(got), want) {
			t.Errorf("requirements = %v, want the derived %v", texts(got), want)
		}
		if got.Summary != "a synopsis" {
			t.Errorf("summary = %q, want the payload's", got.Summary)
		}
	})

	t.Run("an empty list in the payload counts as stating none", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "emptylist", derivedGo)
		set(t, id, `{"requirements":[]}`)

		got := readRequirements(t, pool, id)
		if len(got.Requirements) != 2 {
			t.Errorf("requirements = %v, want the derived two — an empty array is not a reading", texts(got))
		}
	})

	t.Run("neither source yields no requirements at all", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "neither", `[]`)
		set(t, id, `{"summary":"a synopsis"}`)

		if got := readRequirements(t, pool, id); len(got.Requirements) != 0 {
			t.Errorf("requirements = %v, want none", texts(got))
		}
	})

	// The point of the overlay: the statement assigns the blob wholesale, so without
	// it every enrichment run would silently erase what ingest derived.
	t.Run("a second run cannot erase the derived list", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "survives", derivedGo)
		set(t, id, `{"summary":"first"}`)
		set(t, id, `{"summary":"second"}`)

		got := readRequirements(t, pool, id)
		if len(got.Requirements) != 2 {
			t.Errorf("requirements = %v, want the derived two still present", texts(got))
		}
	})

	// The derivation is orthogonal to the model: it must not make an unenriched job
	// look enriched, because the enrichment queue is gated on exactly that stamp.
	t.Run("the derivation alone leaves a job unenriched", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "stillunenriched", derivedGo)

		var version int32
		var enrichedAt pgtype.Timestamptz
		err := pool.QueryRow(ctx, "SELECT enrichment_version, enriched_at FROM jobs WHERE id = $1", id).
			Scan(&version, &enrichedAt)
		if err != nil {
			t.Fatalf("read provenance: %v", err)
		}
		if version != 0 || enrichedAt.Valid {
			t.Errorf("provenance = version %d / enriched_at %v, want an unenriched job", version, enrichedAt)
		}
	})
}

func TestUpsertJob_RequirementsDerived(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	t.Run("a re-ingest that derives nothing keeps the stored list", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "keepstored", derivedGo)

		// The shape of a failed detail fetch: the row is rewritten with an empty
		// derivation because the description came back empty.
		_, err := pool.Exec(ctx,
			`UPDATE jobs SET requirements_derived = CASE
			     WHEN jsonb_array_length($2::jsonb) > 0 THEN $2::jsonb
			     ELSE requirements_derived
			 END WHERE id = $1`, id, `[]`)
		if err != nil {
			t.Fatalf("simulated re-ingest: %v", err)
		}

		var stored []byte
		if err := pool.QueryRow(ctx, "SELECT requirements_derived FROM jobs WHERE id = $1", id).Scan(&stored); err != nil {
			t.Fatalf("read requirements_derived: %v", err)
		}
		var got []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(stored, &got); err != nil {
			t.Fatalf("decode %s: %v", stored, err)
		}
		if len(got) != 2 {
			t.Errorf("requirements_derived = %s, want the stored two kept", stored)
		}
	})

	t.Run("the column defaults to an empty array, never null", func(t *testing.T) {
		truncate(t, pool)
		var id int64
		err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', 'defaulted', 'http://example.test', 'A job', 'job-defaulted')
			 RETURNING id`).Scan(&id)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		var stored []byte
		if err := pool.QueryRow(ctx, "SELECT requirements_derived FROM jobs WHERE id = $1", id).Scan(&stored); err != nil {
			t.Fatalf("read requirements_derived: %v", err)
		}
		if string(stored) != "[]" {
			t.Errorf("requirements_derived = %s, want []", stored)
		}
	})
}

// The backfill's two queries: the chunk read must see only open postings, and the
// batched write must be idempotent — those two properties are what make the pass safe
// to stop, resume and re-run.
func TestRequirementsBackfillQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	closeJob := func(t *testing.T, id int64) {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", id); err != nil {
			t.Fatalf("close job: %v", err)
		}
	}

	t.Run("the chunk read skips closed postings", func(t *testing.T) {
		truncate(t, pool)
		open := insertJobWithDerivedRequirements(t, pool, "openrow", `[]`)
		closed := insertJobWithDerivedRequirements(t, pool, "closedrow", `[]`)
		closeJob(t, closed)

		rows, err := q.ListJobsForRequirementsBackfill(ctx, ListJobsForRequirementsBackfillParams{
			FromID: 0, ToID: closed + 1,
		})
		if err != nil {
			t.Fatalf("ListJobsForRequirementsBackfill: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != open {
			t.Errorf("rows = %+v, want only the open posting %d", rows, open)
		}
	})

	t.Run("the chunk read honours its id range", func(t *testing.T) {
		truncate(t, pool)
		first := insertJobWithDerivedRequirements(t, pool, "first", `[]`)
		second := insertJobWithDerivedRequirements(t, pool, "second", `[]`)

		rows, err := q.ListJobsForRequirementsBackfill(ctx, ListJobsForRequirementsBackfillParams{
			FromID: second, ToID: second + 1,
		})
		if err != nil {
			t.Fatalf("ListJobsForRequirementsBackfill: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != second {
			t.Errorf("rows = %+v, want only %d (not %d)", rows, second, first)
		}
	})

	t.Run("the batched write fills a row and a re-run writes nothing", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "tofill", `[]`)
		payload := []byte(derivedGo)

		n, err := q.SetJobsRequirementsDerived(ctx, SetJobsRequirementsDerivedParams{
			Ids: []int64{id}, Payloads: [][]byte{payload},
		})
		if err != nil {
			t.Fatalf("SetJobsRequirementsDerived: %v", err)
		}
		if n != 1 {
			t.Errorf("first write updated %d rows, want 1", n)
		}

		again, err := q.SetJobsRequirementsDerived(ctx, SetJobsRequirementsDerivedParams{
			Ids: []int64{id}, Payloads: [][]byte{payload},
		})
		if err != nil {
			t.Fatalf("SetJobsRequirementsDerived (re-run): %v", err)
		}
		if again != 0 {
			t.Errorf("re-run updated %d rows, want 0 — the IS DISTINCT FROM guard is what "+
				"makes stopping and resuming the pass free", again)
		}
	})

	t.Run("the batched write fills every row of a chunk", func(t *testing.T) {
		truncate(t, pool)
		a := insertJobWithDerivedRequirements(t, pool, "batcha", `[]`)
		b := insertJobWithDerivedRequirements(t, pool, "batchb", `[]`)

		n, err := q.SetJobsRequirementsDerived(ctx, SetJobsRequirementsDerivedParams{
			Ids:      []int64{a, b},
			Payloads: [][]byte{[]byte(derivedGo), []byte(`[{"text":"Rust","priority":"required"}]`)},
		})
		if err != nil {
			t.Fatalf("SetJobsRequirementsDerived: %v", err)
		}
		if n != 2 {
			t.Errorf("updated %d rows, want 2", n)
		}
	})
}
