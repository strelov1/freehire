// Command gen-skill-descriptions drafts the next wave of glossary entries for
// internal/dict/skilltag and PRINTS them. It never writes descriptions.tsv.
//
// That is the whole point. The shipped text is a reviewed artifact — a human reads each
// sentence, fixes the ones the model was confident and wrong about, and commits. A tool
// that edited the file would make "curated" a claim about the file rather than a
// property of how it got there, and internal/dict is the one block where a dictionary
// never guesses.
//
// A wave is ordered by how many open postings name the skill, because the vocabulary is
// hundreds of entries and every one costs a human read: a glossary that defines as400
// before kubernetes is a glossary nobody has used yet. Re-running after a wave is merged
// simply shows what is left — the tool subtracts what is already described.
//
// Usage:
//
//	go run ./cmd/gen-skill-descriptions -limit 100 >> internal/dict/skilltag/descriptions.tsv
//
// then edit every line before committing. Needs LLM_BASE_URL / LLM_API_KEY / LLM_MODEL.
// It reads demand from the public facets endpoint, so it needs no database.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/llm"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-skill-descriptions:", err)
		os.Exit(1)
	}
}

func run() error {
	limit := flag.Int("limit", 100, "how many skills to draft; 0 for every undescribed one")
	concurrency := flag.Int("concurrency", 4, "how many model calls to keep in flight")
	apiURL := flag.String("api", "https://freehire.me/api/v1", "base URL of the API the demand ordering is read from")
	flag.Parse()

	cfg := config.LoadLLM()
	if err := cfg.Require(); err != nil {
		return err
	}
	client, flush, err := llm.NewClient(cfg.Settings(cfg.Model), "skill-descriptions")
	if err != nil {
		return fmt.Errorf("llm client: %w", err)
	}
	defer flush()

	ctx := context.Background()
	demand, err := fetchSkillDemand(ctx, *apiURL)
	if err != nil {
		return err
	}

	batch := wave(skilltag.Canonicals(), skilltag.Descriptions(), demand, *limit)
	if len(batch) == 0 {
		fmt.Fprintln(os.Stderr, "gen-skill-descriptions: every canonical is already described")
		return nil
	}
	fmt.Fprintf(os.Stderr, "gen-skill-descriptions: drafting %d of %d undescribed skills\n",
		len(batch), len(skilltag.Canonicals())-len(skilltag.Descriptions()))

	return printDrafts(ctx, client, batch, demand, *concurrency, os.Stdout)
}

// printDrafts drafts the batch concurrently and prints it in the batch's own order, so
// the output is the wave the operator asked for rather than whatever finished first.
//
// A skill the model fails on is reported on stderr and skipped rather than failing the
// run: a wave is dozens of independent calls, and losing 79 good drafts to one gateway
// hiccup would mean paying for them twice. The run fails only when nothing survived.
func printDrafts(ctx context.Context, d drafter, batch []string, demand map[string]int, concurrency int, out io.Writer) error {
	drafts := make([]string, len(batch))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(max(concurrency, 1))
	var mu sync.Mutex
	var failed int
	for i, canonical := range batch {
		g.Go(func() error {
			s := skill{
				canonical: canonical,
				label:     skilltag.Label(canonical),
				aliases:   skilltag.Aliases(canonical),
			}
			line, err := draft(ctx, d, s)
			if err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				fmt.Fprintln(os.Stderr, err)
				return nil
			}
			drafts[i] = line
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Nothing is emitted when every draft failed: a file of comments and no rows is a
	// diff that looks like work and carries none.
	if failed < len(batch) {
		for i, line := range drafts {
			if line == "" {
				continue
			}
			// The count rides along as a comment so the reviewer can see what they are
			// vouching for: a sentence about a skill on 9,000 postings deserves more of
			// their attention than one on three.
			//
			// A write failure is fatal rather than counted. The whole output of this run
			// is one wave meant to be redirected into a file, and half a wave on a closed
			// pipe is worse than none: it looks like a complete batch.
			if _, err := fmt.Fprintf(out, "# %s — %d open postings\n%s\t%s\n",
				batch[i], demand[batch[i]], batch[i], line); err != nil {
				return fmt.Errorf("writing drafts: %w", err)
			}
		}
	}
	if failed == len(batch) {
		return fmt.Errorf("every one of the %d drafts failed", failed)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "gen-skill-descriptions: %d of %d skills failed; re-run to retry them\n", failed, len(batch))
	}
	return nil
}

// fetchSkillDemand reads the live skills distribution: canonical → open postings.
func fetchSkillDemand(ctx context.Context, apiURL string) (map[string]int, error) {
	url := apiURL + "/jobs/facets?facets=skills"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("facets request: %w", err)
	}
	// The skills distribution is the widest facet in the catalogue, so this one is
	// allowed to be slow — it is one call at the start of a run that then spends
	// minutes in the model.
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reading %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	return parseSkillDemand(body)
}
