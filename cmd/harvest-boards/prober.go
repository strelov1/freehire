package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/strelov1/freehire/internal/sources"
)

// errMissing is the sentinel a test getter returns for an unmapped URL. In production the
// real client returns its own transport error for a missing board, treated identically.
var errMissing = errors.New("not found")

// greenhouseBoardsAPI is the public boards API root (mirrors sources.greenhouseBaseURL,
// which is unexported; this tool lives outside the sources package).
const greenhouseBoardsAPI = "https://boards-api.greenhouse.io/v1/boards"

// prober checks one candidate board on its ATS platform, returning the company name the
// platform reports and the number of open jobs. A board that is absent, closed, or
// unreachable yields ("", 0, nil) — a skip, never a fatal error — so one dead candidate
// cannot abort the harvest. A non-nil error is reserved for failures a prober genuinely
// wants surfaced (the caller logs and skips those too).
type prober interface {
	probe(ctx context.Context, c sources.JSONGetter, slug string) (company string, openJobs int, err error)
}

// greenhouseProber probes the Greenhouse public boards API. The jobs endpoint lists only
// live postings, so a non-empty list means a live board. The company name comes from the
// board-metadata endpoint, fetched only once a board is known to have jobs.
type greenhouseProber struct{}

func (greenhouseProber) probe(ctx context.Context, c sources.JSONGetter, slug string) (string, int, error) {
	var jr struct {
		Jobs []struct {
			ID int64 `json:"id"`
		} `json:"jobs"`
	}
	// A missing/moved board returns 4xx and the client surfaces it as an error. For harvest
	// that simply means "not a live greenhouse board" — skip silently, do not propagate.
	if err := c.GetJSON(ctx, fmt.Sprintf("%s/%s/jobs", greenhouseBoardsAPI, slug), &jr); err != nil {
		return "", 0, nil
	}
	if len(jr.Jobs) == 0 {
		return "", 0, nil
	}
	var meta struct {
		Name string `json:"name"`
	}
	_ = c.GetJSON(ctx, fmt.Sprintf("%s/%s", greenhouseBoardsAPI, slug), &meta)
	name := meta.Name
	if name == "" {
		name = slug
	}
	return name, len(jr.Jobs), nil
}

// leverProber probes the Lever postings API. The JSON-mode endpoint returns a bare array
// of live postings, so a non-empty array is a live board. Lever exposes no company name, so
// the name falls back to the slug.
type leverProber struct{}

func (leverProber) probe(ctx context.Context, c sources.JSONGetter, slug string) (string, int, error) {
	var postings []struct {
		ID string `json:"id"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", slug), &postings); err != nil {
		return "", 0, nil
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	return slug, len(postings), nil
}

// ashbyProber probes the Ashby public job-board API. The list endpoint returns the live
// postings, so a non-empty list is a live board; the name falls back to the (case-sensitive)
// slug, which Ashby itself uses as the board identity.
type ashbyProber struct{}

func (ashbyProber) probe(ctx context.Context, c sources.JSONGetter, slug string) (string, int, error) {
	var resp struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Jobs) == 0 {
		return "", 0, nil
	}
	return slug, len(resp.Jobs), nil
}

// bamboohrProber probes the BambooHR per-subdomain careers list. A non-empty result is a
// live board; the name falls back to the slug (the subdomain), as the list carries none.
type bamboohrProber struct{}

func (bamboohrProber) probe(ctx context.Context, c sources.JSONGetter, slug string) (string, int, error) {
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.bamboohr.com/careers/list", slug), &list); err != nil {
		return "", 0, nil
	}
	if len(list.Result) == 0 {
		return "", 0, nil
	}
	return slug, len(list.Result), nil
}

// probers maps a provider key to its prober. Adding an ATS is one entry here plus the
// prober type — the same shape as sources.All.
var probers = map[string]prober{
	"greenhouse": greenhouseProber{},
	"lever":      leverProber{},
	"ashby":      ashbyProber{},
	"bamboohr":   bamboohrProber{},
}
