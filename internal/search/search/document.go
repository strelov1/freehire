package search

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ai/aiarchetype"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/dict/roletag"
	"github.com/strelov1/freehire/internal/dict/roletype"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
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
//
// The skill vectors (see JobDocument.Vectors) are the other large contributor to that
// footprint, and unlike this cap their width is fixed rather than tunable: it is
// declared in the index settings, so trimming it means a full rebuild.
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
	// RoleType is "people_manager" when the title names a people-management role, and
	// empty otherwise — empty meaning "no marker found", never "individual
	// contributor" (see internal/dict/roletype for why the other side is not detectable).
	// Like Roles and AIArchetype it lives on the document rather than jobview.Job, so
	// it backs the `role_type` facet without entering the public wire shape, and it
	// needs no jobs column and no cmd/backfill-derive pass — a reindex is what
	// reaches existing postings.
	RoleType string `json:"role_type"`
	// CompanySlugFolded is company_slug with its hyphens removed — the same fold
	// jobs.company_slug_folded stores and the aggregator-suppression pass compares on
	// (migration 0109). Like Roles, it is declared on the document, not jobview.Job, so
	// it is filterable but never part of the served public wire shape.
	//
	// It exists so the ingest-time coverage gate can match an employer whichever way the
	// two sides spell it. Filtering company_slug alone matches only where the aggregator
	// and the ATS agree letter for letter, and asking about the folded SPELLING (the
	// stopgap this replaces) reaches only the direction where the ATS is the unhyphenated
	// side — "reid-health" finding "reidhealth", never the reverse, because there is no
	// guessing where hyphens go. Folding BOTH sides needs the fold stored, and here it is.
	CompanySlugFolded string `json:"company_slug_folded"`
	// Vectors carries the job's skill vector under Meilisearch's reserved `_vectors`
	// key — the userProvided embedder that backs the match sort (see
	// internal/dict/skillvec). Like Roles and RoleType it lives on the document rather
	// than jobview.Job, so it never enters the public wire shape.
	//
	// The key is ALWAYS present, and carries one of two values:
	//
	//   {skills: vec}   the job's vector.
	//   {skills: nil}   serialises as `"skills":null`, which is Meilisearch's documented
	//                   opt-out. It both declines to provide a vector AND clears any
	//                   previously stored one — the latter matters because the indexers
	//                   push with PUT (add-or-update), which merges fields, so a job
	//                   that lost its skills would otherwise keep ranking by them.
	//
	// Omitting the key is NOT a third state: with the embedder declared, Meilisearch
	// rejects the whole document ("no vectors provided for document"), which would drop
	// the posting out of the index rather than merely out of the match ordering. That
	// costs a searchable job; a lost vector costs an ordering the next rebuild restores.
	//
	// The consequence is worth naming: while the rarity weights are unavailable, every
	// document written carries a null and loses its vector. The indexers log loudly for
	// exactly this reason, and the next rebuild with weights repairs it.
	Vectors map[string][]float32 `json:"_vectors"`
}

// SkillEmbedder is the name of the Meilisearch embedder carrying skill vectors. It
// keys both the document's _vectors object and the hybrid search request, so the
// stored side and the query side cannot drift apart.
const SkillEmbedder = "skills"

// FromJob maps a database job row to its index document. An empty or absent
// enrichment payload yields the zero Enrichment (the job is still fully
// searchable by its text). Geography (regions/countries) and work_mode ride the
// document top-level — the resolved union of parsed-location and enrichment
// values — and are filtered via those bare attributes. The reality signal is NOT
// set here (it needs the caller's clock and role-cluster counts); the caller
// attaches it to the returned document via doc.Reality so it backs the
// `reality.class` facet.
//
// The skill weights ARE a parameter, unlike the reality signal, and deliberately so:
// they are a plain value rather than something needing a clock and cluster counts, so
// taking them here makes the compiler catch an indexer that forgets. A document
// silently missing its vector would drop out of the match ordering with nothing
// raised anywhere. The zero Weights is legitimate and yields a document with no
// vector — the state before the rarity rollup has ever run.
func FromJob(j db.Job, w skillvec.Weights) (JobDocument, error) {
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
		RoleType:    roletype.Derive(j.Title),
		// Read from the stored column rather than re-folded here, so the index can never
		// disagree with the pass that compares on it. A row that predates the column (the
		// backfill is chunked and paced) simply carries no folded value and is matched by
		// its exact slug alone, which is what happened before this field existed.
		CompanySlugFolded: j.CompanySlugFolded.String,
	}
	if eff := jobview.EffectivePostedAt(j.PostedAt, j.CreatedAt, time.Now()); eff.Valid {
		doc.PostedTS = eff.Time.Unix()
	}
	// ALWAYS set the key. With the embedder declared, Meilisearch REJECTS any document
	// that omits it — "no vectors provided for document" — so an omission is not a
	// no-op, it drops the posting out of the index entirely.
	doc.Vectors = map[string][]float32{SkillEmbedder: w.Vector(j.Skills)}
	return doc, nil
}

// CategoryUnresolved reports whether a job's category is unresolved by both the
// deterministic title dictionary and the LLM: internal/dict/classify's title match left
// jobs.category empty, and enrichment either never found one or fell back to the
// catch-all "other". Such a job carries no meaningful category facet, so it is
// excluded from the index rather than diluting it with the undifferentiated bulk a
// broad ATS crawl brings in (painters, stockers, drivers — postings no category
// filter, and often no keyword search, was ever meant to surface). It reads the raw
// enrichment JSON rather than jobview's folded Enrichment.Category, which the
// dictionary column always overwrites (see internal/dict/classify/AGENTS.md) and so
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
