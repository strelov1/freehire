package search

import (
	"strings"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
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
}

// FromJob maps a database job row to its index document. An empty or absent
// enrichment payload yields the zero Enrichment (the job is still fully
// searchable by its text). Geography (regions/countries) and work_mode ride the
// document top-level — the resolved union of parsed-location and enrichment
// values — and are filtered via those bare attributes.
func FromJob(j db.Job) (JobDocument, error) {
	view, err := jobview.FromRow(j)
	if err != nil {
		return JobDocument{}, err
	}
	// Cap the indexed description (see maxIndexedDescriptionRunes). This trims only
	// the search document — the detail endpoint serves the full description from
	// its own jobview.FromRow, unaffected by this local copy.
	view.Description = truncateRunes(view.Description, maxIndexedDescriptionRunes)
	return JobDocument{ID: j.ID, Job: view}, nil
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
