//go:build integration

// Integration test for the hydrating-source seen-set query: ExistingExternalIDs returns the
// external_ids stored for one provider (and only that provider), so a hydrating adapter fetches
// per-posting detail only for postings the catalogue lacks. A SQL behavior, verifiable only
// against a real Postgres. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// farFuture is a hydration cutoff every row predates, which reduces the seen-set to its
// pre-hydration-retry behaviour: everything stored counts as seen.
func farFuture() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}
}

// bodyRefreshOff is how cmd/ingest expresses "do not re-read stale bodies": a slot no row can
// hash to, since the predicate computes abs(hashtext(external_id)) % slices and that is never
// negative. The divisor still has to be positive — a modulus of zero raises rather than
// disabling anything, which is deliberate: a caller that forgets these parameters should fail
// loudly rather than quietly withhold or quietly keep every row.
const bodyRefreshOffSlot = -1

func bodyRefreshOff() (pgtype.Timestamptz, int64, int64) {
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}, 1, bodyRefreshOffSlot
}

func TestExistingExternalIDsScopedToProvider(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Two greenhouse rows and one lever row.
	for _, ext := range []string{"acme:1", "acme:2"} {
		if _, err := ingestUpsert(ctx, q, ingestParams(ext, "Engineer")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	lever := ingestParams("other:9", "Designer")
	lever.Source = "lever"
	if _, err := ingestUpsert(ctx, q, lever); err != nil {
		t.Fatalf("upsert lever: %v", err)
	}

	offCutoff, offSlices, offSlot := bodyRefreshOff()
	rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:            "greenhouse",
		HydrationCutoff:   farFuture(),
		BodyRefreshCutoff: offCutoff,
		BodyRefreshSlices: offSlices,
		BodyRefreshSlot:   offSlot,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs: %v", err)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ExternalID
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"acme:1", "acme:2"}) {
		t.Errorf("ids = %v, want [acme:1 acme:2] (lever row excluded)", ids)
	}
}

// A multi-board provider loads the seen-set once per board, so the query must read one board's
// namespace rather than the provider's whole catalogue. Two hazards are covered: a sibling board
// whose name extends this one's, and an underscore in a board name — LIKE reads it as "any one
// character", which is why the caller passes an escaped pattern.
func TestExistingExternalIDsScopedToBoard(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	boards := map[string][]string{
		"dollartreeus": {"1", "2"}, // the board under test
		"dollartreeca": {"3"},      // a sibling whose name extends "dollartree"
		"Capital_One":  {"4"},      // the underscore hazard
		"CapitalXOne":  {"5"},      // matches "Capital_One:%" unless the underscore is escaped
	}
	for board, ids := range boards {
		for _, id := range ids {
			p := ingestParams(externalid.Namespace(board, id), "Engineer")
			p.Source = "workday"
			// The refresh path judges the catalogue filter on the row's stored evidence, so
			// the seen-set carries it: mark the first posting of each board technical.
			p.IsTech = pgtype.Bool{Bool: id == ids[0], Valid: true}
			if _, err := ingestUpsert(ctx, q, p); err != nil {
				t.Fatalf("upsert %s: %v", p.ExternalID, err)
			}
		}
	}

	for _, tc := range []struct {
		board    string
		want     []string
		wantTech string // the id whose row carries tech evidence
	}{
		{"dollartreeus", []string{"dollartreeus:1", "dollartreeus:2"}, "dollartreeus:1"},
		{"dollartreeca", []string{"dollartreeca:3"}, "dollartreeca:3"},
		{"Capital_One", []string{"Capital_One:4"}, "Capital_One:4"},
	} {
		offCutoff, offSlices, offSlot := bodyRefreshOff()
		rows, err := q.ExistingExternalIDsByBoard(ctx, ExistingExternalIDsByBoardParams{
			Source:            "workday",
			Pattern:           externalid.BoardPattern(tc.board),
			HydrationCutoff:   farFuture(),
			BodyRefreshCutoff: offCutoff,
			BodyRefreshSlices: offSlices,
			BodyRefreshSlot:   offSlot,
		})
		if err != nil {
			t.Fatalf("ExistingExternalIDsByBoard(%s): %v", tc.board, err)
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ExternalID
			if wantTech := r.ExternalID == tc.wantTech; r.IsTech.Bool != wantTech {
				t.Errorf("board %s: %s is_tech = %v, want %v", tc.board, r.ExternalID, r.IsTech.Bool, wantTech)
			}
		}
		slices.Sort(ids)
		if !slices.Equal(ids, tc.want) {
			t.Errorf("board %s: ids = %v, want %v", tc.board, ids, tc.want)
		}
	}
}

