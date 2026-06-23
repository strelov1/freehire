package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// seedItem is one candidate board from a seed file. A seed may be a plain JSON array of
// board tokens (Company empty) or an array of {board, company} objects — the latter lets a
// discovery source that already knows the employer (e.g. harvest-role, which reads it from
// role.com's JSON-LD) supply a name for providers whose own API exposes none (Oracle).
type seedItem struct {
	Board   string `json:"board"`
	Company string `json:"company"`
}

// loadSeedItems reads a seed file in either supported shape: a JSON array of strings or a
// JSON array of {board, company} objects.
func loadSeedItems(path string) ([]seedItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed %s: %w", path, err)
	}
	var strs []string
	if json.Unmarshal(data, &strs) == nil {
		items := make([]seedItem, len(strs))
		for i, s := range strs {
			items[i] = seedItem{Board: s}
		}
		return items, nil
	}
	var items []seedItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse seed %s: %w", path, err)
	}
	return items, nil
}

// chooseCompany picks the board entry's company label. The prober's API-reported name wins
// when it is a real name (not just the board id echoed back by the slug fallback); otherwise
// a seed-supplied company fills in, and the board id is the last resort.
func chooseCompany(proberName, seedCompany, board string) string {
	if proberName != "" && proberName != board {
		return proberName
	}
	if seedCompany != "" {
		return seedCompany
	}
	return board
}
