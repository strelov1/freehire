package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// darwinbox adapts Darwinbox career portals, the HRMS most large Indian (and a growing share of
// South-East Asian) employers run. The board is the tenant's whole careers HOST
// ("atherenergy.darwinbox.in", "carsome.darwinbox.com"): the TLD is the tenant's data region, so
// the subdomain alone does not name the portal.
//
// The listing is the endpoint the portal's own Angular app posts to,
// /ms/candidateapi/job/alljobs. It pages at 100 postings whatever limit is asked for, carries
// the board's total as job_counts, and every row already holds the posting's whole description
// (jd), so no per-posting request is needed. The API sits behind Cloudflare, which refuses Go's
// default TLS+HTTP/2 fingerprint with a 403 and serves the Chrome one (measured 2026-10-25 from
// the crawl host), hence the fingerprint transport in the registry.
type darwinbox struct {
	http JSONPoster
}

// NewDarwinbox builds the Darwinbox adapter over the given POST-capable client.
func NewDarwinbox(c JSONPoster) Source { return darwinbox{http: c} }

func (darwinbox) Provider() string { return "darwinbox" }

// fullBoardListing: the walk ends only at an empty page, and a board whose walk does not add up
// to the job_counts the first page declared fails rather than returning what it gathered.
func (darwinbox) fullBoardListing() {}

// darwinboxPageSize is the page the API serves whatever larger limit is requested.
const darwinboxPageSize = 100

// darwinboxMaxPages bounds the walk at 50k postings, well past the largest board measured
// (684); reaching it fails the crawl instead of ending it as a success.
const darwinboxMaxPages = 500

// darwinboxHostPattern is the shape of a board: one tenant label on one of the two regional
// apexes. The adapter builds its request URL from the board, so nothing else may pass.
var darwinboxHostPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.darwinbox\.(?:in|com)$`)

type darwinboxPage struct {
	Status    string             `json:"status"`
	Data      []darwinboxPosting `json:"data"`
	JobCounts *int               `json:"job_counts"`
}

type darwinboxPosting struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	JD          string   `json:"jd"`
	Country     string   `json:"country"`
	Locations   []string `json:"officelocations_without_area"`
	IsRemote    int      `json:"is_remote"`
	EmpTypeName string   `json:"emp_type_name"`
	PostedOn    int64    `json:"posted_on"`
}

func (s darwinbox) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !darwinboxHostPattern.MatchString(e.Board) {
		return nil, fmt.Errorf("darwinbox: board %q is not a <tenant>.darwinbox.(in|com) host", e.Board)
	}
	base := "https://" + e.Board
	var (
		jobs  []Job
		seen  = map[string]bool{}
		total = -1
	)
	for page := 1; ; page++ {
		if page > darwinboxMaxPages {
			return nil, fmt.Errorf("darwinbox: %s: no empty page within %d pages", e.Board, darwinboxMaxPages)
		}
		var resp darwinboxPage
		body := map[string]any{"page": page, "limit": darwinboxPageSize}
		if err := s.http.PostJSON(ctx, base+"/ms/candidateapi/job/alljobs", body, &resp); err != nil {
			return nil, fmt.Errorf("darwinbox: list %s page %d: %w", e.Board, page, err)
		}
		if resp.Status != "success" || resp.JobCounts == nil {
			return nil, fmt.Errorf("darwinbox: %s page %d: unexpected answer (status %q)", e.Board, page, resp.Status)
		}
		if total < 0 {
			total = *resp.JobCounts
		}
		// The raw row count, not the newly kept one, is what proves the end: a page of
		// already-seen rows is a re-served page, not the last one.
		if len(resp.Data) == 0 {
			break
		}
		for _, p := range resp.Data {
			if p.ID == "" || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			jobs = append(jobs, darwinboxJob(base, e.Company, p))
		}
	}
	if len(jobs) != total {
		return nil, fmt.Errorf("darwinbox: %s: walk gathered %d postings, the board declares %d", e.Board, len(jobs), total)
	}
	return jobs, nil
}

func darwinboxJob(base, company string, p darwinboxPosting) Job {
	loc := ""
	if len(p.Locations) > 0 {
		// The API writes a stray carriage return before the country ("Mumbai, Maharashtra\r, India").
		loc = strings.Join(strings.Fields(strings.ReplaceAll(p.Locations[0], "\r", "")), " ")
	}
	j := Job{
		ExternalID:     p.ID,
		URL:            base + "/ms/candidatev2/main/careers/jobDetails/" + p.ID,
		Title:          strings.TrimSpace(p.Title),
		Company:        company,
		Location:       loc,
		Description:    sanitizeHTML(p.JD),
		EmploymentType: darwinboxEmploymentType(p.EmpTypeName),
		PostedAt:       parseEpochSeconds(p.PostedOn),
	}
	// is_remote is a structured flag, but 0 is also what a tenant that never sets it serves,
	// so only the positive answer is a statement about the work arrangement.
	if p.IsRemote == 1 {
		j.WorkMode = "remote"
		j.Remote = true
	} else {
		j.Remote = isRemote(loc) || isRemote(j.Title)
	}
	if c := location.NormalizeCountry(p.Country); c != "" {
		j.Countries = []string{c}
	}
	return j
}

// darwinboxEmploymentType maps the tenant's own employee-type label. Tenants name these after
// their HR categories ("Permanent", "Probationary", "BRANCH RESOURCE", "P-P8-Probationer-HO
// Executive"), so only a label that plainly states the arrangement is mapped; the rest is left to
// the description parser rather than guessed.
func darwinboxEmploymentType(label string) string {
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "intern"):
		return "internship"
	case strings.Contains(l, "part time"), strings.Contains(l, "part-time"):
		return "part_time"
	case strings.Contains(l, "contract"), strings.Contains(l, "consultant"):
		return "contract"
	case strings.Contains(l, "full time"), strings.Contains(l, "full-time"), strings.Contains(l, "fulltime"),
		strings.Contains(l, "permanent"), l == "fte":
		return "full_time"
	}
	return ""
}
