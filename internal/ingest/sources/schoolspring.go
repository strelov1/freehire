package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// schoolspring adapts SchoolSpring (schoolspring.com), the K-12 education job board PowerSchool
// runs. Its central site is a React SPA over a keyless JSON API at api.schoolspring.com, and that
// API is what this adapter reads — a listing endpoint that answers a search, and a per-posting
// detail endpoint that holds the body the listing omits.
//
// # The board is a SEARCH KEYWORD, and the reason is the whole design decision
//
// SchoolSpring has no per-tenant board. One central index serves every employer (80,611 open
// postings on 2026-09-02), pagination reaches all of it — page 807 at size 100 returns the last
// 11 rows and the count endpoint agrees exactly — and an employer is a FILTER (`organization`)
// over that one index, read back off each posting as `employerID`/`employerName`. So the
// per-employer shape does not exist to be crawled.
//
// Crawling the whole index would be wrong for a different reason: it is a school board, not an IT
// one, and freehire's own dictionaries say so twice. `classify.IsTech` fires on 36 of the 80,611
// titles (0.04%); a generous substring sweep for IT vocabulary finds 328 (0.4%). But
// `classify.ConfirmedNonTech` — the gate that decides what a crawl may store — turns away only
// 39,068 and **admits 41,543**, because it is a non-tech TITLE dictionary written against
// tech-company ATS boards and it has no term for K-12 classified staff. What it lets through is
// school psychologists, bus monitors, crossing guards, football coaches and cafeteria workers. An
// unscoped crawl would move forty thousand non-technical postings into the catalogue and re-fetch
// their bodies every run (a rejected posting is never stored, so it is never `seen`).
//
// The platform's own `category` taxonomy is the obvious thing to scope with — a server-side facet
// on the same index, the shape hh and trudvsem use — and it was measured before the keyword list
// was: it does not work here. 37% of postings carry no category at all (the union of all 239
// categories is 50,644 of 80,611), the densest categories reach only about a quarter of the
// technical postings, and the categories overlap so heavily — Administration/Technology and
// Student Services/Educational Technology share 335 of their 418 and 419 postings — that one board
// per category would store 40% of its rows twice.
//
// `keyword` is what selects a crawlable slice. It is a plain case-insensitive SUBSTRING match over
// a posting's title and its employer name (which is why "it manager" matches "Unit Manager" and
// why the keywords in sources/schoolspring.yml are whole phrases), it filters rather than ranks —
// "software developer" answers zero rather than padding with near matches, the way WhatJobs' feed
// does — and every hit for a term carries that term. That is the same board-is-a-keyword shape
// sources/whatjobs.yml uses, and sources/hh.yml's board-is-a-professional-role before it.
//
// # TEDK12 is a different product, and this adapter does not crawl it
//
// The per-tenant TEDK12 sites (pembinatrails.tedk12.ca and the like) are PowerSchool Applicant
// Tracking, a separate ATS: per-district hosts, server-rendered ASP.NET with a __VIEWSTATE and no
// JSON of any kind, board = the tenant host. They are not another spelling of this board and would
// be their own adapter with its own host discovery.
//
// They are also largely already here. 332 of the 377 distinct upstream hosts this crawl's postings
// link back to are `*.tedk12.com` tenants, carrying 622 of its 1,261 postings — SchoolSpring is
// where the US TEDK12 fleet publishes, so crawling both would mostly re-crawl the same postings
// under a second identity. What it would ADD is the fleet outside the US index: not one
// `*.tedk12.ca` host appears among those 377, and Pembina Trails' postings are absent from the
// national index entirely (searched live). A Canadian TEDK12 adapter is therefore a real, separate
// piece of work rather than a variant of this one.
//
// # Consequences worth knowing
//
//   - A posting whose title matches two keywords is stored twice, under each board's external_id
//     namespace. The first prod-shaped run stored 383 rows, 35 of them (9%) a second copy of a
//     posting another board also listed. It is stable rather than churning — both boards list
//     their copy on every crawl, so neither row ages out — and the duplicate markers collapse the
//     pair in search.
//   - The index carries stale inventory the platform never retired: 17,700 of the 80,611 postings
//     display a date before 2025, some as far back as 2010, and 33 of the 383 rows this crawl stores
//     (9%) are among them. They are served as open and no field says otherwise, so nothing here
//     can filter them — and nothing downstream closes them either: they are re-listed on every
//     crawl, so the unseen sweep never fires, and cmd/liveness skips registered providers. Only
//     the ghost signal marks them, and only cmd/prune removes them. Giving them a verdict would
//     mean a prober of this API's own (the detail resource answers success=false for a posting
//     that is gone, which a plain GET of the SPA posting page cannot see) — a separate piece of
//     work, noted here rather than guessed at.
//   - SchoolSpring re-publishes postings that live on the employer's own ATS (jobInfo.infoURL
//     points at teachermatch, applitrack, tedk12, searchsoft and others), so it is an aggregator:
//     a first-party copy of the same posting wins over this one.
//   - No metering observed: 82 listing pages at size 1000, 239 category probes at 6-way
//     concurrency and 1,261 detail fetches at 8-way (≈13 req/s) all answered 200. So no pacer is
//     wired; pacer.go is where one would go.
type schoolspring struct {
	http JSONGetter
}

