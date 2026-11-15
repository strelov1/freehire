package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// seedItem is one candidate board from a seed file: a plain board token, or a {board, company}
// object so a discovery source that knows the employer (e.g. harvest-role) can name it for providers
// whose API exposes none (Oracle).
//
// ExpectID names a posting the board is expected to contain, in the platform's id space. The probe
// then identifies the board by that evidence instead of by name resemblance, which makes a slug
// guessed offline safe to propose.
type seedItem struct {
	Board    string `json:"board"`
	Company  string `json:"company"`
	ExpectID string `json:"expect_id"`
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

// chooseCompany picks the board entry's company label. A prober returns "" when the platform
// publishes no employer name, so a non-empty name is always the platform's and wins; then the seed's
// company; the board id is the last resort.
func chooseCompany(proberName, seedCompany, board string) string {
	if proberName != "" {
		return proberName
	}
	if seedCompany != "" {
		return seedCompany
	}
	return board
}
