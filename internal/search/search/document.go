package search

import (
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ai/aiarchetype"
	"github.com/strelov1/freehire/internal/dict/roletype"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// maxIndexedDescriptionRunes caps the description text in the search document. The full
// text stays in Postgres; the index only needs the opening for keyword matching, and the
// inverted index over description dominates facet index size and rebuild disk use.
const maxIndexedDescriptionRunes = 1000

// JobDocument is a job as stored in the Meilisearch index: the internal id (primary key)
// plus the public jobview.Job wire shape, flattened into one JSON object. The id is never
// returned to clients. Enrichment facets are filtered via dot paths ("enrichment.seniority").
type JobDocument struct {
	// IsClosed is internal search state. A full replacement of an open job
	// clears the marker, while legacy documents without it remain searchable.
	IsClosed bool `json:"is_closed,omitempty"`
	// IsDuplicate hides a retained copy without discarding its text and vectors.
	IsDuplicate bool  `json:"is_duplicate,omitempty"`
	ID          int64 `json:"id"`
	jobview.Job
	// PostedTS is the effective posting date in unix seconds, for the "posted within N days"
	// range filter (Meili ranges need a number). Kept off jobview.Job so it is never served.
	PostedTS int64 `json:"posted_ts"`
	// CreatedTS is when ingest first wrote the row, in unix seconds. It backs the "open within
	// N days" bound: some boards restate PostedTS on every crawl (fake_freshness), while
	// created_at cannot be rewritten by a source.
	CreatedTS int64 `json:"created_ts"`
	// AIArchetype is derived at index time from resolved skills and category; empty outside
	// ai_engineering/ml_ai or when no rule matches. Backs the ai_archetype facet, not served.
	AIArchetype string `json:"ai_archetype"`
	// RoleType is "people_manager" when the title names a people-management role. Empty means
	// "no marker found", never "individual contributor" (see internal/dict/roletype). Backs the
	// role_type facet, not served; a reindex reaches existing postings.
	RoleType string `json:"role_type"`
	// SourceType is the posting's source kind (one of vocab.SourceTypeValues), empty when no
	// kind applies. The caller resolves it because sources.ProviderKind is not importable here.
	//
	// The key is written even when empty: Meilisearch's `!=` does not match a document lacking
	// the attribute, so omitting it would make source_type_exclude=aggregator drop every
	// unclassified posting.
	SourceType string `json:"source_type"`
	// AIInterview is true when the company has at least one un-retracted AI-interview report.
	// Kept beside the count because filtering an integer would be an unguessable range query.
	// Not served.
	//
	// Written only when true, so the negative is asked as NOT of the positive (see
	// aiInterviewFragment), which also reaches documents that omit it.
	AIInterview bool `json:"ai_interview,omitempty"`
	// Vectors carries the skill vector under Meilisearch's reserved `_vectors` key, the
	// userProvided embedder behind the match sort (see internal/dict/skillvec). Not served.
	//
	// The key is ALWAYS present. {skills: nil} serialises as `"skills":null`, Meilisearch's
	// opt-out: it declines a vector and clears a stored one. Omitting the key makes Meilisearch
	// reject the whole document ("no vectors provided for document").
	Vectors DocumentVectors `json:"_vectors"`
}

// DocumentVectors is a document's `_vectors` object: one always-serialised field per
// declared embedder (see JobDocument.Vectors). A struct so a writer cannot omit an embedder;
// TestDocumentVectorsKeysMatchEmbedders pins the JSON names.
type DocumentVectors struct {
	// Skills is the match sort's single skill vector.
	Skills []float32 `json:"skills"`
}

// SkillEmbedder names the embedder carrying skill vectors, shared by the document's _vectors
// and the hybrid search request so the two sides cannot drift.
const SkillEmbedder = "skills"

// FromJob maps a job row to its index document. Empty enrichment yields the zero Enrichment.
// Geography and work_mode ride top-level as the resolved union of parsed-location and
// enrichment values. The reality signal is not set here (it needs the caller's clock and
// cluster counts); the caller attaches it via doc.Reality.
//
// sourceType is a parameter, not attached later, because a forgotten attach would silently
// drop the posting from every source_type filter.
func FromJob(j db.Job, sourceType string) (JobDocument, error) {
	view, err := jobview.FromRow(j)
	if err != nil {
		return JobDocument{}, err
	}
	// jobview serves the dictionary column, or the model's category where the column is empty.
	// A posting neither files is indexed under the residual "other" bucket (#4412).
	if view.Enrichment.Category == "" {
		view.Enrichment.Category = "other"
	}
	// Cap the indexed description; the detail endpoint serves the full text from its own FromRow.
	view.Description = truncateRunes(view.Description, maxIndexedDescriptionRunes)
	doc := JobDocument{
		ID:          j.ID,
		IsClosed:    j.ClosedAt.Valid,
		IsDuplicate: j.DuplicateOf.Valid,
		Job:         view,
		AIArchetype: aiarchetype.Derive(j.Skills, j.Category),
		RoleType:    roletype.Derive(j.Title),
		SourceType:  sourceType,
		AIInterview: view.AIInterviewReports > 0,
	}
	if eff := jobview.EffectivePostedAt(j.PostedAt, j.CreatedAt, time.Now()); eff.Valid {
		doc.PostedTS = eff.Time.Unix()
	}
	// created_at is never absent in production; the Valid guard covers a zero db.Job in tests.
	if j.CreatedAt.Valid {
		doc.CreatedTS = j.CreatedAt.Time.Unix()
	}
	// Always set the key: Meilisearch rejects a document that omits it.
	doc.Vectors = DocumentVectors{Skills: skillvec.JobVector(j.Skills)}
	return doc, nil
}

// AttachApplyQuestions sets the document's apply_questions from a batch's counts
// (applyform.QuestionCounts, which this package may not import — the writers read them and
// hand them in). A job missing from counts has no captured form and keeps the key absent:
// that is "we do not know", which the `apply_questions_max` filter must not read as short.
func (d *JobDocument) AttachApplyQuestions(counts map[int64]int) {
	if n, ok := counts[d.ID]; ok {
		d.ApplyQuestions = &n
	}
}

// DescriptionMissing reports whether a job has no visible posting body (a failed detail fetch
// is a normal, recoverable ingest state). Such a job is excluded from the index and re-enters
// once a crawl fills the body.
//
// It tests the visible text, not the raw column: an empty rich-text field arrives as markup
// with no words ("<p>&nbsp;</p>"), which the sanitizer keeps.
func DescriptionMissing(j db.Job) bool {
	return stripToPlainText(j.Description) == ""
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
