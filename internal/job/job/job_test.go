package job_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobhash"
)

// New is the single construction door: it runs the deterministic derivation
// internally, so a constructed Job always carries facets consistent with its
// source fields. A caller never touches the facet fields.
func TestNew_DerivesFacetsFromDraft(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang and PostgreSQL.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	f := j.Fields()

	// Identity is preserved verbatim.
	if f.Source != "manual" || f.ExternalID != "https://acme.example/jobs/1" {
		t.Errorf("identity = %q/%q", f.Source, f.ExternalID)
	}
	// Slugs are minted deterministically from the identity.
	wantSlug := normalize.JobSlug("Senior Go Developer", "Acme", "manual", "https://acme.example/jobs/1")
	if f.PublicSlug != wantSlug {
		t.Errorf("PublicSlug = %q, want %q", f.PublicSlug, wantSlug)
	}
	if f.CompanySlug != normalize.Slug("Acme") {
		t.Errorf("CompanySlug = %q, want %q", f.CompanySlug, normalize.Slug("Acme"))
	}
	// Facets are derived from the dictionaries — the caller supplied none.
	if len(f.Countries) == 0 || f.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de ...]", f.Countries)
	}
	if !reflect.DeepEqual(f.Skills, []string{"go", "postgresql"}) {
		t.Errorf("Skills = %v, want [go postgresql]", f.Skills)
	}
	if f.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", f.WorkMode)
	}
}

// A freshly constructed Job is open and unenriched: no lifecycle or enrichment
// state until the write/enrich paths set it.
// A form label inside the location ("Locations: San Francisco") is stripped before the
// facet derivation reads it, so the posting gets its geography and displays the place.
func TestNew_StripsALocationLabelBeforeDeriving(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: "greenhouse", ExternalID: "1", Title: "Engineer", Company: "Acme",
		Location: "Locations: San Francisco",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()
	if f.Location != "San Francisco" {
		t.Errorf("Location = %q, want %q", f.Location, "San Francisco")
	}
	if !slices.Contains(f.Countries, "us") {
		t.Errorf("Countries = %v, want us", f.Countries)
	}
}

