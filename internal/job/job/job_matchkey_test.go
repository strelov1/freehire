package job_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites is the soundness condition of
// the cheap ingest write path (see the cut-ingest-write-amplification change): when a re-seen
// posting matches the stored row on RefreshUnchangedJob's key, ingest issues a narrow
// last_seen_at refresh instead of the full upsert, so every OTHER column keeps whatever the
// row already holds. That is correct only while the key cannot stand still through a real
// difference — a column that moves without moving the key is one the cheap path would
// silently leave stale.
//
// The key is content_hash plus the columns UpsertJob writes that jobhash.Of does not read, and
// this test is the authority on why it needs them. cities is one: a caller's structured city
// list overrides the location-derived one (jobderive.Derive), so it can move while every hashed
// field stands still. english_level is another, and became one the day it gained a structured
// tier of its own — until then it was read out of the hashed description and so could not move
// on its own. Note that education_level sits on the OTHER side of the same choice: it is hashed,
// because it was hashed before either had a structured tier. Folding cities into Of instead would change every
// stored hash at once and make the first crawl after deploy rewrite and re-index the whole
// catalogue — the write storm this change exists to remove. So the predicate carries it.
//
// Adding another derived column Of does not read fails here, and the fix is the same choice:
// hash it, or widen the key and say so.
//
// The walk is over jobderive.Input, the write path's actual entry point, and compares what
// UpsertParams produces — the production composition (derivation, mapping, and withDerived's
// stamp together) rather than a reconstruction of it. The Draft fields outside Input (URL,
// Remote, PostedAt) reach params without derivation and are all read by Of, which
// TestOf_ChangesWhenAnyIndexedFieldChanges already guards.
func TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites(t *testing.T) {
	base := fullDraft()
	baseParams := mustParams(t, base)

	typ := reflect.TypeOf(base.Input)
	for i := range typ.NumField() {
		field := typ.Field(i)
		mutated, ok := mutateInput(base, i)
		if !ok {
			t.Fatalf("no mutation for Input.%s of type %s: extend mutateInput — an unmutated "+
				"field is an unchecked one that reads as checked", field.Name, field.Type)
		}

		t.Run(field.Name, func(t *testing.T) {
			got := mustParams(t, mutated)
			moved := movedColumns(baseParams, got)
			if len(moved) == 0 {
				return // the field reaches no column; nothing for the key to cover
			}
			if matchKeyHeld(baseParams, got) {
				t.Errorf("Input.%s moved %v but left RefreshUnchangedJob's match key "+
					"(content_hash, cities, salary_*_source, english_level, is_tech, engagement) unchanged: an "+
					"unchanged re-ingest would skip the row and leave those columns stale",
					field.Name, moved)
			}
		})
	}
}

// A source's structured engagement signal (a Brazilian board's CLT/PJ picklist) moves the
// engagement column while every hashed field stands still — the generic walk above cannot
// show it, because its slice mutation appends a value outside the vocabulary, which Derive
// drops. The cheap write's key must therefore see the column, or a re-crawl would keep the
// stored terms forever.
func TestUpsertParams_EngagementSignalMovesTheKeyNotTheHash(t *testing.T) {
	without, with := fullDraft(), fullDraft()
	with.Engagement = []string{"pj"}
	a, b := mustParams(t, without), mustParams(t, with)
	if a.ContentHash != b.ContentHash {
		t.Fatalf("content_hash moved; the test needs a signal the hash does not read")
	}
	if slices.Equal(a.Engagement, b.Engagement) {
		t.Fatalf("engagement did not move: %v", b.Engagement)
	}
	if matchKeyHeld(a, b) {
		t.Errorf("the match key held while engagement moved %v -> %v", a.Engagement, b.Engagement)
	}
}

