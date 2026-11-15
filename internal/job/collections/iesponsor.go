package collections

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// DETE's "Employment permits issued to companies": one workbook per calendar year,
// listing every employer granted at least one Irish employment permit. Ireland keeps no
// register of sponsoring employers, so this is the official record of who sponsors.
//
// Discovery is two hops, because DETE renames both: the statistics index links one page per
// year, and each year page links that year's workbook.
const (
	ieStatsIndexURL = "https://enterprise.gov.ie/en/what-we-do/workplace-and-skills/employment-permits/statistics/"
	// ieYearWindow is the current, partial year plus the two before it, so an
	// employer does not lose the credential in January for not yet having been
	// granted a permit that year.
	ieYearWindow = 3
)

// ieYearPageLink matches the index's per-year page links. Case-insensitive because
// the path is capitalised before 2023, and "stati\w*" because 2013's is misspelt.
var ieYearPageLink = regexp.MustCompile(`(?i)href="(/en/publications/employment-permit-stati\w*-(\d{4})\.html)"`)

// ieWorkbookLink matches a year page's link to its permits-issued-to-companies
// workbook.
var ieWorkbookLink = regexp.MustCompile(`(?i)href="([^"]*/publication-files/[^"]*issued-to-companies-\d{4}\.xlsx)"`)

// ieYearPage is one year's statistics page, as linked from the index.
type ieYearPage struct {
	Year int
	Path string
}

// pickRecentIEYearPages returns the n most recent year pages, newest first. Fewer
// than n is an error, for the same reason as the H-1B and LMIA windows.
func pickRecentIEYearPages(index []byte, n int) ([]ieYearPage, error) {
	seen := make(map[int]bool)
	var pages []ieYearPage
	for _, m := range ieYearPageLink.FindAllStringSubmatch(string(index), -1) {
		year, err := strconv.Atoi(m[2])
		if err != nil || seen[year] {
			continue
		}
		seen[year] = true
		pages = append(pages, ieYearPage{Year: year, Path: m[1]})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Year > pages[j].Year })
	if len(pages) < n {
		return nil, fmt.Errorf("collections: ie permit statistics index listed %d years, want at least %d", len(pages), n)
	}
	return pages[:n], nil
}

// ieNonEmployerCells are the column-A values that are layout, not employers. The
// workbooks change shape every year (header, total and banner rows), but the employer name
// stays in column A.
var ieNonEmployerCells = map[string]bool{
	"employer name": true,
	"grand total":   true,
	"total":         true,
	"issued":        true,
}

// ParseIEPermitEmployers reads one year's workbook into one record per employer.
// The name is column A of the first sheet; banner, header and total rows are
// skipped by their text, and a cell with no letter at all (a year in a banner row)
// is not a name either.
//
// No identity field: the file carries none (no CRO number, no address), so the
// geography gate is the only defence against a same-named business, as for Canada.
func ParseIEPermitEmployers(data []byte) ([]Record, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("collections: open ie permits workbook: %w", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("collections: ie permits workbook has no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("collections: read ie permits sheet: %w", err)
	}
	var records []Record
	for _, row := range rows {
		name := cleanCell(cellAt(row, 0))
		if name == "" || ieNonEmployerCells[strings.ToLower(name)] || !strings.ContainsFunc(name, unicode.IsLetter) {
			continue
		}
		records = append(records, Record{Name: name})
	}
	if len(records) == 0 {
		return nil, errors.New("collections: ie permits workbook parsed to no employers")
	}
	return records, nil
}

// fetchIEPermitEmployers walks index → year pages → workbooks for the ieYearWindow
// most recent years and concatenates them. Any hop failing fails the call: a
// partial read would reconcile the credential off every company it missed.
func fetchIEPermitEmployers(ctx context.Context, client *http.Client, indexURL string) ([]Record, error) {
	base, err := url.Parse(indexURL)
	if err != nil {
		return nil, fmt.Errorf("collections: parse ie index url: %w", err)
	}
	index, err := get(ctx, client, indexURL)
	if err != nil {
		return nil, fmt.Errorf("collections: fetch ie permit statistics index: %w", err)
	}
	pages, err := pickRecentIEYearPages(index, ieYearWindow)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, p := range pages {
		page, err := get(ctx, client, resolveRef(base, p.Path))
		if err != nil {
			return nil, fmt.Errorf("collections: fetch ie %d statistics page: %w", p.Year, err)
		}
		m := ieWorkbookLink.FindSubmatch(page)
		if m == nil {
			return nil, fmt.Errorf("collections: ie %d statistics page links no permits-issued-to-companies workbook", p.Year)
		}
		body, err := get(ctx, client, resolveRef(base, string(m[1])))
		if err != nil {
			return nil, fmt.Errorf("collections: fetch ie %d workbook: %w", p.Year, err)
		}
		parsed, err := ParseIEPermitEmployers(body)
		if err != nil {
			return nil, fmt.Errorf("collections: parse ie %d workbook: %w", p.Year, err)
		}
		records = append(records, parsed...)
	}
	return records, nil
}

// FetchIEPermitEmployers is the registry-facing self-fetcher, bound to the live
// DETE statistics index.
func FetchIEPermitEmployers(ctx context.Context, client *http.Client) ([]Record, error) {
	return fetchIEPermitEmployers(ctx, client, ieStatsIndexURL)
}

// resolveRef resolves a page-relative link against base, leaving an absolute or
// unparseable one as it is (the fetch then reports it).
func resolveRef(base *url.URL, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}