func TestNew_FreshJobIsOpenAndUnenriched(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{Source: "manual", ExternalID: "1", Title: "Engineer", Company: "Acme"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !j.IsOpen() {
		t.Error("fresh job should be open")
	}
	if f := j.Fields(); f.EnrichmentVersion != 0 {
		t.Errorf("fresh job EnrichmentVersion = %d, want 0", f.EnrichmentVersion)
	}
}

// Facets depend on content, never on which write path constructed the job: a
// Telegram-extracted posting and a board-ingested posting with the same title,
// description, and location resolve identical dictionary facets (only the slugs,
// minted from identity, differ). This is the deterministic-facets guarantee that a
// single construction door delivers — the tg-extract inline-derive divergence is
// now unrepresentable.
func TestNew_FacetsIndependentOfWritePath(t *testing.T) {
	content := job.Draft{Input: jobderive.Input{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang and Kubernetes.",
	}}
	tg := content
	tg.Source, tg.ExternalID = "telegram", "chan/1/0"
	board := content
	board.Source, board.ExternalID = "greenhouse", "acme:42"

	tj, err := job.New(tg)
	if err != nil {
		t.Fatalf("New(tg): %v", err)
	}
	bj, err := job.New(board)
	if err != nil {
		t.Fatalf("New(board): %v", err)
	}
	tf, bf := tj.Fields(), bj.Fields()

	if !reflect.DeepEqual(tf.Countries, bf.Countries) || !reflect.DeepEqual(tf.Regions, bf.Regions) ||
		!reflect.DeepEqual(tf.Cities, bf.Cities) || tf.WorkMode != bf.WorkMode ||
		!reflect.DeepEqual(tf.Skills, bf.Skills) || tf.Seniority != bf.Seniority || tf.Category != bf.Category {
		t.Errorf("facets diverged between write paths:\n tg    = %+v\n board = %+v", tf, bf)
	}
	// Slugs are minted from identity, so they legitimately differ.
	if tf.PublicSlug == bf.PublicSlug {
		t.Errorf("public slugs should differ by identity, both = %q", tf.PublicSlug)
	}
}

// The factory rejects an identity-less draft: source and external id together are
// the dedup key, and a title-less posting is not a job.
func TestNew_RejectsMissingIdentity(t *testing.T) {
	cases := map[string]job.Draft{
		"no source":      {Input: jobderive.Input{ExternalID: "1", Title: "Engineer"}},
		"no external id": {Input: jobderive.Input{Source: "manual", Title: "Engineer"}},
		"no title":       {Input: jobderive.Input{Source: "manual", ExternalID: "1"}},
	}
	for name, d := range cases {
		if _, err := job.New(d); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

// An explicit region/city on the draft is authoritative: it overrides what the location
// dictionary would derive, while an unsupplied facet still derives (see jobderive).
func TestNew_ExplicitRegionCityOverrideDerivation(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang.",
		Regions:     []string{"north_america"},
		Cities:      []string{"Austin"},
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()
	if !reflect.DeepEqual(f.Regions, []string{"north_america"}) {
		t.Errorf("Regions = %v, want [north_america] (explicit wins)", f.Regions)
	}
	if !reflect.DeepEqual(f.Cities, []string{"Austin"}) {
		t.Errorf("Cities = %v, want [Austin] (explicit wins)", f.Cities)
	}
}

// A manual salary supplied on the draft is carried verbatim onto the Job as a base
// field — it is authoritative, never derived — and is absent by default.
func TestNew_CarriesManualSalary(t *testing.T) {
	min, max := 90000, 120000
	j, err := job.New(job.Draft{
		Input:        jobderive.Input{Source: "manual", ExternalID: "https://acme.example/jobs/1", Title: "Senior Go Developer", Company: "Acme"},
		ManualSalary: &job.Salary{Min: &min, Max: &max, Currency: "EUR", Period: "year"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()
	if f.ManualSalary == nil {
		t.Fatal("ManualSalary = nil, want a value")
	}
	if f.ManualSalary.Min == nil || *f.ManualSalary.Min != 90000 || f.ManualSalary.Max == nil || *f.ManualSalary.Max != 120000 {
		t.Errorf("ManualSalary range = %v/%v, want 90000/120000", f.ManualSalary.Min, f.ManualSalary.Max)
	}
	if f.ManualSalary.Currency != "EUR" || f.ManualSalary.Period != "year" {
		t.Errorf("ManualSalary currency/period = %q/%q, want EUR/year", f.ManualSalary.Currency, f.ManualSalary.Period)
	}
}

func TestNew_NoManualSalaryByDefault(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{Source: "manual", ExternalID: "u", Title: "Go Dev", Company: "Acme"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if j.Fields().ManualSalary != nil {
		t.Errorf("ManualSalary = %v, want nil (none supplied)", j.Fields().ManualSalary)
	}
}

// The write mapping owns every derived column a persisted posting carries. A write
// path cannot omit the content fingerprint or the role fingerprint by forgetting a
// step, because there is no step to forget.
func TestUpsertParams_FillsDerivedColumns(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Description: "We use Golang.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	params := j.Fields().UpsertParams()

	if !params.ContentHash.Valid || params.ContentHash.String == "" {
		t.Errorf("ContentHash = %v, want a fingerprint", params.ContentHash)
	}
	if !params.RoleFingerprint.Valid || params.RoleFingerprint.String == "" {
		t.Errorf("RoleFingerprint = %v, want a fingerprint", params.RoleFingerprint)
	}
	// A description with no requirements list still fills the column, because the
	// column is NOT NULL and "found nothing" is the answer, not the absence of one.
	if string(params.RequirementsDerived) != "[]" {
		t.Errorf("RequirementsDerived = %s, want []", params.RequirementsDerived)
	}
}

// The derived requirements ride the same shared mapping the fingerprints do, so no
// write path can persist a posting whose description states a requirements list
// without the list.
func TestUpsertParams_DerivesTheRequirementsAPostingLists(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:     "manual",
		ExternalID: "https://acme.example/jobs/2",
		Title:      "Senior Go Developer",
		Company:    "Acme",
		Description: `<p>Join us.</p>
		              <h3>Requirements</h3><ul><li>5+ years of Go</li><li>Postgres</li></ul>
		              <h3>Nice to have</h3><ul><li>Kubernetes</li></ul>
		              <h3>What we offer</h3><ul><li>Free lunch</li></ul>`,
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var got []enrich.Requirement
	if err := json.Unmarshal(j.Fields().UpsertParams().RequirementsDerived, &got); err != nil {
		t.Fatalf("RequirementsDerived is not a requirements list: %v", err)
	}

	want := []enrich.Requirement{
		{Text: "5+ years of Go", Priority: "required"},
		{Text: "Postgres", Priority: "required"},
		{Text: "Kubernetes", Priority: "preferred"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RequirementsDerived = %+v, want %+v (the benefits list must not appear)", got, want)
	}
}

// The two derived columns answer different questions, and posted_at is what separates
// them: content_hash fingerprints the indexed content (posted_at included, so a
// re-ingest with a bumped date counts as changed), while role_fingerprint is the role
// IDENTITY and deliberately excludes it, so a repost still clusters with its original.
func TestUpsertParams_PostedAtMovesContentHashButNotRoleFingerprint(t *testing.T) {
	draft := func(postedAt *time.Time) job.Draft {
		return job.Draft{
			Input: jobderive.Input{
				Source:      "manual",
				ExternalID:  "https://acme.example/jobs/1",
				Title:       "Senior Go Developer",
				Company:     "Acme",
				Description: "We use Golang.",
			},
			PostedAt: postedAt,
		}
	}
	earlier := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	later := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)

	first, err := job.New(draft(&earlier))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	second, err := job.New(draft(&later))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	a, b := first.Fields().UpsertParams(), second.Fields().UpsertParams()

	if a.ContentHash.String == b.ContentHash.String {
		t.Errorf("ContentHash unchanged across posted_at = %q; the hash does not cover the posted time actually written", a.ContentHash.String)
	}
	if a.RoleFingerprint.String != b.RoleFingerprint.String {
		t.Errorf("RoleFingerprint = %q and %q; the role identity must not move with posted_at", a.RoleFingerprint.String, b.RoleFingerprint.String)
	}
}

// A posting carries the same two fingerprints for the same content whatever wrote it:
// the moderator mapping and the automated one share one computation, so a hand-curated
// vacancy is comparable with the crawled copy of the same role rather than sitting
// outside clustering with NULL columns.
func TestUpsertManualParams_DerivedColumnsMatchTheAutomatedMapping(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "workatastartup",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Berlin",
		Description: "We use Golang.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()

	automated := f.UpsertParams()
	moderator := f.UpsertManualParams(7)

	if moderator.ContentHash != automated.ContentHash {
		t.Errorf("ContentHash = %v, want %v", moderator.ContentHash, automated.ContentHash)
	}
	if moderator.RoleFingerprint != automated.RoleFingerprint {
		t.Errorf("RoleFingerprint = %v, want %v", moderator.RoleFingerprint, automated.RoleFingerprint)
	}
}

// The moderator edit is the write that MUST move the fingerprints: re-deriving facets
// from edited content is exactly when they change. A stale content_hash would leave
// `semantic_embedded_hash IS DISTINCT FROM content_hash` false, freezing the vector on
// the pre-edit text.
func TestUpdateManualParams_EditedContentMovesTheContentHash(t *testing.T) {
	fields := func(description string) job.Fields {
		j, err := job.New(job.Draft{Input: jobderive.Input{
			Source:      "manual",
			ExternalID:  "https://acme.example/jobs/1",
			Title:       "Senior Go Developer",
			Company:     "Acme",
			Description: description,
		}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return j.Fields()
	}

	before := fields("We use Golang.").UpdateManualParams("acme-senior-go-developer", 7)
	after := fields("We use Golang and PostgreSQL.").UpdateManualParams("acme-senior-go-developer", 7)

	if before.ContentHash == after.ContentHash {
		t.Errorf("ContentHash unchanged across an edited description = %q", before.ContentHash.String)
	}
}

// The edit mapping addresses the row by public slug and stamps the acting moderator,
// and its derived columns agree with every other write path's for the same content.
func TestUpdateManualParams_CarriesSlugActorAndDerivedColumns(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Description: "We use Golang.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()

	// The slug argument names the row's REAL, persisted public_slug — here deliberately
	// f.PublicSlug itself, so this test isolates "derived columns agree for the same
	// content" from the persisted-vs-rederived-slug divergence
	// TestUpdateManualParams_ContentHashUsesThePersistedSlugNotTheRederivedOne covers.
	params := f.UpdateManualParams(f.PublicSlug, 7)
	automated := f.UpsertParams()

	if params.PublicSlug != f.PublicSlug {
		t.Errorf("PublicSlug = %q, want %q", params.PublicSlug, f.PublicSlug)
	}
	if params.UpdatedBy != 7 {
		t.Errorf("UpdatedBy = %d, want 7", params.UpdatedBy)
	}
	if params.Title != f.Title || params.Description != f.Description {
		t.Errorf("content = %q/%q", params.Title, params.Description)
	}
	if params.ContentHash != automated.ContentHash || params.RoleFingerprint != automated.RoleFingerprint {
		t.Errorf("derived = %v/%v, want %v/%v", params.ContentHash, params.RoleFingerprint, automated.ContentHash, automated.RoleFingerprint)
	}
}

// UpdateManualJob's own SQL comment is explicit that public_slug is "deliberately NOT
// updatable" by that statement — moderation.Service.Update re-derives Fields (and so a
// fresh PublicSlug) from the edited title/company, but discards the recomputed slug and
// passes the row's EXISTING one through UpdateManualParams' slug argument instead. The
// stamped ContentHash must therefore be derived from that existing, actually-persisted
// slug — not from f.PublicSlug, which job.New just recomputed from the new title and
// will never be written anywhere for this row.
func TestUpdateManualParams_ContentHashUsesThePersistedSlugNotTheRederivedOne(t *testing.T) {
	// A title change moves job.New's derived slug, exactly like an employer/title edit
	// through the moderation workspace does.
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Staff Go Developer",
		Company:     "Acme",
		Description: "We use Golang.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()

	const existingSlug = "acme-senior-go-developer" // the row's real, pre-edit slug
	if f.PublicSlug == existingSlug {
		t.Fatalf("test fixture is not exercising a divergence: f.PublicSlug (%q) already equals existingSlug", f.PublicSlug)
	}

	params := f.UpdateManualParams(existingSlug, 7)

	wantParams := f.UpsertParams()
	wantParams.PublicSlug = existingSlug
	want := jobhash.Of(wantParams)

	if params.ContentHash.String != want {
		t.Errorf("ContentHash = %q, want %q (derived from the persisted slug %q)", params.ContentHash.String, want, existingSlug)
	}

	rederived := jobhash.Of(f.UpsertParams()) // f.PublicSlug, the value that never lands in the row
	if params.ContentHash.String == rederived {
		t.Error("ContentHash was derived from f.PublicSlug (the freshly re-derived, never-persisted slug), not the row's actual public_slug")
	}

	// RoleFingerprint deliberately excludes public_slug, so it must be unaffected by
	// which slug ContentHash used.
	if params.RoleFingerprint != wantParams.RoleFingerprint {
		t.Errorf("RoleFingerprint = %q, want %q — it must not move with public_slug", params.RoleFingerprint.String, wantParams.RoleFingerprint.String)
	}
}

// UpdateEmployerParams is UpdateManualParams' employer-self-service sibling: same mapping,
// but it additionally stamps ActorID — the extra WHERE-clause scope UpdateEmployerJob's SQL
// needs and UpdateManualJob's does not (see internal/ingest/employer/job.go).
func TestUpdateEmployerParams_CarriesSlugActorAndDerivedColumns(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "employer",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Description: "We use Golang.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()

	params := f.UpdateEmployerParams(f.PublicSlug, 7)
	automated := f.UpsertParams()

	if params.PublicSlug != f.PublicSlug {
		t.Errorf("PublicSlug = %q, want %q", params.PublicSlug, f.PublicSlug)
	}
	if params.UpdatedBy != 7 {
		t.Errorf("UpdatedBy = %d, want 7", params.UpdatedBy)
	}
	if params.ActorID != 7 {
		t.Errorf("ActorID = %d, want 7 — this is the extra WHERE-clause scope UpdateManualParams has no need for", params.ActorID)
	}
	if params.Title != f.Title || params.Description != f.Description {
		t.Errorf("content = %q/%q", params.Title, params.Description)
	}
	if params.ContentHash != automated.ContentHash || params.RoleFingerprint != automated.RoleFingerprint {
		t.Errorf("derived = %v/%v, want %v/%v", params.ContentHash, params.RoleFingerprint, automated.ContentHash, automated.RoleFingerprint)
	}
}

func TestFields_MetrosReachEveryWriteParams(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/2",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Location:    "Folsom, CA",
		Description: "We use Golang.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []string{"Sacramento (US)"}
	f := j.Fields()
	if !slices.Equal(f.Metros, want) {
		t.Fatalf("Fields.Metros = %v, want %v", f.Metros, want)
	}
	if got := f.UpsertParams().Metros; !slices.Equal(got, want) {
		t.Errorf("UpsertParams.Metros = %v", got)
	}
	if got := f.UpsertManualParams(1).Metros; !slices.Equal(got, want) {
		t.Errorf("UpsertManualParams.Metros = %v", got)
	}
}

func TestUpsertParams_PreservesSourceApplicationDeadline(t *testing.T) {
	deadline := time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC)
	draft := fullDraft()
	draft.ApplicationDeadline = &deadline
	got := mustParams(t, draft)
	if !got.ApplicationDeadline.Valid || got.ApplicationDeadline.Time.Format(time.DateOnly) != "2026-11-12" {
		t.Fatalf("ApplicationDeadline = %+v, want source-stated calendar day", got.ApplicationDeadline)
	}
	draft.ApplicationDeadline = nil
	if got := mustParams(t, draft); got.ApplicationDeadline.Valid {
		t.Fatalf("unknown deadline must stay NULL, got %+v", got.ApplicationDeadline)
	}
}

// Team is the source's own label, trimmed and carried through to the upsert params.
func TestNew_TrimsTeamAndCarriesItToTheUpsert(t *testing.T) {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{Source: "lever", ExternalID: "1", Title: "Engineer", Company: "Acme"},
		Team:  "  Platform \n",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := j.Fields().UpsertParams().Team; got != "Platform" {
		t.Errorf("UpsertParams().Team = %q, want %q", got, "Platform")
	}
}
