// Command harvest-erecruiter turns a list of company careers pages into validated
// eRecruiter catalog boards. eRecruiter's board id is a cfg token embedded in each
// company's own careers page (there is no slug to guess and no cfg in the justjoin apply
// forms), so onboarding a company means resolving its cfg from its careers URL and
// confirming the board is live before adding it.
//
// Input is one company per line, "Company Name<TAB>https://careers.url" (a line without a
// tab is treated as a bare URL, with the host as the company name). For each: fetch the
// page, extract the cfg widget token, probe its first list page, and keep it only when
// the board returns offers and the cfg is not already in the catalog. A company that
// fails any step is logged and skipped without aborting the run.
//
// It reports by default and writes only under --apply, the same convention as
// cmd/add-board. A kept board enters at status='pending'.
//
//	go run ./cmd/harvest-erecruiter <companies.tsv>
//	go run ./cmd/harvest-erecruiter --apply <companies.tsv>
//
// Needs DATABASE_URL.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// provider is the catalog provider these boards crawl under.
const provider = "erecruiter"

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually add the boards it validated; without it the run only reports")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Printf("usage: harvest-erecruiter [--apply] <companies.tsv>")
		return 2
	}

	inputs, err := readInputs(flag.Arg(0))
	if err != nil {
		log.Printf("harvest-erecruiter: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	ins := boardcatalog.NewInserter(repo, sources.All(sources.NewClient()))

	existing, err := existingCfgs(ctx, repo)
	if err != nil {
		log.Printf("harvest-erecruiter: %v", err)
		return 1
	}

	client := sources.NewClient()

	kept := 0
	for _, in := range inputs {
		cfg, offers, err := resolve(ctx, client, in.careersURL)
		if err != nil {
			log.Printf("harvest-erecruiter: %s: %v", in.company, err)
			continue
		}
		if cfg == "" {
			log.Printf("harvest-erecruiter: %s: no eRecruiter cfg on %s", in.company, in.careersURL)
			continue
		}
		if offers == 0 {
			log.Printf("harvest-erecruiter: %s: cfg %s resolved but board has no offers", in.company, cfg)
			continue
		}
		if existing[strings.ToLower(cfg)] {
			log.Printf("harvest-erecruiter: %s: cfg %s already in the catalog", in.company, cfg)
			continue
		}
		existing[strings.ToLower(cfg)] = true
		fmt.Printf("%s — %s — %d offers\n", cfg, in.company, offers)
		kept++
		if *apply {
			if err := addBoard(ctx, ins, cfg, in.company); err != nil {
				log.Printf("harvest-erecruiter: %v", err)
				return 1
			}
		}
	}
	log.Printf("harvest-erecruiter: %d input(s), %d new live board(s)", len(inputs), kept)
	if kept > 0 && !*apply {
		log.Printf("harvest-erecruiter: re-run with --apply to add them")
	}
	return 0
}

// addBoard persists one validated cfg at status='pending'. A duplicate is not an error:
// another run may have landed the same cfg between the catalog read and this insert, and
// the run should still onboard the rest.
func addBoard(ctx context.Context, ins *boardcatalog.Inserter, cfg, company string) error {
	b, err := ins.Insert(ctx, boardcatalog.InsertInput{
		Provider: provider,
		Board:    cfg,
		Company:  company,
		Surface:  "cli",
	}, boardcatalog.StatusPending)
	switch {
	case errors.Is(err, boardcatalog.ErrDuplicateBoard):
		return nil
	case err != nil:
		return fmt.Errorf("add %s: %w", cfg, err)
	case b.Status == boardcatalog.StatusRejected:
		// Validation refused a cfg that just answered with offers: a bug here, not a
		// bad input.
		return fmt.Errorf("cfg %s rejected by validation: %s", cfg, b.RejectedReason)
	}
	return nil
}

// input is one company to probe: its display name and the careers URL to scan for a cfg.
type input struct {
	company    string
	careersURL string
}

// resolve fetches a careers page, extracts its eRecruiter cfg, and live-probes it, returning
// the cfg and its first-page offer count (0 when absent or empty).
func resolve(ctx context.Context, client *sources.Client, careersURL string) (string, int, error) {
	page, err := client.GetText(ctx, careersURL)
	if err != nil {
		return "", 0, fmt.Errorf("fetch careers page: %w", err)
	}
	cfg := sources.ExtractErecruiterCfg(page)
	if cfg == "" {
		return "", 0, nil
	}
	offers, err := sources.ProbeErecruiterCfg(ctx, client, cfg)
	if err != nil {
		return cfg, 0, fmt.Errorf("probe cfg %s: %w", cfg, err)
	}
	return cfg, offers, nil
}

// readInputs parses the "Company<TAB>URL" worklist; a line without a tab is a bare URL whose
// host becomes the company name. Blank lines and lines without a usable URL are skipped.
func readInputs(path string) ([]input, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read inputs %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var out []input
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		company, careersURL := "", line
		if name, u, ok := strings.Cut(line, "\t"); ok {
			company, careersURL = strings.TrimSpace(name), strings.TrimSpace(u)
		}
		if company == "" {
			if u, err := url.Parse(careersURL); err == nil {
				company = u.Host
			}
		}
		if careersURL == "" {
			continue
		}
		out = append(out, input{company: company, careersURL: careersURL})
	}
	return out, sc.Err()
}

// existingCfgs returns the cfg board ids already live in the catalog (lower-cased for a
// case-insensitive compare), so a re-run does not re-propose a company already onboarded.
func existingCfgs(ctx context.Context, repo boardcatalog.Repository) (map[string]bool, error) {
	listed, err := boardcatalog.LoadForProvider(ctx, repo, provider)
	if err != nil {
		return nil, fmt.Errorf("load catalog for %s: %w", provider, err)
	}
	out := make(map[string]bool, len(listed))
	for _, e := range listed {
		out[strings.ToLower(e.Board)] = true
	}
	return out, nil
}
