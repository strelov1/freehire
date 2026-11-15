package collections

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// The ESDC list of employers issued a positive Labour Market Impact Assessment. One XLSX per
// quarter is added to a single CKAN dataset, so the files are discovered per run from its
// metadata: a pinned quarter would stop meaning "the last year" within three months.
const (
	caLMIAPackageURL = "https://open.canada.ca/data/api/action/package_show?id=90fed587-1364-4f33-a9ee-208181dc0b97"
	caLMIAWindow     = 4 // quarters: one year of approvals
)

// caWorkStreams are the LMIA program streams that mean an employer sponsored a
// skilled role. Primary Agriculture and Low Wage approvals are most of the file and would
// badge a farm or fast-food franchise as a sponsor for an engineering role (as ukWorkRoutes).
// Prefix matched by RequireRoute; the stream cells carry trailing padding.
var caWorkStreams = []string{
	"High Wage",
	"Global Talent",
	"Permanent Resident",
}

// caLMIAFileName identifies a quarterly English positive-employers file by its
// download name, e.g. ".../download/tfwp_2026q1_pos_en.xlsx". The resource title is prose
// ESDC rewords between releases; the file name is stable.
var caLMIAFileName = regexp.MustCompile(`/tfwp_(\d{4})q([1-4])_pos_en\.xlsx$`)

// lmiaFile is one quarter's positive-employers workbook.
type lmiaFile struct {
	Quarter int // year*10 + quarter, so the order is chronological
	URL     string
}

// pickRecentLMIAFiles reads the CKAN package_show payload and returns the n most
// recent quarterly files, newest first. Fewer than n is an error, not a shorter
// window: a dataset that lost files is indistinguishable from a real drop in
// approvals and must not quietly narrow the credential.
func pickRecentLMIAFiles(payload []byte, n int) ([]lmiaFile, error) {
	var resp struct {
		Result struct {
			Resources []struct {
				URL string `json:"url"`
			} `json:"resources"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("collections: parse ca lmia dataset metadata: %w", err)
	}
	var files []lmiaFile
	for _, r := range resp.Result.Resources {
		m := caLMIAFileName.FindStringSubmatch(r.URL)
		if m == nil {
			continue
		}
		year, _ := strconv.Atoi(m[1])
		quarter, _ := strconv.Atoi(m[2])
		files = append(files, lmiaFile{Quarter: year*10 + quarter, URL: r.URL})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Quarter > files[j].Quarter })
	if len(files) < n {
		return nil, fmt.Errorf("collections: ca lmia dataset listed %d quarterly files, want at least %d", len(files), n)
	}
	return files[:n], nil
}

// ParseCALMIASponsors reads one quarter's workbook into one record per approval
// row, keeping the program stream under "route" so RequireRoute can gate on it.
// The sheet opens with a title row and closes with footnotes, so the header is located by
// name and a row with no employer is skipped.
//
// No identity field is kept: there is no registration number, and the address differs per
// location of ONE employer, so using it would make DropAmbiguous delete every multi-site
// employer. The geography gate is the defence instead.
func ParseCALMIASponsors(data []byte) ([]Record, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("collections: open ca lmia workbook: %w", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("collections: ca lmia workbook has no sheets")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("collections: read ca lmia sheet: %w", err)
	}

	employerCol, streamCol := -1, -1
	var records []Record
	for _, row := range rows {
		if employerCol < 0 {
			employer, stream := columnIndex(row, "Employer"), columnIndex(row, "Program Stream")
			if employer >= 0 && stream >= 0 {
				employerCol, streamCol = employer, stream
			}
			continue
		}
		name := cleanCell(cellAt(row, employerCol))
		if name == "" {
			continue
		}
		records = append(records, Record{Name: name, Meta: map[string]string{"route": cleanCell(cellAt(row, streamCol))}})
	}
	if employerCol < 0 {
		return nil, errors.New("collections: ca lmia workbook has no Employer / Program Stream header")
	}
	if len(records) == 0 {
		return nil, errors.New("collections: ca lmia workbook parsed to no rows")
	}
	return records, nil
}

// fetchCALMIASponsors discovers the caLMIAWindow most recent quarters from the
// dataset metadata at packageURL, fetches and parses each, and concatenates them.
// Any one quarter failing fails the call: a partial read would reconcile the
// credential off every company it failed to reach.
func fetchCALMIASponsors(ctx context.Context, client *http.Client, packageURL string) ([]Record, error) {
	payload, err := get(ctx, client, packageURL)
	if err != nil {
		return nil, fmt.Errorf("collections: fetch ca lmia dataset metadata: %w", err)
	}
	files, err := pickRecentLMIAFiles(payload, caLMIAWindow)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, file := range files {
		body, err := get(ctx, client, file.URL)
		if err != nil {
			return nil, fmt.Errorf("collections: fetch ca lmia %d: %w", file.Quarter, err)
		}
		parsed, err := ParseCALMIASponsors(body)
		if err != nil {
			return nil, fmt.Errorf("collections: parse ca lmia %d: %w", file.Quarter, err)
		}
		records = append(records, parsed...)
	}
	return records, nil
}

// FetchCALMIASponsors is the registry-facing self-fetcher, bound to the live
// open-data dataset.
func FetchCALMIASponsors(ctx context.Context, client *http.Client) ([]Record, error) {
	return fetchCALMIASponsors(ctx, client, caLMIAPackageURL)
}

// columnIndex returns the position of the header cell named want, or -1.
func columnIndex(row []string, want string) int {
	for i, cell := range row {
		if strings.EqualFold(cleanCell(cell), want) {
			return i
		}
	}
	return -1
}

// cellAt returns row[i], or "" for a short row (spreadsheet readers drop trailing
// empty cells).
func cellAt(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}

// cleanCell collapses a spreadsheet cell's whitespace, which these registers pad
// generously ("High Wage          ").
func cleanCell(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
