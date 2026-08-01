package main

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

// fakeStore serves keyset pages of jobs (across all sources) and records every description
// update. It pages by id so the backfill's keyset loop terminates.
type fakeStore struct {
	jobs    []db.Job
	updates []db.UpdateJobDescriptionParams
}

func (f *fakeStore) ListJobsByIDAfter(_ context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error) {
	return f.page(arg.AfterID, arg.BatchSize, ""), nil
}

func (f *fakeStore) ListJobsBySourceAfter(_ context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error) {
	return f.page(arg.AfterID, arg.BatchSize, arg.Source), nil
}

// page returns one keyset page after afterID, optionally filtered to a single source.
func (f *fakeStore) page(afterID int64, batch int32, source string) []db.Job {
	var out []db.Job
	for _, j := range f.jobs {
		if j.ID <= afterID || (source != "" && j.Source != source) {
			continue
		}
		out = append(out, j)
		if int32(len(out)) == batch {
			break
		}
	}
	return out
}

func (f *fakeStore) UpdateJobDescription(_ context.Context, arg db.UpdateJobDescriptionParams) (int64, error) {
	f.updates = append(f.updates, arg)
	return 1, nil
}

// TestBackfillDecodesOnlyEncodedDescriptions asserts the backfill rewrites only the rows whose
// stored description is still percent-encoded (marker "%3C"), re-decodes them in place, and
// leaves clean rows untouched — across sources, open and closed alike.
func TestBackfillDecodesOnlyEncodedDescriptions(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		// taleo, encoded HTML with a literal "%" that strict PathUnescape choked on.
		{ID: 1, Source: "taleo", Title: "A", Description: `%3Cp style=%22line-height%5C:115%;%22%3EWrite Go. 100% remote. C++%3C/p%3E`},
		// already clean — must be skipped.
		{ID: 2, Source: "greenhouse", Title: "B", Description: "<p>Clean HTML.</p>"},
		// a different source that somehow stored encoded HTML — the marker is source-agnostic.
		{ID: 3, Source: "icims", Title: "C", Description: `%3Cp%3EHello%3C%2Fp%3E`},
	}}

	scanned, updated, err := backfillAll(context.Background(), store, "", 0)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 3 || updated != 2 {
		t.Fatalf("scanned=%d updated=%d, want 3/2", scanned, updated)
	}

	if len(store.updates) != 2 {
		t.Fatalf("updates = %d, want 2 (jobs 1 and 3)", len(store.updates))
	}
	got := map[int64]db.UpdateJobDescriptionParams{}
	for _, u := range store.updates {
		got[u.ID] = u
	}

	u1, ok := got[1]
	if !ok {
		t.Fatal("job 1 not updated")
	}
	if strings.Contains(u1.Description, "%3C") || strings.Contains(u1.Description, "%22") {
		t.Errorf("job 1 still encoded: %q", u1.Description)
	}
	for _, want := range []string{"Write Go.", "100% remote", "C++"} {
		if !strings.Contains(u1.Description, want) {
			t.Errorf("job 1 missing %q: %q", want, u1.Description)
		}
	}
	// content_hash must fingerprint the row with the decoded description, matching what a
	// re-ingest of the fixed adapter would produce.
	want1 := jobhash.OfRow(store.jobs[0], u1.Description)
	if !u1.ContentHash.Valid || u1.ContentHash.String != want1 {
		t.Errorf("job 1 ContentHash = %+v, want %q", u1.ContentHash, want1)
	}

	if _, ok := got[2]; ok {
		t.Error("clean job 2 must not be updated")
	}
	if _, ok := got[3]; !ok {
		t.Error("encoded job 3 (icims) must be updated")
	}
}

// TestBackfillScopedToSource asserts a source-scoped run only touches that provider's encoded
// rows — the fast path for a repair known to be one provider's.
func TestBackfillScopedToSource(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "taleo", Title: "A", Description: `%3Cp%3Ehi%3C%2Fp%3E`},
		{ID: 2, Source: "icims", Title: "B", Description: `%3Cp%3Ehi%3C%2Fp%3E`},
	}}

	scanned, updated, err := backfillAll(context.Background(), store, "taleo", 0)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 1 {
		t.Fatalf("scanned=%d updated=%d, want 1/1 (only taleo scanned)", scanned, updated)
	}
	if len(store.updates) != 1 || store.updates[0].ID != 1 {
		t.Fatalf("updates = %+v, want only taleo job 1", store.updates)
	}
}