// NewSchoolSpring builds the SchoolSpring adapter over the given HTTP client. Both the listing
// and the detail resource are keyless JSON GETs.
func NewSchoolSpring(c JSONGetter) Source { return schoolspring{http: c} }

func (schoolspring) Provider() string { return "schoolspring" }

// aggregator marks SchoolSpring as a multi-company aggregator — the employer comes from each
// posting and many of those employers also publish the same posting on their own ATS, so the
// cross-source dedup pass prefers the first-party copy. It still requires a board (the search
// keyword) to bound the crawl, so it is not boardless.
func (schoolspring) aggregator() {}

const (
	// schoolspringAPI is the SPA's own backend. The human-facing site (www.schoolspring.com)
	// renders client-side and serves no posting data of its own.
	schoolspringAPI = "https://api.schoolspring.com"
	// schoolspringDomain names the job board to search. SchoolSpring hosts per-district boards
	// on subdomains (vansd.schoolspring.com and the like); the national domain is the one that
	// sees every employer, and it is what the API falls back to anyway — stating it keeps the
	// request saying which board it means.
	schoolspringDomain = "www.schoolspring.com"
	// schoolspringPageSize is the listing page size. The endpoint honours far larger values, but
	// no committed keyword reaches even one page of 100, so the modest size only ever costs the
	// one request it takes to see the slice's end.
	schoolspringPageSize = 100
	// schoolspringMaxPages backstops the page loop. The walk's real stop condition is a page that
	// adds no posting; this only bounds an endpoint that keeps answering with fresh inventory.
	schoolspringMaxPages = 50
	// schoolspringDateLayout is the API's zoneless timestamp ("2026-09-02T07:00:00"); read as UTC,
	// which is close enough for a posted-at that only orders postings by day.
	schoolspringDateLayout = "2006-01-02T15:04:05"
)

// schoolspringEnvelope is the wrapper every endpoint answers with. Success matters: a request for
// a posting that is gone is answered 200 with success=false and a null jobInfo, so a caller
// reading only the payload would see an empty posting rather than an error.
type schoolspringEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Value   T      `json:"value"`
}

