package collections

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
)

// The USCIS H-1B Employer Data Hub bulk-file archive: one CSV per completed fiscal
// year, published at a stable index page. The credential combines several years, so the year
// files are discovered each run: a pinned year would stop meaning "the most recent window"
// within one cycle.
const (
	ush1bIndexURL   = "https://www.uscis.gov/archive/h-1b-employer-data-hub-files"
	ush1bYearWindow = 5

	// uscis.gov's bot filter 403s Go's default User-Agent and accepts a browser-like one.
	ush1bUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
)

// fyFile is one fiscal year's data-hub CSV, as listed on the archive index page.
type fyFile struct {
	Year int
	URL  string
}

// fyLinkPattern matches the index page's per-year download links, e.g.
// <a href="/sites/default/files/document/data/h1b_datahubexport-2023.csv" ...>.
// Anchored on the file name, not the decorative link text.
var fyLinkPattern = regexp.MustCompile(`(?is)<a\s+href="([^"]*h1b_datahubexport-(\d{4})\.csv)"[^>]*>`)

// discoverRecentFYFiles extracts every (year, URL) pair from the archive index
// page and returns the n most recent, sorted descending. Fewer than n years
// listed is an error, not a shorter result: a shrunk index is indistinguishable from a real
// drop in coverage and must not silently narrow the window.
func discoverRecentFYFiles(page []byte, n int) ([]fyFile, error) {
	matches := fyLinkPattern.FindAllStringSubmatch(string(page), -1)
	files := make([]fyFile, 0, len(matches))
	for _, m := range matches {
		year, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		files = append(files, fyFile{Year: year, URL: m[1]})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Year > files[j].Year })
	if len(files) < n {
		return nil, fmt.Errorf("collections: us h1b data hub index listed %d fiscal-year files, want at least %d", len(files), n)
	}
	return files[:n], nil
}

// ParseUSH1BSponsorsCSV reads one fiscal year's data-hub file into one record per
// employer with at least one petition approved (initial or continuing) that year.
// An employer whose only rows are denials contributes nothing: the credential
// means "has been approved to sponsor," not "has applied." The last-4-digit tax
// identifier is retained as the record's identity field, the same role the UK
// register's town and the Dutch register's KvK number play.
func ParseUSH1BSponsorsCSV(data []byte) ([]Record, error) {
	rows, err := csvColumns(data, ',',
		"Employer", "Initial Approval", "Initial Denial", "Continuing Approval", "Continuing Denial", "Tax ID")
	if err != nil {
		// Older files (FY2019) pluralize these column names, so fall back to that spelling.
		rows, err = csvColumns(data, ',',
			"Employer", "Initial Approvals", "Initial Denials", "Continuing Approvals", "Continuing Denials", "Tax ID")
		if err != nil {
			return nil, err
		}
	}
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		name := row[0]
		if name == "" {
			continue
		}
		// A malformed count reads as 0 rather than aborting the row, as csvColumns does for
		// ragged rows.
		initial, _ := strconv.Atoi(row[1])
		continuing, _ := strconv.Atoi(row[3])
		if initial+continuing < 1 {
			continue
		}
		records = append(records, Record{Name: name, Meta: map[string]string{"tin4": row[5]}})
	}
	if len(records) == 0 {
		return nil, errors.New("collections: us h1b employer file parsed to no approved rows")
	}
	return records, nil
}

// fetchUSH1BSponsors fetches the archive index at indexURL, discovers the
// ush1bYearWindow most recent fiscal-year files, fetches and parses each, and
// concatenates them into one member list. Any single year failing fails the whole call: a
// partial read would reconcile the credential off every company it failed to reach.
// Parameterized on indexURL for tests.
func fetchUSH1BSponsors(ctx context.Context, client *http.Client, indexURL string) ([]Record, error) {
	page, err := getWithUserAgent(ctx, client, indexURL, ush1bUserAgent)
	if err != nil {
		return nil, fmt.Errorf("collections: fetch us h1b data hub index: %w", err)
	}
	files, err := discoverRecentFYFiles(page, ush1bYearWindow)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(indexURL)
	if err != nil {
		return nil, fmt.Errorf("collections: parse us h1b index url: %w", err)
	}

	var records []Record
	for _, f := range files {
		ref, err := url.Parse(f.URL)
		if err != nil {
			return nil, fmt.Errorf("collections: resolve us h1b fy%d url: %w", f.Year, err)
		}
		body, err := getWithUserAgent(ctx, client, base.ResolveReference(ref).String(), ush1bUserAgent)
		if err != nil {
			return nil, fmt.Errorf("collections: fetch us h1b fy%d file: %w", f.Year, err)
		}
		parsed, err := ParseUSH1BSponsorsCSV(body)
		if err != nil {
			return nil, fmt.Errorf("collections: parse us h1b fy%d file: %w", f.Year, err)
		}
		records = append(records, parsed...)
	}
	return records, nil
}

// FetchUSH1BSponsors is the registry-facing self-fetcher, bound to the live USCIS
// archive index.
func FetchUSH1BSponsors(ctx context.Context, client *http.Client) ([]Record, error) {
	return fetchUSH1BSponsors(ctx, client, ush1bIndexURL)
}

// getWithUserAgent is get (see uksponsor.go) with a custom User-Agent, kept local because only
// this source needs one.
func getWithUserAgent(ctx context.Context, client *http.Client, url, userAgent string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
