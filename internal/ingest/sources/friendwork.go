package sources

import (
	"context"
	"fmt"
	"strconv"
)

// friendwork adapts FriendWork (friend.work), a Russian ATS whose customers publish a career
// portal at jobs.friend.work/<token>. The portal is a SPA over a keyless endpoint,
// Publication/CareerPortal?token=<token>, which returns the tenant's WHOLE published list in one
// response — its page/perPage parameters are ignored (151 postings came back for perPage=5 and
// for page=2 alike) — with each posting's HTML description inline, so one request assembles every
// Job. The token is the configured board: a readable alias (tedo, skillbox) or a 32-hex hash.
//
// The API meters simultaneity: parallel workers draw 429s while sequential requests are clean, so the registry wires it through limitedFriendWorkGetter (pacer.go).
type friendwork struct {
	http JSONGetter
}

const (
	friendworkListURL = "https://api.friend.work/Publication/CareerPortal?token=%s"
	// friendworkSiteURL is the career portal's origin; a publication's externalPublicationUrl is
	// a path on it ("/TeDo/97846").
	friendworkSiteURL = "https://jobs.friend.work"
)

// NewFriendWork builds the FriendWork adapter over the given HTTP client.
func NewFriendWork(c JSONGetter) Source { return friendwork{http: c} }

func (friendwork) Provider() string { return "friendwork" }

// fullBoardListing: Fetch is a single unpaginated request returning the tenant's whole published
// list — no loop that could stop early. See the fullBoardListing interface for the bar.
func (friendwork) FullBoardListing() {}

// friendworkResponse is the CareerPortal payload. Company is the tenant's own name; the
// per-publication "company" field is null on tenants seen, so it is not modelled.
type friendworkResponse struct {
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	Publications []friendworkPublication `json:"publications"`
}

// friendworkPublication is one published vacancy. The enum fields are strings, with "-1" for
// unset. Salary amounts are whole units, 0 meaning unstated; Currency is the platform's numeric
// enum. Dates carry no zone.
type friendworkPublication struct {
	PublishedJobID         int64   `json:"publishedJobId"`
	Title                  string  `json:"title"`
	Description            string  `json:"description"`
	SalaryFrom             int     `json:"salaryFrom"`
	SalaryTo               int     `json:"salaryTo"`
	Currency               int     `json:"currency"`
	City                   string  `json:"city"`
	Address                string  `json:"address"`
	ExpYearsMin            *int    `json:"expYearsMin"`
	EmploymentType         string  `json:"employmentType"`
	WorkplaceType          string  `json:"workplaceType"`
	PublishDateTime        string  `json:"publishDateTime"`
	DateCreated            string  `json:"dateCreated"`
	ExternalPublicationURL *string `json:"externalPublicationUrl"`
}

// friendworkEmploymentType maps the employmentType enum onto freehire's vocabulary. "Temporary"
// has no counterpart (a fixed-term hire is not a contract engagement), and an unmapped value is
// left for the dictionaries.
var friendworkEmploymentType = map[string]string{
	"Fulltime":   "full_time",
	"Halftime":   "part_time",
	"Internship": "internship",
}

// friendworkCurrency maps the currency enum to ISO 4217, read off the career portal's own
// renderer (its currency label table).
var friendworkCurrency = map[int]string{
	0: "RUB", 1: "USD", 2: "EUR", 3: "UAH", 4: "KZT", 5: "BYN", 6: "AED", 7: "UZS",
}

// friendworkDateLayout is the zone-less timestamp the API emits ("2025-11-05T12:12:54.867"). It
// is read as UTC; the platform is Moscow-hosted, so posted_at may sit a few hours early, which
// is acceptable for an approximate publish date.
const friendworkDateLayout = "2006-01-02T15:04:05.999999999"

func (s friendwork) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp friendworkResponse
	if err := s.http.GetJSON(ctx, fmt.Sprintf(friendworkListURL, e.Board), &resp); err != nil {
		return nil, fmt.Errorf("friendwork: token %q: %w", e.Board, err)
	}
	company := firstNonEmpty(e.Company, resp.Company.Name)
	jobs := make([]Job, 0, len(resp.Publications))
	for _, p := range resp.Publications {
		if p.PublishedJobID == 0 || p.Title == "" {
			continue
		}
		jobs = append(jobs, p.job(e.Board, company))
	}
	return jobs, nil
}

func (p friendworkPublication) job(board, company string) Job {
	id := strconv.FormatInt(p.PublishedJobID, 10)
	url := friendworkSiteURL + "/" + board + "/" + id
	if p.ExternalPublicationURL != nil && *p.ExternalPublicationURL != "" {
		url = friendworkSiteURL + *p.ExternalPublicationURL
	}
	location := firstNonEmpty(p.City, p.Address)
	workMode := workplaceTypeMode(p.WorkplaceType)
	j := Job{
		ExternalID:         id,
		URL:                url,
		Title:              p.Title,
		Company:            company,
		Location:           location,
		Description:        sanitizeHTML(p.Description),
		WorkMode:           workMode,
		Remote:             workMode == "remote" || isRemote(location),
		EmploymentType:     friendworkEmploymentType[p.EmploymentType],
		ExperienceYearsMin: p.ExpYearsMin,
		PostedAt:           parseLayout(friendworkDateLayout, firstNonEmpty(p.PublishDateTime, p.DateCreated)),
	}
	p.applySalary(&j)
	return j
}

// applySalary sets the structured salary when the posting states an amount in a known currency.
// The period is the platform's, not a guess: the portal renders every amount as
// "<from> <to> <currency>/month" and offers no other period.
func (p friendworkPublication) applySalary(j *Job) {
	currency, ok := friendworkCurrency[p.Currency]
	if !ok || (p.SalaryFrom <= 0 && p.SalaryTo <= 0) {
		return
	}
	if p.SalaryFrom > 0 {
		j.SalaryMin = &p.SalaryFrom
	}
	if p.SalaryTo > 0 {
		j.SalaryMax = &p.SalaryTo
	}
	j.SalaryCurrency = currency
	j.SalaryPeriod = "month"
}
