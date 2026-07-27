// Command backfill-descriptions repairs job descriptions that were stored still
// percent-encoded. The Taleo adapter decoded its description HTML with the strict
// url.PathUnescape, which rejects the whole string on a single stray "%" — common in
// Word-pasted postings (CSS "line-height:115%") — and the old fallback stored the raw,
// fully percent-encoded blob (rendered as literal "%3Cp class=%22..."). The adapter now
// decodes leniently (internal/sources.LenientPercentUnescape); this one-off worker fixes the
// rows already in the catalogue.
//
// It also repairs the other way a source can store markup as text: HTML entity-encoding
// ("&lt;p&gt;"), which arbeitnow serves for part of its feed. sanitizeHTML cannot recover that on
// its own — bluemonday reads "&lt;p&gt;" as a text node and re-encodes it — so the adapter now
// decodes it (internal/sources.UnescapeEncodedHTML). Because the arbeitnow job-board API is a
// rolling window of recent postings, rows that aged out of it can never be reached by a
// re-ingest; in-place repair is the only route.
//
// A third kind of damage is not an encoding at all: Himalayas brands the bodies it mirrors,
// ending every posting with an "Originally posted on Himalayas" trailer and rewriting each
// mention of the hiring company into a backlink to its own page. The adapter now strips both
// (internal/sources.StripHimalayasSelfPromo); because the feed is a recency-ordered window the
// crawl only ever revisits its freshest slice, so the rows already in the catalogue can only be
// repaired in place. Scope this one to its provider (`backfill-descriptions himalayas`).
//
// It pages by keyset and re-decodes every row whose description still carries an encoding marker
// ("%3C" or "&lt;"), re-running the same sanitize+decode pipeline the fixed adapters use and
// refreshing content_hash so the row re-indexes. The markers are source-agnostic: any encoded
// description is repaired the same way, open or closed. Idempotent — a re-decoded row no longer
// decodes to anything different, so a second run rewrites nothing.
//
// With no argument it sweeps the whole catalogue (universal). Pass a source name as the first
// argument (e.g. `backfill-descriptions taleo`) to scope the sweep to that provider — the
// affected rows are known to be a single provider's, and scoping skips detoasting every other
// row, so a targeted repair does not read the whole table.
//
// Follow it with `cmd/backfill-derive` — the rewritten body changes what the deterministic
// columns derive from, and role_fingerprint hashes the description — and then `make reindex` so
// the search/recommendation index picks up the fixed text.
package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// backfillBatchSize bounds how many rows are read per keyset page.
const backfillBatchSize = 500

// A description stored still encoded carries its "<" encoded one of two ways, each the
// signature of a different upstream habit: percent-encoded ("%3C", the strict-PathUnescape
// fallback the Taleo adapter used to hit) or HTML entity-encoded ("&lt;", which arbeitnow
// serves for part of its feed). Either marker selects candidate rows cheaply, without a
// content-scanning SQL predicate.
const (
	percentEncodedMarker = "%3C"
	entityEncodedMarker  = "&lt;"
)

// himalayasSource is the one provider whose stored bodies carry the aggregator's own branding
// (a promo trailer plus self-backlinks over every company mention). Unlike the encoding markers
// this repair is not source-agnostic: the same links under another provider are the employer's.
const himalayasSource = "himalayas"

// jobStore is the slice of the data layer the backfill needs: page rows by keyset (whole table or
// one source) and rewrite a row's description + content_hash. *db.Queries satisfies it; the test
// uses a fake.
type jobStore interface {
	ListJobsByIDAfter(ctx context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error)
	ListJobsBySourceAfter(ctx context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error)
	UpdateJobDescription(ctx context.Context, arg db.UpdateJobDescriptionParams) (int64, error)
}

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	source := ""
	if len(os.Args) > 1 {
		source = os.Args[1]
	}

	scanned, updated, err := backfillAll(ctx, db.New(pool), source)
	if err != nil {
		log.Printf("backfill-descriptions: %v", err)
		return 1
	}
	scope := "all sources"
	if source != "" {
		scope = "source=" + source
	}
	log.Printf("backfill-descriptions done (%s): scanned=%d updated=%d (run `make reindex` to refresh the index)", scope, scanned, updated)
	return 0
}

