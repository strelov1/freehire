package main

import (
	"encoding/csv"
	"fmt"
	"io"
)

// inventoryRow is one row of an ATS-company inventory CSV: a company name, the source
// inventory's own slug (unused — freehire derives its own board id from url), and the
// company's public careers URL.
type inventoryRow struct {
	Name string
	Slug string
	URL  string
}

// requiredInventoryColumns are the header columns parseInventory must find. Order in the
// input file does not matter, extra columns are ignored, and each is looked up by name.
// "slug" is deliberately absent here: it is never read downstream (see inventoryRow's doc
// comment), so an inventory that carries no slug column of its own (e.g. Phenom's) must
// still parse.
var requiredInventoryColumns = []string{"name", "url"}

// parseInventory reads a `name,slug,url` CSV into rows — "slug" is optional. It returns an
// error — and no rows — when the header is missing a required column or the body is not
// valid CSV (inconsistent field count, unterminated quote).
func parseInventory(r io.Reader) ([]inventoryRow, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}
	for _, name := range requiredInventoryColumns {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("missing required column %q", name)
		}
	}
	slugIdx, hasSlug := col["slug"]

	var rows []inventoryRow
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse row: %w", err)
		}
		row := inventoryRow{
			Name: record[col["name"]],
			URL:  record[col["url"]],
		}
		if hasSlug {
			row.Slug = record[slugIdx]
		}
		rows = append(rows, row)
	}
	return rows, nil
}