// TestBackfillDecodesEntityEncodedDescriptions covers the second way a source can store its
// markup as text: HTML entity-encoding ("&lt;p&gt;") rather than percent-encoding. arbeitnow
// served bodies this way, and its feed is a rolling window, so rows that aged out of it can
// never be repaired by a re-ingest — only in place.
func TestBackfillDecodesEntityEncodedDescriptions(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		// The arbeitnow shape: entity-encoded body plus the board's live-HTML promo footer.
		{ID: 1, Source: "arbeitnow", Title: "A", Description: `&lt;h2&gt;Role&lt;/h2&gt;&lt;ul&gt;&lt;li&gt;Go&lt;/li&gt;&lt;/ul&gt;<p>Find more <a href="https://x.test/jobs">jobs</a></p>`},
		// Prose that deliberately encodes a less-than sign: live tags dominate, so decoding
		// it would corrupt the row. Must be left alone.
		{ID: 2, Source: "arbeitnow", Title: "B", Description: `<p>Standort Düsseldorf</p><p>--&gt; Let´s go Live &lt;--</p><p>Wir suchen dich.</p>`},
		// Already clean.
		{ID: 3, Source: "arbeitnow", Title: "C", Description: "<p>Clean HTML.</p>"},
	}}

	scanned, updated, err := backfillAll(context.Background(), store, "arbeitnow", 0)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 3 || updated != 1 {
		t.Fatalf("scanned=%d updated=%d, want 3/1 (only the entity-encoded body)", scanned, updated)
	}
	if len(store.updates) != 1 || store.updates[0].ID != 1 {
		t.Fatalf("updates = %+v, want only job 1", store.updates)
	}

	u := store.updates[0]
	for _, want := range []string{"<h2>Role</h2>", "<li>Go</li>", "<p>Find more jobs</p>"} {
		if !strings.Contains(u.Description, want) {
			t.Errorf("job 1 missing decoded markup %q: %q", want, u.Description)
		}
	}
	if strings.Contains(u.Description, "&lt;") {
		t.Errorf("job 1 still entity-encoded: %q", u.Description)
	}
	want := jobhash.OfRow(store.jobs[0], u.Description)
	if !u.ContentHash.Valid || u.ContentHash.String != want {
		t.Errorf("job 1 ContentHash = %+v, want %q", u.ContentHash, want)
	}
}

// TestBackfillStripsHimalayasSelfPromo covers the third way a stored body carries text the
// employer never wrote: Himalayas brands what it mirrors with an "Originally posted on
// Himalayas" trailer. Unlike the other repairs this one is provider-scoped — the same trailer
// under another provider would be that employer's own words — and it must fire on an otherwise
// well-formed body, so it cannot ride on the "did anything decode?" gate.
func TestBackfillStripsHimalayasSelfPromo(t *testing.T) {
	promo := `<p>Join <a href="https://himalayas.app/companies/x" rel="nofollow">X</a>.</p>` +
		`<p>Originally posted on <a href="https://himalayas.app" rel="nofollow">Himalayas</a></p>`
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "himalayas", Title: "A", Description: promo},
		// Same body under another provider: its links go, like every other source's, but the
		// trailer is left standing — under greenhouse those are the employer's own words.
		{ID: 2, Source: "greenhouse", Title: "B", Description: promo},
		// A himalayas row already cleaned by an earlier run must not be rewritten again.
		{ID: 3, Source: "himalayas", Title: "C", Description: `<p>Join X.</p>`},
	}}

	scanned, updated, err := backfillAll(context.Background(), store, "", 0)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 3 || updated != 2 {
		t.Fatalf("scanned=%d updated=%d, want 3/2 (both linked rows, not the clean one)", scanned, updated)
	}
	got := map[int64]db.UpdateJobDescriptionParams{}
	for _, u := range store.updates {
		got[u.ID] = u
	}
	if _, rewritten := got[3]; rewritten {
		t.Error("the already-clean himalayas row must not be rewritten")
	}
	if want := `<p>Join X.</p>`; got[1].Description != want {
		t.Errorf("himalayas Description = %q, want %q", got[1].Description, want)
	}
	if want := `<p>Join X.</p><p>Originally posted on Himalayas</p>`; got[2].Description != want {
		t.Errorf("greenhouse Description = %q, want %q", got[2].Description, want)
	}
	// The row re-indexes only if content_hash moves with the text.
	want := jobhash.OfRow(store.jobs[0], got[1].Description)
	if !got[1].ContentHash.Valid || got[1].ContentHash.String != want {
		t.Errorf("ContentHash = %+v, want %q", got[1].ContentHash, want)
	}
}

