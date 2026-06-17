package sources

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// wantedkr adapts wanted.co.kr, a South-Korean tech job board. Boardless (one public API,
// no per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the API. The list endpoint paginates by offset and carries only
// ids + summaries, so each posting's description comes from a per-job detail fetch
// (bounded-concurrency). Postings are Korean-language.
type wantedkr struct {
	http JSONGetter
}

const (
	wantedkrListURL   = "https://www.wanted.co.kr/api/v4/jobs?country=kr&job_sort=job.latest_order&years=-1&limit=%d&offset=%d"
	wantedkrDetailURL = "https://www.wanted.co.kr/api/v4/jobs/%d"
	wantedkrJobURL    = "https://www.wanted.co.kr/wd/%d"
	wantedkrPageSize  = 100
	// wantedkrMaxPages bounds offset pagination as a runaway guard.
	wantedkrMaxPages = 100
)

// NewWantedKR builds the Wanted (Korea) adapter over the given HTTP client.
func NewWantedKR(c JSONGetter) Source { return wantedkr{http: c} }

func (wantedkr) Provider() string { return "wantedkr" }

func (wantedkr) boardless() {}

func (wantedkr) aggregator() {}

func (s wantedkr) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var ids []int64
	for page := 0; page < wantedkrMaxPages; page++ {
		var resp struct {
			Data []struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		url := fmt.Sprintf(wantedkrListURL, wantedkrPageSize, page*wantedkrPageSize)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("wantedkr: list offset %d: %w", page*wantedkrPageSize, err)
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, j := range resp.Data {
			ids = append(ids, j.ID)
		}
		if len(resp.Data) < wantedkrPageSize {
			break
		}
	}

	return fetchDetails(ids, defaultDetailWorkers, func(id int64) (Job, bool) {
		return s.detail(ctx, id)
	}), nil
}

// wantedkrJob is the detail payload's job object.
type wantedkrJob struct {
	ID       int64  `json:"id"`
	Position string `json:"position"`
	Company  struct {
		Name string `json:"name"`
	} `json:"company"`
	Address struct {
		FullLocation string `json:"full_location"`
		Country      string `json:"country"`
	} `json:"address"`
	Detail struct {
		Intro           string `json:"intro"`
		MainTasks       string `json:"main_tasks"`
		Requirements    string `json:"requirements"`
		PreferredPoints string `json:"preferred_points"`
		Benefits        string `json:"benefits"`
	} `json:"detail"`
}

// detail fetches one posting and maps it to a Job, returning ok=false when the fetch fails
// or the company is missing (which would break the slug), so the caller skips just that one.
func (s wantedkr) detail(ctx context.Context, id int64) (Job, bool) {
	var resp struct {
		Job wantedkrJob `json:"job"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(wantedkrDetailURL, id), &resp); err != nil {
		return Job{}, false
	}
	j := resp.Job
	if j.ID == 0 || j.Company.Name == "" {
		return Job{}, false
	}
	location := firstNonEmpty(j.Address.FullLocation, j.Address.Country)
	return Job{
		ExternalID:  fmt.Sprintf("%d", j.ID),
		URL:         fmt.Sprintf(wantedkrJobURL, j.ID),
		Title:       j.Position,
		Company:     j.Company.Name,
		Location:    location,
		Description: j.description(),
		Remote:      isRemote(location),
		// wanted.co.kr exposes no posting timestamp; posted_at is left to ingest time.
	}, true
}

// description assembles the plain-text detail sections into sanitized HTML, each under a
// heading, with newlines turned into <br>. A section with no text is omitted.
func (j wantedkrJob) description() string {
	var b strings.Builder
	writeWantedkrSection(&b, "About", j.Detail.Intro)
	writeWantedkrSection(&b, "Responsibilities", j.Detail.MainTasks)
	writeWantedkrSection(&b, "Requirements", j.Detail.Requirements)
	writeWantedkrSection(&b, "Preferred", j.Detail.PreferredPoints)
	writeWantedkrSection(&b, "Benefits", j.Detail.Benefits)
	return sanitizeHTML(b.String())
}

// writeWantedkrSection appends an <h3> heading and the HTML-escaped text (newlines → <br>)
// for a non-empty section.
func writeWantedkrSection(b *strings.Builder, heading, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	b.WriteString("<h3>" + heading + "</h3><p>")
	b.WriteString(strings.ReplaceAll(html.EscapeString(text), "\n", "<br>"))
	b.WriteString("</p>")
}
