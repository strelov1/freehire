package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// bairesdev adapts BairesDev, a Latin-American software-outsourcing company. It has no crawlable
// ATS board — its applicants portal's openings list is login-gated — but its public talent site
// (talent.bairesdev.com) embeds the whole open-req list client-side in a Duda job widget's base64
// config, each row carrying an apply link to applicants.bairesdev.com/job/<career>/<id>. So the
// crawl reads that single page, dedups the country/city-template rows by the numeric job id (the
// site lists one posting once per location under a shared id), and hydrates each distinct id from
// the public per-job endpoint (/api/JobPosting?JobPostingId=<id>) — the same schema.org JSON-LD
// the linksource adapter reads, giving a clean title, description, date, and remote flag.
// Single-company (every posting is BairesDev) and boardless, so its config entry is a placeholder.
type bairesdev struct {
	http bairesDevHTTP
}

// bairesDevHTTP is the transport bairesdev needs: the talent site's raw HTML (to read the embedded
// widget config) plus the JSON per-job endpoint.
type bairesDevHTTP interface {
	TextGetter
	JSONGetter
}

const (
	bairesDevListURL = "https://talent.bairesdev.com/"
	// bairesDevJobPostingURL is the SEO JSON-LD endpoint: clean title, real posted date, and
	// remote flag, but only a short teaser description.
	bairesDevJobPostingURL = "https://applicants.bairesdev.com/api/JobPosting?JobPostingId=%s"
	// bairesDevJobDetailURL is the apply-flow endpoint: it carries the FULL HTML description
	// (JobPosting's is truncated to a blurb), but no date/location, so the two are merged.
	bairesDevJobDetailURL = "https://applicants.bairesdev.com/api/Job?JobOfferId=%s"
	bairesDevApplyURL     = "https://applicants.bairesdev.com/job/%s/%s/apply"
)

// NewBairesDev builds the BairesDev adapter over the given HTTP client.
func NewBairesDev(c bairesDevHTTP) Source { return bairesdev{http: c} }

func (bairesdev) Provider() string { return "bairesdev" }

// bairesdev has one global talent listing, so its config entry carries no board.
func (bairesdev) boardless() {}

// bairesDevRef is one distinct posting to hydrate: the career-site id and the job id parsed from a
// listing row's apply link.
type bairesDevRef struct{ careerID, jobID string }

// bairesDevWidgetConfig extracts the base64 Duda job-widget config from the talent site HTML.
var bairesDevWidgetConfig = regexp.MustCompile(`data-widget-config="([A-Za-z0-9+/=]+)"`)

// bairesDevApplyPath captures the career-site id and job id from an apply URL.
var bairesDevApplyPath = regexp.MustCompile(`/job/(\d+)/(\d+)/apply`)

func (b bairesdev) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	page, err := b.http.GetText(ctx, bairesDevListURL)
	if err != nil {
		return nil, fmt.Errorf("bairesdev: fetch talent listing: %w", err)
	}
	refs, err := parseBairesDevListing(page)
	if err != nil {
		return nil, fmt.Errorf("bairesdev: %w", err)
	}
	// The listing carries only the apply id; the clean title, description, date, and remote flag
	// come from the per-job endpoint, so each ref is hydrated (skipped on a per-job failure so one
	// bad id never aborts the crawl).
	return fetchDetails(refs, defaultDetailWorkers, func(r bairesDevRef) (Job, bool) {
		return b.detail(ctx, r)
	}), nil
}

// parseBairesDevListing decodes the talent site's embedded widget config and returns each distinct
// posting, deduped by job id (the site lists the same posting once per country/city under a shared
// id, so dedup collapses the location templates to the real openings).
func parseBairesDevListing(page string) ([]bairesDevRef, error) {
	m := bairesDevWidgetConfig.FindStringSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("no job widget on talent listing")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		return nil, fmt.Errorf("decode widget config: %w", err)
	}
	var cfg struct {
		JobList []struct {
			URL string `json:"page_item_url"`
		} `json:"jobList"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse widget config: %w", err)
	}
	var refs []bairesDevRef
	seen := map[string]struct{}{}
	for _, j := range cfg.JobList {
		mm := bairesDevApplyPath.FindStringSubmatch(j.URL)
		if mm == nil {
			continue
		}
		jobID := mm[2]
		if _, ok := seen[jobID]; ok {
			continue
		}
		seen[jobID] = struct{}{}
		refs = append(refs, bairesDevRef{careerID: mm[1], jobID: jobID})
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no apply links in widget config")
	}
	return refs, nil
}

// bairesDevPosting selects the JobPosting ld+json fields the public endpoint returns.
type bairesDevPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// detail hydrates one posting: the JobPosting endpoint supplies the clean title, real date, and
// remote flag; the Job endpoint supplies the full description (JobPosting's is truncated). The
// external id and URL match the linksource adapter exactly, so a job crawled here and one followed
// from a Telegram link dedup into a single row. ok=false (skip) when the id no longer resolves to a
// live posting.
func (b bairesdev) detail(ctx context.Context, r bairesDevRef) (Job, bool) {
	var p bairesDevPosting
	if err := b.http.GetJSON(ctx, fmt.Sprintf(bairesDevJobPostingURL, r.jobID), &p); err != nil {
		return Job{}, false
	}
	if strings.TrimSpace(p.Title) == "" {
		return Job{}, false // no live posting under this id
	}

	// The full description lives on the apply-flow endpoint; fall back to the JobPosting teaser
	// when it is unavailable, so a Job-endpoint hiccup degrades the text instead of dropping the job.
	desc := b.fullDescription(ctx, r.jobID)
	if desc == "" {
		desc = p.Description
	}
	// datePosted has no timezone (e.g. "2025-06-02T10:23:21.503"), so RFC3339 rejects it; fall
	// back to the date alone, keeping the approximate posted_at.
	posted := parseRFC3339(p.DatePosted)
	if posted == nil && len(p.DatePosted) >= 10 {
		posted = parseDate(p.DatePosted[:10])
	}
	company := p.HiringOrganization.Name
	if company == "" {
		company = "BairesDev"
	}
	mode := ""
	if strings.EqualFold(p.JobLocationType, "TELECOMMUTE") {
		mode = "remote"
	}
	return Job{
		ExternalID:  r.jobID,
		URL:         fmt.Sprintf(bairesDevApplyURL, r.careerID, r.jobID),
		Title:       p.Title,
		Company:     company,
		Description: sanitizeHTML(desc),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    posted,
	}, true
}

// fullDescription reads the full HTML description from the apply-flow endpoint
// (jobResults[0].description), or "" when the endpoint fails or carries no result.
func (b bairesdev) fullDescription(ctx context.Context, jobID string) string {
	var resp struct {
		JobResults []struct {
			Description string `json:"description"`
		} `json:"jobResults"`
	}
	if err := b.http.GetJSON(ctx, fmt.Sprintf(bairesDevJobDetailURL, jobID), &resp); err != nil {
		return ""
	}
	if len(resp.JobResults) == 0 {
		return ""
	}
	return resp.JobResults[0].Description
}