// TestBackfillStripsLinksAcrossSources pins the universal repair: any stored description that
// still carries an anchor is re-sanitized, whatever its provider, and a body that never had one
// is left byte-for-byte alone.
func TestBackfillStripsLinksAcrossSources(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "greenhouse", Title: "A",
			Description: `<p>We use <a href="https://k8s.io" rel="nofollow">Kubernetes</a>.</p>`},
		{ID: 2, Source: "lever", Title: "B",
			Description: `<p>Apply: <a href="https://x.co/1" rel="nofollow">https://x.co/1</a></p>`},
		{ID: 3, Source: "workday", Title: "C", Description: `<p>No links here.</p>`},
	}}

	scanned, updated, err := backfillAll(context.Background(), store, "", 0)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 3 || updated != 2 {
		t.Fatalf("scanned=%d updated=%d, want 3/2 (the two linked rows)", scanned, updated)
	}
	got := map[int64]db.UpdateJobDescriptionParams{}
	for _, u := range store.updates {
		got[u.ID] = u
	}
	if want := `<p>We use Kubernetes.</p>`; got[1].Description != want {
		t.Errorf("job 1 Description = %q, want %q", got[1].Description, want)
	}
	// The self-labelled anchor goes with its text, and the block it emptied goes too.
	if want := `<p>Apply: </p>`; got[2].Description != want {
		t.Errorf("job 2 Description = %q, want %q", got[2].Description, want)
	}
	if _, rewritten := got[3]; rewritten {
		t.Error("the link-free row must not be rewritten")
	}
}

// TestBackfillIsIdempotent asserts a second sweep over rows the first one repaired writes
// nothing — the property that makes it safe to re-run an interrupted catalogue-wide pass.
func TestBackfillIsIdempotent(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "himalayas", Title: "A",
			Description: `<p>Join <a href="https://himalayas.app/companies/x">X</a>.</p><p>Originally posted on <a href="https://himalayas.app">Himalayas</a></p>`},
		{ID: 2, Source: "taleo", Title: "B", Description: `%3Cp%3EApply at %3Ca href=%22https://x.co%22%3Ehttps://x.co%3C/a%3E%3C/p%3E`},
	}}

	if _, updated, err := backfillAll(context.Background(), store, "", 0); err != nil || updated != 2 {
		t.Fatalf("first pass: updated=%d err=%v, want 2/nil", updated, err)
	}
	for i, u := range store.updates {
		store.jobs[i].Description = u.Description
	}
	store.updates = nil

	if _, updated, err := backfillAll(context.Background(), store, "", 0); err != nil || updated != 0 {
		t.Fatalf("second pass: updated=%d err=%v, want 0/nil\nrewrote: %+v", updated, err, store.updates)
	}
}

// TestBackfillRepairsEncodedHimalayasBody asserts the repairs compose: a himalayas row that is
// also entity-encoded is decoded, re-sanitized, and then stripped of the branding the decode
// just made visible.
func TestBackfillRepairsEncodedHimalayasBody(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{{ID: 1, Source: "himalayas", Title: "A",
		Description: `&lt;p&gt;Join us.&lt;/p&gt;&lt;p&gt;Originally posted on &lt;a href="https://himalayas.app"&gt;Himalayas&lt;/a&gt;&lt;/p&gt;`}}}

	if _, updated, err := backfillAll(context.Background(), store, "himalayas", 0); err != nil || updated != 1 {
		t.Fatalf("backfillAll: updated=%d err=%v, want 1/nil", updated, err)
	}
	got := store.updates[0].Description
	if strings.Contains(got, "himalayas.app") || strings.Contains(got, "Originally posted on") {
		t.Errorf("branding survived the decode: %q", got)
	}
	if !strings.Contains(got, "Join us.") {
		t.Errorf("body lost: %q", got)
	}
}
