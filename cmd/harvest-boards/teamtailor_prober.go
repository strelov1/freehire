package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
)

// teamtailorProber probes a Teamtailor career site, whose board is the host itself (vendor
// sub-domain or the employer's own). There is no JSON feed, so liveness is judged like the adapter:
// a live board links ≥1 posting permalink. The employer is og:site_name, which every site sets to
// the company, so a name-derived seed is gated against it.
type teamtailorProber struct{}

// boardID turns a seed token into a host. A bare company name can only predict the vendor
// sub-domain; a token containing a dot already names a host and is kept.
func (teamtailorProber) boardID(token string) string {
	if strings.Contains(token, ".") {
		return token
	}
	return token + ".teamtailor.com"
}

// teamtailorJobPattern is the posting permalink /jobs/<numeric id>. It must not match the bare
// /jobs nav anchor, which every listing carries.
var teamtailorJobPattern = regexp.MustCompile(`/jobs/\d+`)

func (teamtailorProber) probe(ctx context.Context, c httpClient, host string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s/jobs?page=1", host))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, teamtailorJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	// A vendor sub-domain often redirects to the employer's own domain (tibber.teamtailor.com serves
	// jobs.tibber.com); kept, that is the SAME board under a second id and ingest would store every
	// posting twice. og:url names the host that served the page; a board is only this host if it says
	// so. The redirect target is logged in a fixed shape so a second run can probe it under its own id.
	if u, err := url.Parse(metaProperty(root, "og:url")); err == nil && u.Host != "" && !strings.EqualFold(u.Host, host) {
		log.Printf("harvest-boards: teamtailor %s serves %s", host, strings.ToLower(u.Host))
		return "", 0, nil
	}
	return metaProperty(root, "og:site_name"), n, nil
}
