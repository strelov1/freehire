package sources

import (
	"context"
	"fmt"
	"net/url"

	"golang.org/x/net/html"
)

func linkedInApplication(card *html.Node) (ApplicationMethod, string, bool) {
	var onsite, external, apply bool
	var target string
	walk(card, func(n *html.Node) bool {
		switch Attr(n, "data-tracking-control-name") {
		case "public_jobs_apply-link-onsite":
			onsite, apply = true, true
		case "public_jobs_apply-link-offsite":
			external, apply = true, true
			u, err := url.Parse(Attr(n, "href"))
			if err == nil && u.Scheme == "https" && u.User == nil && u.Hostname() != "" && !linkedInHost(u.Hostname()) {
				target = u.String()
			}
		}
		if Attr(n, "data-modal") == "job-details-topcard-apply-modal" {
			apply = true
			walk(n, func(child *html.Node) bool {
				if Attr(child, "data-svg-class-name") == "apply-button__offsite-apply-icon-svg" {
					external = true
				}
				return true
			})
		}
		return true
	})
	if onsite && external {
		return "", "", apply
	}
	if onsite {
		return ApplyOnSite, "", apply
	}
	if external {
		return ApplyExternal, target, apply
	}
	return "", "", apply
}

// Posting rechecks one stored LinkedIn identity without relying on the bounded
// recent listing. It shares the adapter's ordinary request budget.
func (s linkedIn) Posting(ctx context.Context, raw string) (Job, error) {
	id := LinkedInPostingID(raw)
	if id == "" {
		return Job{}, fmt.Errorf("linkedin: invalid posting identity")
	}
	root, final, err := s.get(ctx, "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"+id)
	if err != nil {
		return Job{}, err
	}
	return LinkedInPosting(root, final, id)
}
