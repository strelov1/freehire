package sources

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ukgready adapts UKG Ready career sites (formerly Kronos Workforce Ready, still served from
// SaaSHR-era hosts). It is a DIFFERENT product from the ukg adapter (UKG Pro Recruiting, formerly
// UltiPro, on rec.pro.ukg.net): different hosts, API and board shape.
//
// A board is "<host>/<tenant>", e.g. "secure4.saashr.com/6162397". The host selects the
// ENVIRONMENT the tenant lives in (US-4-PROD, AU-1-PROD, ...); every other host answers "Company
// not found" for it, and the environment is not derivable from the tenant id, so the board id
// carries the host (as ukg and workday do). Tenant ids are case-insensitive, so boards are lower case.
//
// One environment is fronted by several white-label hosts (secure*.saashr.com,
// secure*.entertimeonline.com, secure*.yourpayrollhr.com, secure.workforceready.<cc>,
// *.mykronos.com) serving the same tenants, so the host is branding, not identity: see
// ukgreadyTenant.
//
// The career page (https://<host>/ta/<tenant>.careers) renders client-side, so the adapter reads
// the SPA's keyless REST API:
//
//	listing: /ta/rest/ui/recruitment/companies/|<tenant>/job-requisitions?offset=&size=
//	detail:  /ta/rest/ui/recruitment/companies/|<tenant>/job-requisitions/<id>
//
// The listing carries identity, address, pay and the remote flag, but its job_description is a
// ~250-character preview; only the detail holds the body. Bodies are heavy (employers paste
// base64 data-URI images), so this is a HydratingSource: every crawl lists the whole board and
// fetches a body only for a posting the catalogue does not have yet.
type ukgready struct {
	http JSONGetter
}

// NewUKGReady builds the UKG Ready adapter over the given HTTP client. The listing, each
// posting's detail and the tenant's format settings are all plain keyless JSON GETs.
func NewUKGReady(c JSONGetter) Source { return ukgready{http: c} }

func (ukgready) Provider() string { return "ukgready" }

// ukgreadyPageSize is the listing page size. 200 is the API's maximum — 250 is refused with
// "Invalid parameter format. Parameter: size" — and _paging.total bounds the walk exactly.
const ukgreadyPageSize = 200

// ukgreadyBoard is a parsed board id: the tenant's pod host and its numeric-ish tenant token.
type ukgreadyBoard struct {
	host, tenant string
}

// parseUKGReadyBoard splits a "<host>/<tenant>" board id, requiring both parts and rejecting
// any further path — a third segment would silently address a URL this adapter never builds.
func parseUKGReadyBoard(board string) (ukgreadyBoard, error) {
	host, tenant, ok := strings.Cut(board, "/")
	if !ok || host == "" || tenant == "" || strings.Contains(tenant, "/") {
		return ukgreadyBoard{}, fmt.Errorf("ukgready: board %q must be \"<host>/<tenant>\"", board)
	}
	return ukgreadyBoard{host: host, tenant: tenant}, nil
}

// requisitionsURL is the tenant's job-requisition collection. The tenant is addressed as
// "|<tenant>" — UKG's company selector, whose pipe is percent-encoded because a literal one is
// not a legal URL character.
func (b ukgreadyBoard) requisitionsURL() string {
	return fmt.Sprintf("https://%s/ta/rest/ui/recruitment/companies/%%7C%s/job-requisitions", b.host, b.tenant)
}

// settingsURL is the tenant's format settings, which state the currency its pay figures are in.
func (b ukgreadyBoard) settingsURL() string {
	return fmt.Sprintf("https://%s/ta/rest/ui/bootstrap/companies/%%7C%s/format-settings", b.host, b.tenant)
}

// jobURL is the public career-page URL for one posting — the SPA route, not the API one.
func (b ukgreadyBoard) jobURL(id string) string {
	return fmt.Sprintf("https://%s/ta/%s.careers?ShowJob=%s", b.host, b.tenant, id)
}

// ukgreadyTenant folds a board to the tenant it addresses, so two board ids differing only in the
// white-label host are ONE crawl target, not two external_id namespaces. It is boardIdentity's
// entry for this provider; see the icims note there for the false-closes that costs.
//
// The fold ignores the host, which relies on a tenant id naming one tenant everywhere: an id from
// another environment answers "Company not found" rather than resolving to a different company.
// A board it cannot parse folds to itself, so a malformed entry collides only with an identical one.
func ukgreadyTenant(board string) string {
	if b, err := parseUKGReadyBoard(board); err == nil {
		return b.tenant
	}
	return board
}

// ukgreadyPosting is one job requisition. The listing and the detail resource serve the same
// shape; the detail adds the full job_description and the job_requirement block.
type ukgreadyPosting struct {
	ID       int64            `json:"id"`
	JobTitle string           `json:"job_title"`
	Location ukgreadyLocation `json:"location"`
	// JobDescription is a truncated preview in the listing and the full HTML body in the detail.
	JobDescription string `json:"job_description"`
	JobRequirement string `json:"job_requirement"`
	// BasePayFrom/To are bare numbers; BasePayFrequency is UKG's period enum and the tenant's
	// format settings hold the currency (see currency).
	BasePayFrom      *float64 `json:"base_pay_from"`
	BasePayTo        *float64 `json:"base_pay_to"`
	BasePayFrequency string   `json:"base_pay_frequency"`
	IsRemoteJob      bool     `json:"is_remote_job"`
	EmployeeType     struct {
		Name string `json:"name"`
	} `json:"employee_type"`
}

// ukgreadyLocation is a posting's address. The street lines and zip are dropped: they narrow a
// posting to a building, which the geo dictionary cannot use and the catalogue does not show.
type ukgreadyLocation struct {
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
}

// text renders the address as "City, State, Country" (ISO alpha-3 country, as UKG states it),
// skipping the parts a posting leaves blank.
func (l ukgreadyLocation) text() string { return joinNonEmpty(l.City, l.State, l.Country) }

// externalID is the posting's native id as the pipeline's dedup key spells it (the pipeline
// namespaces it by board).
func (p ukgreadyPosting) externalID() string { return strconv.FormatInt(p.ID, 10) }

// payBounds returns the posting's pay bounds as freehire integers, dropping a bound the
// platform left unset or set to zero (roundSalaryPart's "not stated" shape). The two are
// independent, so a one-sided "from $X" range still comes through.
func (p ukgreadyPosting) payBounds() (min, max *int) {
	if p.BasePayFrom != nil {
		min = roundSalaryPart(*p.BasePayFrom)
	}
	if p.BasePayTo != nil {
		max = roundSalaryPart(*p.BasePayTo)
	}
	return min, max
}

// statesPay reports whether the posting carries a pay figure this adapter could publish — a
// bound AND a period our vocabulary has a value for. It gates the one extra request the
// tenant's currency costs, so a board that never states pay never makes it.
func (p ukgreadyPosting) statesPay() bool {
	if ukgreadySalaryPeriod(p.BasePayFrequency) == "" {
		return false
	}
	min, max := p.payBounds()
	return min != nil || max != nil
}

// needsBody reports whether this crawl will fetch the posting's body: the catalogue does not
// already hold it. seen is nil on the list-only path, where every posting is hydrated.
func (p ukgreadyPosting) needsBody(seen func(externalID string) bool) bool {
	return seen == nil || !seen(p.externalID())
}

// applySalary copies the posting's structured pay onto the job, but only as a complete
// statement: an amount whose period or currency is unknown is dropped rather than published
// half-qualified, since "1200" reads as a wage or a salary depending on which is missing.
func (p ukgreadyPosting) applySalary(job *Job, currency string) {
	period := ukgreadySalaryPeriod(p.BasePayFrequency)
	if period == "" || currency == "" {
		return
	}
	min, max := p.payBounds()
	if min == nil && max == nil {
		return
	}
	job.SalaryMin, job.SalaryMax = min, max
	job.SalaryCurrency, job.SalaryPeriod = currency, period
}

func (s ukgready) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// List-only fallback (no seen set): hydrate every posting.
	return s.crawl(ctx, e, nil)
}