// backfillAll pages jobs by keyset (id > last seen, so concurrent writes cannot skip or repeat
// rows) and re-decodes the ones whose stored description still carries the encoded marker. An
// empty source sweeps the whole table; a non-empty source scopes the sweep to that provider. The
// decode reproduces the fixed adapter's pipeline exactly (LenientPercentUnescape then
// SanitizeHTML), so the recomputed content_hash matches what a re-ingest would produce.
func backfillAll(ctx context.Context, store jobStore, source string) (scanned, updated int, err error) {
	var afterID int64
	for {
		jobs, err := pageJobs(ctx, store, source, afterID)
		if err != nil {
			return scanned, updated, err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		for _, j := range jobs {
			scanned++
			desc := repairDescription(j.Source, j.Description)
			if desc == j.Description {
				continue // clean row, or an encoding marker that turned out to be real prose
			}
			hash := jobhash.Of(hashParams(j, desc))
			if _, err := store.UpdateJobDescription(ctx, db.UpdateJobDescriptionParams{
				ID:          j.ID,
				Description: desc,
				ContentHash: pgtype.Text{String: hash, Valid: true},
			}); err != nil {
				return scanned, updated, err
			}
			updated++
		}

		if len(jobs) < backfillBatchSize {
			break
		}
	}
	return scanned, updated, nil
}

// repairDescription reproduces the fixed adapters' pipeline for a stored description: decode
// whichever encoding the row was stored with, then re-sanitize, then drop the branding the
// serving aggregator added. It returns stored unchanged when there was nothing to repair, so a
// clean row is never rewritten (and never re-sanitized, keeping the pass free of any dependence
// on sanitizeHTML being byte-for-byte idempotent).
//
// Each decoder runs only when its own marker is present, so a row mangled by one encoding is
// never put through the other's decoder — percent-decoding an entity-encoded body would rewrite
// unrelated "%NN"-looking prose. The entity decode is itself conditional on encoded markup
// outweighing live markup (see sources.UnescapeEncodedHTML), which is what leaves a posting that
// merely writes "&lt;" as a less-than sign alone.
//
// The de-branding is keyed on source, not on a marker: the markers are the aggregator's own
// links, and a posting from any other provider that links to them wrote that link itself. It
// also runs on a body nothing decoded, which is the normal case — branding rides on well-formed
// HTML — so it sits outside the decode gate.
func repairDescription(source, stored string) string {
	decoded := stored
	if strings.Contains(decoded, percentEncodedMarker) {
		decoded = sources.LenientPercentUnescape(decoded)
	}
	if strings.Contains(decoded, entityEncodedMarker) {
		decoded = sources.UnescapeEncodedHTML(decoded)
	}
	repaired := stored
	if decoded != stored {
		repaired = sources.SanitizeHTML(decoded)
	}
	if source == himalayasSource {
		repaired = sources.StripHimalayasSelfPromo(repaired)
	}
	return repaired
}

// pageJobs fetches one keyset page after afterID — from the whole table when source is empty, or
// scoped to a single provider otherwise (which lets the query use the (source, ...) index and
// skip detoasting every other row).
func pageJobs(ctx context.Context, store jobStore, source string, afterID int64) ([]db.Job, error) {
	if source == "" {
		return store.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{
			AfterID:   afterID,
			BatchSize: backfillBatchSize,
		})
	}
	return store.ListJobsBySourceAfter(ctx, db.ListJobsBySourceAfterParams{
		Source:    source,
		AfterID:   afterID,
		BatchSize: backfillBatchSize,
	})
}

// hashParams builds the content_hash inputs for a row with a replaced description — the exact
// indexed fields jobhash.Of fingerprints (see internal/jobhash), so the recomputed hash matches
// what the ingest write path would produce for the same row.
func hashParams(j db.Job, description string) db.UpsertJobParams {
	return db.UpsertJobParams{
		URL:                j.URL,
		Title:              j.Title,
		Company:            j.Company,
		CompanySlug:        j.CompanySlug,
		Location:           j.Location,
		Remote:             j.Remote,
		Description:        description,
		PostedAt:           j.PostedAt,
		PublicSlug:         j.PublicSlug,
		Countries:          j.Countries,
		Regions:            j.Regions,
		WorkMode:           j.WorkMode,
		Skills:             j.Skills,
		Seniority:          j.Seniority,
		Category:           j.Category,
		PostingLanguage:    j.PostingLanguage,
		EmploymentType:     j.EmploymentType,
		EducationLevel:     j.EducationLevel,
		ExperienceYearsMin: j.ExperienceYearsMin,
	}
}
