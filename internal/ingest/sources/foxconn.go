package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// foxconn adapts Hon Hai (Foxconn)'s own Taiwan recruitment portal (recruit.foxconn.com, the
// isite-web-tw app). Single-company and boardless. Foxconn's regional subsidiaries hire through
// ATS tenants the catalogue already crawls (SuccessFactors in Czechia, JazzHR and Pinpoint in the
// US, Workday for ZF Foxconn); this is the parent's own board.
//
// The listing is ONE unauthenticated GET that returns every open vacancy (no pagination — paging
// parameters are ignored), carrying the duties but not the requirements. A posting the catalogue
// does not hold yet is hydrated from the vacancy endpoint (HydratingSource); a seen one costs no
// request.
//
// Everything is Traditional Chinese, and there is no job-category field. What the board does
// carry is the recruiting track (job_type), and a few of those tracks are R&D programmes that
// state a posting is technical (foxconnTechTracks).
//
// Not fullBoardListing: the single response states no total to check it against, the same
// reason quanta, whose one-response dump was once seen truncated, does not claim it.
type foxconn struct {
	http JSONGetter
}

const (
	foxconnListURL   = "https://recruit.foxconn.com/hh_recruit_tw_api/portal_api/JobVacancies"
	foxconnDetailURL = "https://recruit.foxconn.com/hh_recruit_tw_api/portal_api/JobVacancies/"
	foxconnJobURL    = "https://recruit.foxconn.com/isite-web-tw/main/jobsearch/detail?id="
	// foxconnOtherPlace is the portal's "other" location, which names no place.
	foxconnOtherPlace = "其他"
)

// foxconnTechTracks are the recruiting tracks that are R&D programmes: the communications,
// server, personal-systems, cloud and imaging R&D tracks, information technology, and the AI
// and Kaohsiung software-centre zones. The rest — the management-associate, general-hiring,
// disability-friendly, intern and the dated campaign tracks — span every function.
var foxconnTechTracks = map[string]bool{
	"通訊研發":    true, // communications R&D
	"4G":      true,
	"伺服器研發":   true, // server R&D
	"個人系統研發":  true, // personal-systems R&D
	"雲端應用":    true, // cloud applications
	"光學及數位影像": true, // optics and digital imaging
	"資訊科技":    true, // information technology
	"IAI":     true, // industrial internet / AI talent zone
	"IAI_KS":  true, // Kaohsiung software R&D centre zone
}

var foxconnIDRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

// NewFoxconn builds the Foxconn (Hon Hai) Taiwan portal adapter over the given HTTP client.
func NewFoxconn(c JSONGetter) Source { return foxconn{http: c} }

func (foxconn) Provider() string { return "foxconn" }

// foxconn is single-company, so its config entry carries no board.
func (foxconn) boardless() {}

// foxconnVacancy is one vacancy, from the listing or the detail. The body is split over
// numbered desc_N fields; which ones a posting fills varies (desc_4 is always the duties).
type foxconnVacancy struct {
	ID     string  `json:"id"`
	Title  string  `json:"job_name"`
	Tracks *string `json:"job_type"` // listing only; null on the detail
	Place  string  `json:"loc_name"`
	Desc1  *string `json:"desc_1"`
	Desc2  *string `json:"desc_2"`
	Desc3  *string `json:"desc_3"`
	Desc4  *string `json:"desc_4"`
	Desc5  *string `json:"desc_5"`
	Desc6  *string `json:"desc_6"`
	Desc7  *string `json:"desc_7"`
	Desc8  *string `json:"desc_8"`
	Desc9  *string `json:"desc_9"`
	Desc10 *string `json:"desc_10"`
	Desc11 *string `json:"desc_11"`
}

// body is the duties (desc_4) followed by every other filled section in order.
func (v foxconnVacancy) body() string {
	var b strings.Builder
	for _, d := range []*string{v.Desc4, v.Desc1, v.Desc2, v.Desc3, v.Desc5, v.Desc6, v.Desc7, v.Desc8, v.Desc9, v.Desc10, v.Desc11} {
		if d != nil {
			b.WriteString(plainTextToHTML(*d))
		}
	}
	return b.String()
}

func (s foxconn) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s foxconn) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	var list []foxconnVacancy
	if err := s.http.GetJSON(ctx, foxconnListURL, &list); err != nil {
		return nil, fmt.Errorf("foxconn: listing: %w", err)
	}
	if len(list) == 0 {
		// The portal always carries vacancies; none is a changed API, and must not read as every
		// posting closed.
		return nil, fmt.Errorf("foxconn: the listing carries no vacancies")
	}
	ids := map[string]bool{}
	asked := 0
	for _, v := range list {
		if !foxconnIDRE.MatchString(v.ID) || ids[v.ID] {
			return nil, fmt.Errorf("foxconn: vacancy id %q is malformed or listed twice", v.ID)
		}
		ids[v.ID] = true
		if !seen(v.ID) {
			asked++
		}
	}
	jobs := fetchDetails(list, defaultDetailWorkers, func(v foxconnVacancy) (Job, bool) {
		if seen(v.ID) {
			return Job{ExternalID: v.ID, URL: foxconnJobURL + v.ID, Title: foxconnTitle(v.Title), Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, v)
	})
	if asked > 0 && len(jobs) == len(list)-asked {
		return nil, fmt.Errorf("foxconn: none of %d new postings could be read", asked)
	}
	return jobs, nil
}

// detail hydrates a new posting from the vacancy endpoint, which adds the requirements. A failed
// request defers the posting to the next crawl rather than storing it without them, since a
// stored posting is never hydrated again; the endpoint answers a withdrawn id with 404, which
// lands here too and is retried until the listing drops it.
func (s foxconn) detail(ctx context.Context, e CompanyEntry, listed foxconnVacancy) (Job, bool) {
	var v foxconnVacancy
	if err := s.http.GetJSON(ctx, foxconnDetailURL+listed.ID, &v); err != nil {
		log.Printf("foxconn: vacancy %s failed; deferring: %v", listed.ID, err)
		return Job{}, false
	}
	if v.ID != listed.ID {
		log.Printf("foxconn: vacancy %s answered as %q; deferring", listed.ID, v.ID)
		return Job{}, false
	}
	title := foxconnTitle(firstNonEmpty(v.Title, listed.Title))
	body := v.body()
	if title == "" || body == "" {
		return Job{}, false
	}
	place := strings.TrimSpace(firstNonEmpty(v.Place, listed.Place))
	if place == foxconnOtherPlace {
		place = ""
	}
	location, countries := twLocation(place)
	job := Job{
		ExternalID:  listed.ID,
		URL:         foxconnJobURL + listed.ID,
		Title:       title,
		Company:     e.Company,
		Description: sanitizeHTML(body),
		Location:    location,
		Countries:   countries,
	}
	if listed.Tracks != nil {
		for _, t := range strings.Split(*listed.Tracks, ",") {
			if foxconnTechTracks[strings.TrimSpace(t)] {
				job.IsTechHint = true
			}
		}
	}
	return job, true
}

func foxconnTitle(s string) string { return strings.Join(strings.Fields(s), " ") }
