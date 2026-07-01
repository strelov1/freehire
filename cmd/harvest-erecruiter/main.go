// Command harvest-erecruiter turns a list of company careers pages into validated
// sources/erecruiter.yml entries. eRecruiter's board id is a cfg token embedded in each
// company's own careers page (there is no slug to guess and no cfg in the justjoin apply
// forms), so onboarding a company means resolving its cfg from its careers URL and
// confirming the board is live before committing it.
//
// Input is one company per line, "Company Name<TAB>https://careers.url" (a line without a
// tab is treated as a bare URL, with the host as the company name). For each: fetch the
// page, extract the cfg widget token, probe its first list page, and print an entry only
// when the board returns offers and the cfg is not already in sources/erecruiter.yml. A
// company that fails any step is logged and skipped without aborting the run.
//
// Run-once host tool; review the printed entries before appending them to the board file.
//
//	go run ./cmd/harvest-erecruiter <companies.tsv>
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) != 2 {
		log.Printf("usage: harvest-erecruiter <companies.tsv>")
		return 2
	}

	inputs, err := readInputs(os.Args[1])
	if err != nil {
		log.Printf("harvest-erecruiter: %v", err)
		return 1
	}

	existing, err := existingCfgs("sources/erecruiter.yml")
	if err != nil {
		log.Printf("harvest-erecruiter: %v", err)
		return 1
	}

	ctx := context.Background()
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
			log.Printf("harvest-erecruiter: %s: cfg %s already in board file", in.company, cfg)
			continue
		}
		existing[strings.ToLower(cfg)] = true
		fmt.Printf("- company: %s\n  board: %s\n", in.company, cfg)
		kept++
	}
	log.Printf("harvest-erecruiter: %d input(s), %d new live board(s)", len(inputs), kept)
	return 0
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
	defer f.Close()

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

// existingCfgs returns the cfg board ids already listed in the board file (lower-cased for a
// case-insensitive compare), so a re-run does not re-emit a company already onboarded.
func existingCfgs(path string) (map[string]bool, error) {
	cfg, err := sources.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(cfg.Sources))
	for _, e := range cfg.Sources {
		out[strings.ToLower(e.Board)] = true
	}
	return out, nil
}
