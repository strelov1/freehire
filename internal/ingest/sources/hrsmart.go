package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// hrsmart reads Deltek HRsmart's public listing and posting pages.
// Board is the full host, including its regional instance.
type hrsmart struct{ http HTMLResolvedGetter }

func NewHRsmart(c HTMLResolvedGetter) Source { return hrsmart{http: c} }
func (hrsmart) Provider() string             { return "hrsmart" }
func (hrsmart) fullBoardListing()            {}

var (
	hrsmartHost    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*\.hrdepartment\.com$`)
	hrsmartPosting = regexp.MustCompile(`^/hrsmart/ats/Posting/view/([0-9]+)(?:/.*)?$`)
	hrsmartTotal   = regexp.MustCompile(`(?i)Displaying\s+[0-9,]+\s*-\s*[0-9,]+\s+of\s+([0-9,]+)`)
)

func (s hrsmart) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s hrsmart) FetchNew(ctx context.Context, e CompanyEntry, seen func(string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !hrsmartHost.MatchString(board) || strings.Contains(board, "..") {
		return nil, fmt.Errorf("hrsmart: invalid tenant host %q", e.Board)
	}
	listed, err := s.list(ctx, board)
	if err != nil {
		return nil, err
	}
	jobs := fetchDetails(listed, 2, func(j Job) (Job, bool) {
		j.Company = e.Company
		if seen(j.ExternalID) {
			j.SeenRefresh = true
			return j, true
		}
		root, final, err := s.http.GetHTMLResolved(ctx, j.URL)
		if err == nil {
			u, parseErr := url.Parse(final)
			if parseErr != nil || u.Host != board || hrsmartID(u.Path) != j.ExternalID {
				err = fmt.Errorf("hrsmart: posting redirected away from its identity")
			}
		}
		if err == nil {
			title := firstByID(root, "job_details_ats_requisition_title")
			body := firstByID(root, "job_details_ats_requisition_description")
			if title == nil || body == nil || strings.TrimSpace(textContent(body)) == "" {
				err = fmt.Errorf("hrsmart: missing posting fields")
			} else {
				j.Title = strings.Join(strings.Fields(textContent(title)), " ")
				j.Description = sanitizeHTML(innerHTML(body))
				if loc := firstByID(root, "job_details_hua_location_id"); loc != nil {
					j.Location = strings.Join(strings.Fields(textContent(loc)), " ")
				}
				if j.Title == "" {
					err = fmt.Errorf("hrsmart: empty posting title")
				}
			}
		}
		if err != nil {
			if !detailUnreadable(err) {
				return Job{}, false
			}
			j.Description = ""
			j.Unreadable = true
		}
		return j, true
	})
	for _, j := range jobs {
		if !j.Unreadable {
			return jobs, nil
		}
	}
	if len(listed) > 0 {
		return jobs, fmt.Errorf("hrsmart: none of %d listed postings could be read", len(listed))
	}
	return jobs, nil
}

func hrsmartID(path string) string {
	if m := hrsmartPosting.FindStringSubmatch(path); len(m) == 2 {
		return m[1]
	}
	return ""
}

func (s hrsmart) list(ctx context.Context, board string) ([]Job, error) {
	next := "https://" + board + "/hrsmart/ats/JobSearch/viewAll"
	seenPages, seenJobs := map[string]bool{}, map[string]bool{}
	var jobs []Job
	total := -1
	for page := 0; page < 100; page++ {
		if seenPages[next] {
			return nil, fmt.Errorf("hrsmart: repeated listing page")
		}
		seenPages[next] = true
		root, final, err := s.http.GetHTMLResolved(ctx, next)
		if err != nil {
			return nil, fmt.Errorf("hrsmart: listing: %w", err)
		}
		base, err := url.Parse(final)
		if err != nil || base.Scheme != "https" || base.Host != board || !strings.HasPrefix(base.Path, "/hrsmart/ats/JobSearch/viewAll") {
			return nil, fmt.Errorf("hrsmart: listing redirected away from tenant")
		}
		table := firstByID(root, "jobSearchResultsGrid_table")
		if table == nil {
			return nil, fmt.Errorf("hrsmart: missing results table")
		}
		walk(table, func(n *html.Node) bool {
			if n.Type != html.ElementNode || n.Data != "a" {
				return true
			}
			u, err := base.Parse(Attr(n, "href"))
			if err != nil || u.Scheme != "https" || u.Host != board {
				return true
			}
			id := hrsmartID(u.Path)
			if id != "" && !seenJobs[id] {
				seenJobs[id] = true
				jobs = append(jobs, Job{ExternalID: id, URL: "https://" + board + "/hrsmart/ats/Posting/view/" + id, Title: strings.TrimSpace(textContent(n))})
			}
			return true
		})
		m := hrsmartTotal.FindStringSubmatch(textContent(root))
		if len(m) != 2 {
			return nil, fmt.Errorf("hrsmart: missing listing total")
		}
		count, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		if err != nil || (total >= 0 && total != count) {
			return nil, fmt.Errorf("hrsmart: listing total changed during pagination")
		}
		total = count
		control := firstByClass(root, "paginateNext_div")
		if control == nil {
			return nil, fmt.Errorf("hrsmart: missing pagination control")
		}
		if HasClass(control, "disabled") {
			if len(jobs) != total {
				return nil, fmt.Errorf("hrsmart: listing returned %d of %d postings", len(jobs), total)
			}
			return jobs, nil
		}
		anchor := firstByTag(control, "a")
		if anchor == nil {
			return nil, fmt.Errorf("hrsmart: next page has no link")
		}
		u, err := base.Parse(Attr(anchor, "href"))
		if err != nil || u.Scheme != "https" || u.Host != board || !strings.HasPrefix(u.Path, "/hrsmart/ats/JobSearch/viewAll/") {
			return nil, fmt.Errorf("hrsmart: invalid next page")
		}
		next = u.String()
	}
	return nil, fmt.Errorf("hrsmart: listing exceeded pagination limit")
}