// FetchNew is the hydrating crawl: it lists the whole board, but fetches a body only for a
// posting the catalogue does not already have. A seen posting is emitted as a liveness refresh
// (identity only, no detail request, no content rewrite).
func (s ukgready) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.crawl(ctx, e, seen)
}

// crawl lists the board and maps each posting to a Job, hydrating through the shared bounded
// worker pool. seen is nil on the list-only path, where every posting is hydrated.
func (s ukgready) crawl(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parseUKGReadyBoard(e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := s.list(ctx, b)
	if err != nil {
		return nil, err
	}
	currency := s.currency(ctx, b, postings, seen)
	return fetchDetails(postings, defaultDetailWorkers, func(p ukgreadyPosting) (Job, bool) {
		if !p.needsBody(seen) {
			return Job{
				ExternalID:  p.externalID(),
				URL:         b.jobURL(p.externalID()),
				Title:       p.JobTitle,
				Company:     e.Company,
				SeenRefresh: true,
			}, true
		}
		return s.hydrate(ctx, b, e, p, currency)
	}), nil
}

// list walks the tenant's requisitions a page at a time until _paging.total is collected. An
// empty page also stops the walk, so an overstated total cannot spin the loop.
//
// Trap: "offset" is a 1-based PAGE NUMBER, not a row offset. The API echoes whatever was asked
// back in _paging and answers an empty page, so advancing it by row count jumps to page 200 and
// silently truncates every board past one page.
func (s ukgready) list(ctx context.Context, b ukgreadyBoard) ([]ukgreadyPosting, error) {
	var postings []ukgreadyPosting
	for pageNum := 1; ; pageNum++ {
		url := fmt.Sprintf("%s?offset=%d&size=%d", b.requisitionsURL(), pageNum, ukgreadyPageSize)
		var page struct {
			JobRequisitions []ukgreadyPosting `json:"job_requisitions"`
			Paging          struct {
				Total int `json:"total"`
			} `json:"_paging"`
		}
		if err := s.http.GetJSON(ctx, url, &page); err != nil {
			// The first page failing is a board-level error; a later one ends the walk with
			// what has been gathered, so a mid-listing hiccup costs a page rather than a board.
			if pageNum == 1 {
				return nil, fmt.Errorf("ukgready: list board %s: %w", b.tenant, err)
			}
			break
		}
		if len(page.JobRequisitions) == 0 {
			break
		}
		postings = append(postings, page.JobRequisitions...)
		if len(postings) >= page.Paging.Total {
			break
		}
	}
	return postings, nil
}

// hydrate fetches one posting's body and maps it to a Job. It returns ok=false when the detail
// request fails, so the posting is skipped rather than stored with the listing's truncated
// preview: a hydrating adapter never revisits a body it has already stored, so a stub would be
// permanent, whereas a skipped posting stays unseen and the next crawl retries it.
//
// PostedAt is left nil: UKG Ready publishes no posting date on either resource, so freshness
// falls back to the pipeline's first-seen stamp.
func (s ukgready) hydrate(ctx context.Context, b ukgreadyBoard, e CompanyEntry, p ukgreadyPosting, currency string) (Job, bool) {
	var detail ukgreadyPosting
	if err := s.http.GetJSON(ctx, b.requisitionsURL()+"/"+p.externalID(), &detail); err != nil {
		return Job{}, false
	}

	location := p.Location.text()
	job := Job{
		ExternalID:  p.externalID(),
		URL:         b.jobURL(p.externalID()),
		Title:       p.JobTitle,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(ukgreadyBody(detail)),
		Remote:      p.IsRemoteJob || isRemote(location),
		// is_remote_job is the platform's own structured flag, so true means remote; false is
		// left unresolved rather than read as onsite (workModeFromRemote).
		WorkMode:       workModeFromRemote(p.IsRemoteJob),
		Countries:      countryFromCode(p.Location.Country),
		EmploymentType: ukgreadyEmploymentType(p.EmployeeType.Name),
	}
	p.applySalary(&job, currency)
	return job, true
}

// ukgreadyBody assembles the description from the detail resource: the body, then the requirements
// block the platform stores separately. job_preview is left out: it is a per-tenant banner
// repeated verbatim on every posting, and would blur the description hash that collapses reposts.
func ukgreadyBody(detail ukgreadyPosting) string {
	return detail.JobDescription + detail.JobRequirement
}

// currency resolves the tenant's pay currency from its format settings, returning "" when this
// crawl has no posting to publish a pay figure for, or when the request fails.
// The pay figures are bare numbers; the currency is a property of the TENANT's locale, read from
// its format settings. It is fetched once per board, and only when a posting this crawl will
// hydrate states publishable pay, so a steady-state re-crawl spends nothing. Without it the
// amounts are dropped rather than guessed (the same number differs in USD and AUD).
func (s ukgready) currency(ctx context.Context, b ukgreadyBoard, postings []ukgreadyPosting,
	seen func(externalID string) bool) string {
	publishable := func(p ukgreadyPosting) bool { return p.needsBody(seen) && p.statesPay() }
	if !slices.ContainsFunc(postings, publishable) {
		return ""
	}
	var settings struct {
		Locale struct {
			CurrencyCode string `json:"currency_code"`
		} `json:"locale"`
	}
	if err := s.http.GetJSON(ctx, b.settingsURL(), &settings); err != nil {
		return ""
	}
	return settings.Locale.CurrencyCode
}

// ukgreadySalaryPeriod maps UKG Ready's base-pay frequency enum onto freehire's salary periods.
// The enum holds exactly YEAR, MONTH, WEEK and HOUR (the career page localizes those four and
// no others); WEEK has no value in vocab.SalaryPeriodValues, so it — like any value UKG might
// add later — yields "" and the amount is dropped with it.
func ukgreadySalaryPeriod(frequency string) string {
	switch strings.ToUpper(strings.TrimSpace(frequency)) {
	case "YEAR":
		return "year"
	case "MONTH":
		return "month"
	case "HOUR":
		return "hour"
	default:
		return ""
	}
}

// ukgreadyEmploymentType maps a posting's employee type onto freehire's vocabulary.
//
// The field is a per-tenant picklist the employer writes ("Full Time", "FT Non-Exempt", FLSA
// classes like "Exempt"), not a platform enum. Only a full-time or part-time marker is read;
// everything else, including a label carrying both, yields "" so the description parser decides.
func ukgreadyEmploymentType(employeeType string) string {
	// Fold every non-letter to a space so "Full-Time", "Reg FT - Non-Exempt" and
	// "Regular (Full Time)" all present their marker as a whole word, and pad the ends so the
	// first and last word are bounded by a space like every other one.
	words := strings.FieldsFunc(strings.ToLower(employeeType), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	padded := " " + strings.Join(words, " ") + " "
	states := func(markers ...string) bool {
		return slices.ContainsFunc(markers, func(m string) bool {
			return strings.Contains(padded, " "+m+" ")
		})
	}
	full, part := states("full time", "fulltime", "ft"), states("part time", "parttime", "pt")
	switch {
	case full && !part:
		return "full_time"
	case part && !full:
		return "part_time"
	default:
		return ""
	}
}
