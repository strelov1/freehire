package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// hrofficeProber validates an HR Office career site (the <board> of middendelfland.inhroffice.com)
// by counting the vacancies of the listing the adapter reads, /api/nl/vacancies; the employer is
// the home page's og:site_name. An unknown or switched-off label answers the listing with HTTP
// 500 and a "MiniJobPortalNotFoundException" body, which is the skip: a live site has never
// answered 500.
type hrofficeProber struct{}

func (hrofficeProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !sources.HROfficeBoard.MatchString(board) {
		return "", 0, nil
	}
	host := "https://" + board + ".inhroffice.com"
	var vacancies []struct {
		ID int64 `json:"id"`
	}
	if err := c.GetJSON(ctx, host+"/api/nl/vacancies", &vacancies); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusInternalServerError || se.Code == http.StatusNotFound) {
			return "", 0, nil
		}
		return "", 0, err
	}
	if len(vacancies) == 0 {
		return "", 0, nil
	}
	name := ""
	if root, err := c.GetHTML(ctx, host+"/"); err == nil {
		name = strings.TrimSpace(metaProperty(root, "og:site_name"))
	}
	return name, len(vacancies), nil
}

// discover enumerates candidate sites from Common Crawl's index of *.inhroffice.com.
func (hrofficeProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.inhroffice.com", recognizedBoard("hroffice"))
}
