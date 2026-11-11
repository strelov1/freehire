package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// These boardless aggregators do not claim fullCatalog: a company leaving their
// listing falls outside the company-scoped unseen sweep. Reed reads a bounded
// keyword union and may skip failed keywords, so it has the same scope gap. The November 2026 audit
// confirmed removed postings return 404/410 on their own stored URLs. Keep search's
// unseen withholding until the existing backlog has been worked off.
func catalogStatusPasses(client *http.Client) []pacedPass {
	var passes []pacedPass
	for _, site := range []struct{ source, host, path string }{
		{"instaffo", "jobs.instaffo.com", "/de/job/"},
		{"remotedotcom", "remote.com", "/jobs/"},
		{"thehub", "thehub.io", "/jobs/"},
		{"reed", "www.reed.co.uk", "/jobs/"},
	} {
		passes = append(passes, pacedPass{
			source: site.source, interval: 2 * time.Second, maxPerRun: 500,
			check: func(ctx context.Context, c candidate) pacedVerdict {
				u, err := url.Parse(c.url)
				if err != nil || u.Host != site.host || u.Scheme != "https" || !strings.HasPrefix(u.Path, site.path) || u.Path == site.path {
					return pacedVerdict{verdict: liveness.Uncertain}
				}
				return checkCatalogStatus(ctx, client, c)
			},
		})
	}
	return passes
}

// Status is evidence only when it belongs to the requested posting. A redirect
// onto a listing, login, or another host says nothing about this posting, even if
// the destination returns 404. Employer prose is deliberately never classified.
func checkCatalogStatus(ctx context.Context, client *http.Client, c candidate) pacedVerdict {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("liveness: probe %s failed: %v", c.publicSlug, err)
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	}
	if !strings.EqualFold(req.URL.Host, resp.Request.URL.Host) || req.URL.EscapedPath() != resp.Request.URL.EscapedPath() {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return pacedVerdict{verdict: liveness.Expired, reason: "http_gone"}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return pacedVerdict{verdict: liveness.Live}
	default:
		return pacedVerdict{verdict: liveness.Uncertain}
	}
}
