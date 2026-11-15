package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
)

// A Gupy board is a career-page subdomain (<page>.gupy.io), the key sources.gupy lists by.
const (
	// gupyListURL lists one career page's published postings (mirrors sources.gupyJobsURL,
	// which is unexported; this tool lives outside the sources package).
	gupyListURL = "https://private-api.gupy.io/job-publication/public/jobs"
	// gupyCareerPageURL describes one career page, including the name it publishes under.
	gupyCareerPageURL = "https://private-api.gupy.io/v2/public-career-page/"
	// gupyFeedURL is the multi-employer job search behind portal.gupy.io — the only public
	// list that spans career pages, and so the discovery source.
	gupyFeedURL = "https://portal.gupy.io/api/job-search/jobs"
)

const (
	// gupyPageSize is the discovery page size for the portal feed.
	gupyPageSize = 100
	// gupyMaxOffset is where the portal feed ends: offset 9900 still answers a full page and 10000
	// answers nothing, so one sweep sees the newest 10,000 postings, not every page Gupy hosts.
	gupyMaxOffset = 10000
)

// gupyProber probes and discovers Gupy career pages. There is no public seed list, so the
// prober also discovers its own candidates from the portal feed (see discover).
type gupyProber struct{}

// probe counts one career page's published postings and reads the name it publishes under.
// A missing page (Gupy answers 404 "Career Page not Found") or one with no postings yields
// ("", 0, nil) — a skip — and the name falls back to the subdomain when the page states none.
func (gupyProber) probe(ctx context.Context, c httpClient, page string) (string, int, error) {
	var list struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, gupyListURL+"?subdomain="+url.QueryEscape(page), &list); err != nil {
		return "", 0, nil
	}
	if len(list.Data) == 0 {
		return "", 0, nil
	}
	name := page
	var cp struct {
		CareerPage struct {
			Name            string `json:"name"`
			PublicationName string `json:"publicationName"`
		} `json:"careerPage"`
	}
	if err := c.GetJSON(ctx, gupyCareerPageURL+url.PathEscape(page), &cp); err == nil {
		for _, n := range []string{cp.CareerPage.PublicationName, cp.CareerPage.Name} {
			if n = strings.TrimSpace(n); n != "" {
				name = n
				break
			}
		}
	}
	return name, len(list.Data), nil
}

// gupyJobHost captures the career-page subdomain of a portal job URL
// (https://caju.gupy.io/job/<token>?jobBoardSource=gupy_portal).
var gupyJobHost = regexp.MustCompile(`(?i)^https?://([a-z0-9_-]+)\.gupy\.io/`)

// discover pages the portal feed collecting each posting's distinct career page in first-seen
// order, until a page returns no postings or the offset reaches gupyMaxOffset. A page that
// fails to fetch ends the sweep with the pages gathered so far, so one bad page truncates
// rather than aborts the harvest.
func (gupyProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	var pages []string
	seen := make(map[string]bool)
	for offset := 0; offset < gupyMaxOffset; offset += gupyPageSize {
		var resp struct {
			Data []struct {
				JobURL string `json:"jobUrl"`
			} `json:"data"`
		}
		feed := fmt.Sprintf("%s?limit=%d&offset=%d", gupyFeedURL, gupyPageSize, offset)
		if err := c.GetJSON(ctx, feed, &resp); err != nil {
			// A page failing mid-sweep truncates rather than aborts; log so a short harvest
			// is not mistaken for an exhausted feed.
			log.Printf("gupy discover: offset %d: %v (returning %d pages so far)", offset, err, len(pages))
			break
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, j := range resp.Data {
			m := gupyJobHost.FindStringSubmatch(j.JobURL)
			if m == nil {
				continue
			}
			if page := strings.ToLower(m[1]); !seen[page] {
				seen[page] = true
				pages = append(pages, page)
			}
		}
	}
	return pages, nil
}