// schoolspringListing is one search-result row. It carries identity and geography but no body,
// which is what makes this a HydratingSource. Its `employer` is deliberately not read: that is the
// individual SCHOOL a posting is filed under, and only the detail resource names the district.
type schoolspringListing struct {
	JobID    int64  `json:"jobId"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

// externalID is the posting's native id as the pipeline's dedup key spells it (the pipeline
// namespaces it by board).
func (l schoolspringListing) externalID() string { return strconv.FormatInt(l.JobID, 10) }

// refreshJob is the liveness refresh for a posting the catalogue already holds: identity only, no
// detail request, and no content that would overwrite the body hydrated when it was new. Company
// is left empty on purpose — the pipeline matches the stored row by identity and re-judges it on
// its title, and the only employer the listing states is the individual school, which is not the
// one the row is filed under.
func (l schoolspringListing) refreshJob() Job {
	return Job{
		ExternalID:  l.externalID(),
		URL:         schoolspringJobURL(l.JobID),
		Title:       schoolspringText(l.Title),
		SeenRefresh: true,
	}
}

// schoolspringDetail is the posting resource: the job itself, plus the places it names.
type schoolspringDetail struct {
	JobInfo      schoolspringJobInfo    `json:"jobInfo"`
	JobLocations []schoolspringLocation `json:"jobLocations"`
}

// schoolspringJobInfo is the posting's own record. Only the fields below are read; the rest of the
// resource is the employer's contact block and the applicant-side application state.
type schoolspringJobInfo struct {
	JobTitle       string `json:"jobTitle"`
	JobDescription string `json:"jobDescription"`
	// EmployerName is the DISTRICT — the employer freehire files the posting under.
	// DisplayEmployer is the individual school, which would scatter one district's postings
	// across dozens of company slugs.
	EmployerName string `json:"employerName"`
	// DisplayDate is when the posting went live, and is the date the site shows. PostDate, the
	// record's own creation stamp, is deliberately not read: the two differ by days on a posting
	// scheduled ahead, and the published date is the one a reader means.
	DisplayDate string `json:"displayDate"`
	// JobTypeName is a closed platform enum (see schoolspringEmploymentType).
	JobTypeName string `json:"jobTypeName"`
	// PayDisplay is the employer's own "publish this figure" switch, and it is off on most
	// postings that carry one — 154 of 200 sampled. PayMin/PayMax are bare numbers.
	PayDisplay int     `json:"payDisplay"`
	PayMin     float64 `json:"payMin"`
	PayMax     float64 `json:"payMax"`
	// SalaryCode is employer-authored free text and PayTypeName the platform's tidier version;
	// the site renders the first and falls back to the second, and so does schoolspringSalaryPeriod.
	SalaryCode  string `json:"salaryCode"`
	PayTypeName string `json:"payTypeName"`
	// CountryName is the employer's contact country, unset on about 40% of postings. It is the
	// only country the platform states anywhere (see applySalary).
	CountryName string `json:"countryName"`
}

// schoolspringLocation is one place a posting names. A posting may name several (217 of 1,261
// live postings did), and only the display string is useful — the lat/lng and the school's own
// name narrow a posting to a building.
type schoolspringLocation struct {
	DisplayLocation string `json:"displayLocation"`
}

func (s schoolspring) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// List-only fallback (no seen set): hydrate every posting.
	return s.crawl(ctx, e, nil)
}

// FetchNew is the hydrating crawl: it lists the whole keyword slice, but fetches a body only for a
// posting the catalogue does not already have. A seen posting is emitted as a liveness refresh
// (identity only, no detail request, no content rewrite).
func (s schoolspring) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.crawl(ctx, e, seen)
}

// crawl lists the keyword's postings and maps each to a Job, hydrating through the shared bounded
// worker pool. seen is nil on the list-only path, where every posting is hydrated.
func (s schoolspring) crawl(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	keyword := strings.TrimSpace(e.Board)
	if keyword == "" {
		return nil, fmt.Errorf("schoolspring: company %q has no search keyword (board)", e.Company)
	}
	postings, err := s.list(ctx, keyword)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(l schoolspringListing) (Job, bool) {
		if seen != nil && seen(l.externalID()) {
			return l.refreshJob(), true
		}
		return s.hydrate(ctx, l)
	}), nil
}

// list pages the keyword's search results until a page adds no posting it has not already
// collected. The endpoint reports no total, so that is the only end-of-slice signal; the walk was
// verified exhaustive against the count endpoint over the whole index (80,611 rows, 81 pages, no
// posting repeated and none missing).
func (s schoolspring) list(ctx context.Context, keyword string) ([]schoolspringListing, error) {
	var out []schoolspringListing
	collected := map[int64]bool{}
	for page := 1; page <= schoolspringMaxPages; page++ {
		rows, err := s.page(ctx, keyword, page)
		if err != nil {
			// The first page failing is a board-level error; a later one ends the walk with what
			// has been gathered, so a mid-listing hiccup costs a page rather than a board. Say so:
			// a truncated slice reads downstream as a keyword that shrank, and the unseen sweep
			// closes the tail it never reached.
			if page == 1 {
				return nil, err
			}
			log.Printf("schoolspring: keyword %q truncated at page %d with %d postings: %v",
				keyword, page, len(out), err)
			return out, nil
		}
		added := 0
		for _, l := range rows {
			if l.JobID == 0 || collected[l.JobID] {
				continue
			}
			collected[l.JobID] = true
			out = append(out, l)
			added++
		}
		if added == 0 {
			return out, nil
		}
	}
	log.Printf("schoolspring: keyword %q still had new postings at the %d-page cap (%d collected); "+
		"narrow it or raise schoolspringMaxPages", keyword, schoolspringMaxPages, len(out))
	return out, nil
}

// page fetches one page of a keyword search. A refusal is answered 200 with success=false, so the
// envelope's flag is an error here as much as a transport failure is — reading only the payload
// would turn a refused search into an empty slice.
func (s schoolspring) page(ctx context.Context, keyword string, page int) ([]schoolspringListing, error) {
	var resp schoolspringEnvelope[struct {
		JobsList []schoolspringListing `json:"jobsList"`
	}]
	if err := s.http.GetJSON(ctx, schoolspringSearchURL(keyword, page), &resp); err != nil {
		return nil, fmt.Errorf("schoolspring: search %q page %d: %w", keyword, page, err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("schoolspring: search %q page %d: %s", keyword, page, resp.Message)
	}
	return resp.Value.JobsList, nil
}

// hydrate fetches one posting's detail and maps it to a Job. ok is false when the request fails or
// when the posting carries no employer or no body — in every one of those cases the posting is
// skipped rather than stored.
//
// Skipping rather than storing is the point. A hydrating adapter re-offers a stored row for
// hydration only while it is younger than pipeline.HydrationRetryWindow, after which `seen`
// reports it like any other row, so a row stored without a body would keep its empty description
// permanently and never reach the search index. A skipped posting stays unseen and the next crawl
// retries it. Twelve of 1,261 live postings publish no body at all; those cost one detail request
// per crawl forever, which is the cheaper half of the trade.
func (s schoolspring) hydrate(ctx context.Context, l schoolspringListing) (Job, bool) {
	var resp schoolspringEnvelope[schoolspringDetail]
	if err := s.http.GetJSON(ctx, schoolspringDetailURL(l.JobID), &resp); err != nil || !resp.Success {
		return Job{}, false
	}
	info := resp.Value.JobInfo
	company := schoolspringText(info.EmployerName)
	body := sanitizeHTML(info.JobDescription)
	if company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}

	location := distinctJoin(resp.Value.JobLocations, "; ", func(p schoolspringLocation) string {
		return schoolspringText(p.DisplayLocation)
	})
	if location == "" {
		location = schoolspringText(l.Location)
	}
	job := Job{
		ExternalID:  l.externalID(),
		URL:         schoolspringJobURL(l.JobID),
		Title:       schoolspringText(firstNonEmpty(info.JobTitle, l.Title)),
		Company:     company,
		Location:    location,
		Description: body,
		Remote:      isRemote(location),
		// The platform states no work arrangement in any structured field, so WorkMode is left
		// for the pipeline's location and description dictionaries. countryName is the employer's
		// contact country as a NAME, which countryFromCode resolves alongside the codes it is
		// usually handed, and it is unset on about 40% of postings.
		Countries:      countryFromCode(info.CountryName),
		EmploymentType: schoolspringEmploymentType(info.JobTypeName),
		PostedAt:       NotFuture(parseLayout(schoolspringDateLayout, info.DisplayDate)),
	}
	info.applySalary(&job)
	return job, true
}

// applySalary copies the posting's structured pay onto the job, but only as a complete statement.
// Four things must hold, and each of them is a thing the platform can leave unsaid: the employer
// switched publishing on (PayDisplay), a bound is present, the period is one freehire has a value
// for, and the posting states the United States as its country.
//
// The country requirement is what makes the currency a fact instead of a guess. The figures are
// bare numbers, nothing on either resource names a currency, and the site's own UI simply renders
// a "$" in front of them — an assumption that is right for a US district and wrong for the
// international schools on the same index (a posting from Athens states €900–1,100 in its body).
// So the only currency this adapter publishes is the one the posting's own country vouches for.
// The employer contact block that carries it is filled on about 60% of postings; the rest yield no
// structured salary and the enrichment pass reads the figure out of the body instead.
func (info schoolspringJobInfo) applySalary(job *Job) {
	period := schoolspringSalaryPeriod(firstNonEmpty(info.SalaryCode, info.PayTypeName))
	if info.PayDisplay != 1 || period == "" || !info.statesUS() {
		return
	}
	min, max := roundSalaryPart(info.PayMin), roundSalaryPart(info.PayMax)
	if min == nil && max == nil {
		return
	}
	job.SalaryMin, job.SalaryMax = min, max
	job.SalaryCurrency, job.SalaryPeriod = "USD", period
}

// statesUS reports whether the posting places its employer in the United States — the country
// whose currency the platform's bare pay figures are then known to be in. countryName is a NAME
// rather than a code, which NormalizeCountry resolves through the same lookup a location string's
// country token goes through, and it answers "" for anything outside the curated set.
func (info schoolspringJobInfo) statesUS() bool {
	return location.NormalizeCountry(info.CountryName) == "us"
}

// schoolspringSalaryPeriod maps the posting's pay period onto freehire's salary periods.
//
// salaryCode is free text the employer types — the live values run from "Per Year" and "Per Hour"
// through "BU Salary Schedule" and "Salary Range: Per P.S.E. Contract Schedule" to "SY" — so this
// recognises the handful of spellings that state a period and yields "" for everything else,
// which drops the amount rather than publishing it half-qualified.
func schoolspringSalaryPeriod(code string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(code)), "per ") {
	case "year", "yearly", "annual", "annually":
		return "year"
	case "month", "monthly":
		return "month"
	case "day", "daily":
		return "day"
	case "hour", "hourly":
		return "hour"
	default:
		return ""
	}
}

// schoolspringEmploymentType maps the platform's job-type enum onto freehire's vocabulary. The
// enum is closed and small (its dictionary endpoint lists exactly these seven values), and the
// three it has no freehire counterpart for — "Summer", "After school/Evening" and the
// "Not provided" default — yield "" so the description parser decides. The unpaid variants map on
// their schedule: freehire's employment type says how much of a week a role takes, not whether it
// pays.
func schoolspringEmploymentType(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "full-time", "full-time unpaid":
		return "full_time"
	case "part-time", "part-time unpaid":
		return "part_time"
	default:
		return ""
	}
}

// schoolspringText decodes the HTML entities the API leaves in its plain-text fields. The listing
// serves them encoded ("Senior Network &amp; Systems Engineer") while the detail resource serves
// the same field decoded, so both paths go through here and end up with one spelling of a title.
func schoolspringText(s string) string { return strings.TrimSpace(html.UnescapeString(s)) }

// schoolspringSearchURL builds one page of a keyword search. The filter parameters the SPA sends
// alongside are omitted: the endpoint treats an absent filter exactly as it treats an empty one
// (verified live), so sending eight empty ones would only obscure which two do the work.
func schoolspringSearchURL(keyword string, page int) string {
	return fmt.Sprintf("%s/api/Jobs/GetPagedJobsWithSearch?domainName=%s&keyword=%s&page=%d&size=%d&sortDateAscending=false",
		schoolspringAPI, schoolspringDomain, url.QueryEscape(keyword), page, schoolspringPageSize)
}

// schoolspringDetailURL is one posting's own resource.
func schoolspringDetailURL(jobID int64) string {
	return fmt.Sprintf("%s/api/Jobs/%d?domainName=%s", schoolspringAPI, jobID, schoolspringDomain)
}

// schoolspringJobURL is the public posting page — the SPA route a reader lands on, not the API one.
func schoolspringJobURL(jobID int64) string {
	return fmt.Sprintf("https://%s/jobdetail?jobId=%d", schoolspringDomain, jobID)
}
