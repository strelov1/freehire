package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// scalis adapts Scalis, a multi-tenant ATS (<board>.scalis.ai). The board is the tenant
// subdomain. The listing page is a Next.js App Router app whose RSC flight inlines a
// complete, richly-structured job object per posting — title, company, locations,
// employment/workplace enums, skills, salary, and a full HTML description reachable as a
// "$<id>" reference into the flight's own text rows — so, unlike topco, no per-posting
// detail fetch is needed at all. The listing is paginated (confirmed live: 10 results per
// page, an empty result list past the last page, no redirect trap), so Fetch pages to
// exhaustion using the shared fetchFlight/bracketSlice/nextFlightTextRows primitives every
// other RSC-flight adapter (deel, topco, micro1) already uses.
type scalis struct {
	http HTMLGetter
}

// NewScalis builds the Scalis adapter over the given HTML client.
func NewScalis(c HTMLGetter) Source { return scalis{http: c} }

func (scalis) Provider() string { return "scalis" }

// scalisPageSize is the listing's fixed page size (confirmed live).
const scalisPageSize = 10

// scalisMaxPages bounds the page walk. The natural stop is a page whose result list is
// empty (confirmed live, including past the true last page), but a misbehaving tenant that
// never returns an empty page would otherwise loop forever. `limit` is confirmed
// server-clamped (a live `limit=100` request still answered 10 results per page), so a
// larger tenant genuinely needs more pages rather than a bigger page — this is a wide
// safety margin over the one measured tenant (46 postings, 5 pages), not a real board size,
// and reaching it is a hard failure (see Fetch), never a silent partial success, the same
// lesson `teamtailor.ttMaxPages` already paid for on a real board.
const scalisMaxPages = 500

// fullBoardListing: jobURLs proves completeness by paginating to a genuinely empty page,
// and treats a later-page failure or reaching the scalisMaxPages safety ceiling as a hard
// Fetch failure rather than a partial success — see the fullBoardListing interface's own
// bar (source.go) and scalisMaxPages's comment for why a reachable ceiling must fail loudly.
func (scalis) fullBoardListing() {}

func (s scalis) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	refs, resolved := 0, 0
	for page := 1; page <= scalisMaxPages; page++ {
		url := fmt.Sprintf("https://%s.scalis.ai/jobs?page=%d&limit=%d&sortBy=SORT_BEST_MATCH",
			e.Board, page, scalisPageSize)
		flight, err := fetchFlight(ctx, s.http, url)
		if err != nil {
			return nil, fmt.Errorf("scalis: board %q page %d: %w", e.Board, page, err)
		}
		listing, err := extractScalisListing(flight)
		if err != nil {
			return nil, fmt.Errorf("scalis: board %q page %d: %w", e.Board, page, err)
		}
		if len(listing.Results) == 0 {
			// Postings reference their descriptions by id into the flight's text rows
			// (see scalisToJob). If every reference on the whole board failed to
			// resolve, the row parse broke — e.g. the marker format changed — so fail
			// loudly rather than ship a board of empty-bodied jobs; a single
			// unresolved reference on an otherwise-healthy board still yields its
			// posting with an empty description (tolerated degradation), matching deel.
			if refs > 0 && resolved == 0 {
				return nil, fmt.Errorf("scalis: board %q: %d description references but none resolved", e.Board, refs)
			}
			return jobs, nil
		}
		rows := nextFlightTextRows(flight)
		for _, p := range listing.Results {
			desc, hadRef := scalisDescription(p, rows)
			if hadRef {
				refs++
				if desc != "" {
					resolved++
				}
			}
			if j, ok := scalisToJob(e, desc, p); ok {
				jobs = append(jobs, j)
			}
		}
	}
	// The loop ran out scalisMaxPages without ever seeing an empty page — the board is
	// not proven to have ended, so this is not "here is what we found," it is a failure.
	return nil, fmt.Errorf("scalis: board %q: reached the %d-page safety ceiling without finding the board's end",
		e.Board, scalisMaxPages)
}

// scalisDescription resolves a posting's description, preferring descriptionHtml (it
// preserves paragraph/list structure) and falling back to the plain-text description when
// the HTML field's own reference fails to resolve — a row-parse hiccup on just that one
// field still keeps a body rather than losing it. hadRef reports whether either field was a
// "$<id>" reference at all, which Fetch uses for the board-wide resolution health check: it
// is the single source of truth both scalisToJob and that check read, so they can never
// disagree about whether a given posting's reference resolved.
func scalisDescription(p scalisPosting, rows map[string]string) (desc string, hadRef bool) {
	for _, field := range []string{p.DescriptionHTML, p.Description} {
		ref, isRef := strings.CutPrefix(field, "$")
		if !isRef {
			if field != "" {
				return field, hadRef // an inline (non-reference) value wins outright
			}
			continue
		}
		hadRef = true
		if v := rows[ref]; v != "" {
			return v, true
		}
	}
	return "", hadRef
}

