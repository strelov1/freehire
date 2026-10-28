package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// velorahr adapts Velora HR, a Spanish ATS whose customers — contract caterers and cleaners
// (Serunion, Derichebourg, ONET), retail chains (Grupo Batta in Mexico, Tiendas MGI, Herbolario
// Navarro), hotels and restaurants — publish career sites at
// https://<tenant>.career.velorahr.com/<slug>-<site id>. The board is that numeric CAREER-SITE
// id: one tenant may run several sites, and the site, not the tenant host, is what the
// platform's public API is keyed by:
//
//	GET api.velorahr.com/api/public/career-site/<id>          the site (404 when there is none)
//	GET api.velorahr.com/api/public/career-site/<id>/offers   its offers, WITH their bodies
//
// The listing carries every offer in full — description, requirements, place, schedule,
// salary — so a crawl is two requests per board and nothing is hydrated. The offers endpoint
// answers an unknown id with an empty 200, which is why the site is read first: its 404 is the
// only way the platform says a board is gone.
//
// A MASTER site re-lists the offers of its child sites, each naming the site that owns it
// (career_site_id); those are left to their owner's board, so one offer is not stored twice.
//
// robots.txt allows everything; the requests of every board still share one limiter, since
// one API host serves every customer.
type velorahr struct {
	http    JSONGetter
	limiter waiter
}

// NewVelorahr builds the Velora HR adapter over the given JSON client.
func NewVelorahr(c JSONGetter) Source {
	return velorahr{http: c, limiter: rate.NewLimiter(rate.Every(velorahrRequestInterval), 1)}
}

func (velorahr) Provider() string { return "velorahr" }

// fullBoardListing: one request asks for a window far wider than any site, and an answer that
// fills the window fails the crawl, so a successful crawl is the site's whole listing. See the
// fullBoardListing interface.
func (velorahr) fullBoardListing() {}

const (
	velorahrAPIBase = "https://api.velorahr.com/api/public/career-site/"
	// velorahrWindow is how many offers the listing request asks for. The largest site measured
	// (2026-10-27) lists 310.
	velorahrWindow = 1000
	// velorahrRequestInterval spaces the provider's requests to the one API host.
	velorahrRequestInterval = time.Second
)

var (
	velorahrBoard = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	// velorahrListingQuery asks for every offer, newest first, in one window.
	velorahrListingQuery = url.Values{
		"filter": {"{}"},
		"range":  {fmt.Sprintf("[0,%d]", velorahrWindow-1)},
		"sort":   {`["id","DESC"]`},
	}.Encode()
)

// velorahrSite is the part of a career site's configuration the adapter reads.
type velorahrSite struct {
	Data struct {
		CompanyName string `json:"company_name"`
	} `json:"data"`
}

// velorahrListing is a site's offers.
type velorahrListing struct {
	Data []velorahrOffer `json:"data"`
}

// velorahrOffer is one listed offer.
type velorahrOffer struct {
	ID               int64  `json:"id"`
	CareerSiteID     *int64 `json:"career_site_id"`
	Title            string `json:"title"`
	CleanTitle       string `json:"clean_title"`
	DescriptionHTML  string `json:"description_html"`
	RequirementsHTML string `json:"requirements_html"`
	City             string `json:"city"`
	Province         string `json:"province"`
	Country          string `json:"country"`
	WorkMode         string `json:"work_mode"`
	WorkSchedule     string `json:"work_schedule"`
	ContractType     string `json:"contract_type"`
	ShowSalary       bool   `json:"show_salary"`
	SalaryFrom       int    `json:"salary_from"`
	SalaryTo         int    `json:"salary_to"`
	SalaryCurrency   string `json:"salary_currency"`
	CreatedAt        string `json:"created_at"`
	OfferURL         string `json:"offer_url"`
	OptionalFields   *struct {
		SalaryType string `json:"salaryType"`
	} `json:"optional_fields"`
}

// Fetch reads the site and its whole listing.
func (s velorahr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.TrimSpace(e.Board)
	if !velorahrBoard.MatchString(board) {
		return nil, fmt.Errorf("velorahr: board %q is not a career-site id", e.Board)
	}
	siteID, _ := strconv.ParseInt(board, 10, 64)

	var site velorahrSite
	if err := s.get(ctx, velorahrAPIBase+board, &site); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return nil, fmt.Errorf("velorahr: board %s: %w", board, ErrBoardGone)
		}
		return nil, fmt.Errorf("velorahr: board %s: %w", board, err)
	}
	var listing velorahrListing
	if err := s.get(ctx, velorahrAPIBase+board+"/offers?"+velorahrListingQuery, &listing); err != nil {
		return nil, fmt.Errorf("velorahr: board %s: offers: %w", board, err)
	}
	if len(listing.Data) >= velorahrWindow {
		return nil, fmt.Errorf("velorahr: board %s: the listing fills the whole %d-offer window, so it may be truncated",
			board, velorahrWindow)
	}

	company := firstNonEmpty(strings.TrimSpace(e.Company), strings.TrimSpace(site.Data.CompanyName))
	var jobs []Job
	dropped := 0
	for _, o := range listing.Data {
		if o.CareerSiteID != nil && *o.CareerSiteID != siteID {
			continue // owned, and crawled, by another site
		}
		j, ok := o.job(company)
		if !ok {
			dropped++
			continue
		}
		jobs = append(jobs, j)
	}
	if dropped > 0 {
		log.Printf("velorahr: board %s: %d offers without an id or a career-site link dropped", board, dropped)
	}
	return jobs, nil
}

