// Command backfill-descriptions repairs stored job descriptions that carry markup damage. It
// pages by keyset and rewrites every row whose description still carries a repair marker,
// re-running the adapters' sanitize+decode pipeline and refreshing content_hash so the row
// re-indexes. The markers are source-agnostic (open or closed rows alike). Idempotent: a
// repaired row carries no marker, so a second run rewrites nothing.
//
// Repairs, each now fixed at ingest and needed here for rows already stored:
//   - percent-encoded bodies ("%3C"): Taleo's strict url.PathUnescape rejected the whole string
//     on one stray "%" (sources.LenientPercentUnescape).
//   - HTML entity-encoded bodies ("&lt;p&gt;"), served by arbeitnow (sources.UnescapeEncodedHTML).
//     sanitizeHTML cannot recover them, and arbeitnow's feed is a rolling window, so aged-out
//     rows are reachable only by in-place repair.
//   - the Himalayas "Originally posted on Himalayas" trailer (sources.StripHimalayasSelfPromo).
//     Scope it with `backfill-descriptions himalayas`: the feed is recency-ordered, so the crawl
//     never revisits older rows.
//   - anchors ("<a"): descriptions no longer carry links, so a body still containing one predates
//     that rule and is re-sanitized. Universal and by far the largest.
//   - a literal "\n" left by double-escaped JSON.
//   - quote references ("&#39;", "&#34;", and the doubled "&amp;#39;" / "&amp;#34;"): Sanitize now
//     writes those two as characters, so plain-text readers that only strip tags show them clean.
//
// BACKFILL_PAUSE_MS (default 100) is how long the sweep waits after a page in which it wrote
// rows: descriptions live in TOAST, and rewriting millions back to back saturates disk I/O. Set
// 0 for a small targeted repair; raise it if the host is under load.
//
// With no argument it sweeps the whole catalogue. A source name as the first argument (e.g.
// `backfill-descriptions taleo`) scopes it to that provider and skips detoasting every other row.
//
// Follow it with `cmd/backfill-derive` (the rewritten body changes what the deterministic
// columns derive from, and role_fingerprint hashes the description) and then `make reindex`.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// backfillBatchSize bounds how many rows are read per keyset page.
const backfillBatchSize = 500

// defaultPause is how long the sweep rests after a page in which it wrote rows, when
// BACKFILL_PAUSE_MS is unset. Non-zero because each rewrite is a TOAST write.
const defaultPause = 100 * time.Millisecond

// A row needing repair announces it in its stored text. Two markers are encodings:
// percent-encoded ("%3C") or HTML entity-encoded ("&lt;"). The third is an anchor, which no
// description may carry any more. They select candidate rows without a content-scanning SQL
// predicate.
const (
	percentEncodedMarker = "%3C"
	entityEncodedMarker  = "&lt;"
	anchorMarker         = "<a"
	// literalNewlineMarker is a backslash followed by an "n", left by an aggregator that
	// escaped its JSON twice. It needs no decoder of its own (the sanitizer repairs it), so
	// the marker only selects the row.
	literalNewlineMarker = `\n`
	// apostropheMarker and quoteMarker are what html.EscapeString writes for ' and ", which
	// Sanitize no longer emits; the doubled markers are the same quote escaped once more.
	// &amp; alone is deliberately not a marker: rewriting it would put a bare & back into
	// stored HTML, and the next sanitize would escape it again.
	apostropheMarker        = "&#39;"
	quoteMarker             = "&#34;"
	doubledApostropheMarker = "amp;#39;"
	doubledQuoteMarker      = "amp;#34;"
)

// himalayasSource is the one provider whose stored bodies carry the aggregator's own branding.
// That repair is not source-agnostic: the same links under another provider are the employer's.
const himalayasSource = "himalayas"

