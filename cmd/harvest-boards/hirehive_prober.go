package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

type hirehiveProber struct{}

var hirehiveBoardPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (hirehiveProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !hirehiveBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	q := url.Values{}
	q.Set("page", "1")
	q.Set("page_size", "1")
	u := "https://" + board + ".hirehive.com/api/v2/jobs?" + q.Encode()
	var body struct {
		Meta struct {
			TotalItems *int `json:"total_items"`
		} `json:"meta"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := c.GetJSON(ctx, u, &body); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return "", 0, nil
		}
		return "", 0, err
	}
	if body.Meta.TotalItems == nil || body.Items == nil {
		return "", 0, fmt.Errorf("hirehive probe: %s: listing omitted its count", board)
	}
	if *body.Meta.TotalItems == 0 {
		return "", 0, nil
	}
	if *body.Meta.TotalItems < 0 {
		return "", 0, fmt.Errorf("hirehive probe: %s: stated total %d", board, *body.Meta.TotalItems)
	}
	page, err := c.GetText(ctx, "https://"+board+".hirehive.com/")
	if err != nil {
		return "", *body.Meta.TotalItems, nil
	}
	return hirehiveEmployer(page), *body.Meta.TotalItems, nil
}

func hirehiveEmployer(page string) string {
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}
	title := strings.TrimSpace(pageTitle(root))
	_, rest, ok := strings.Cut(title, "|")
	if ok {
		title = strings.TrimSpace(rest)
	}
	name, ok := strings.CutPrefix(title, "Jobs at ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}

func (hirehiveProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.hirehive.com", recognizedBoard("hirehive"))
}
