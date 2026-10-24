package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// flatchrProber validates a Flatchr board (the <board> of careers.flatchr.io/<lang>/company/
// <board>) by reading the board document the careers site itself loads, and counting its
// published vacancies. It mirrors internal/ingest/sources/flatchr.go, which is unexported.
//
// A board can be a holding's careers site listing its subsidiaries' vacancies, so the employer
// name is taken only from a vacancy filed under the board's own company — a subsidiary's name
// would reject a live board on the name gate — and left to the seed otherwise.
type flatchrProber struct{}

var flatchrBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (flatchrProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !flatchrBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var doc struct {
		Items *[]struct {
			Status    string `json:"status"`
			Published bool   `json:"published"`
			Vacancy   struct {
				Company struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"company"`
			} `json:"vacancy"`
		} `json:"items"`
	}
	docURL := "https://careers.flatchr.io/company/" + board + ".json"
	if err := c.GetJSON(ctx, docURL, &doc); err != nil {
		// An unknown board answers 404. A switched-off careers site answers 200 with an HTML
		// fragment, which fails the decode; the same address read as text shows which it was,
		// the way the adapter tells it apart. Any other failure is a probe that did not happen.
		var se *sources.StatusError
		if errors.As(err, &se) {
			if se.Code == http.StatusNotFound {
				return "", 0, nil
			}
			return "", 0, err
		}
		if body, terr := c.GetText(ctx, docURL); terr == nil && strings.Contains(body, `class="notFound"`) {
			return "", 0, nil
		}
		return "", 0, err
	}
	// A document without the key is a reshaped answer, not an empty board.
	if doc.Items == nil {
		return "", 0, fmt.Errorf("flatchr probe: %s: document carries no items", board)
	}
	name, open := "", 0
	for _, it := range *doc.Items {
		if !it.Published || it.Status != "published" {
			continue
		}
		open++
		if name == "" && it.Vacancy.Company.Slug == board {
			name = strings.TrimSpace(it.Vacancy.Company.Name)
		}
	}
	return name, open, nil
}

// discover enumerates candidate boards from Common Crawl's index of careers.flatchr.io.
func (flatchrProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "careers.flatchr.io/*", recognizedBoard("flatchr"))
}
