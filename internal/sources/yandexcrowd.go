package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// yandexcrowd adapts Yandex Crowd's public vacancy catalogue (crowd.yandex.ru/vacancies) — the
// gig/support arm of Yandex: call-centre, support, moderation, courier, and back-office roles. It
// is a single-company boardless adapter: the /vacancies landing page server-renders its whole
// catalogue as one JSON array in a <script id="data"> tag, grouped by direction, so a single GET
// yields every posting and there is no per-vacancy detail fetch. That array is a STALE catalogue —
// it keeps taken-down postings whose own pages 404 — so the crawl keeps only entries flagged
// `available`, which still resolve. Identity is the vacancy's URL path (a stable slug such as
// "support/finteh_chat"); the float `id` is a positional index and is ignored.
type yandexcrowd struct {
	http TextGetter
}

// NewYandexCrowd builds the Yandex Crowd adapter over the given HTTP client.
func NewYandexCrowd(c TextGetter) Source { return yandexcrowd{http: c} }

func (yandexcrowd) Provider() string { return "yandexcrowd" }

// yandexcrowd is single-company, so its config entry carries no board.
func (yandexcrowd) boardless() {}

const (
	yandexCrowdListURL = "https://crowd.yandex.ru/vacancies"
	yandexCrowdHost    = "https://crowd.yandex.ru/"
	yandexCrowdDataKey = `<script id="data" type="application/json">`
)

// yandexCrowdGroup is one direction section of the catalogue (support, sales, …); only its
// vacancies matter to the crawl.
type yandexCrowdGroup struct {
	Vacancies []yandexCrowdVacancy `json:"vacancies"`
}

// yandexCrowdVacancy is one posting. URL is both the detail/apply page and the source of the
// stable external id; Available gates taken-down postings (their pages 404). Tags.Remotely is the
// structured work-arrangement signal and Tags.Employment the employment-type signal.
type yandexCrowdVacancy struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Available   bool   `json:"available"`
	Tags        struct {
		Remotely   string   `json:"remotely"`
		Employment []string `json:"employment"`
	} `json:"tags"`
}

func (y yandexcrowd) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	body, err := y.http.GetText(ctx, yandexCrowdListURL)
	if err != nil {
		return nil, fmt.Errorf("yandexcrowd: listing: %w", err)
	}
	raw, ok := bracketSlice(body, yandexCrowdDataKey, '[', ']')
	if !ok {
		return nil, fmt.Errorf(`yandexcrowd: no <script id="data"> catalogue in listing`)
	}
	var groups []yandexCrowdGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, fmt.Errorf("yandexcrowd: decode catalogue: %w", err)
	}

	var jobs []Job
	for _, g := range groups {
		for _, v := range g.Vacancies {
			if j, ok := yandexCrowdJob(e, v); ok {
				jobs = append(jobs, j)
			}
		}
	}
	return jobs, nil
}

// yandexCrowdJob maps one vacancy to a Job, returning ok=false for a taken-down posting (its page
// 404s, flagged available:false) or one missing the URL that forms its identity.
func yandexCrowdJob(e CompanyEntry, v yandexCrowdVacancy) (Job, bool) {
	if !v.Available {
		return Job{}, false
	}
	id := yandexCrowdExternalID(v.URL)
	if id == "" {
		return Job{}, false
	}
	mode := yandexCrowdWorkMode(v.Tags.Remotely)
	return Job{
		ExternalID:     id,
		URL:            strings.TrimSpace(v.URL),
		Title:          sanitizeHTML(v.Title),
		Company:        e.Company,
		Description:    sanitizeHTML(v.Description),
		Remote:         mode == "remote",
		WorkMode:       mode,
		EmploymentType: yandexCrowdEmployment(v.Tags.Employment),
	}, true
}

// yandexCrowdExternalID derives the stable dedup key from a vacancy URL: its path under the crowd
// host (e.g. "support/finteh_chat"), which is stable across catalogue reordering unlike the float
// id. Returns "" when the URL is empty or not a crowd host path.
func yandexCrowdExternalID(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimPrefix(u, yandexCrowdHost)
	u = strings.TrimPrefix(u, "http://crowd.yandex.ru/")
	return strings.Trim(u, "/")
}

// yandexCrowdWorkMode maps the structured `remotely` tag to the work-mode vocabulary. Observed
// values are "удалённо" (remote) and "локально" (onsite); anything else is left unknown ("").
func yandexCrowdWorkMode(remotely string) string {
	switch r := strings.ToLower(strings.TrimSpace(remotely)); {
	case strings.Contains(r, "удал"):
		return "remote"
	case strings.Contains(r, "локал"):
		return "onsite"
	default:
		return ""
	}
}

// yandexCrowdEmployment maps the structured `employment` tag onto the employment-type vocabulary,
// returning the first recognized value. Observed values are "полная" (full_time) and "частичная"
// (part_time); an unknown/absent value yields "" so the description parser decides.
func yandexCrowdEmployment(employment []string) string {
	for _, e := range employment {
		switch t := strings.ToLower(strings.TrimSpace(e)); {
		case strings.Contains(t, "полн"):
			return "full_time"
		case strings.Contains(t, "частичн"):
			return "part_time"
		}
	}
	return ""
}
