package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/liveness"
)

// Dice lists only the last day. Probe older listings at the crawler's pace and
// cap this pass at the crawler's 500-detail-request budget.
func dicePass(client *http.Client) pacedPass {
	return pacedPass{
		source:    "dice",
		interval:  time.Second,
		maxPerRun: 500,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			return checkDiceLive(ctx, client, c)
		},
	}
}

func checkDiceLive(ctx context.Context, client *http.Client, c candidate) pacedVerdict {
	unknown := pacedVerdict{verdict: liveness.Uncertain}
	// The ingest pipeline namespaces native IDs by board, even for a boardless source.
	id := c.externalID[strings.LastIndex(c.externalID, ":")+1:]
	if _, err := uuid.Parse(id); len(id) != 36 || err != nil {
		return unknown
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.dice.com/job-detail/"+id, nil)
	if err != nil {
		return unknown
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("liveness: Dice probe %s failed: %v", c.publicSlug, err)
		return unknown
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		unknown.refused = true
		return unknown
	}
	final := resp.Request.URL
	if final.Scheme != req.URL.Scheme || final.Host != req.URL.Host || final.EscapedPath() != req.URL.EscapedPath() {
		return unknown
	}
	switch resp.StatusCode {
	case http.StatusGone:
		// Removed pages can still carry JobPosting markup.
		return pacedVerdict{verdict: liveness.Expired, reason: "http_gone"}
	case http.StatusOK:
		const bodyLimit = 2 << 20
		body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
		if err != nil || len(body) > bodyLimit {
			return unknown
		}
		root, err := html.Parse(bytes.NewReader(body))
		if err == nil && sources.LDJobPosting(root, &struct{}{}) {
			return pacedVerdict{verdict: liveness.Live}
		}
	}
	return unknown
}
