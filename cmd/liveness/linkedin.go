package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const (
	linkedInGuestPosting = "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"
	// linkedInProxyTimeout is the proxied client's own timeout (linkedInClient).
	linkedInProxyTimeout = 60 * time.Second
)

// linkedInClient routes the LinkedIn pass through the Scrapling tier's rotating proxy, the way
// the crawl reaches LinkedIn (sources.scraplingProviders): a direct request from the crawl host
// is refused (two of four runs before 2026-11-22 stopped on their first probe), and the pass
// then could not keep up with the ~5k crawled postings a day that drop out of the search slice,
// most of them closed. Without SCRAPLING_FETCH_URL it keeps the direct client and its old budget.
func linkedInClient(direct *http.Client) (*http.Client, bool) {
	raw := strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_URL"))
	if raw == "" {
		return direct, false
	}
	tr, err := scraplingfetch.New(scraplingfetch.Config{URL: raw, Token: strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_TOKEN"))},
		scraplingfetch.ModeHTTP, true, linkedInProxyTimeout)
	if err != nil {
		log.Printf("liveness: linkedin stays direct: %v", err)
		return direct, false
	}
	return &http.Client{Transport: tr}, true
}

// LinkedIn's guest search is only a recent slice, so absence is never a removal. The job's own
// explicit closed notice is evidence even though it returns HTTP 200, and so is a 404/410 for the
// guest posting URL itself (an unknown id answers 404; every listed posting sampled answered 200).
// Through the proxy each request leaves from a new exit IP, so a refusal names one throttled exit
// and is retried; only a refusal on every attempt stops the pass.
func linkedInPass(client *http.Client, proxied bool) pacedPass {
	p := pacedPass{source: "linkedin", interval: 2 * time.Second, maxPerRun: 300}
	attempts, timeout := 1, probeTimeout
	if proxied {
		// The fetch tier retries a stalled exit itself (up to ~40 s each); a shorter deadline here
		// would cancel that recovery and turn it into an uncertain verdict.
		p.interval, attempts, timeout = time.Second, 3, linkedInProxyTimeout
		p.maxPerRun = int(worker.EnvInt32OrDefault("LINKEDIN_LIVENESS_MAX_PER_RUN", 6000))
	}
	p.check = func(ctx context.Context, c candidate) pacedVerdict {
		id := sources.LinkedInPostingID(c.url)
		if id == "" {
			return pacedVerdict{verdict: liveness.Uncertain}
		}
		var v pacedVerdict
		for range attempts {
			if v = linkedInProbe(ctx, client, id, timeout); !v.refused {
				break
			}
		}
		return v
	}
	return p
}

func linkedInProbe(ctx context.Context, client *http.Client, id string, timeout time.Duration) pacedVerdict {
	unknown := pacedVerdict{verdict: liveness.Uncertain}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, linkedInGuestPosting+id, nil)
	if err != nil {
		return unknown
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return unknown
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusTooManyRequests, 999:
		unknown.refused = true
		return unknown
	case http.StatusNotFound, http.StatusGone:
		// Only the posting's own URL answering gone counts; a redirect elsewhere proves nothing.
		if resp.Request.URL.String() == linkedInGuestPosting+id {
			return pacedVerdict{verdict: liveness.Expired, reason: "linkedin_post_gone"}
		}
		return unknown
	default:
		return unknown
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return unknown
	}
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return unknown
	}
	job, err := sources.LinkedInPosting(root, resp.Request.URL.String(), id)
	if err != nil {
		return unknown
	}
	if job.Removed {
		return pacedVerdict{verdict: liveness.Expired, reason: "linkedin_post_closed"}
	}
	return pacedVerdict{verdict: liveness.Live}
}