// jobStore is the slice of the data layer the backfill needs. *db.Queries satisfies it; the
// test uses a fake.
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

	scanned, updated, err := backfillAll(ctx, db.New(pool), source, backfillPause())
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
// rows) and repairs the ones whose stored description still carries a marker. An empty source
// sweeps the whole table. The repair reproduces the adapter pipeline exactly (decode then
// SanitizeHTML), so the recomputed content_hash matches a re-ingest.
//
// pause rests between pages that wrote rows, so a catalogue-wide sweep does not monopolise the
// disk; a page that changed nothing does not wait.
func backfillAll(ctx context.Context, store jobStore, source string, pause time.Duration) (scanned, updated int, err error) {
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

		wrote := false
		for _, j := range jobs {
			scanned++
			desc := repairDescription(j.Source, j.Description)
			if desc == j.Description {
				continue // clean row, or a marker that turned out to be real prose
			}
			hash := jobhash.OfRow(j, desc)
			if _, err := store.UpdateJobDescription(ctx, db.UpdateJobDescriptionParams{
				ID:          j.ID,
				Description: desc,
				ContentHash: pgtype.Text{String: hash, Valid: true},
			}); err != nil {
				return scanned, updated, err
			}
			updated++
			wrote = true
		}

		if len(jobs) < backfillBatchSize {
			break
		}
		if wrote {
			select {
			case <-ctx.Done():
				return scanned, updated, ctx.Err()
			case <-time.After(pause):
			}
		}
	}
	return scanned, updated, nil
}

// backfillPause reads the between-pages rest from BACKFILL_PAUSE_MS, falling back to
// defaultPause for an unset or unparseable value. An explicit 0 disables the wait.
func backfillPause() time.Duration {
	if ms, err := strconv.Atoi(os.Getenv("BACKFILL_PAUSE_MS")); err == nil && ms >= 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return defaultPause
}

// repairDescription reproduces the adapters' pipeline for a stored description: decode
// whichever encoding the row was stored with, re-sanitize, then drop the serving aggregator's
// branding. It returns stored unchanged when there was nothing to repair.
//
// Each decoder runs only when its own marker is present, so percent-decoding never touches an
// entity-encoded body (which would rewrite unrelated "%NN"-looking prose). The entity decode is
// itself conditional on encoded markup outweighing live markup (sources.UnescapeEncodedHTML),
// which leaves a posting that merely writes "&lt;" as a less-than sign alone.
//
// A stored literal "\n" or anchor sends the body through the sanitizer, whatever its source:
// the repair lives there (sources.repairLiteralNewlines, sources.sanitizeHTML), not in a decoder
// here. This leans on the sanitizer being idempotent, which its own test pins. The anchor marker
// is a plain prefix, so it also matches "<abbr" and friends; those elements are not on the
// allowlist either, so the sanitizer has work to do on such a row regardless.
//
// The de-branding is keyed on source, not a marker: an "Originally posted on Himalayas" trailer
// under another provider is that employer's own text. It sits outside the repair gate because
// branding rides on well-formed HTML nothing else touched.
func repairDescription(source, stored string) string {
	decoded := stored
	if strings.Contains(decoded, percentEncodedMarker) {
		decoded = sources.LenientPercentUnescape(decoded)
	}
	if strings.Contains(decoded, entityEncodedMarker) {
		decoded = sources.UnescapeEncodedHTML(decoded)
	}
	repaired := stored
	if decoded != stored || strings.Contains(decoded, anchorMarker) || strings.Contains(decoded, literalNewlineMarker) ||
		strings.Contains(decoded, apostropheMarker) || strings.Contains(decoded, quoteMarker) ||
		strings.Contains(decoded, doubledApostropheMarker) || strings.Contains(decoded, doubledQuoteMarker) {
		repaired = htmltext.Sanitize(decoded)
	}
	if source == himalayasSource {
		repaired = sources.StripHimalayasSelfPromo(repaired)
	}
	return repaired
}

// pageJobs fetches one keyset page after afterID, from the whole table when source is empty or
// scoped to one provider (which uses the (source, ...) index).
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