// A row stored without a description is half-ingested: its detail fetch failed, and because
// being stored is what marks a posting seen, nothing would ever retry it. It is withheld from
// the seen-set while it is younger than the cutoff (so the crawl hydrates it as if new) and
// counts as seen once it is older (so a source that publishes no body stops costing a detail
// request every crawl). freehire#1866.
func TestExistingExternalIDsWithholdsUnhydratedRowsUntilCutoff(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	withBody := ingestParams("acme:hydrated", "Engineer")
	empty := ingestParams("acme:empty", "Engineer")
	empty.Description = ""
	for _, p := range []UpsertJobParams{withBody, empty} {
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("upsert %s: %v", p.ExternalID, err)
		}
	}

	offCutoff, offSlices, offSlot := bodyRefreshOff()
	// Cutoff in the past: both rows are newer, so the body-less one is withheld for retry.
	rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:            "greenhouse",
		HydrationCutoff:   pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		BodyRefreshCutoff: offCutoff,
		BodyRefreshSlices: offSlices,
		BodyRefreshSlot:   offSlot,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs (fresh): %v", err)
	}
	ids := seenIDs(rows)
	if !slices.Equal(ids, []string{"acme:hydrated"}) {
		t.Errorf("within the window: ids = %v, want [acme:hydrated] (body-less row withheld)", ids)
	}

	// Cutoff in the future: the body-less row has outlived its retry window and is seen again.
	rows, err = q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:            "greenhouse",
		HydrationCutoff:   farFuture(),
		BodyRefreshCutoff: offCutoff,
		BodyRefreshSlices: offSlices,
		BodyRefreshSlot:   offSlot,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs (aged out): %v", err)
	}
	ids = seenIDs(rows)
	if !slices.Equal(ids, []string{"acme:empty", "acme:hydrated"}) {
		t.Errorf("past the window: ids = %v, want both (retry given up)", ids)
	}
}

// The board-scoped seen-set withholds a body-less row on the same rule as the provider-wide one.
func TestExistingExternalIDsByBoardWithholdsUnhydratedRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"acme:hydrated", "acme:empty"} {
		p := ingestParams(ext, "Engineer")
		p.Source = "workday"
		if ext == "acme:empty" {
			p.Description = ""
		}
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}

	offCutoff, offSlices, offSlot := bodyRefreshOff()
	rows, err := q.ExistingExternalIDsByBoard(ctx, ExistingExternalIDsByBoardParams{
		Source:            "workday",
		Pattern:           externalid.BoardPattern("acme"),
		HydrationCutoff:   pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		BodyRefreshCutoff: offCutoff,
		BodyRefreshSlices: offSlices,
		BodyRefreshSlot:   offSlot,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDsByBoard: %v", err)
	}
	if ids := seenBoardIDs(rows); !slices.Equal(ids, []string{"acme:hydrated"}) {
		t.Errorf("ids = %v, want [acme:hydrated] (body-less row withheld)", ids)
	}
}

// seenIDs collects a provider-wide seen-set result's external_ids, sorted for comparison.
func seenIDs(rows []ExistingExternalIDsRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ExternalID
	}
	slices.Sort(ids)
	return ids
}

// seenBoardIDs is seenIDs for the board-scoped result, which sqlc gives its own row type.
func seenBoardIDs(rows []ExistingExternalIDsByBoardRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ExternalID
	}
	slices.Sort(ids)
	return ids
}

