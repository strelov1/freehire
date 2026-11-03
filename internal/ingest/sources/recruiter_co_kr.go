package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// recruiterCoKR adapts Recruiter (recruiter.co.kr, "Jobflex"), Saramin's B2B career-site
// builder used by Korean employers. The board is the tenant's full career host, e.g.
// "cosmax.recruiter.co.kr": the career site is a client-rendered Next.js shell with no
// postings in its HTML, and the same tenant may sit on a custom domain, so the host (not a
// subdomain label) is the identity.
//
// The site's own JavaScript reads a keyless JSON API on api-recruiter.recruiter.co.kr, which
// resolves the tenant from a `prefix` request header carrying the career host
// (verified 2026-11-03 against cosmax and onedskr-people):
//
//	POST /position/v1/jobflex      {"pageableRq":{"page","size"},"filter":{...}}
//	                               -> {"pagination":{...},"list":[{positionSn,title,...}]}
//	GET  /position/v2/jobflex/<sn> -> {"jobDescription":"<html>", ...}
//
// The listing carries title and dates but no body, so each open posting is hydrated with one
// detail request. It also lists postings whose submission window has ended
// (submissionStatus POST_SUBMISSION), which stay on the site as history; those are skipped,
// as they are no longer open to applicants. Neither endpoint states a location, so Location
// is left empty for the pipeline's dictionaries; titles and bodies stay in Korean.
type recruiterCoKR struct {
	http interface {
		HeaderJSONGetter
		HeaderJSONPoster
	}
}

// NewRecruiterCoKR builds the Recruiter (recruiter.co.kr) adapter over the given HTTP client.
func NewRecruiterCoKR(c interface {
	HeaderJSONGetter
	HeaderJSONPoster
}) Source {
	return recruiterCoKR{http: c}
}

func (recruiterCoKR) Provider() string { return "recruiter_co_kr" }

const (
	rckAPI      = "https://api-recruiter.recruiter.co.kr"
	rckPageSize = 100
	// rckMaxPages is a ceiling, not an expected end: 100 rows a page admits 10,000 postings
	// against 42 on the largest tenant measured. Reaching it fails the crawl.
	rckMaxPages = 100
	// rckOffsetKST is the offset of the zone the platform's naive timestamps are written in.
	rckOffsetKST = 9 * 60 * 60
)

var rckZone = time.FixedZone("KST", rckOffsetKST)

// rckHost validates a board as a bare career host and returns it lower-cased.
func rckHost(board string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(board))
	if h == "" || strings.ContainsAny(h, "/?# \t:@") || !strings.Contains(h, ".") {
		return "", fmt.Errorf("recruiter_co_kr: board %q is not a career host (want e.g. cosmax.recruiter.co.kr)", board)
	}
	return h, nil
}

type rckListPage struct {
	Pagination struct {
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
	List *[]rckListing `json:"list"`
}

type rckListing struct {
	PositionSn       int64  `json:"positionSn"`
	Title            string `json:"title"`
	SubmissionStatus string `json:"submissionStatus"`
	RecruitmentType  string `json:"recruitmentType"`
	StartDateTime    string `json:"startDateTime"`
	EndDateTime      string `json:"endDateTime"`
}

type rckDetail struct {
	JobDescription string `json:"jobDescription"`
}

func (s recruiterCoKR) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host, err := rckHost(e.Board)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"prefix": host}

	rows, err := s.list(ctx, headers)
	if err != nil {
		return nil, fmt.Errorf("recruiter_co_kr: board %s: %w", host, err)
	}
	var gone atomic.Int64
	jobs := fetchDetails(rows, defaultDetailWorkers, func(r rckListing) (Job, bool) {
		id := strconv.FormatInt(r.PositionSn, 10)
		jobURL := "https://" + host + "/career/jobs/" + id
		var d rckDetail
		if err := s.http.GetJSONWithHeaders(ctx, rckAPI+"/position/v2/jobflex/"+id, headers, &d); err != nil {
			if rckGone(err) {
				gone.Add(1)
				return Job{}, false
			}
			return unreadableDetail(id, jobURL, e.Company), true
		}
		j := Job{
			ExternalID:  id,
			URL:         jobURL,
			Title:       strings.TrimSpace(r.Title),
			Company:     e.Company,
			Description: sanitizeHTML(d.JobDescription),
			PostedAt:    NotFuture(rckTime(r.StartDateTime)),
		}
		// An ongoing posting (PERMANENT) carries a placeholder end date years out; only a
		// dated posting states a real last application day.
		if r.RecruitmentType != "PERMANENT" {
			// The calendar day as the employer wrote it (Korea time), as a bare UTC date.
			if len(r.EndDateTime) >= 10 {
				if day, err := time.Parse("2006-01-02", r.EndDateTime[:10]); err == nil {
					j.ApplicationDeadline = &day
				}
			}
		}
		return j, true
	})
	// The platform answers 400 for an unknown position, but a 400 is also what a bad request
	// looks like. Every listed posting "gone" at once is that, not a board emptied between two
	// requests: fail rather than let the sweep close the whole board.
	if len(rows) > 0 && int(gone.Load()) == len(rows) {
		return nil, fmt.Errorf("recruiter_co_kr: board %s: all %d listed postings answered gone", host, len(rows))
	}
	return jobs, nil
}

// rckGone reports whether the detail call states the posting no longer exists. The platform
// answers 400 (CannotFindPositionException) for an unknown position, not 404.
func rckGone(err error) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return false
	}
	return se.Code == http.StatusBadRequest || se.Code == http.StatusNotFound || se.Code == http.StatusGone
}

// list walks the listing to its own totalPages and returns the postings still open to
// applicants. A response without the list array is an error document, not an empty board.
func (s recruiterCoKR) list(ctx context.Context, headers map[string]string) ([]rckListing, error) {
	var open []rckListing
	seen := map[int64]bool{}
	for page := 1; ; page++ {
		if page > rckMaxPages {
			return nil, fmt.Errorf("listing exceeded %d pages", rckMaxPages)
		}
		body := map[string]any{
			"pageableRq": map[string]int{"page": page, "size": rckPageSize},
			"filter":     map[string]any{"resumeLanguageTypeList": []string{"KOR"}},
		}
		var p rckListPage
		if err := s.http.PostJSONWithHeaders(ctx, rckAPI+"/position/v1/jobflex", headers, body, &p); err != nil {
			return nil, fmt.Errorf("listing page %d: %w", page, err)
		}
		if p.List == nil {
			return nil, fmt.Errorf("listing page %d carries no list", page)
		}
		for _, r := range *p.List {
			if r.PositionSn == 0 || seen[r.PositionSn] {
				continue // no native id would collide on the dedup key
			}
			seen[r.PositionSn] = true
			if r.SubmissionStatus == "POST_SUBMISSION" {
				continue // the application window has ended
			}
			open = append(open, r)
		}
		if page >= p.Pagination.TotalPages {
			return open, nil
		}
	}
}

// rckTime parses the platform's zone-less "2006-01-02T15:04:05" timestamp as Korea time.
func rckTime(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", strings.TrimSpace(s), rckZone)
	if err != nil {
		return nil
	}
	u := t.UTC()
	return &u
}
