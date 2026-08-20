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
// ending every posting with an "Originally posted on Himalayas" trailer. The adapter now strips
// it (internal/sources.StripHimalayasSelfPromo); because the feed is a recency-ordered window the
// crawl only ever revisits its freshest slice, so the rows already in the catalogue can only be
// repaired in place. Scope this one to its provider (`backfill-descriptions himalayas`).
//
// The fourth repair is universal and by far the largest: descriptions no longer carry links at
// all. sanitizeHTML unwraps every anchor to its text (and drops the ones whose text merely
// repeated their own address), so a stored body that still contains "<a" predates that rule and
// is re-sanitized here. Expect this to rewrite most of the catalogue on its first run — see the
// pacing note below.
//
// It pages by keyset and rewrites every row whose description still carries a repair marker — an
// encoding ("%3C" or "&lt;") or an anchor ("<a") — re-running the same sanitize+decode pipeline
// the adapters use and refreshing content_hash so the row re-indexes. The markers are
// source-agnostic: any such description is repaired the same way, open or closed. Idempotent — a
// repaired row carries none of the markers, so a second run rewrites nothing.
//
// BACKFILL_PAUSE_MS (default 100) is how long the sweep waits after a page in which it actually
// wrote rows. Postgres on the app host is untuned and a description lives in TOAST, so rewriting
// millions of them back to back saturates disk I/O and takes the site down with it. Set it to 0
// for a small, targeted repair; raise it if the host is already under load.
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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// backfillBatchSize bounds how many rows are read per keyset page.
const backfillBatchSize = 500

// defaultPause is how long the sweep rests after a page in which it wrote rows, when
// BACKFILL_PAUSE_MS says nothing. It is deliberately non-zero: the universal link strip
// rewrites most of the catalogue, and each rewrite is a TOAST write.
const defaultPause = 100 * time.Millisecond

// A row needing repair announces it in its stored text. Two markers are encodings, each the
// signature of a different upstream habit: percent-encoded ("%3C", the strict-PathUnescape
// fallback the Taleo adapter used to hit) or HTML entity-encoded ("&lt;", which arbeitnow
// serves for part of its feed). The third is an anchor, which no description may carry any
// more. Any of them selects candidate rows cheaply, without a content-scanning SQL predicate.
const (
	percentEncodedMarker = "%3C"
	entityEncodedMarker  = "&lt;"
	anchorMarker         = "<a"
	// literalNewlineMarker is the fourth signature: a backslash followed by an "n", left by an
	// aggregator that escaped its JSON twice (whatjobs markets and adzuna, which resell through
	// the same upstream network). Unlike the encodings this needs no decoder of its own — the
	// sanitizer repairs it — so the marker only has to select the row.
	literalNewlineMarker = `\n`
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
// sweeps the whole table; a non-empty source scopes the sweep to that provider. The repair
// reproduces the adapter pipeline exactly (decode then SanitizeHTML), so the recomputed
// content_hash matches what a re-ingest would produce.
//
// pause rests between pages that wrote rows, keeping a catalogue-wide sweep from monopolising
// the database's disk. A page that changed nothing costs nothing, so it does not wait.
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

// repairDescription reproduces the adapters' pipeline for a stored description: decode whichever
// encoding the row was stored with, then re-sanitize, then drop the branding the serving
// aggregator added. It returns stored unchanged when there was nothing to repair, so a row with
// no marker is never rewritten.
//
// Each decoder runs only when its own marker is present, so a row mangled by one encoding is
// never put through the other's decoder — percent-decoding an entity-encoded body would rewrite
// unrelated "%NN"-looking prose. The entity decode is itself conditional on encoded markup
// outweighing live markup (see sources.UnescapeEncodedHTML), which is what leaves a posting that
// merely writes "&lt;" as a less-than sign alone.
//
// A stored literal "\n" sends the body through the sanitizer for the same reason as an anchor:
// the repair lives there (see internal/sources.repairLiteralNewlines), not in a decoder here,
// because deciding what an escape meant needs the markup around it. The marker is narrow enough
// to select almost nothing — 15 rows in a 20k-row sample of the live catalogue — so it does not
// widen the universal sweep in any meaningful way.
//
// A stored anchor also sends the body through the sanitizer, whatever its source: descriptions
// carry no links any more (see internal/sources.sanitizeHTML), and the rows stored before that
// rule can only be brought in line by re-running it. Unlike the decodes this path does lean on
// the sanitizer being idempotent, which its own test pins. The marker is a plain prefix, so it
// also matches "<abbr" and friends — those elements are not on the allowlist either, so the
// sanitizer has work to do on such a row regardless.
//
// The de-branding is keyed on source, not on a marker: an "Originally posted on Himalayas"
// trailer under any other provider is that employer's own text. It also runs on a body nothing
// else touched, which is the normal case — branding rides on well-formed HTML — so it sits
// outside the repair gate.
func repairDescription(source, stored string) string {
	decoded := stored
	if strings.Contains(decoded, percentEncodedMarker) {
		decoded = sources.LenientPercentUnescape(decoded)
	}
	if strings.Contains(decoded, entityEncodedMarker) {
		decoded = sources.UnescapeEncodedHTML(decoded)
	}
	repaired := stored
	if decoded != stored || strings.Contains(decoded, anchorMarker) || strings.Contains(decoded, literalNewlineMarker) {
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
