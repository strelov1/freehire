// Command import-jobseek onboards the board catalog of the retiring Jobseek project
// (github.com/colophon-group/jobseek) into our `boards` table. Jobseek kept its whole
// crawl fleet in two CSVs — boards.csv (one row per crawled board, with its monitor type
// and the career-page URL) and companies.csv (one row per employer, with the display name
// and website) — and shut down in November 2026, so the CSVs are read-only input and this
// tool never talks to Jobseek itself.
//
// Each row is turned into a catalog candidate in three passes, cheapest first:
//
//   - a URL whose host is a recognized ATS goes through internal/ingest/atsboard
//     unchanged — provider, board id and canonical URL all come from the URL;
//   - join.com/companies/<slug> is resolved to the numeric Join company id our join
//     adapter addresses, read from the careers page's embedded JSON (which also carries
//     the real display name);
//   - an Eightfold tenant on the vendor host is onboarded as board "<host>/<domain>",
//     the shape our eightfold boards carry — the domain is read from the tenant's own
//     careers page (a website-derived guess is only a fallback) and the candidate is
//     verified against one of the two list-API generations before it is kept.
//
// Everything else Jobseek crawled — custom DOM/sitemap/RSS scrapes of corporate career
// sites, vanity hosts of ATS whose board identity is a host we have not vetted for
// external_id namespacing — is NOT onboardable here and is reported as skipped, by
// reason, for a later pass.
//
// Every kept candidate goes through boardcatalog.Inserter at status='pending', exactly
// like the other harvest tools: pending boards ARE crawled, and the first successful
// crawl activates them, so an adapter that cannot actually read a board never becomes
// active. The default run only reports; --apply writes.
//
// Usage:
//
//	go run ./cmd/import-jobseek /path/to/jobseek/apps/crawler/data
//	go run ./cmd/import-jobseek --apply /path/to/jobseek/apps/crawler/data
//
// Needs DATABASE_URL (and the network for the join/eightfold passes).
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	url "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

// candidate is one onboardable board.
type candidate struct {
	provider, board, region, company string
	// joined is the join.com slug the id was resolved from, empty for the other passes.
	joined string
}

func run() int {
	apply := flag.Bool("apply", false, "actually insert the kept boards; without it the run only reports")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Print("import-jobseek: pass the jobseek data directory (the one holding boards.csv and companies.csv)")
		return 1
	}
	dir := flag.Arg(0)

	companies, err := readCompanies(filepath.Join(dir, "companies.csv"))
	if err != nil {
		log.Printf("import-jobseek: %v", err)
		return 1
	}
	boards, err := readBoards(filepath.Join(dir, "boards.csv"))
	if err != nil {
		log.Printf("import-jobseek: %v", err)
		return 1
	}

	client := sources.NewClient()
	kept, skipped := collect(context.Background(), client, boards, companies)

	report(kept, skipped)
	if len(kept) == 0 {
		return 0
	}
	if !*apply {
		log.Printf("import-jobseek: dry run, nothing written. Re-run with --apply to insert these %d boards.", len(kept))
		return 0
	}
	return insertAll(kept)
}