// scalisListing is the "initialData" object a listing page's flight carries.
type scalisListing struct {
	Results []scalisPosting `json:"results"`
}

// scalisPosting is one listing result. description/descriptionHtml are each either a
// "$<id>" reference into the flight's text rows or, defensively, an inline string.
type scalisPosting struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	DescriptionHTML string   `json:"descriptionHtml"`
	Employment      string   `json:"employment"`
	Workplace       string   `json:"workplace"`
	Payment         string   `json:"payment"`
	Skills          []string `json:"skills"`
	// Company is deliberately NOT decoded: a Scalis board is single-tenant, so the
	// curator-configured CompanyEntry.Company is authoritative. React's RSC flight also
	// deduplicates repeated identical objects across an array — every posting after the
	// first carries "company" as a path-backreference STRING ("$5:2:props:...") rather
	// than a literal object, which a typed field would fail to unmarshal (this broke
	// every crawl of boldbusiness in production). Leaving the field out entirely means
	// json.Unmarshal skips it regardless of its shape.
	Locations []struct {
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"locations"`
	Salary struct {
		Min      *int   `json:"min"`
		Max      *int   `json:"max"`
		Currency string `json:"currency"`
	} `json:"salary"`
	CreatedAt string `json:"createdAt"`
}

// extractScalisListing decodes the initialData object out of a listing page's flight. It
// anchors on the payload's own opening — `"initialData":{"results"` — rather than a bare
// `"initialData":`, so a stray unrelated match cannot pick the wrong object.
func extractScalisListing(flight string) (scalisListing, error) {
	raw, ok := bracketSlice(flight, `"initialData":{"results"`, '{', '}')
	if !ok {
		return scalisListing{}, fmt.Errorf("no initialData object found")
	}
	var l scalisListing
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		return scalisListing{}, fmt.Errorf("decode initialData: %w", err)
	}
	return l, nil
}

// scalisToJob maps one listing result to a Job, given its already-resolved description
// (see scalisDescription). ok is false when the posting carries no id, which would collide
// on the (source, external_id) dedup key.
func scalisToJob(e CompanyEntry, desc string, p scalisPosting) (Job, bool) {
	if p.ID == "" {
		return Job{}, false
	}

	var locParts []string
	for _, l := range p.Locations {
		locParts = append(locParts, joinNonEmpty(l.City, l.Country))
	}
	location := joinNonEmpty(locParts...)

	workMode := scalisWorkMode(p.Workplace)
	salaryMin, salaryMax, salaryCurrency, salaryPeriod := scalisSalary(p)

	return Job{
		ExternalID:     p.ID,
		URL:            fmt.Sprintf("https://%s.scalis.ai/job/%s", e.Board, p.ID),
		Title:          strings.TrimSpace(p.Title),
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(desc),
		Remote:         workMode == "remote" || isRemote(p.Title+" "+location),
		WorkMode:       workMode,
		EmploymentType: scalisEmploymentType(p.Employment),
		Skills:         p.Skills,
		SalaryMin:      salaryMin,
		SalaryMax:      salaryMax,
		SalaryCurrency: salaryCurrency,
		SalaryPeriod:   salaryPeriod,
		PostedAt:       parseRFC3339(p.CreatedAt),
	}, true
}

// scalisWorkMode maps Scalis's workplace enum onto freehire's work-mode vocabulary,
// returning "" for an unrecognized value so the pipeline's location heuristic decides.
func scalisWorkMode(workplace string) string {
	switch strings.ToUpper(strings.TrimSpace(workplace)) {
	case "REMOTE":
		return "remote"
	case "HYBRID":
		return "hybrid"
	case "ON_SITE":
		return "onsite"
	}
	return ""
}

// scalisEmploymentType maps Scalis's employment enum onto vocab.EmploymentTypeValues,
// returning "" for an unrecognized value. TEMPORARY folds onto "contract", matching how
// this codebase's other adapters (e.g. talenthr) already read a temporary engagement.
func scalisEmploymentType(employment string) string {
	switch strings.ToUpper(strings.TrimSpace(employment)) {
	case "FULL_TIME":
		return "full_time"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	}
	return ""
}

// scalisSalary maps a posting's salary bounds and the payment enum's own unit into
// freehire's structured salary fields, returning all four empty/nil when neither bound is
// stated. The payment enum states the unit directly ("SALARY" is an annual figure,
// "HOURLY" an hourly one), so no bound is reported without a period to attach it to.
func scalisSalary(p scalisPosting) (min, max *int, currency, period string) {
	if p.Salary.Min == nil && p.Salary.Max == nil {
		return nil, nil, "", ""
	}
	switch strings.ToUpper(strings.TrimSpace(p.Payment)) {
	case "SALARY":
		period = "year"
	case "HOURLY":
		period = "hour"
	default:
		return nil, nil, "", "" // an unrecognized unit would misstate the figure
	}
	return p.Salary.Min, p.Salary.Max, p.Salary.Currency, period
}
