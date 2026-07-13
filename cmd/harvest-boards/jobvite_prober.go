package main

import (
	"context"
	"fmt"
	"regexp"
)

// jobviteJobLinkPattern matches a Jobvite job permalink's /<board>/job/<code> segment. The
// /<board>/jobs listing and /<board>/jobAlerts nav links carry no /job/<code> segment and are
// not counted. Mirrors sources.jobviteJobIDPattern (unexported; this tool lives outside the
// package).
var jobviteJobLinkPattern = regexp.MustCompile(`/job/[A-Za-z0-9]+(?:[/?#]|$)`)

// jobviteProber probes a Jobvite careersite. Jobvite exposes no public JSON list, so liveness
// is judged from the server-rendered listing HTML: a live board links ≥1 job permalink. The
// page carries no reliable company name, so it falls back to the slug (the seed supplies the
// company). Best-effort: a fetch error counts the board as not live rather than aborting.
type jobviteProber struct{}

func (jobviteProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://jobs.jobvite.com/%s/jobs", slug))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, jobviteJobLinkPattern)
	if n == 0 {
		return "", 0, nil
	}
	return slug, n, nil
}
