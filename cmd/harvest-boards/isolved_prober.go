package main

import (
	"context"
	"fmt"
	"regexp"
)

// isolvedSitemapJobID captures the numeric posting id from a /jobs/<id> URL in an iSolved
// Hire / ApplicantPro sitemap.
var isolvedSitemapJobID = regexp.MustCompile(`/jobs/(\d+)`)

// isolvedProber probes an iSolved Hire / ApplicantPro tenant (slug = subdomain) by counting
// the distinct /jobs/<id> URLs in its sitemap.xml; a non-zero count is a live board. Host
// distinguishes the two sibling platforms. The employer name comes from the seed.
type isolvedProber struct{ host string }

func (p isolvedProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	body, err := c.GetText(ctx, fmt.Sprintf("https://%s.%s/sitemap.xml", slug, p.host))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]struct{}{}
	for _, m := range isolvedSitemapJobID.FindAllStringSubmatch(body, -1) {
		ids[m[1]] = struct{}{}
	}
	return "", len(ids), nil
}
