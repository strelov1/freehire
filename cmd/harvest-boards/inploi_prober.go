package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type inploiProber struct{}

var (
	inploiBoardPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)
	inploiKeyPattern     = regexp.MustCompile(`\bpk_[A-Za-z0-9_-]{12,}\b`)
	inploiSegmentPattern = regexp.MustCompile(`(?i)\\?"segment_ids\\?"\s*,\s*\\?"segment\\?"\s*,\s*\\?"(\d+)\\?"`)
	inploiTitlePattern   = regexp.MustCompile(`(?i)<title>([^<]*)</title>`)
	inploiAtPattern      = regexp.MustCompile(`(?i) at `)
)

func (inploiProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !inploiBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	page := "https://www.inploi.com/company/" + url.PathEscape(board) + "/search"
	body, err := c.GetText(ctx, page)
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return "", 0, nil
		}
		return "", 0, err
	}
	key := inploiKeyPattern.FindString(body)
	seg := inploiSegmentPattern.FindStringSubmatch(body)
	if key == "" || len(seg) != 2 {
		return "", 0, fmt.Errorf("inploi probe: %s: search page did not name its key and segment", board)
	}
	u := fmt.Sprintf("https://api.inploi.com/search/results?filters[segment_ids][0]=%s&query=&page=1&per_page=1", url.QueryEscape(seg[1]))
	var resp struct {
		Data       json.RawMessage `json:"data"`
		Pagination *struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := c.GetJSONWithHeaders(ctx, u, map[string]string{"x-publishable-key": key}, &resp); err != nil {
		return "", 0, err
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" || resp.Pagination == nil {
		return "", 0, fmt.Errorf("inploi probe: %s: listing omitted its data", board)
	}
	if resp.Pagination.Total == 0 {
		return "", 0, nil
	}
	return inploiCompanyName(body), resp.Pagination.Total, nil
}

func inploiCompanyName(page string) string {
	m := inploiTitlePattern.FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	title := strings.TrimSpace(html.UnescapeString(m[1]))
	at := inploiAtPattern.FindStringIndex(title)
	if at == nil {
		return ""
	}
	return strings.TrimSpace(strings.Split(title[at[1]:], "|")[0])
}

func (inploiProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "www.inploi.com/company/*", recognizedBoard("inploi"))
}
