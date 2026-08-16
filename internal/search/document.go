package search

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/aiarchetype"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/roletag"
)

// maxIndexedDescriptionRunes caps the description text stored in the search
// document. The full description lives in Postgres and is served verbatim by the
// detail endpoint (its own jobview.FromRow); the index only needs enough of it
// for keyword matching. The inverted index over `description` dominates the facet
// index size (~5x the raw document bytes), and a full rebuild's transient on-disk
// footprint scales with it — capping the indexed text to the meaningful opening
// keeps a fresh rebuild small enough to swap in within the host's free disk.
// Descriptions average ~4900 runes; 1000 captures the role summary and the first
// requirements, where keyword matches that matter overwhelmingly land.
const maxIndexedDescriptionRunes = 1000

// JobDocument is a job as stored in the Meilisearch index: the internal id (the
// primary key) plus the public jobview.Job — the exact wire shape served by the
// list and detail endpoints, so search hits render with the same SPA components.
// The embedded view flattens into the document JSON, so the stored document is
// `{ "id": ..., "public_slug": ..., ... }` and Meilisearch reads "id" as the
// primary key. The id is never returned to clients — handlers respond with the
// embedded view alone. Meilisearch filters/sorts on the nested enrichment facets
// via dot paths (e.g. "enrichment.seniority", "enrichment.salary_min").
type JobDocument struct {
	ID int64 `json:"id"`
	jobview.Job
	// PostedTS is the job's effective posting date in unix seconds — the numeric
	// encoding of the same date jobview.Job.PostedAt renders as an RFC3339 string.
	// It exists only to back the Meilisearch range filter for "posted within N days"
	// (Meili range operators need a number, not a string); it is declared on the
	// document, not on jobview.Job, so it is filterable but never served to clients.
	PostedTS int64 `json:"posted_ts"`
	// Roles are the job's natural role slugs derived at index time by roletag from
	// its seniority, category, and title. Like PostedTS, Roles is declared on the
	// document (not jobview.Job), so it backs the `roles` facet but is never part
	// of the served public wire shape.
	Roles []string `json:"roles"`
	// AIArchetype is the job's skill-signature AI archetype, derived at index
	// time by aiarchetype from its already-resolved skills and category (empty
	// for categories outside its ai_engineering/ml_ai scope, or when no rule
	// matches). Like Roles, it is declared on the document, not jobview.Job, so
	// it backs the `ai_archetype` facet but is never part of the served public
	// wire shape.
	AIArchetype string `json:"ai_archetype"`
}

// FromJob maps a database job row to its index document. An empty or absent
// enrichment payload yields the zero Enrichment (the job is still fully
// searchable by its text). Geography (regions/countries) and work_mode ride the
// document top-level — the resolved union of parsed-location and enrichment
// values — and are filtered via those bare attributes. The reality signal is NOT
// set here (it needs the caller's clock and role-cluster counts); the caller
// attaches it to the returned document via doc.Reality so it backs the
// `reality.class` facet.
func FromJob(j db.Job) (JobDocument, error) {
	view, err := jobview.FromRow(j)
	if err != nil {
		return JobDocument{}, err
	}
	// Cap the indexed description (see maxIndexedDescriptionRunes). This trims only
	// the search document — the detail endpoint serves the full description from
	// its own jobview.FromRow, unaffected by this local copy.
	view.Description = truncateRunes(view.Description, maxIndexedDescriptionRunes)
	doc := JobDocument{
		ID:          j.ID,
		Job:         view,
		Roles:       roletag.Derive(j.Seniority, j.Category, j.Title),
		AIArchetype: aiarchetype.Derive(j.Skills, j.Category),
	}
	if eff := jobview.EffectivePostedAt(j.PostedAt, j.CreatedAt, time.Now()); eff.Valid {
		doc.PostedTS = eff.Time.Unix()
	}
	return doc, nil
}

// CategoryUnresolved reports whether a job's category is unresolved by both the
// deterministic title dictionary and the LLM: internal/classify's title match left
// jobs.category empty, and enrichment either never found one or fell back to the
// catch-all "other". Such a job carries no meaningful category facet, so it is
// excluded from the index rather than diluting it with the undifferentiated bulk a
// broad ATS crawl brings in (painters, stockers, drivers — postings no category
// filter, and often no keyword search, was ever meant to surface). It reads the raw
// enrichment JSON rather than jobview's folded Enrichment.Category, which the
// dictionary column always overwrites (see internal/classify/AGENTS.md) and so
// never carries the LLM's own answer.
func CategoryUnresolved(j db.Job) bool {
	if j.Category != "" {
		return false
	}
	var e enrich.Enrichment
	if len(j.Enrichment) > 0 {
		_ = json.Unmarshal(j.Enrichment, &e)
	}
	return e.Category == "" || e.Category == "other"
}

// DescriptionMissing reports whether a job carries no posting body at all. An adapter
// keeps a posting whose detail fetch failed — the listing is authoritative for the job
// existing, and a later crawl can still hydrate it — so a body-less row is a normal,
// recoverable ingest state rather than an error. It is not something to SHOW: a vacancy
// page with a title and nothing under it tells a candidate nothing and cannot be applied
// to on its merits, so such a job is excluded from the index exactly like an
// unresolved-category one, and re-enters it by itself the moment a crawl fills the body.
//
// The test is on the VISIBLE text, not the raw column: a source that publishes an empty
// rich-text field serves markup with no words in it ("<p>&nbsp;</p>"), which the ingest
// sanitizer keeps because those tags are legal.
func DescriptionMissing(j db.Job) bool {
	return stripToPlainText(j.Description) == ""
}

// MergeClusterGeography widens a canonical document's geography facets with the union
// across its role cluster's open rows, so a collapsed multi-city/multi-country role
// stays findable by every city and country it is open in — not only the canon's own.
// Each facet becomes the sorted, deduped union of the document's own values and the
// cluster's; an empty cluster slice leaves that facet unchanged.
//
// Every writer of a job document must call this, not only the full reindex. The push is a
// field-level document update and these three facets are always present in the payload, so
// a writer that skips the union does not merely fail to widen the canon — it replaces the
// widened values with the canon's own. The reindex reads its cluster geography from the
// whole-catalogue RoleClusterGeoAll; the per-row writers read theirs from RoleClusterGeo.
func (d *JobDocument) MergeClusterGeography(countries, regions, cities []string) {
	d.Countries = unionSorted(d.Countries, countries)
	d.Regions = unionSorted(d.Regions, regions)
	d.Cities = unionSorted(d.Cities, cities)
}

// unionSorted returns the sorted, deduped union of two facet slices. A nil result
// stays nil (no cluster addition and no own values) so an untouched facet is omitted.
func unionSorted(own, extra []string) []string {
	if len(extra) == 0 {
		return own
	}
	merged := append(append([]string(nil), own...), extra...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// truncateRunes returns the first n runes of s (UTF-8 safe), backed off to the
// last space within the cut so a word is not split mid-token. Strings already
// within the cap are returned unchanged.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return cut
}
