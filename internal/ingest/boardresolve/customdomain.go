package boardresolve

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
)

var (
	oraclePodHost = regexp.MustCompile(`^[a-z0-9-]+\.fa\.[a-z0-9-]+\.oraclecloud\.com$`)
	oracleSite    = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// resolveCustomDomain reads the vendor metadata that a plain scan of links misses.
// Oracle needs its real API pod and site number. Odoo crawls /jobs on the exact host.
func resolveCustomDomain(body, rawURL string) (source, board, canonical string, ok bool) {
	if !strings.Contains(body, "data-apibaseurl") && !strings.Contains(body, "web.layout.odooscript") {
		return "", "", "", false
	}

	var odooScript, odooRecruitment bool
	tokens := html.NewTokenizer(strings.NewReader(body))
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			break
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		tag := tokens.Token()
		var apiBaseURL, siteNumber string
		for _, attr := range tag.Attr {
			switch attr.Key {
			case "data-apibaseurl":
				apiBaseURL = attr.Val
			case "data-sitenumber":
				siteNumber = attr.Val
			case "id":
				odooScript = odooScript || tag.Data == "script" && attr.Val == "web.layout.odooscript"
			case "class":
				classes := strings.Fields(attr.Val)
				odooRecruitment = odooRecruitment || slices.Contains(classes, "js_hr_recruitment") ||
					slices.Contains(classes, "o_website_hr_recruitment_jobs_list")
			}
		}
		if tag.Data == "base" && oracleSite.MatchString(siteNumber) {
			if pod, err := url.Parse(apiBaseURL); err == nil && pod.Scheme == "https" &&
				pod.User == nil && (pod.Port() == "" || pod.Port() == "443") &&
				oraclePodHost.MatchString(strings.ToLower(pod.Hostname())) {
				oracleURL := "https://" + strings.ToLower(pod.Hostname()) + "/hcmUI/CandidateExperience/en/sites/" + siteNumber
				return atsboard.Recognize(oracleURL)
			}
		}
	}
	if odooScript && odooRecruitment {
		if u, err := url.Parse(rawURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.Port() == "" {
			host := strings.ToLower(u.Hostname())
			return "odoo", host, "https://" + host + "/jobs", true
		}
	}
	return "", "", "", false
}
