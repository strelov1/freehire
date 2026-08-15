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
	// himalayasLimit is the page size requested per offset page. Himalayas caps the page
	// size at 20 regardless of the requested value, so this matches the cap; the loop
	// advances by the count actually returned (not by this), staying correct even if the
	// cap changes.
	himalayasLimit = 20
	// himalayasMaxPages is a per-run page budget, not just a runaway guard. Himalayas rate-
	// limits (429) after ~150 rapid requests, so a single run crawling the full ~88k catalogue
	// would grind for many minutes against the limit. This budget keeps each run under the
	// limit (≈ himalayasMaxPages × himalayasLimit freshest jobs per run), so the crawl is fast
	// and never trips the 429; the idempotent upsert plus the cron cadence keep coverage fresh.
	// (Full back-catalogue coverage would need a persisted offset cursor across runs — a seam,
	// not built: the feed is recency-ordered, so the freshest slice is what matters.)
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
	for offset, page := 0, 0; page < himalayasMaxPages; page++ {
		var resp himalayasResponse
		url := fmt.Sprintf(himalayasListURL, himalayasLimit, offset)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			// Himalayas rate-limits (429) after a number of rapid pages. Once we have
			// collected jobs, treat a page failure as the end of what we can fetch this run
			// and return the partial result (freshest jobs first) rather than discarding
			// everything; the idempotent upsert means the next run picks up where the rate
			// limit allows. Only a failure on the very first page is a real board error.
			if len(jobs) == 0 {
				return nil, fmt.Errorf("himalayas: list offset %d: %w", offset, err)
			}
			return jobs, nil
		}
		for _, p := range resp.Jobs {
			if job, ok := p.toJob(); ok {
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
// Himalayas' own feed renders companyName as the literal string "name" for a chunk of
// postings (~20% in a sample crawl) — a template field left unresolved on their end.
// companySlug, a separate field carrying the same company, is unaffected, so a posting
// caught by this falls back to it (e.g. "talentcross") rather than being dropped or showing
// the literal "name" as the employer. That slug is the same shape internal/companyname
// already resolves for other slug-only boards; Himalayas has no resolver registered there yet.
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
// already hold from the company's own ATS (see internal/jobhash.RoleFingerprint, which
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
