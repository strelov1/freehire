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

type seamlesshiringProber struct{}

var seamlesshiringBoardPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (seamlesshiringProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !seamlesshiringBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	u := "https://" + board + ".seamlesshiring.com/v2/jobs/job-list?limit=1&page=1"
	var body struct {
		StatusCode int    `json:"status_code"`
		Status     string `json:"status"`
		Data       *struct {
			Jobs *struct {
				Data *[]struct {
					Company struct {
						Name string `json:"name"`
					} `json:"company"`
				} `json:"data"`
				Total int `json:"total"`
			} `json:"jobs"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, u, &body); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return "", 0, nil
		}
		return "", 0, err
	}
	if body.Status != "success" || body.StatusCode != 200 || body.Data == nil || body.Data.Jobs == nil || body.Data.Jobs.Data == nil {
		return "", 0, fmt.Errorf("seamlesshiring probe: %s: listing omitted its jobs", board)
	}
	jobs := body.Data.Jobs
	if jobs.Total == 0 {
		return "", 0, nil
	}
	if len(*jobs.Data) == 0 {
		return "", 0, fmt.Errorf("seamlesshiring probe: %s: total %d with no rows", board, jobs.Total)
	}
	return strings.TrimSpace((*jobs.Data)[0].Company.Name), jobs.Total, nil
}

func (seamlesshiringProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.seamlesshiring.com", recognizedBoard("seamlesshiring"))
}
