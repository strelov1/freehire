// Command catalog-coverage measures how much of an outside job catalogue ours already holds.
// It reads a sample of that catalogue's postings and sorts each one into a bucket: we have the
// role (company + normalised title, jobhash.RoleKey), its ATS board is ours but the posting is
// missing (a crawl gap), its board is one we recognise but do not crawl (a harvest candidate),
// or neither — split by whether we know the employer at all.
//
// The input is a CSV with a header naming name (the employer), title and url (the apply or
// posting link), plus an optional ref naming the catalogue the row came from. The same file
// feeds cmd/seed-from-inventory, which turns its new_board rows into a cmd/harvest-boards seed.
//
//	catalog-coverage -in sample.csv -out report.csv   # needs DATABASE_URL, reads only
//
// The report is the input with bucket, provider and board columns appended; the summary
// (per ref: count per bucket and the share we hold) goes to stderr. Run-once operator tool.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// keyBatch bounds one ANY() array, so a large sample is many short index lookups rather than
// one statement that holds a snapshot open against prod.
const keyBatch = 500

func main() { worker.Main(run) }

func run() int {
	in := flag.String("in", "", "sample CSV (name,title,url[,ref])")
	out := flag.String("out", "coverage.csv", "path to write the per-row report to")
	flag.Parse()
	if *in == "" {
		log.Printf("catalog-coverage: -in is required")
		return 2
	}

	f, err := os.Open(*in)
	if err != nil {
		log.Printf("catalog-coverage: %v", err)
		return 1
	}
	samples, err := parseSamples(f)
	_ = f.Close() // read-only: a close error loses nothing
	if err != nil {
		log.Printf("catalog-coverage: %s: %v", *in, err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)

	cat, err := loadCatalogue(ctx, q, samples)
	if err != nil {
		log.Printf("catalog-coverage: %v", err)
		return 1
	}

	verdicts := make([]verdict, len(samples))
	for i, s := range samples {
		if verdicts[i], err = classify(s, cat); err != nil {
			log.Printf("catalog-coverage: board lookup: %v", err)
			return 1
		}
	}

	w, err := os.Create(*out)
	if err != nil {
		log.Printf("catalog-coverage: %v", err)
		return 1
	}
	if err := writeReport(w, samples, verdicts); err != nil {
		_ = w.Close() // the write already failed; that error is the one reported
		log.Printf("catalog-coverage: write %s: %v", *out, err)
		return 1
	}
	if err := w.Close(); err != nil {
		log.Printf("catalog-coverage: write %s: %v", *out, err)
		return 1
	}
	writeSummary(os.Stderr, samples, verdicts)
	log.Printf("catalog-coverage: wrote %s", *out)
	return 0
}

// loadCatalogue reads the open postings of every employer the sample names, and wires board
// lookups through a cache so a board the sample repeats is read once.
func loadCatalogue(ctx context.Context, q *db.Queries, samples []sample) (catalogue, error) {
	var keys []string
	for _, s := range samples {
		if k := normalize.CompanyKey(s.Name); k != "" {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)

	cat := catalogue{roles: map[string]bool{}, companies: map[string]bool{}}
	for start := 0; start < len(keys); start += keyBatch {
		rows, err := q.OpenJobTitlesByFoldedCompany(ctx, keys[start:min(start+keyBatch, len(keys))])
		if err != nil {
			return catalogue{}, fmt.Errorf("open postings: %w", err)
		}
		for _, r := range rows {
			cat.companies[r.CompanyKey] = true
			if role := jobhash.RoleKey(r.CompanyKey, r.Title); role != "" {
				cat.roles[role] = true
			}
		}
	}

	seen := map[[2]string]boardPostings{}
	cat.board = func(provider, board string) (boardPostings, error) {
		k := [2]string{provider, board}
		if b, ok := seen[k]; ok {
			return b, nil
		}
		b, err := loadBoard(ctx, q, provider, board)
		if err != nil {
			return boardPostings{}, err
		}
		seen[k] = b
		return b, nil
	}
	return cat, nil
}

// loadBoard reads one board's open postings under the board name the catalogue stores, which
// may differ in case from the one a link spells. A retired board is one we no longer crawl, so
// it reads as not ours: its postings are a harvest question, not a crawl gap.
func loadBoard(ctx context.Context, q *db.Queries, provider, board string) (boardPostings, error) {
	row, err := q.GetBoardForResolve(ctx, db.GetBoardForResolveParams{Provider: provider, Board: board})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && row.Status == "retired" {
		return boardPostings{}, nil
	}
	if err != nil {
		return boardPostings{}, err
	}
	jobs, err := q.OpenJobsOnBoard(ctx, db.OpenJobsOnBoardParams{Source: provider, BoardPattern: externalid.BoardPattern(row.Board)})
	if err != nil {
		return boardPostings{}, fmt.Errorf("board %s/%s: %w", provider, row.Board, err)
	}
	b := boardPostings{known: true, roles: map[string]bool{}}
	for _, j := range jobs {
		b.ids = append(b.ids, strings.TrimPrefix(j.ExternalID, row.Board+":"))
		if role := jobhash.RoleKey(boardRoleScope, j.Title); role != "" {
			b.roles[role] = true
		}
	}
	return b, nil
}

func writeReport(w io.Writer, samples []sample, verdicts []verdict) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ref", "name", "title", "url", "bucket", "provider", "board"})
	for i, s := range samples {
		v := verdicts[i]
		_ = cw.Write([]string{s.Ref, s.Name, s.Title, s.URL, v.Bucket, v.Provider, v.Board})
	}
	cw.Flush()
	return cw.Error()
}

// writeSummary prints, per ref, how many rows fell in each bucket and the share we hold. The
// share leaves out unusable rows: a posting with no company or title says nothing either way.
func writeSummary(w io.Writer, samples []sample, verdicts []verdict) {
	counts := map[string]map[string]int{}
	for i, s := range samples {
		if counts[s.Ref] == nil {
			counts[s.Ref] = map[string]int{}
		}
		counts[s.Ref][verdicts[i].Bucket]++
	}
	refs := make([]string, 0, len(counts))
	for r := range counts {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	for _, r := range refs {
		c := counts[r]
		usable := 0
		for _, b := range buckets {
			if b != bucketUnusable {
				usable += c[b]
			}
		}
		share := 0.0
		if usable > 0 {
			share = 100 * float64(c[bucketHave]) / float64(usable)
		}
		_, _ = fmt.Fprintf(w, "%s\thave %.1f%% of %d", r, share, usable)
		for _, b := range buckets {
			_, _ = fmt.Fprintf(w, "\t%s=%d", b, c[b])
		}
		_, _ = fmt.Fprintln(w)
	}
}
