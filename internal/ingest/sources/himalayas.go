package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// himalayas adapts himalayas.app, a remote-jobs aggregator. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. The /jobs/api endpoint pages by offset/limit over a
// reported totalCount; the site is remote-only, so every job is remote.
type himalayas struct {
	http JSONGetter
}

const (
	// himalayasLimit is the page size requested. Himalayas caps it at 20; the loop advances by the
	// count actually returned, so it stays correct if the cap changes.
	himalayasLimit = 20
	// himalayasMaxPages is a per-run page budget, not just a runaway guard: Himalayas rate-limits
	// (429) after ~150 rapid requests, so the budget keeps each run under the limit (about
	// himalayasMaxPages × himalayasLimit freshest jobs); the idempotent upsert plus the cron
	// cadence keep coverage fresh. Seam: full back-catalogue coverage would need a persisted
	// offset cursor, not built because the feed is recency-ordered.
	himalayasMaxPages = 120
	himalayasListURL  = "https://himalayas.app/jobs/api?limit=%d&offset=%d"
)

// himalayasCompanyNameSentinel is the literal value Himalayas' feed sends as companyName for
// a subset of postings, instead of the real company — see toJob.
const himalayasCompanyNameSentinel = "name"

// NewHimalayas builds the Himalayas adapter over the given HTTP client.
func NewHimalayas(c JSONGetter) Source { return himalayas{http: c} }

func (himalayas) Provider() string { return "himalayas" }

func (himalayas) boardless() {}

func (himalayas) aggregator() {}

// himalayas pages only its freshest slice per run; cmd/liveness checks the site's own sitemap.
func (himalayas) livenessProbed() {}

// himalayasResponse is one offset page: the postings plus the catalogue-wide total used to
// decide whether another page is due.
type himalayasResponse struct {
	TotalCount int                `json:"totalCount"`
	Jobs       []himalayasPosting `json:"jobs"`
}

// himalayasPosting is one posting, body inline (no detail call). pubDate is epoch seconds.
type himalayasPosting struct {
	Title                string   `json:"title"`
	CompanyName          string   `json:"companyName"`
	CompanySlug          string   `json:"companySlug"`
	ApplicationLink      string   `json:"applicationLink"`
	GUID                 string   `json:"guid"`
	LocationRestrictions []string `json:"locationRestrictions"`
	Description          string   `json:"description"`
	PubDate              int64    `json:"pubDate"`
}

func (s himalayas) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	seen := make(map[string]bool)
	for offset, page := 0, 0; page < himalayasMaxPages; page++ {
		var resp himalayasResponse
		url := fmt.Sprintf(himalayasListURL, himalayasLimit, offset)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			// Himalayas rate-limits (429) after a number of rapid pages. Once jobs are collected,
			// a page failure ends the run with the partial (freshest-first) result; only a failure
			// on the first page is a real board error.
			if len(jobs) == 0 {
				return nil, fmt.Errorf("himalayas: list offset %d: %w", offset, err)
			}
			return jobs, nil
		}
		for _, p := range resp.Jobs {
			if job, ok := p.toJob(); ok && !seen[job.ExternalID] {
				// Offset pages can repeat a GUID. Emit it once so concurrent
				// ingest writers cannot race on the same identity and public slug.
				seen[job.ExternalID] = true
				jobs = append(jobs, job)
			}
		}
		// Advance by the count actually returned: Himalayas caps the page size below the
		// requested limit, so a fixed stride would skip postings. Stop on an empty page or
		// once the offset covers the reported total.
		offset += len(resp.Jobs)
		if len(resp.Jobs) == 0 || offset >= resp.TotalCount {
			break
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// guid to key on, or no company which would break the slug). Himalayas lists only remote jobs.
//
// Himalayas' feed renders companyName as the literal string "name" for a subset of postings (an
// unresolved template field on their end). companySlug carries the same company, so such a posting
// falls back to it (e.g. "talentcross") rather than being dropped or showing "name" as the
// employer. Himalayas has no resolver registered in internal/dict/companyname yet.
func (p himalayasPosting) toJob() (Job, bool) {
	company := p.CompanyName
	if strings.EqualFold(strings.TrimSpace(company), himalayasCompanyNameSentinel) {
		company = p.CompanySlug
	}
	if p.GUID == "" || company == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  p.GUID,
		URL:         p.ApplicationLink,
		Title:       p.Title,
		Company:     company,
		Location:    joinNonEmpty(p.LocationRestrictions...),
		Description: StripHimalayasSelfPromo(sanitizeHTML(p.Description)),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseEpochSeconds(p.PubDate),
	}, true
}

// Himalayas brands the bodies it serves: every posting ends with an "Originally posted on
// Himalayas" trailer. That paragraph is not the employer's text, and the extra visible text
// is what stops a mirrored posting from fingerprinting to the same role as the copy we
// already hold from the company's own ATS (see internal/job/jobhash.RoleFingerprint, which
// hashes visible text). The board's other habit — rewriting each mention of the hiring
// company into a backlink to its himalayas.app page — needs no handling here: sanitizeHTML
// unwraps every anchor in every source, leaving the company's name as the plain text it was.
//
// The anchor is optional in the pattern because the trailer reaches this function both ways:
// unwrapped when it arrives through sanitizeHTML, and still linked in the catalogue rows
// stored before descriptions dropped their links.
var himalayasPromoTrailer = regexp.MustCompile(`(?is)\s*<p>\s*Originally posted on\s*(?:<a\b[^>]*href="https?://himalayas\.app/?"[^>]*>\s*)?Himalayas\s*(?:</a>\s*)?</p>\s*$`)

// StripHimalayasSelfPromo removes the trailing "Originally posted on Himalayas" paragraph
// from a posting body. A body without one is returned unchanged, and a cleaned body is
// unchanged by a second pass, so the backfill can re-run over rows it already fixed.
func StripHimalayasSelfPromo(s string) string {
	return himalayasPromoTrailer.ReplaceAllString(s, "")
}
