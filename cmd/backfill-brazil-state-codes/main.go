// Command backfill-brazil-state-codes re-derives the places of the Brazilian postings
// stored before internal/dict/location read a Brazilian state code as the state (#3717 and
// its follow-up for "RS, Brasil"). Until then "Porto Alegre, RS, Brasil" was Brazil AND
// Serbia, "Salvador, BA, Brasil" Bosnia, "Florianópolis, SC, Brasil" South Carolina: measured
// 2026-10-30, 13,936 open postings of these sources carried such a country (13,167 of them
// gupy), and 13,672 of them read differently under the current dictionary.
//
// Neither ordinary path reaches them. cmd/backfill-derive feeds a row's stored countries
// back into jobderive.Derive as the structured signal (storedGeography), so the stored
// Serbia is carried forward; ingest only refreshes a seen posting's liveness and never
// re-derives it. INGEST_REFETCH_ALL would, at one detail request per listed posting
// (~82k for gupy alone) to correct a set this pass reads by primary key.
//
// Dropping the stored country is safe here, and only here, because no adapter in
// candidateSources states a structured country: every country such a row carries is the
// parser's own output, so re-deriving from the location is what a fresh crawl would write.
// A source that DOES state one (SEEK's countryCode) must never join the list — that is the
// signal backfill-derive keeps on purpose.
//
// Writes countries, regions, cities and metros only. It never touches the company slug,
// so it never needs the companies sync that holds a long transaction over the catalogue.
//
// Reports by default; --apply writes. Idempotent (SetJobPlaces is IS DISTINCT FROM-guarded):
// a re-run writes nothing, and stopping it mid-way costs nothing to resume.
//
// Follow --apply with a full `make reindex`: none of these columns is part of content_hash,
// so an incremental push would not reach rows already in the index.
//
// Needs only DATABASE_URL.
package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// candidateSources are the Brazilian sources whose adapters state no structured country,
// each with the FallbackCountries its adapter passes (solides: a town the dictionary does
// not know is still in Brazil).
var candidateSources = map[string][]string{
	"gupy":    nil,
	"vagas":   nil,
	"inhire":  nil,
	"quickin": nil,
	"solides": {"br"},
}

// readBatch is how many rows one round trip fetches; descriptions are TOASTed, so it bounds
// memory more than query time.
const readBatch = 500

// pauseBetweenBatches lets the host breathe: the pass is never urgent and competes with
// ingest.
const pauseBetweenBatches = 100 * time.Millisecond

// places is the geography a row stores.
type places struct {
	Countries, Regions, Cities, Metros []string
}

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "write the corrections (default: report only)")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	sources := slices.Sorted(maps.Keys(candidateSources))
	ids, err := q.ListJobIDsWithBrazilianStateCollision(ctx, sources)
	if err != nil {
		log.Printf("backfill-brazil-state-codes: list candidates: %v", err)
		return 1
	}
	log.Printf("backfill-brazil-state-codes: %d candidates over %s", len(ids), strings.Join(sources, ","))

	var read, changed, written int64
	perSource := map[string]int64{}
	transitions := map[string]int64{}
	for start := 0; start < len(ids); start += readBatch {
		end := min(start+readBatch, len(ids))
		rows, err := q.JobsForPlaceRecheckByIDs(ctx, ids[start:end])
		if err != nil {
			log.Printf("backfill-brazil-state-codes: read %d..%d after %d written: %v", start, end, written, err)
			return 1
		}
		for _, r := range rows {
			read++
			p := recomputePlaces(r)
			if slices.Equal(p.Countries, r.Countries) && slices.Equal(p.Regions, r.Regions) &&
				slices.Equal(p.Cities, r.Cities) && slices.Equal(p.Metros, r.Metros) {
				continue
			}
			changed++
			perSource[r.Source]++
			transitions[fmt.Sprintf("%s: %v -> %v", r.Source, r.Countries, p.Countries)]++
			if !*apply {
				continue
			}
			n, err := q.SetJobPlaces(ctx, db.SetJobPlacesParams{
				ID: r.ID, Countries: p.Countries, Regions: p.Regions, Cities: p.Cities, Metros: p.Metros,
			})
			if err != nil {
				log.Printf("backfill-brazil-state-codes: write id=%d after %d written: %v", r.ID, written, err)
				return 1
			}
			written += n
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-brazil-state-codes: cancelled after %d written, resume by re-running", written)
			return 1
		case <-time.After(pauseBetweenBatches):
		}
	}

	for _, s := range sources {
		log.Printf("backfill-brazil-state-codes: %-8s %d rows change", s, perSource[s])
	}
	keys := slices.Collect(maps.Keys(transitions))
	slices.SortFunc(keys, func(a, b string) int { return cmp.Compare(transitions[b], transitions[a]) })
	for _, k := range keys[:min(len(keys), 30)] {
		log.Printf("backfill-brazil-state-codes: %6d  %s", transitions[k], k)
	}
	if !*apply {
		log.Printf("backfill-brazil-state-codes: report only, read=%d would_change=%d (re-run with --apply to write)", read, changed)
		return 0
	}
	log.Printf("backfill-brazil-state-codes: done, read=%d changed=%d written=%d (follow with a reindex)", read, changed, written)
	return 0
}

// recomputePlaces re-derives a row's places the way ingest would for a fresh crawl of its
// source: from the stored title, location and description, with the adapter's fallback
// country and the stored work mode, and WITHOUT the stored countries (see the package doc).
func recomputePlaces(r db.JobsForPlaceRecheckByIDsRow) places {
	d := jobderive.Derive(jobderive.Input{
		Title:             r.Title,
		Source:            r.Source,
		Location:          r.Location,
		Description:       r.Description,
		WorkMode:          r.WorkMode,
		FallbackCountries: candidateSources[r.Source],
	})
	return places{Countries: d.Countries, Regions: d.Regions, Cities: d.Cities, Metros: d.Metros}
}
