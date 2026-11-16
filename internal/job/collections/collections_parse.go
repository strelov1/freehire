package collections

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ParseSlugList parses a newline-delimited slug list (the embedded eastern_roots.txt
// and indian_roots.txt): one entry per line, blank lines and #-comment lines skipped,
// surrounding whitespace trimmed. Entries are returned verbatim (Collection.Members
// normalizes them).
func ParseSlugList(data []byte) ([]string, error) {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ParseEUStartups extracts the company names from the icp-radar European-startups
// dataset (a JSON array of company objects with a "Name" field).
func ParseEUStartups(data []byte) ([]string, error) {
	var raw []euStartup
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("collections: parse european dataset: %w", err)
	}
	names := make([]string, 0, len(raw))
	for _, c := range raw {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return names, nil
}

// ParseYC reads a yc-oss dataset payload (a JSON array of company objects) into
// records carrying each company's website. Companies YC lists as Inactive or Acquired are
// dropped, for the reason every portfolio drops its exits (see portfolios.go): their names
// would match whatever later company took them.
func ParseYC(data []byte) ([]Record, error) {
	var raw []ycCompany
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("collections: parse yc dataset: %w", err)
	}
	out := make([]Record, 0, len(raw))
	for _, c := range raw {
		if c.Name == "" || c.Status == "Inactive" || c.Status == "Acquired" {
			continue
		}
		r := record(c.Name, c.Website)
		if c.TeamSize > 0 {
			r.Meta[MetaTeamSize] = strconv.Itoa(c.TeamSize)
		}
		out = append(out, r)
	}
	return out, nil
}

// parseCSVColumn extracts the named column from a CSV with the given delimiter: it
// locates the column by header, so an upstream reorder cannot silently read the wrong field,
// and returns each non-empty value.
func parseCSVColumn(data []byte, delim rune, colName string) ([]string, error) {
	rows, err := csvColumns(data, delim, colName)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if row[0] != "" {
			names = append(names, row[0])
		}
	}
	return names, nil
}

// csvColumns extracts the named columns from a CSV with the given delimiter,
// returning one trimmed slice per data row in the order the columns were asked for.
// Columns are located by header rather than by index, so an upstream reorder cannot
// silently read the wrong field, and a missing column is an error rather than a
// column of blanks.
func csvColumns(data []byte, delim rune, colNames ...string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.Comma = delim
	r.FieldsPerRecord = -1 // tolerate ragged rows rather than aborting the whole parse
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("collections: read csv header: %w", err)
	}

	idx := make([]int, len(colNames))
	for n, want := range colNames {
		idx[n] = -1
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), want) {
				idx[n] = i
				break
			}
		}
		if idx[n] < 0 {
			return nil, fmt.Errorf("collections: csv has no %q column", want)
		}
	}

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("collections: read csv: %w", err)
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		vals := make([]string, len(idx))
		for n, i := range idx {
			if i < len(row) {
				vals[n] = strings.TrimSpace(row[i])
			}
		}
		out = append(out, vals)
	}
	return out, nil
}
