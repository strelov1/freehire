package main

import (
	"context"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// cvwarehouseProber validates a CVWarehouse career site (the <board> of
// jobpage.cvwarehouse.com/?companyGuid=<board>) by counting the vacancies its pages list, the
// home page's and, for a sectioned site, those of every section it links. The employer is the
// page's <meta name="author">. It mirrors internal/ingest/sources/cvwarehouse.go, which is
// unexported. An unknown companyGuid answers 200 with the platform's "not available" page, whose
// <body> is not class "home"; that is the skip.
type cvwarehouseProber struct{}

const cvwarehouseHome = "https://jobpage.cvwarehouse.com/?companyGuid="

var (
	cvwarehouseBoardPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	cvwarehouseCareerPage   = regexp.MustCompile(`<body[^>]*\bclass="home\b`)
	cvwarehouseListedJob    = regexp.MustCompile(`class="jobLink"[^>]*\bdata-jobid="([0-9]+)"`)
	cvwarehouseSectionLink  = regexp.MustCompile(`href="/\?companyGuid=([0-9a-fA-F-]{36})[^"]*?section=([0-9a-f-]{36})"`)
	cvwarehouseAuthor       = regexp.MustCompile(`<meta name="author" content="([^"]*)"`)
)

func (cvwarehouseProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(board)
	if !cvwarehouseBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	body, err := c.GetText(ctx, cvwarehouseHome+board)
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	if !cvwarehouseCareerPage.MatchString(body) {
		return "", 0, nil
	}
	name := ""
	if m := cvwarehouseAuthor.FindStringSubmatch(body); m != nil {
		name = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	jobs := map[string]bool{}
	for _, m := range cvwarehouseListedJob.FindAllStringSubmatch(body, -1) {
		jobs[m[1]] = true
	}
	sections := map[string]bool{}
	for _, m := range cvwarehouseSectionLink.FindAllStringSubmatch(body, -1) {
		if strings.ToLower(m[1]) != board || sections[m[2]] {
			continue
		}
		sections[m[2]] = true
		page, err := c.GetText(ctx, cvwarehouseHome+board+"&section="+m[2])
		if err != nil {
			return "", 0, err
		}
		for _, j := range cvwarehouseListedJob.FindAllStringSubmatch(page, -1) {
			jobs[j[1]] = true
		}
	}
	return name, len(jobs), nil
}

// discover enumerates candidate companyGuids from Common Crawl's index of the career-site host.
func (cvwarehouseProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "jobpage.cvwarehouse.com/*", recognizedBoard("cvwarehouse"))
}