// get spaces the request behind the provider's limiter and decodes the JSON answer.
func (s velorahr) get(ctx context.Context, u string, v any) error {
	if err := s.limiter.Wait(ctx); err != nil {
		return err
	}
	return s.http.GetJSON(ctx, u, v)
}

// job maps an offer. An offer without an id, a title, or a link onto a Velora career site is
// not a posting anyone can open, and is dropped.
func (o velorahrOffer) job(company string) (Job, bool) {
	link, ok := velorahrOfferURL(o.OfferURL)
	title := strings.TrimSpace(firstNonEmpty(o.CleanTitle, o.Title))
	if !ok || o.ID <= 0 || title == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     strconv.FormatInt(o.ID, 10),
		URL:            link,
		Title:          title,
		Company:        company,
		Location:       velorahrPlace(o.City, o.Province, o.Country),
		Description:    sanitizeHTML(o.DescriptionHTML + o.RequirementsHTML),
		Countries:      countryFromCode(o.Country),
		WorkMode:       velorahrWorkMode(o.WorkMode),
		EmploymentType: velorahrEmploymentType(o.ContractType, o.WorkSchedule),
	}
	j.Remote = j.WorkMode == "remote"
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(o.CreatedAt)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	if o.OptionalFields != nil {
		if period := velorahrSalaryPeriod(o.OptionalFields.SalaryType); o.ShowSalary && period != "" &&
			o.SalaryFrom > 0 && o.SalaryTo >= o.SalaryFrom && len(o.SalaryCurrency) == 3 {
			lo, hi := o.SalaryFrom, o.SalaryTo
			j.SalaryMin, j.SalaryMax = &lo, &hi
			j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(o.SalaryCurrency), period
		}
	}
	return j, true
}

// velorahrOfferURL pins an offer's link to a Velora career site over https, so nothing the API
// returns can publish a link elsewhere.
func velorahrOfferURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, ".career.velorahr.com") {
		return "", false
	}
	u.Scheme, u.Host, u.Fragment = "https", host, ""
	return u.String(), true
}

// velorahrPlace is "<city>, <province>, <country code>". A bilingual town name
// ("Pamplona/Iruña", "Alicante/Alacant") keeps its Spanish half, which is the one the location
// dictionary knows, and a province that merely repeats the city is said once. The country code
// is kept in the line because it is what lets the dictionary read the town as a town rather than
// reading the province as a second city.
func velorahrPlace(city, province, country string) string {
	city = strings.TrimSpace(city)
	if spanish, _, ok := strings.Cut(city, "/"); ok {
		city = strings.TrimSpace(spanish)
	}
	province = strings.TrimSpace(province)
	if strings.EqualFold(province, city) {
		province = ""
	}
	return joinNonEmpty(city, province, strings.ToUpper(strings.TrimSpace(country)))
}

// velorahrWorkMode maps the platform's work-mode enum; anything else stays unknown.
func velorahrWorkMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "remoto":
		return "remote"
	case "hibrido", "híbrido":
		return "hybrid"
	case "presencial":
		return "onsite"
	default:
		return ""
	}
}

// velorahrEmploymentType reads a traineeship from the contract type and otherwise the working
// day: a contract's duration ("permanent", "fixed-term", "discontinuous") says nothing about
// the hours, and "Indiferente" says nothing at all.
func velorahrEmploymentType(contract, schedule string) string {
	switch strings.ToLower(strings.TrimSpace(contract)) {
	case "training":
		return "internship"
	case "part-time":
		return "part_time"
	}
	switch strings.ToLower(strings.TrimSpace(schedule)) {
	case "jornada completa":
		return "full_time"
	case "media jornada", "por horas":
		return "part_time"
	}
	return ""
}

// velorahrSalaryPeriod maps the salary's stated period; without one a figure is not published.
func velorahrSalaryPeriod(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "yearly":
		return "year"
	case "monthly":
		return "month"
	case "hourly":
		return "hour"
	default:
		return ""
	}
}