// TestUpsertParams_IsTechHintMovesIsTechIndependentlyOfContentHash is the focused
// counterpart TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites's generic
// per-field walk cannot exercise for IsTechHint: fullDraft's Category ("backend") and
// Title ("Senior Go Developer") already resolve as technical on their own, so toggling IsTechHint
// there moves no column and the generic subtest returns before ever reaching
// matchKeyHeld. This test uses a title and category that resolve neither tech nor
// non-tech on their own (mirroring issue #2601's Profession postings), so IsTechHint is
// the only source of is_tech evidence — proving directly that it moves is_tech without
// moving content_hash, and that matchKeyHeld correctly reports the row as changed.
func TestUpsertParams_IsTechHintMovesIsTechIndependentlyOfContentHash(t *testing.T) {
	draft := func(hint bool) job.Draft {
		return job.Draft{Input: jobderive.Input{
			Source: "profession", ExternalID: "itdev:1",
			Title: "Windows rendszermérnök", Company: "Acme",
			IsTechHint: hint,
		}}
	}

	unhinted := mustParams(t, draft(false))
	hinted := mustParams(t, draft(true))

	if unhinted.IsTech.Valid {
		t.Fatalf("unhinted IsTech = %+v, want unknown — the fixture's title/category must resolve neither tech nor non-tech on their own", unhinted.IsTech)
	}
	if !hinted.IsTech.Valid || !hinted.IsTech.Bool {
		t.Fatalf("hinted IsTech = %+v, want true", hinted.IsTech)
	}
	if unhinted.ContentHash != hinted.ContentHash {
		t.Fatalf("ContentHash moved with IsTechHint alone: %q vs %q — the fixture must hold every hashed field constant", unhinted.ContentHash.String, hinted.ContentHash.String)
	}
	if matchKeyHeld(unhinted, hinted) {
		t.Error("matchKeyHeld held across an IsTechHint-driven is_tech change with an unchanged content_hash — RefreshUnchangedJob would wrongly skip this row")
	}
}

// matchKeyHeld reports whether two params agree on everything RefreshUnchangedJob matches on,
// i.e. whether the cheap path would treat b as an unchanged re-ingest of a.
func matchKeyHeld(a, b db.UpsertJobParams) bool {
	return a.ContentHash == b.ContentHash &&
		slices.Equal(a.Cities, b.Cities) &&
		a.SalaryMinSource == b.SalaryMinSource &&
		a.SalaryMaxSource == b.SalaryMaxSource &&
		a.SalaryCurrencySource == b.SalaryCurrencySource &&
		a.SalaryPeriodSource == b.SalaryPeriodSource &&
		a.EnglishLevel == b.EnglishLevel &&
		a.IsTech == b.IsTech &&
		slices.Equal(a.Engagement, b.Engagement)
}

// fullDraft populates every jobderive.Input field, including the optional structured signals,
// so a field is never exercised only as zero -> non-zero.
func fullDraft() job.Draft {
	posted := time.Unix(1_700_000_000, 0).UTC()
	years := 5
	return job.Draft{
		Input: jobderive.Input{
			Source:             "greenhouse",
			ExternalID:         "acme:1",
			Title:              "Senior Go Developer",
			Company:            "Acme",
			Location:           "Berlin, Germany",
			Description:        "We use Golang and PostgreSQL.",
			WorkMode:           "remote",
			Regions:            []string{"eu"},
			Cities:             []string{"Berlin"},
			Seniority:          "senior",
			Category:           "backend",
			EmploymentType:     "full_time",
			Skills:             []string{"go"},
			ExperienceYearsMin: &years,
			IsTechHint:         true,
		},
		URL:      "https://acme.example/jobs/1",
		Remote:   true,
		PostedAt: &posted,
	}
}

func mustParams(t *testing.T, d job.Draft) db.UpsertJobParams {
	t.Helper()
	j, err := job.New(d)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return j.Fields().UpsertParams()
}

// mutateInput returns a copy of d with exactly field i of its embedded Input changed to a
// value distinct from the one it held, reporting false for a type it cannot change.
func mutateInput(d job.Draft, i int) (job.Draft, bool) {
	f := reflect.ValueOf(&d.Input).Elem().Field(i)
	switch v := f.Interface().(type) {
	case string:
		f.SetString(v + " mutated")
	case []string:
		f.Set(reflect.ValueOf(append(slices.Clone(v), "mutated")))
	case *int:
		n := 1
		if v != nil {
			n = *v + 1
		}
		f.Set(reflect.ValueOf(&n))
	case bool:
		f.SetBool(!v)
	default:
		return d, false
	}
	return d, true
}

// movedColumns names the UpsertJobParams columns that differ between two params structs,
// content_hash and role_fingerprint excluded — they are signals about the row's content, not
// columns whose staleness a reader would ever see. cities is NOT excluded: it is half the
// match key and also a column in its own right, so a move must still be reported.
func movedColumns(a, b db.UpsertJobParams) []string {
	var moved []string
	typ := reflect.TypeOf(a)
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		if name == "ContentHash" || name == "RoleFingerprint" {
			continue
		}
		if !reflect.DeepEqual(va.Field(i).Interface(), vb.Field(i).Interface()) {
			moved = append(moved, name)
		}
	}
	return moved
}
