// Command gen-skill-descriptions drafts the next wave of glossary entries for
// internal/dict/skilltag and PRINTS them. It never writes descriptions.tsv: the shipped text
// is a reviewed artifact, and internal/dict is the one block where a dictionary never guesses.
//
// A wave is ordered by how many open postings name the skill, so a human's read goes to the
// skills readers meet first. Re-running after a merge shows what is left.
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

// draftTimeout bounds one model call. See the note where it is applied.
const draftTimeout = 4 * time.Minute

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
	// The shared 90s default sits inside this model's latency spread and drops paid drafts.
	// Nothing waits on this hand-run worker, so it can wait longer.
	client = client.WithTimeout(draftTimeout)

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

// printDrafts drafts the batch concurrently and prints it in the batch's own order. A skill
// the model fails on is reported on stderr and skipped; the run fails only when nothing
// survived.
func printDrafts(ctx context.Context, d drafter, batch []string, demand map[string]int, concurrency int, out io.Writer) error {
	drafts := make([]string, len(batch))

	// A plain group, not WithContext: one refusal must not cancel the drafts that worked.
	var g errgroup.Group
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
	_ = g.Wait() // no goroutine returns an error; failures are counted above

	for i, line := range drafts {
		if line == "" {
			continue
		}
		// The count rides along so the reviewer sees how many postings they are vouching for.
		//
		// A write failure is fatal: half a wave on a closed pipe looks like a complete batch.
		if _, err := fmt.Fprintf(out, "# %s — %d open postings\n%s\t%s\n",
			batch[i], demand[batch[i]], batch[i], line); err != nil {
			return fmt.Errorf("writing drafts: %w", err)
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
	// The skills distribution is the widest facet, so allow it to be slow.
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
