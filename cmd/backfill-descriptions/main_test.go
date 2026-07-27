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

	scanned, updated, err := backfillAll(context.Background(), store, "")
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
	want1 := jobhash.Of(hashParams(store.jobs[0], u1.Description))
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

	scanned, updated, err := backfillAll(context.Background(), store, "taleo")
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

	scanned, updated, err := backfillAll(context.Background(), store, "arbeitnow")
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
	for _, want := range []string{"<h2>Role</h2>", "<li>Go</li>", `href="https://x.test/jobs"`} {
		if !strings.Contains(u.Description, want) {
			t.Errorf("job 1 missing decoded markup %q: %q", want, u.Description)
		}
	}
	if strings.Contains(u.Description, "&lt;") {
		t.Errorf("job 1 still entity-encoded: %q", u.Description)
	}
	want := jobhash.Of(hashParams(store.jobs[0], u.Description))
	if !u.ContentHash.Valid || u.ContentHash.String != want {
		t.Errorf("job 1 ContentHash = %+v, want %q", u.ContentHash, want)
	}
}

// TestBackfillStripsHimalayasSelfPromo covers the third way a stored body carries text the
// employer never wrote: Himalayas brands what it mirrors. Unlike the encoding repairs this one
// is provider-scoped — the markers are Himalayas' own links, and only its rows may be rewritten
// on sight of them. It is also the only repair that must fire on an otherwise well-formed body,
// so it cannot ride on the "did anything decode?" gate the encoding repairs share.
func TestBackfillStripsHimalayasSelfPromo(t *testing.T) {
	promo := `<p>Join <a href="https://himalayas.app/companies/x" rel="nofollow">X</a>.</p>` +
		`<p>Originally posted on <a href="https://himalayas.app" rel="nofollow">Himalayas</a></p>`
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "himalayas", Title: "A", Description: promo},
		// Same body under another provider: not ours to rewrite. A greenhouse posting that
		// genuinely links to himalayas.app is the employer's own text.
		{ID: 2, Source: "greenhouse", Title: "B", Description: promo},
		// A himalayas row already cleaned by an earlier run must not be rewritten again.
		{ID: 3, Source: "himalayas", Title: "C", Description: `<p>Join X.</p>`},
	}}

	scanned, updated, err := backfillAll(context.Background(), store, "")
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 3 || updated != 1 {
		t.Fatalf("scanned=%d updated=%d, want 3/1 (only the himalayas promo row)", scanned, updated)
	}
	if len(store.updates) != 1 || store.updates[0].ID != 1 {
		t.Fatalf("updates = %+v, want only himalayas job 1", store.updates)
	}
	if want := `<p>Join X.</p>`; store.updates[0].Description != want {
		t.Errorf("Description = %q, want %q", store.updates[0].Description, want)
	}
	// The row re-indexes only if content_hash moves with the text.
	want := jobhash.Of(hashParams(store.jobs[0], store.updates[0].Description))
	if !store.updates[0].ContentHash.Valid || store.updates[0].ContentHash.String != want {
		t.Errorf("ContentHash = %+v, want %q", store.updates[0].ContentHash, want)
	}
}

// TestBackfillRepairsEncodedHimalayasBody asserts the repairs compose: a himalayas row that is
// also entity-encoded is decoded, re-sanitized, and then stripped of the branding the decode
// just made visible.
func TestBackfillRepairsEncodedHimalayasBody(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{{ID: 1, Source: "himalayas", Title: "A",
		Description: `&lt;p&gt;Join us.&lt;/p&gt;&lt;p&gt;Originally posted on &lt;a href="https://himalayas.app"&gt;Himalayas&lt;/a&gt;&lt;/p&gt;`}}}

	if _, updated, err := backfillAll(context.Background(), store, "himalayas"); err != nil || updated != 1 {
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
