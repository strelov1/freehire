package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// djinni adapts djinni.co, a Ukrainian/CEE IT job board. Each anonymous listing page
// (/jobs/?page=N) embeds one application/ld+json block that is an ARRAY of full schema.org
// JobPosting objects — description included — so one fetch per page yields every posting
// without a per-posting detail request. Boardless (djinni.co is one site with no per-tenant
// board) and an aggregator (many companies; each posting's company comes from the feed), so it
// stays in the source facet and inherits the reindex aggregator/ATS suppression.
type djinni struct {
	http HTMLResolvedGetter
}

const (
	// djinniListBase is the guest listing; the caller appends the "page=N" (1-based) marker,
	// which doubles as the end-of-feed sentinel checked against the resolved final URL.
	djinniListBase = "https://djinni.co/jobs/?"
	// djinniMaxPages caps pagination as a backstop, comfortably above the observed corpus
	// (~488 pages). The empty-page stop below is the primary terminator; this only bounds a
	// pathological feed that never empties.
	djinniMaxPages = 600
)

// djinniPageDelay paces the sequential page crawl. Djinni rate-limits a fast burst from a
// datacenter IP with a 403 (a no-delay crawl 403'd around page ~200 from prod, while the same
// pages spaced ~1s apart return 200), so we throttle to stay under the limit and be a polite
// crawler; partial-on-error bounds the damage if a 403 still lands. A var so tests zero it.
var djinniPageDelay = 600 * time.Millisecond

// NewDjinni builds the djinni listing adapter over the given HTML client.
func NewDjinni(c HTMLResolvedGetter) Source { return djinni{http: c} }

func (djinni) Provider() string { return "djinni" }

func (djinni) boardless() {}

func (djinni) aggregator() {}

// Fetch pages the listing from page 1 upward, mapping each page's JSON-LD JobPosting array to
// jobs. It stops at the end of the feed, detected by the redirect: a past-the-end page 302s to
// the bare listing (/jobs/), so the FINAL URL no longer carries the requested page marker.
// (Following the redirect would otherwise re-serve page 1 indefinitely, since page 1 is not
// empty.) A genuinely empty non-redirected page is a secondary stop.
//
// A page fetch that fails partway (Djinni 403s a datacenter IP that crawls too fast) does NOT
// discard the crawl: the pages already collected are the freshest postings (Djinni orders by
// recency), so Fetch keeps them and stops. It fails the whole board only when page 1 itself
// fails — an empty successful crawl would otherwise let the unseen-sweep close the catalogue.
func (s djinni) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= djinniMaxPages; page++ {
		if page > 1 {
			select {
			case <-ctx.Done():
				return jobs, ctx.Err()
			case <-time.After(djinniPageDelay):
			}
		}
		pageMarker := fmt.Sprintf("page=%d", page)
		root, final, err := s.http.GetHTMLResolved(ctx, djinniListBase+pageMarker)
		if err != nil {
			if len(jobs) == 0 {
				return nil, fmt.Errorf("djinni: fetch page %d: %w", page, err)
			}
			log.Printf("djinni: page %d failed (%v); keeping %d jobs from pages 1..%d", page, err, len(jobs), page-1)
			break // partial crawl — keep the freshest pages rather than losing everything
		}
		if !strings.Contains(final, pageMarker) {
			break // redirected off the end of the feed (past-the-end page 302s to /jobs/)
		}
		nodes := LDJobPostings(root)
		if len(nodes) == 0 {
			break // a non-redirected page with no postings — nothing more to read
		}
		for _, raw := range nodes {
			var p djinniPosting
			if json.Unmarshal(raw, &p) != nil {
				continue // a posting that fails to decode is skipped, never aborting the page
			}
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// djinniPosting is one schema.org JobPosting from the listing. identifier is a bare integer;
// jobLocationType (TELECOMMUTE/ON_SITE) is the work-arrangement signal; the applicant location
// requirement carries only a country.
type djinniPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	URL                string `json:"url"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	Identifier         int64  `json:"identifier"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	ApplicantLocationRequirements struct {
		Address struct {
			AddressCountry string `json:"addressCountry"`
		} `json:"address"`
	} `json:"applicantLocationRequirements"`
}

// toJob maps a posting to a Job, returning ok=false for a posting with no identifier (no dedup
// key), no url (no canonical address), or no company (which would break the company slug).
func (p djinniPosting) toJob() (Job, bool) {
	if p.Identifier == 0 || p.URL == "" || p.HiringOrganization.Name == "" {
		return Job{}, false
	}
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	return Job{
		ExternalID:     strconv.FormatInt(p.Identifier, 10),
		URL:            p.URL,
		Title:          p.Title,
		Company:        p.HiringOrganization.Name,
		Location:       p.ApplicantLocationRequirements.Address.AddressCountry,
		Description:    sanitizeHTML(plainTextToHTML(p.Description)),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       djinniDate(p.DatePosted),
	}, true
}

// djinniDate parses Djinni's datePosted, an ISO-8601 local datetime WITHOUT a zone (e.g.
// "2026-07-16T04:43:20.486231"). parseRFC3339 rejects the missing zone, so this reads it as
// UTC, trying parseRFC3339 first (should Djinni ever emit a zoned value) and a bare date last.
func djinniDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	if t := parseLayout("2006-01-02T15:04:05.999999999", s); t != nil {
		return t
	}
	return parseDate(s)
}

// djinni's plain-text JSON-LD description is rebuilt into structural HTML by the shared
// plainTextToHTML helper (see plaintext.go), reused by other plain-text sources (lumenalta).
