package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// companySite pairs a company name with its website — the input the resolve step
// follows to a careers page.
type companySite struct {
	Name    string `json:"name"`
	Website string `json:"website"`
}

// siteParsers maps a collection slug to a parser that extracts (name, website)
// from that dataset's payload. Only datasets that carry a website are listed; the
// hand-list collections (bigtech/mag7/ai) have none. The dataset URLs come from
// collections.All, so this only encodes each dataset's website field shape.
var siteParsers = map[string]func([]byte) ([]companySite, error){
	"yc":         parseYCSites,
	"european":   parseEUSites,
	"techstars":  parseTechstarsSites,
	"fortune500": func(b []byte) ([]companySite, error) { return parseCSVSites(b, ',', "Company", "Website") },
	"unicorn":    func(b []byte) ([]companySite, error) { return parseCSVSites(b, ',', "Company", "Company Website") },
}

func parseYCSites(data []byte) ([]companySite, error) {
	var raw []struct {
		Name    string `json:"name"`
		Website string `json:"website"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yc sites: %w", err)
	}
	out := make([]companySite, 0, len(raw))
	for _, c := range raw {
		if c.Name != "" && c.Website != "" {
			out = append(out, companySite{Name: c.Name, Website: c.Website})
		}
	}
	return out, nil
}

func parseEUSites(data []byte) ([]companySite, error) {
	var raw []struct {
		Name    string `json:"Name"`
		Website string `json:"Website URL"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse european sites: %w", err)
	}
	out := make([]companySite, 0, len(raw))
	for _, c := range raw {
		if c.Name != "" && c.Website != "" {
			out = append(out, companySite{Name: c.Name, Website: c.Website})
		}
	}
	return out, nil
}

// parseTechstarsSites reads the semicolon-separated Techstars CSV (name;urls;…),
// taking the first URL of the comma-separated urls list as the homepage.
func parseTechstarsSites(data []byte) ([]companySite, error) {
	rows, _, err := csvColumns(data, ';', "name", "urls")
	if err != nil {
		return nil, err
	}
	out := make([]companySite, 0, len(rows))
	for _, r := range rows {
		site := strings.TrimSpace(strings.SplitN(r[1], ",", 2)[0])
		if r[0] != "" && site != "" {
			out = append(out, companySite{Name: r[0], Website: site})
		}
	}
	return out, nil
}

// parseCSVSites reads a delimited CSV, taking the name and website from the named
// columns (located by header).
func parseCSVSites(data []byte, delim rune, nameCol, urlCol string) ([]companySite, error) {
	rows, _, err := csvColumns(data, delim, nameCol, urlCol)
	if err != nil {
		return nil, err
	}
	out := make([]companySite, 0, len(rows))
	for _, r := range rows {
		if r[0] != "" && r[1] != "" {
			out = append(out, companySite{Name: r[0], Website: r[1]})
		}
	}
	return out, nil
}

// csvColumns returns each data row's (col0, col1) values for the two named columns,
// located by header (not index, so an upstream reorder can't read the wrong field).
func csvColumns(data []byte, delim rune, col0, col1 string) (rows [][2]string, header []string, err error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.Comma = delim
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read csv header: %w", err)
	}
	i0, i1 := colIndex(head, col0), colIndex(head, col1)
	if i0 < 0 || i1 < 0 {
		return nil, head, fmt.Errorf("csv missing column %q or %q", col0, col1)
	}
	all, err := r.ReadAll()
	if err != nil {
		return nil, head, fmt.Errorf("read csv: %w", err)
	}
	for _, row := range all {
		var v0, v1 string
		if i0 < len(row) {
			v0 = strings.TrimSpace(row[i0])
		}
		if i1 < len(row) {
			v1 = strings.TrimSpace(row[i1])
		}
		rows = append(rows, [2]string{v0, v1})
	}
	return rows, head, nil
}

func colIndex(header []string, name string) int {
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return i
		}
	}
	return -1
}

// filterUnmatched drops companies whose normalized-name slug is already in the
// catalogue (present in existing), leaving the ones worth discovering boards for.
func filterUnmatched(sites []companySite, existing map[string]bool) []companySite {
	out := make([]companySite, 0, len(sites))
	for _, s := range sites {
		if !existing[normalize.Slug(s.Name)] {
			out = append(out, s)
		}
	}
	return out
}

// dedupeByWebsite collapses companies that share a website (the same company can
// appear in several collections), keeping first-seen order, so a site is fetched once.
func dedupeByWebsite(sites []companySite) []companySite {
	seen := make(map[string]bool, len(sites))
	out := make([]companySite, 0, len(sites))
	for _, s := range sites {
		key := strings.ToLower(strings.TrimRight(strings.TrimSpace(s.Website), "/"))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// splitLines splits text into lines, trimming surrounding whitespace (incl. \r).
func splitLines(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = strings.TrimSpace(l)
	}
	return out
}
