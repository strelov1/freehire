package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

var catalogTenant = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// A successful company-scoped listing can lose a company's last posting, while
// partial, capped, or currently blocked listings cannot close unseen jobs safely.
// These sources have verified own-posting 404/410 controls. Keep search's unseen
// withholding until their existing backlogs have been worked off. Gradcracker
// and Engage retain the adapters' measured request intervals.
func catalogStatusPasses(client *http.Client) []pacedPass {
	var passes []pacedPass
	for _, site := range []struct {
		source, host, path string
		resource           *regexp.Regexp
		interval           time.Duration
	}{
		{"instaffo", "jobs.instaffo.com", "/de/job/", nil, 2 * time.Second},
		{"remotedotcom", "remote.com", "/jobs/", nil, 2 * time.Second},
		{"thehub", "thehub.io", "/jobs/", nil, 2 * time.Second},
		{"reed", "www.reed.co.uk", "/jobs/", nil, 2 * time.Second},
		{"arbeitsagentur", "www.arbeitsagentur.de", "", regexp.MustCompile(`^/jobsuche/jobdetail/([0-9]+-[A-Za-z0-9_]+-[A-Z])/?$`), 2 * time.Second},
		{"xing", "www.xing.com", "", regexp.MustCompile(`^/jobs/[a-z0-9-]+-([0-9]+)/?$`), 2 * time.Second},
		{"gradcracker", "www.gradcracker.com", "", regexp.MustCompile(`^/hub/[0-9]+/[a-z0-9-]+/(?:graduate-job|work-placement-internship|degree-apprenticeship)/([0-9]+)/[a-z0-9-]+/?$`), 10 * time.Second},
		{"engage", "en-gage.net", "", regexp.MustCompile(`^/[A-Za-z0-9_-]+/work_([0-9]+)/$`), 3500 * time.Millisecond},
		{"betterteam", "", "", regexp.MustCompile(`^/([^/]+)/?$`), 2 * time.Second},
	} {
		passes = append(passes, pacedPass{
			source: site.source, interval: site.interval, maxPerRun: 500,
			check: func(ctx context.Context, c candidate) pacedVerdict {
				u, err := url.Parse(c.url)
				if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
					return pacedVerdict{verdict: liveness.Uncertain}
				}
				if site.source == "betterteam" {
					board, _, namespaced := strings.Cut(c.externalID, ":")
					tenant := strings.TrimSuffix(u.Host, ".betterteam.com")
					if tenant == u.Host || !catalogTenant.MatchString(tenant) || (namespaced && board != tenant) {
						return pacedVerdict{verdict: liveness.Uncertain}
					}
				} else if u.Host != site.host {
					return pacedVerdict{verdict: liveness.Uncertain}
				}
				if site.resource != nil {
					native := c.externalID[strings.LastIndex(c.externalID, ":")+1:]
					match := site.resource.FindStringSubmatch(u.Path)
					if len(match) != 2 || native == "" || match[1] != native {
						return pacedVerdict{verdict: liveness.Uncertain}
					}
				} else if !strings.HasPrefix(u.Path, site.path) || u.Path == site.path {
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