// collect turns every jobseek row into either a candidate or a skip, in the three passes
// described in the package comment. A candidate seen twice (jobseek lists some boards
// under two rows) is reported once.
func collect(ctx context.Context, client *sources.Client, boards []boardRow, companies map[string]companyRow) ([]candidate, map[string]int) {
	var kept []candidate
	skipped := map[string]int{}
	seen := map[string]bool{}

	keep := func(c candidate, url, why string) {
		key := c.provider + "|" + c.board + "|" + c.region
		if seen[key] {
			skipped["duplicate row"]++
			return
		}
		seen[key] = true
		kept = append(kept, c)
	}
	skip := func(kind, url string) {
		skipped[kind]++
		_ = url
	}

	for _, b := range boards {
		company := b.company
		if c, ok := companies[company]; ok && c.name != "" {
			company = c.name
		}

		// Pass 1: a recognized ATS, straight from the URL.
		if src, board, _, ok := atsboard.Recognize(b.url); ok && src != "" && board != "" {
			keep(candidate{provider: src, board: board, company: company}, b.url, "")
			continue
		}

		// Pass 2: join.com/companies/<slug> — resolve to the numeric company id.
		if slug, ok := joinSlug(b.url); ok {
			id, name, err := resolveJoin(ctx, client, slug)
			if err != nil {
				skip("join: unresolved ("+err.Error()+")", b.url)
				continue
			}
			if name != "" {
				company = name
			}
			keep(candidate{provider: "join", board: id, company: company, joined: slug}, b.url, "")
			continue
		}

		// Pass 3: an Eightfold tenant on a *.eightfold.ai host — board = "<host>/<domain>".
		if host, ok := eightfoldHost(b.url); ok {
			domain, err := resolveEightfoldDomain(ctx, client, host, b.url, companies[b.company].website)
			if err != nil {
				skip("eightfold: no domain ("+err.Error()+")", b.url)
				continue
			}
			if err := probeEightfold(ctx, client, host, domain); err != nil {
				skip("eightfold: dead tenant ("+err.Error()+")", b.url)
				continue
			}
			keep(candidate{provider: "eightfold", board: host + "/" + domain, company: company}, b.url, "")
			continue
		}

		skip(classifySkip(b), b.url)
	}
	return kept, skipped
}

// classifySkip names why a row has no pass for it, grouped so the report says something
// useful about the residue rather than one line per URL.
func classifySkip(b boardRow) string {
	switch b.monitorType {
	case "join":
		return "join: not a /companies/<slug> URL"
	case "jobylon", "jobs_ch", "wttj", "ycombinator", "headhunter", "seek":
		return "covered by our own aggregator provider"
	case "linkedin", "notion", "inline", "dom", "sitemap", "rss", "api_sniffer", "nextdata":
		return "custom scrape of a corporate career site (no ATS)"
	default:
		return "unrecognized ATS host"
	}
}

// insertAll persists the kept candidates at status='pending', surface='cli'. A duplicate
// (the catalog already lists the board) is counted, not an error.
func insertAll(kept []candidate) int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("import-jobseek: database: %v", err)
		return 1
	}
	defer cleanup()
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	ins := boardcatalog.NewInserter(repo, sources.Taxonomy())

	added, duplicate := 0, 0
	for _, c := range kept {
		b, err := ins.Insert(ctx, boardcatalog.InsertInput{
			Provider: c.provider,
			Board:    c.board,
			Region:   c.region,
			Company:  c.company,
			Surface:  "cli",
		}, boardcatalog.StatusPending)
		switch {
		case errors.Is(err, boardcatalog.ErrDuplicateBoard):
			duplicate++
		case err != nil:
			log.Printf("import-jobseek: add %s/%s: %v", c.provider, c.board, err)
			return 1
		case b.Status == boardcatalog.StatusRejected:
			log.Printf("import-jobseek: %s/%s rejected by validation: %s", c.provider, c.board, b.RejectedReason)
			return 1
		default:
			added++
		}
	}
	log.Printf("import-jobseek: added %d boards (pending), %d already listed", added, duplicate)
	return 0
}

// report prints the kept boards grouped by provider and the skips grouped by reason.
func report(kept []candidate, skipped map[string]int) {
	perProvider := map[string]int{}
	for _, c := range kept {
		perProvider[c.provider]++
	}
	log.Printf("import-jobseek: %d onboardable boards:", len(kept))
	for _, p := range sortedKeys(perProvider) {
		log.Printf("  %-16s %5d", p, perProvider[p])
	}
	log.Printf("import-jobseek: %d rows not onboardable:", sum(skipped))
	for _, k := range sortedKeys(skipped) {
		log.Printf("  %-55s %5d", k, skipped[k])
	}
	for _, c := range kept {
		fmt.Printf("%s\t%s\t%s\t%s\n", c.provider, c.board, c.region, c.company)
	}
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// --- input files -------------------------------------------------------------

type boardRow struct {
	company, url, monitorType string
}

type companyRow struct {
	name, website string
}

func readBoards(path string) ([]boardRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	var out []boardRow
	for {
		rec, err := r.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return out, nil // a torn trailing line costs one row, not the run
		}
		out = append(out, boardRow{
			company:     rec[idx["company_slug"]],
			url:         strings.TrimSpace(rec[idx["board_url"]]),
			monitorType: rec[idx["monitor_type"]],
		})
	}
	return out, nil
}