// A stored body goes stale when the employer edits their posting after we captured it, and the
// crawl must re-read it — otherwise being stored is what stops a posting from ever being fetched
// again (freehire#2555: NVIDIA added "this position is 100% on-site" after our snapshot). The
// row is withheld only when BOTH arms agree: its body is older than the cutoff AND it falls in
// the run's slice.
func TestExistingExternalIDsWithholdsStaleBodiesInSlot(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"acme:stale", "acme:fresh"} {
		if _, err := ingestUpsert(ctx, q, ingestParams(ext, "Engineer")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	// UpsertJob stamps hydrated_at = now(), so age one row by hand rather than waiting.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET hydrated_at = now() - interval '90 days' WHERE external_id = 'acme:stale'`,
	); err != nil {
		t.Fatalf("age acme:stale: %v", err)
	}

	// One slice means every row is in the run's slot, so only the age arm decides.
	rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:            "greenhouse",
		HydrationCutoff:   farFuture(),
		BodyRefreshCutoff: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -45), Valid: true},
		BodyRefreshSlices: 1,
		BodyRefreshSlot:   0,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs (refresh on): %v", err)
	}
	if ids := seenIDs(rows); !slices.Equal(ids, []string{"acme:fresh"}) {
		t.Errorf("ids = %v, want [acme:fresh] (the stale body is withheld for a re-read)", ids)
	}

	// The same catalogue with the feature disabled: nothing is withheld, which is what every
	// deployment sees until an operator sets BODY_REFRESH_DAYS.
	offCutoff, offSlices, offSlot := bodyRefreshOff()
	rows, err = q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:            "greenhouse",
		HydrationCutoff:   farFuture(),
		BodyRefreshCutoff: offCutoff,
		BodyRefreshSlices: offSlices,
		BodyRefreshSlot:   offSlot,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs (refresh off): %v", err)
	}
	if ids := seenIDs(rows); !slices.Equal(ids, []string{"acme:fresh", "acme:stale"}) {
		t.Errorf("disabled: ids = %v, want both (no row is ever withheld)", ids)
	}
}

// A NULL hydrated_at is every row written before migration 0144. It reads as stale rather than
// as fresh, so the backlog is re-read instead of being invisible — and the slice is what keeps
// that backlog from arriving in one run.
func TestExistingExternalIDsTreatsNullHydratedAtAsStale(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	if _, err := ingestUpsert(ctx, q, ingestParams("acme:legacy", "Engineer")); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET hydrated_at = NULL`); err != nil {
		t.Fatalf("clear hydrated_at: %v", err)
	}

	rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source:            "greenhouse",
		HydrationCutoff:   farFuture(),
		BodyRefreshCutoff: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -45), Valid: true},
		BodyRefreshSlices: 1,
		BodyRefreshSlot:   0,
	})
	if err != nil {
		t.Fatalf("ExistingExternalIDs: %v", err)
	}
	if ids := seenIDs(rows); len(ids) != 0 {
		t.Errorf("ids = %v, want none (a never-hydrated row is stale, not fresh)", ids)
	}
}

// The slice is the cost bound: a run takes one slot, and a stale row outside it stays seen. The
// test asserts the partition rather than which slot a given id lands in — hashtext's values are
// not ours to predict — by walking every slot and requiring each row to be withheld by exactly
// one of them.
func TestExistingExternalIDsSlicesStaleRowsAcrossRuns(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const sliceCount = 4
	want := []string{"acme:1", "acme:2", "acme:3", "acme:4", "acme:5", "acme:6"}
	for _, ext := range want {
		if _, err := ingestUpsert(ctx, q, ingestParams(ext, "Engineer")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET hydrated_at = now() - interval '90 days'`); err != nil {
		t.Fatalf("age rows: %v", err)
	}

	withheldIn := map[string]int{}
	for slot := int64(0); slot < sliceCount; slot++ {
		rows, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
			Source:            "greenhouse",
			HydrationCutoff:   farFuture(),
			BodyRefreshCutoff: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -45), Valid: true},
			BodyRefreshSlices: sliceCount,
			BodyRefreshSlot:   slot,
		})
		if err != nil {
			t.Fatalf("ExistingExternalIDs(slot %d): %v", slot, err)
		}
		seen := map[string]bool{}
		for _, r := range rows {
			seen[r.ExternalID] = true
		}
		for _, ext := range want {
			if !seen[ext] {
				withheldIn[ext]++
			}
		}
	}
	for _, ext := range want {
		if withheldIn[ext] != 1 {
			t.Errorf("%s was withheld by %d of %d slots, want exactly 1", ext, withheldIn[ext], sliceCount)
		}
	}
}
