package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// workableProber probes the Workable public widget API. A board is the account subdomain;
// the widget endpoint returns the account name and its open jobs, so a non-empty jobs list
// is a live board.
type workableProber struct{}

func (workableProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Name string `json:"name"`
		Jobs []struct {
			Shortcode string `json:"shortcode"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://apply.workable.com/api/v1/widget/accounts/%s?details=true", slug), &resp); err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	if len(resp.Jobs) == 0 {
		return "", 0, nil
	}
	return resp.Name, len(resp.Jobs), nil
}

// storefrontEmployer reads the employer name from a board's storefront page title, trimming the
// boilerplate suffix. Call it only for boards known to have jobs: one request per LIVE board.
// Anything unreadable yields "" (unreachable page, a title that is only the boilerplate, a 404), and
// the board is then kept on liveness, so the name gate cannot fire for it.
func storefrontEmployer(ctx context.Context, c httpClient, url, suffix string) string {
	root, err := c.GetHTML(ctx, url)
	if err != nil {
		return ""
	}
	title := strings.TrimSpace(pageTitle(root))
	if suffix != "" {
		trimmed, ok := strings.CutSuffix(title, suffix)
		if !ok {
			return ""
		}
		title = trimmed
	}
	return strings.TrimSpace(title)
}

// smartRecruitersProber probes the SmartRecruiters public postings API. The listing carries
// totalFound, so one limit=1 page settles liveness; the company name comes from the
// company-metadata endpoint, fetched only once a board is known to have jobs.
type smartRecruitersProber struct{}

func (smartRecruitersProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var page struct {
		TotalFound int `json:"totalFound"`
		Content    []struct {
			ID string `json:"id"`
		} `json:"content"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings?limit=1", slug), &page); err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	n := page.TotalFound
	if n == 0 {
		n = len(page.Content)
	}
	if n == 0 {
		return "", 0, nil
	}
	var meta struct {
		Name string `json:"name"`
	}
	_ = c.GetJSON(ctx, fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s", slug), &meta)
	return meta.Name, n, nil
}

// dedupKey folds case: SmartRecruiters' API treats "archirodongroup" and "ArchirodonGroup" alike, so
// a name-derived candidate differing only in case must not read as new.
func (smartRecruitersProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

// recruiteeProber probes the Recruitee per-subdomain offers API. A non-empty offers array is
// a live board, and each offer carries the employer's own name, so the board can be gated
// against the name the seed expected rather than accepted on liveness alone.
type recruiteeProber struct{}

func recruiteeOffersURL(slug string) string {
	return fmt.Sprintf("https://%s.recruitee.com/api/offers/", slug)
}

// postingIDs lists the live posting ids; one response carries the complete list.
func (recruiteeProber) postingIDs(ctx context.Context, c httpClient, slug string) ([]string, error) {
	var resp struct {
		Offers []struct {
			ID int64 `json:"id"`
		} `json:"offers"`
	}
	if err := c.GetJSON(ctx, recruiteeOffersURL(slug), &resp); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(resp.Offers))
	for _, o := range resp.Offers {
		ids = append(ids, strconv.FormatInt(o.ID, 10))
	}
	return ids, nil
}

func (recruiteeProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Offers []struct {
			ID          int64  `json:"id"`
			CompanyName string `json:"company_name"`
		} `json:"offers"`
	}
	if err := c.GetJSON(ctx, recruiteeOffersURL(slug), &resp); err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	if len(resp.Offers) == 0 {
		return "", 0, nil
	}
	return resp.Offers[0].CompanyName, len(resp.Offers), nil
}

// pinpointProber probes the Pinpoint public postings API. A non-empty data array is a live
// board; the list carries no company name, so it falls back to the slug.
type pinpointProber struct{}

func (pinpointProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.pinpointhq.com/postings.json", slug), &resp); err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	if len(resp.Data) == 0 {
		return "", 0, nil
	}
	return "", len(resp.Data), nil
}

// breezyProber probes the Breezy per-subdomain JSON list (a bare array of postings). A
// non-empty array is a live board, and each posting embeds the employer's own name, so the
// board can be gated against the name the seed expected rather than accepted on liveness
// alone.
type breezyProber struct{}

func (breezyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var postings []struct {
		ID      string `json:"id"`
		Company struct {
			Name string `json:"name"`
		} `json:"company"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.breezy.hr/json", slug), &postings); err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	return postings[0].Company.Name, len(postings), nil
}

// freshteamJobPattern matches a job permalink's /jobs/<12-char-id> segment, anchored to a path
// boundary so /jobs and /jobs/search do not match.
var freshteamJobPattern = regexp.MustCompile(`/jobs/[A-Za-z0-9_-]{12}(?:[/?#]|$)`)

// freshteamProber probes a Freshteam career site. Its API is auth-gated, so liveness is judged from
// the listing HTML: a live board links ≥1 job permalink. No reliable company name; falls back to the slug.
type freshteamProber struct{}

func (freshteamProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s.freshteam.com/jobs", slug))
	if err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	n := countMatchingLinks(root, freshteamJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// countMatchingLinks counts <a href> values matching pat. A duplicate link inflating the count is
// harmless, since it is only used for liveness.
func countMatchingLinks(root *html.Node, pat *regexp.Regexp) int {
	n := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			for _, a := range node.Attr {
				if a.Key == "href" && pat.MatchString(a.Val) {
					n++
					break
				}
			}
		}
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return n
}

// joinProber probes a Join.com company by its numeric id (the slug path returns empty); a slug is
// resolved to its id during seed building. A non-zero rowCount is a live board.
type joinProber struct{}

func (joinProber) probe(ctx context.Context, c httpClient, id string) (string, int, error) {
	var jobs struct {
		Pagination struct {
			RowCount int `json:"rowCount"`
		} `json:"pagination"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://join.com/api/public/companies/%s/jobs?page=1&pageSize=1", id), &jobs); err != nil {
		return "", 0, nil //nolint:nilerr // an unreachable board is a skip (prober contract)
	}
	if jobs.Pagination.RowCount == 0 {
		return "", 0, nil
	}
	var meta struct {
		Name string `json:"name"`
	}
	_ = c.GetJSON(ctx, fmt.Sprintf("https://join.com/api/public/companies/%s", id), &meta)
	return meta.Name, jobs.Pagination.RowCount, nil
}