func readCompanies(path string) (map[string]companyRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	out := map[string]companyRow{}
	for {
		rec, err := r.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			continue
		}
		out[rec[idx["slug"]]] = companyRow{name: rec[idx["name"]], website: rec[idx["website"]]}
	}
	return out, nil
}

// --- pass 2: join.com ---------------------------------------------------------

// joinSlug reports whether url is a Join.com company careers page, returning the slug.
func joinSlug(url string) (string, bool) {
	const prefix = "https://join.com/companies/"
	if !strings.HasPrefix(url, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(url, prefix)
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

// joinCompanyRe is the embedded company JSON the careers page carries:
// "company":{"id":30107,"name":"1KOMMA5˚",...
var joinCompanyRe = regexp.MustCompile(`"company":\{"id":(\d+),"name":"([^"]*)"`)

// resolveJoin turns a careers-page slug into the numeric company id the join adapter
// addresses, with the real display name the same JSON carries.
func resolveJoin(ctx context.Context, client *sources.Client, slug string) (id, name string, err error) {
	text, err := client.GetText(ctx, "https://join.com/companies/"+slug)
	if err != nil {
		return "", "", err
	}
	m := joinCompanyRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", fmt.Errorf("no company id on the page")
	}
	return m[1], strings.ReplaceAll(m[2], `\"`, `"`), nil
}

// --- pass 3: eightfold ---------------------------------------------------------

// eightfoldHost reports whether url sits on the vendor's own host. Employer-domain
// vanity hosts (careers.micron.com) are included: the board our catalog carries is
// "<host>/<domain>" and carries the vanity host itself.
func eightfoldHost(url string) (string, bool) {
	if !strings.Contains(url, "eightfold.ai") {
		return "", false
	}
	host := url
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if !strings.HasSuffix(host, "eightfold.ai") || host == "eightfold.ai" || host == "www.eightfold.ai" {
		return "", false
	}
	return host, true
}

// eightfoldDomainRe is the tenant key the careers page embeds.
var eightfoldDomainRe = regexp.MustCompile(`"domain":"([a-z0-9.-]+)"`)

// resolveEightfoldDomain finds the tenant domain in three places, cheapest first: the
// URL's own ?domain= parameter, the careers page's embedded JSON, and (as a last
// resort) the host of the company website from companies.csv. The probe in
// probeEightfold is what actually validates whichever guess came back.
func resolveEightfoldDomain(ctx context.Context, client *sources.Client, host, rawURL, website string) (string, error) {
	if u, err := url.Parse(rawURL); err == nil {
		if d := u.Query().Get("domain"); d != "" {
			return strings.ToLower(d), nil
		}
	}
	text, err := client.GetText(ctx, "https://"+host+"/careers")
	if err == nil {
		if m := eightfoldDomainRe.FindStringSubmatch(text); m != nil {
			return m[1], nil
		}
	}
	if u, err := url.Parse(website); err == nil && u.Host != "" {
		d := strings.ToLower(u.Host)
		d = strings.TrimPrefix(d, "www.")
		if d != "" {
			return d, nil
		}
	}
	return "", fmt.Errorf("no domain in the careers page")
}

// probeEightfold verifies the tenant answers one of the two list-API generations —
// the same auto-detection the eightfold adapter does on every crawl. A tenant answering
// neither is dead (Jobseek's row outlived it) and is skipped.
func probeEightfold(ctx context.Context, client *sources.Client, host, domain string) error {
	probe := func(url string) error {
		var v map[string]any
		return client.GetJSON(ctx, url, &v)
	}
	pcsx := fmt.Sprintf("https://%s/api/pcsx/search?domain=%s&query=&start=0&num=1&sort_by=relevance", host, domain)
	if err := probe(pcsx); err == nil {
		return nil
	}
	v2 := fmt.Sprintf("https://%s/api/apply/v2/jobs?domain=%s&start=0&num=1", host, domain)
	if err := probe(v2); err == nil {
		return nil
	}
	return fmt.Errorf("neither list API answered")
}
