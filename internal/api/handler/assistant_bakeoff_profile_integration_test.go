//go:build integration

// Seeding the bake-off's candidate: the CV document every run starts from and the
// experience bank every edit has to cite. Behind the integration tag alone, like the case
// seed beside it, so the half that costs no tokens is checked by the ordinary tagged suite.
//
//	go test -tags=integration ./internal/api/handler/ -run BakeoffProfile
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/db"
)

// bakeoffBankEntry is one employment of the fixture with the achievements filed under it,
// in the shape /me/experience returns.
//
// It is decoded in two passes rather than by embedding, because experience.Employment
// implements UnmarshalJSON: Go hands an embedded Unmarshaler the WHOLE object, so a struct
// that embedded it would decode the place correctly and silently drop every atom. That is
// the same trap employmentWithAtoms.MarshalJSON documents from the writing side, and it
// fails quietly in exactly the way a seed must not.
type bakeoffBankEntry struct {
	Employment experience.Employment
	Atoms      []experience.Atom
}

// decodeBakeoffBankEntry reads one employment-with-atoms from the fixture.
func decodeBakeoffBankEntry(raw json.RawMessage) (bakeoffBankEntry, error) {
	var entry bakeoffBankEntry
	if err := json.Unmarshal(raw, &entry.Employment); err != nil {
		return bakeoffBankEntry{}, fmt.Errorf("employment: %w", err)
	}
	var atoms struct {
		Atoms []experience.Atom `json:"atoms"`
	}
	if err := json.Unmarshal(raw, &atoms); err != nil {
		return bakeoffBankEntry{}, fmt.Errorf("atoms of %q: %w", entry.Employment.Company, err)
	}
	entry.Atoms = atoms.Atoms
	return entry, nil
}

// seedBakeoffProfile writes the fixture's experience bank for one account and returns how
// many achievements it banked.
//
// The rows go in with raw SQL rather than through experience.Store, and that is deliberate.
// Store DERIVES an atom's provenance from who is asserting it and ignores the value it is
// handed — the wall the CV evidence gate stands on. There is no Author that produces
// `cv_import`, so every route through the store would relabel most of this bank, and a
// relabelled bank is not this candidate's bank: the gate would accept claims it refuses in
// production, or refuse claims it accepts, and the bake-off would be measuring the gate.
// The fixture is a recording of the store's own output, so replaying it verbatim is the
// faithful seed and the store's rules are not being circumvented, only replayed.
func seedBakeoffProfile(t *testing.T, pool *pgxpool.Pool, userID int64, p bakeoffProfile) int {
	t.Helper()

	var banked int
	for _, raw := range p.Experience.Employments {
		entry, err := decodeBakeoffBankEntry(raw)
		if err != nil {
			t.Fatalf("bake-off profile: %v", err)
		}
		id := insertBakeoffEmployment(t, pool, userID, entry.Employment)
		for _, a := range entry.Atoms {
			a.EmploymentID = &id
			insertBakeoffAtom(t, pool, userID, a)
			banked++
		}
	}
	// The unplaced achievements are banked too. They are usually the ones volunteered in
	// conversation, so dropping them would quietly remove the evidence a tailoring run is
	// most likely to reach for.
	for _, raw := range p.Experience.Unplaced {
		var a experience.Atom
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatalf("bake-off profile: unplaced atom: %v", err)
		}
		a.EmploymentID = nil
		insertBakeoffAtom(t, pool, userID, a)
		banked++
	}
	return banked
}

// insertBakeoffEmployment writes one place and returns the id its atoms must point at. The
// fixture's own id is NOT reused: it belongs to the production row, and a seed that carried
// it would make a report's evidence ids look like production's.
func insertBakeoffEmployment(t *testing.T, pool *pgxpool.Pool, userID int64, e experience.Employment) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO experience_employments
		   (user_id, kind, company, role, location,
		    period_start_year, period_start_month, period_end_year, period_end_month,
		    is_current, summary, stack, link)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id`,
		userID, e.Kind, e.Company, e.Role, e.Location,
		periodPart(e.Start, func(p perioddate.PeriodDate) int { return p.Year }),
		periodPart(e.Start, func(p perioddate.PeriodDate) int { return p.Month }),
		periodPart(e.End, func(p perioddate.PeriodDate) int { return p.Year }),
		periodPart(e.End, func(p perioddate.PeriodDate) int { return p.Month }),
		e.Current, e.Summary, textArray(e.Stack), e.Link,
	).Scan(&id); err != nil {
		t.Fatalf("seed employment %q: %v", e.Company, err)
	}
	return id
}

// insertBakeoffAtom writes one achievement with the standing the fixture recorded for it.
func insertBakeoffAtom(t *testing.T, pool *pgxpool.Pool, userID int64, a experience.Atom) {
	t.Helper()
	a.Sanitize()
	if err := a.Validate(); err != nil {
		t.Fatalf("seed atom %q: %v", a.Claim, err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO experience_atoms
		   (user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id, claim_key) DO NOTHING`,
		userID, a.EmploymentID, a.Claim, experience.ClaimKey(a.Claim),
		a.Context, textArray(a.Metrics), textArray(a.Skills), string(a.Provenance), a.SourceRef,
	); err != nil {
		t.Fatalf("seed atom %q: %v", a.Claim, err)
	}
}

