// Command harvest-erecruiter discovers eRecruiter career boards for a list of companies.
// eRecruiter exposes no per-company listing keyed by anything we can derive from an apply
// form, so a company's board id (its cfg config token) must be read off the company's own
// careers page. This tool takes a worklist of careers URLs, extracts each page's
// `skk.erecruiter.pl/Code.ashx?cfg=<hex>` widget token, validates the token against the live
// board endpoint (via the same adapter that ingests it), and prints ready-to-paste
// sources/erecruiter.yml entries. Run-once host tool; review the output before committing.
//
// Input: a file (or stdin) of lines "<company name>\t<careers url>". A line with only a URL
// uses the URL host as the company name.
//
//	go run ./cmd/harvest-erecruiter careers-urls.txt
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// cfgRe matches the eRecruiter career-widget reference and captures the cfg token. The token
// is a 32-hex config id; it is matched case-insensitively and independently of the URL scheme.
var cfgRe = regexp.MustCompile(`(?i)Code\.ashx\?cfg=([a-fA-F0-9]{32})`)

// extractCfg returns the eRecruiter cfg board token embedded in a careers page, or "" when
// the page carries no eRecruiter widget.
func extractCfg(page string) string {
	if m := cfgRe.FindStringSubmatch(page); len(m) == 2 {
		return m[1]
	}
	return ""
}

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) > 2 {
		log.Printf("usage: harvest-erecruiter [careers-urls.txt]  (reads stdin if omitted)")
		return 2
	}

	in := os.Stdin
	if len(os.Args) == 2 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			log.Printf("harvest-erecruiter: %v", err)
			return 1
		}
		defer f.Close()
		in = f
	}

	ctx := context.Background()
	client := sources.NewClient()
	adapter := sources.NewErecruiter(client)

	found := map[string]string{} // cfg -> company (deduped by board)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		company, careersURL := parseLine(line)

		page, err := client.GetText(ctx, careersURL)
		if err != nil {
			log.Printf("skip %s: fetch: %v", careersURL, err)
			continue
		}
		cfg := extractCfg(page)
		if cfg == "" {
			log.Printf("skip %s: no eRecruiter widget", careersURL)
			continue
		}
		if _, seen := found[cfg]; seen {
			continue
		}
		// Validate the token against the live board (the adapter returns >0 jobs only when the
		// endpoint served parseable offer rows with usable detail pages).
		jobs, err := adapter.Fetch(ctx, sources.CompanyEntry{Company: company, Board: cfg})
		if err != nil || len(jobs) == 0 {
			log.Printf("skip %s: cfg %s validated empty (err=%v)", careersURL, cfg, err)
			continue
		}
		found[cfg] = company
		log.Printf("ok %s: cfg %s (%d jobs)", company, cfg, len(jobs))
	}
	if err := scanner.Err(); err != nil {
		log.Printf("harvest-erecruiter: read: %v", err)
		return 1
	}

	// Print validated entries, sorted by company for a stable, review-friendly diff.
	entries := make([]string, 0, len(found))
	for cfg, company := range found {
		entries = append(entries, fmt.Sprintf("- company: %s\n  board: %s", company, cfg))
	}
	sort.Strings(entries)
	for _, e := range entries {
		fmt.Println(e)
	}
	log.Printf("harvest-erecruiter: %d validated board(s)", len(found))
	return 0
}

// parseLine splits a worklist line into company and careers URL. A line is
// "<company>\t<url>"; a line with only a URL uses the URL host as the company name.
func parseLine(line string) (company, careersURL string) {
	if company, careersURL, ok := strings.Cut(line, "\t"); ok {
		return strings.TrimSpace(company), strings.TrimSpace(careersURL)
	}
	return urlHost(line), line
}

// urlHost returns the host of a URL with any leading "www." dropped, falling back to the raw
// string when it does not parse as a URL with a host.
func urlHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return raw
}
