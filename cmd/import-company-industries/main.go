// Command import-company-industries rewrites companies.industries through the
// internal/industrytag dictionary, and optionally merges an external company dump
// into it.
//
//	import-company-industries                 # normalize the stored column only
//	import-company-industries companies.jsonl # normalize, then merge the dump
//
// Needs DATABASE_URL. Run-once, exits non-zero on failure, like every other cmd/
// worker. Both passes are idempotent: a second run rewrites nothing.
//
// The normalization pass is DESTRUCTIVE by design — a stored value outside the
// dictionary is dropped, which is what makes the column dict-only. Back the column
// up before running it against a database you care about.
//
// The dump is JSONL of {"slug","name","markets"}. Its slugs are domain-derived
// (circle.com -> "circle-com") while ours come from normalize.Slug(name), so each
// record is indexed under both keys; whichever matches a company wins. The worker
// and its queries never name the dump's origin.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"slices"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/industrytag"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/worker"
)

// pageSize is the keyset page for both walks. Large enough that 380k companies do
// not cost 380k round trips, small enough that one page's rows stay cheap to hold.
const pageSize = 1000

// maxLineBytes bounds one dump record. A company's market list is a few hundred
// bytes; a megabyte means the file is not what we think it is.
const maxLineBytes = 1 << 20

// droppedReportLimit is how many unrecognized labels the run prints. The tail is a
// long list of one-off labels; the head is what a dictionary edit should act on.
const droppedReportLimit = 30

type record struct {
	Slug    string   `json:"slug"`
	Name    string   `json:"name"`
	Markets []string `json:"markets"`
}

// store is the slice of the data layer this worker needs; *db.Queries satisfies it.
type store interface {
	ListCompanyIndustriesPage(ctx context.Context, arg db.ListCompanyIndustriesPageParams) ([]db.ListCompanyIndustriesPageRow, error)
	SetCompanyIndustries(ctx context.Context, arg db.SetCompanyIndustriesParams) (int64, error)
}

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)

	changed, dropped, err := normalizeStored(ctx, q)
	if err != nil {
		log.Printf("import-company-industries: normalize: %v", err)
		return 1
	}
	log.Printf("import-company-industries: normalized, rewrote %d rows", changed)
	reportDropped(dropped)

	if len(os.Args) < 2 {
		return 0
	}

	byKey, err := readSource(os.Args[1])
	if err != nil {
		log.Printf("import-company-industries: source: %v", err)
		return 1
	}

	merged, err := mergeSource(ctx, q, byKey)
	if err != nil {
		log.Printf("import-company-industries: merge: %v", err)
		return 1
	}
	log.Printf("import-company-industries done: merged %d companies from %d keys",
		merged, len(byKey))
	return 0
}

func readSource(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Read-only, so a close error carries nothing a caller could act on — but it is
	// discarded explicitly rather than left for errcheck to flag.
	defer func() { _ = f.Close() }()
	return parseSource(f)
}

// parseSource indexes a dump under both its own slug and a slug rebuilt from the
// company name. A record whose every label is unknown is dropped rather than stored
// empty, so it can never drive an UPDATE that clears a company's industries.
func parseSource(r io.Reader) (map[string][]string, error) {
	out := map[string][]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, err
		}
		industries := industrytag.Canonicalize(rec.Markets)
		if len(industries) == 0 {
			continue
		}
		for _, key := range []string{rec.Slug, normalize.Slug(rec.Name)} {
			if key != "" {
				out[key] = industries
			}
		}
	}
	return out, sc.Err()
}

// normalizeStored walks every company and rewrites its industries through the
// dictionary, tallying the labels the dictionary did not recognize. Without that
// tally a dict-only rule is indistinguishable from silent data loss.
func normalizeStored(ctx context.Context, s store) (int, map[string]int, error) {
	dropped := map[string]int{}
	after, changed := "", 0
	for {
		rows, err := s.ListCompanyIndustriesPage(ctx, db.ListCompanyIndustriesPageParams{
			AfterSlug: after,
			PageLimit: pageSize,
		})
		if err != nil {
			return changed, dropped, err
		}
		if len(rows) == 0 {
			return changed, dropped, nil
		}
		for _, row := range rows {
			after = row.Slug
			if len(row.Industries) == 0 {
				continue
			}
			want := industrytag.Canonicalize(row.Industries)
			if slices.Equal(want, row.Industries) {
				continue
			}
			for _, had := range row.Industries {
				if len(industrytag.Canonicalize([]string{had})) == 0 {
					dropped[had]++
				}
			}
			n, err := s.SetCompanyIndustries(ctx, db.SetCompanyIndustriesParams{
				Slug:       row.Slug,
				Industries: want,
			})
			if err != nil {
				return changed, dropped, err
			}
			changed += int(n)
		}
	}
}

// mergeSource unions the dump's industries into each company it can be matched to.
func mergeSource(ctx context.Context, s store, byKey map[string][]string) (int, error) {
	after, merged := "", 0
	for {
		rows, err := s.ListCompanyIndustriesPage(ctx, db.ListCompanyIndustriesPageParams{
			AfterSlug: after,
			PageLimit: pageSize,
		})
		if err != nil {
			return merged, err
		}
		if len(rows) == 0 {
			return merged, nil
		}
		for _, row := range rows {
			after = row.Slug
			extra, ok := byKey[row.Slug]
			if !ok {
				continue
			}
			want := industrytag.Canonicalize(append(slices.Clone(row.Industries), extra...))
			if slices.Equal(want, row.Industries) {
				continue
			}
			n, err := s.SetCompanyIndustries(ctx, db.SetCompanyIndustriesParams{
				Slug:       row.Slug,
				Industries: want,
			})
			if err != nil {
				return merged, err
			}
			merged += int(n)
		}
	}
}

// reportDropped prints what the dictionary failed to recognize, most frequent
// first, so the next dictionary edit is driven by volume rather than by guesswork.
func reportDropped(dropped map[string]int) {
	if len(dropped) == 0 {
		return
	}
	type entry struct {
		label string
		n     int
	}
	all := make([]entry, 0, len(dropped))
	total := 0
	for label, n := range dropped {
		all = append(all, entry{label, n})
		total += n
	}
	slices.SortFunc(all, func(a, b entry) int {
		if a.n != b.n {
			return b.n - a.n
		}
		return cmpString(a.label, b.label)
	})

	log.Printf("import-company-industries: dropped %d distinct unrecognized labels (%d occurrences)",
		len(all), total)
	for _, e := range all[:min(droppedReportLimit, len(all))] {
		log.Printf("  unrecognized: %-44s %d", e.label, e.n)
	}
}

// cmpString breaks ties by label so the report is stable between runs over the same
// data — a report that reshuffles is one nobody diffs.
func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
