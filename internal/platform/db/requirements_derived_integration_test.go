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

func TestSetJobEnrichment_LeavesTheDerivedRequirementsAlone(t *testing.T) {
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
			t.Errorf("summary = %q, want the payload's", got.Summary)
		}
	})

	// The load-bearing property, and the reason there is no overlay here. Copying the
	// derived list into the blob would make it a SECOND stored value that nothing
	// revises: a later crawl rewrites the column and leaves the copy, so a description
	// edit deleting the requirements section would leave the page quoting a posting
	// that no longer says it, and the backfill could not reach it. The fold lives on
	// the read path instead (jobview.FromDomain), where it re-reads the column every
	// time.
	t.Run("a payload stating none does NOT materialise the derived list", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithDerivedRequirements(t, pool, "nomaterialise", derivedGo)
		set(t, id, `{"summary":"a synopsis"}`)

		got := readRequirements(t, pool, id)
		if len(got.Requirements) != 0 {
			t.Errorf("enrichment.requirements = %v, want none stored — the column is the "+
				"single source and the projection folds it in at read time", texts(got))
		}
		if got.Summary != "a synopsis" {
			t.Errorf("summary = %q, want the payload's", got.Summary)
		}

		// …and the column itself is untouched, so the projection still has it to fold.
		var stored []byte
		if err := pool.QueryRow(ctx, "SELECT requirements_derived FROM jobs WHERE id = $1", id).Scan(&stored); err != nil {
			t.Fatalf("read requirements_derived: %v", err)
		}
		if string(stored) == "[]" {
			t.Error("requirements_derived was cleared by the enrichment write")
		}
	})

	// The enrichment payload is untrusted JSON. None of these shapes can arrive
	// through the typed contract today, but a write that raises would dead-letter the
	// job permanently, so the statement must simply store what it was given.
	t.Run("an odd requirements value in the payload does not raise", func(t *testing.T) {
		for name, payload := range map[string]string{
			"json null": `{"requirements":null}`,
			"an object": `{"requirements":{"text":"Go"}}`,
			"a string":  `{"requirements":"Go"}`,
			"a number":  `{"requirements":3}`,
		} {
			t.Run(name, func(t *testing.T) {
				truncate(t, pool)
				id := insertJobWithDerivedRequirements(t, pool, "oddshape", derivedGo)
				set(t, id, payload) // fails the test if it errors
			})
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

	q := New(pool)

	// These call the real UpsertJob. An earlier version of this test executed a
	// hand-written UPDATE restating the conflict branch's own CASE, which meant
	// deleting that branch from the query left the test green.
	upsert := func(t *testing.T, externalID, description, derived string) {
		t.Helper()
		_, err := q.UpsertJob(ctx, UpsertJobParams{
			Source:              "test",
			ExternalID:          externalID,
			URL:                 "http://example.test",
			Title:               "A job",
			Company:             "Acme",
			CompanySlug:         "acme",
			PublicSlug:          "job-" + externalID,
			Description:         description,
			RequirementsDerived: []byte(derived),
		})
		if err != nil {
			t.Fatalf("UpsertJob: %v", err)
		}
	}

	storedFor := func(t *testing.T, externalID string) string {
		t.Helper()
		var stored []byte
		err := pool.QueryRow(ctx,
			"SELECT requirements_derived FROM jobs WHERE source = 'test' AND external_id = $1",
			externalID).Scan(&stored)
		if err != nil {
			t.Fatalf("read requirements_derived: %v", err)
		}
		return string(stored)
	}

	t.Run("the first write stores what the description derived", func(t *testing.T) {
		truncate(t, pool)
		upsert(t, "first", "<h3>Requirements</h3><ul><li>Go</li></ul>", derivedGo)

		if got := storedFor(t, "first"); got == "[]" {
			t.Errorf("requirements_derived = %s, want the derived list", got)
		}
	})

	// A failed detail fetch upserts the job with an empty description, which derives
	// an empty list. Writing that would wipe a good one.
	t.Run("a re-ingest with an empty description keeps the stored list", func(t *testing.T) {
		truncate(t, pool)
		upsert(t, "keepstored", "<h3>Requirements</h3><ul><li>Go</li></ul>", derivedGo)
		upsert(t, "keepstored", "", `[]`)

		var got []struct {
			Text string `json:"text"`
		}
		stored := storedFor(t, "keepstored")
		if err := json.Unmarshal([]byte(stored), &got); err != nil {
			t.Fatalf("decode %s: %v", stored, err)
		}
		if len(got) != 2 {
			t.Errorf("requirements_derived = %s, want the stored two kept", stored)
		}
	})

	// The other side of the same guard, and the reason it keys on the description
	// rather than on the derivation: an edit that REMOVED the requirements section
	// must clear the list, or the page quotes a posting that no longer says it.
	t.Run("a re-ingest whose description dropped the list clears it", func(t *testing.T) {
		truncate(t, pool)
		upsert(t, "cleared", "<h3>Requirements</h3><ul><li>Go</li></ul>", derivedGo)
		upsert(t, "cleared", "<p>We would rather talk it through than screen on a checklist.</p>", `[]`)

		if got := storedFor(t, "cleared"); got != "[]" {
			t.Errorf("requirements_derived = %s, want [] — the posting no longer states a list", got)
		}
	})

	t.Run("a re-ingest with a new list replaces the stored one", func(t *testing.T) {
		truncate(t, pool)
		upsert(t, "replaced", "<h3>Requirements</h3><ul><li>Go</li></ul>", derivedGo)
		upsert(t, "replaced", "<h3>Requirements</h3><ul><li>Rust</li></ul>",
			`[{"text":"Rust","priority":"required"}]`)

		if got := storedFor(t, "replaced"); got == derivedGo {
			t.Errorf("requirements_derived = %s, want the new list", got)
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
