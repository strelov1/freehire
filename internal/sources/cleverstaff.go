package sources

import (
	"context"
	"fmt"
)

// cleverstaff adapts CleverStaff (cleverstaff.net), a recruiting ATS whose customers publish
// their open roles on public per-tenant pages. Its keyless endpoint,
// getAllOpenVacancy?alias=<tenant>, returns one tenant's whole open list in a single request,
// each object carrying the full HTML description — so one call assembles every Job with no
// per-vacancy detail fetch. The tenant alias is the configured board, making this a per-tenant
// ATS (like greenhouse/lever), not a boardless aggregator.
type cleverstaff struct {
	http JSONGetter
}

const (
	cleverstaffListURL = "https://cleverstaff.net/hr/public/getAllOpenVacancy?alias=%s"
	// cleverstaffVacancyURL builds the public posting URL from a vacancy's short localId.
	cleverstaffVacancyURL = "https://cleverstaff.net/i/vacancy-%s"
)

// NewCleverstaff builds the cleverstaff adapter over the given HTTP client.
func NewCleverstaff(c JSONGetter) Source { return cleverstaff{http: c} }

func (cleverstaff) Provider() string { return "cleverstaff" }

// cleverstaffResponse is the getAllOpenVacancy payload: Status is "ok" on success and Objects
// is the tenant's open-vacancy list.
type cleverstaffResponse struct {
	Status  string               `json:"status"`
	Objects []cleverstaffVacancy `json:"objects"`
}

// cleverstaffVacancy is one open vacancy. VacancyID is the stable dedup key; LocalID is the
// short code the public URL uses; Descr is the full HTML body; ClientName is the real employer
// (used only for hub tenants); DC is the epoch-ms create timestamp used as posted-at.
type cleverstaffVacancy struct {
	VacancyID      string `json:"vacancyId"`
	LocalID        string `json:"localId"`
	Position       string `json:"position"`
	Descr          string `json:"descr"`
	EmploymentType string `json:"employmentType"`
	WorkCondition  string `json:"workCondition"`
	Status         string `json:"status"`
	ClientName     string `json:"clientName"`
	DC             int64  `json:"dc"`
}

// cleverstaffEmploymentType maps CleverStaff's employmentType enum to freehire's controlled
// EmploymentType vocabulary. The keys are the values CleverStaff actually emits (observed live:
// fullEmployment, underemployment, projectWork); an unrecognized value maps to "" so the
// dictionaries decide, rather than guessing at keys the platform does not use.
var cleverstaffEmploymentType = map[string]string{
	"fullEmployment":  "full_time",
	"underemployment": "part_time",
	"projectWork":     "contract",
}

func (s cleverstaff) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp cleverstaffResponse
	if err := s.http.GetJSON(ctx, fmt.Sprintf(cleverstaffListURL, e.Board), &resp); err != nil {
		return nil, fmt.Errorf("cleverstaff: alias %q: %w", e.Board, err)
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("cleverstaff: alias %q: status %q", e.Board, resp.Status)
	}
	var jobs []Job
	for _, v := range resp.Objects {
		// Drop an object we cannot key (no vacancyId), address (no localId → no URL), or title
		// (no position); filter out any vacancy the feed does not report as open. One dropped
		// object never aborts the rest of the tenant.
		if v.VacancyID == "" || v.LocalID == "" || v.Position == "" || !cleverstaffOpen(v.Status) {
			continue
		}
		jobs = append(jobs, s.toJob(e, v))
	}
	return jobs, nil
}

// cleverstaffClosed is the set of terminal vacancy statuses. getAllOpenVacancy serves only open
// vacancies (its documented open set is In Progress / New / Replacement, all seen as "inwork"),
// so this filter is defensive: it denylists explicitly-terminal statuses rather than allowlisting
// open ones, so an undocumented open status is never wrongly dropped while a stale closed object
// still is.
var cleverstaffClosed = map[string]bool{"closed": true, "canceled": true, "cancelled": true}

// cleverstaffOpen reports whether a vacancy status is an open state (anything not denylisted as
// terminal; see cleverstaffClosed).
func cleverstaffOpen(status string) bool { return !cleverstaffClosed[status] }

// toJob maps one vacancy to a Job. Structured facets are carried only for cleanly-mapped values
// (work mode, employment type); PostedAt is the vacancy's create timestamp (dc). The employer is
// the configured company, except on a hub tenant where each vacancy's own clientName is used
// (falling back to the configured company when clientName is blank).
func (cleverstaff) toJob(e CompanyEntry, v cleverstaffVacancy) Job {
	company := e.Company
	if e.Hub {
		company = firstNonEmpty(v.ClientName, e.Company)
	}
	return Job{
		ExternalID:     v.VacancyID,
		URL:            fmt.Sprintf(cleverstaffVacancyURL, v.LocalID),
		Title:          v.Position,
		Company:        company,
		Description:    sanitizeHTML(v.Descr),
		WorkMode:       workplaceTypeMode(v.WorkCondition),
		EmploymentType: cleverstaffEmploymentType[v.EmploymentType],
		PostedAt:       parseEpochMillis(v.DC),
	}
}
