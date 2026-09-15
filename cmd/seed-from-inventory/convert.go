package main

import "github.com/strelov1/freehire/internal/ingest/atsboard"

// seedItem is one candidate board entry, in the shape cmd/harvest-boards's own seed loader
// (cmd/harvest-boards/seed.go) already accepts.
type seedItem struct {
	Board   string `json:"board"`
	Company string `json:"company"`
}

// convert resolves each row's url via atsboard.Recognize into a (provider, board) pair,
// groups the results by provider, and deduplicates identical (provider, board) pairs within
// a provider's group, keeping the first row's company name. A row atsboard cannot resolve
// (a vanity domain, or a custom-domain ATS) is skipped and counted in unrecognized rather
// than aborting the run.
func convert(rows []inventoryRow) (byProvider map[string][]seedItem, unrecognized int) {
	byProvider = map[string][]seedItem{}
	seenBoards := map[string]map[string]bool{}

	for _, row := range rows {
		source, board, _, ok := atsboard.Recognize(row.URL)
		if !ok {
			unrecognized++
			continue
		}
		if seenBoards[source] == nil {
			seenBoards[source] = map[string]bool{}
		}
		if seenBoards[source][board] {
			continue
		}
		seenBoards[source][board] = true
		byProvider[source] = append(byProvider[source], seedItem{Board: board, Company: row.Name})
	}
	return byProvider, unrecognized
}