// textArray sends an absent list as the empty array the column holds, never as NULL: these
// are NOT NULL text[] columns defaulting to '{}', and a nil Go slice encodes as NULL, so a
// place with no stack line would be refused by the constraint rather than stored blank.
func textArray(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// periodPart reads one half of a period date, as the nullable column the schema holds. A
// nil period is a real absence — "Present", or a date the source never stated — and must
// stay NULL rather than becoming year zero.
func periodPart(p *perioddate.PeriodDate, read func(perioddate.PeriodDate) int) *int {
	if p == nil {
		return nil
	}
	v := read(*p)
	if v == 0 {
		return nil
	}
	return &v
}

// loadBakeoffProfileOrSkip reads the local fixture, skipping the test when it has not been
// built. Its absence is not a failure: the file is a real CV and is deliberately not in
// this repository, so CI has no business failing over one it cannot have.
func loadBakeoffProfileOrSkip(t *testing.T) bakeoffProfile {
	t.Helper()
	p, err := loadBakeoffProfile(bakeoffProfileFixture)
	if errors.Is(err, errMissingBakeoffProfile) {
		t.Skipf("%v", err)
	}
	if err != nil {
		t.Fatalf("loadBakeoffProfile: %v", err)
	}
	return p
}

// The bank is read back through the store the tools themselves use, and the provenance is
// checked rather than assumed: every candidate-asserted claim must come back
// candidate-asserted, because the CV evidence gate reads exactly this column. A seed that
// relabelled the bank would measure how well each model works around a gate that was never
// this candidate's.
func TestBakeoffProfileSeedsTheBankWithItsStandingIntact(t *testing.T) {
	pool := startPostgres(t)
	p := loadBakeoffProfileOrSkip(t)
	userID := seedAccount(t, pool, "bakeoff-profile@example.test", true)

	banked := seedBakeoffProfile(t, pool, userID, p)
	if banked == 0 {
		t.Fatal("the fixture banked no achievement; every edit would bounce off the evidence gate")
	}

	bank := experience.NewStore(experience.NewQueriesRepository(db.New(pool)))
	atoms, err := bank.ListAtoms(context.Background(), userID)
	if err != nil {
		t.Fatalf("list atoms: %v", err)
	}
	if len(atoms) == 0 {
		t.Fatal("nothing came back from the bank")
	}

	want := map[experience.Provenance]int{}
	for _, raw := range p.Experience.Employments {
		entry, err := decodeBakeoffBankEntry(raw)
		if err != nil {
			t.Fatalf("re-read fixture: %v", err)
		}
		for _, a := range entry.Atoms {
			want[a.Provenance]++
		}
	}
	for _, raw := range p.Experience.Unplaced {
		var a experience.Atom
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatalf("re-read unplaced: %v", err)
		}
		want[a.Provenance]++
	}

	got := map[experience.Provenance]int{}
	for _, a := range atoms {
		got[a.Provenance]++
	}
	// Compared per label, not in total: a seed that banked every claim but relabelled them
	// would agree on the count and disagree on the only thing the gate reads.
	for label, n := range want {
		if got[label] != n {
			t.Errorf("provenance %q: banked %d, the fixture holds %d", label, got[label], n)
		}
	}

	// Every placed achievement must still be filed under a place, or it reaches the run as
	// unplaced and loses the role it was written about.
	employments, err := bank.ListEmployments(context.Background(), userID)
	if err != nil {
		t.Fatalf("list employments: %v", err)
	}
	if len(employments) != len(p.Experience.Employments) {
		t.Errorf("employments = %d, the fixture holds %d", len(employments), len(p.Experience.Employments))
	}
}
