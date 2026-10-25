package sources

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// talentstorm adapts talentstorm (talentstorm.de, by ontavio), a German ATS for mid-sized
// employers whose career site lives at https://<board>.talentstorm.de. The site is an Angular
// app reading one JSON API on api.talentstorm.de:
//
//	/api/v1/joboffers/basic/<board>?pagination=false   every published offer, with its body
//
// The API answers only a request naming the tenant's own site as its Origin; without one, and
// for a label no tenant holds, it answers 401 "Authentication Required". Since the two cannot be
// told apart, a 401 is an ordinary failure rather than ErrBoardGone. Unpaged the endpoint serves
// 10 offers a page; pagination=false returns the whole list in one answer, which makes the
// listing complete (fullBoardListing). An offer carries its assembled body, its workplaces, the
// employment and the offer type, so nothing is hydrated. Its public page is
// https://<board>.talentstorm.de/stellenangebote/<slug>.
type talentstorm struct {
	http HeaderJSONGetter
}

// NewTalentstorm builds the talentstorm adapter over the given HTTP client.
func NewTalentstorm(c HeaderJSONGetter) Source { return talentstorm{http: c} }

func (talentstorm) Provider() string { return "talentstorm" }

// fullBoardListing: pagination=false answers every published offer at once.
func (talentstorm) fullBoardListing() {}

// talentstormBoard is the shape of a career-site label.
var talentstormBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// talentstormOpenApplication is the offer type of an open application ("Initiativbewerbung"):
// an invitation to apply, not a position, so it is never yielded.
const talentstormOpenApplication = 6

// talentstormOffer is one offer as the API carries it.
type talentstormOffer struct {
	ID           int64  `json:"id"`
	Label        string `json:"label"`
	Slug         string `json:"slug"`
	IsPublished  bool   `json:"isPublished"`
	IDJobtype    int    `json:"idJobtype"`
	Description  string `json:"description"`
	CreationDate string `json:"creationDate"`
	Employment   struct {
		Code string `json:"code"`
	} `json:"employment"`
	Locations []struct {
		Location struct {
			City    string `json:"city"`
			Region  string `json:"region"`
			Country struct {
				Code string `json:"abbreviation2"`
			} `json:"country"`
		} `json:"location"`
	} `json:"jobofferLocations"`
	Pay *struct {
		WageType string   `json:"wageType"`
		WageMin  *float64 `json:"wageMin"`
		WageMax  *float64 `json:"wageMax"`
		NoSalary bool     `json:"noSalary"`
	} `json:"transparencyOfPay"`
}

// Fetch reads the career site's complete offer list.
func (s talentstorm) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !talentstormBoard.MatchString(board) {
		return nil, fmt.Errorf("talentstorm: board %q is not a career-site label", e.Board)
	}
	// A pointer, so a reshaped answer (an object, null) is told apart from a site with no offer.
	var offers *[]talentstormOffer
	u := "https://api.talentstorm.de/api/v1/joboffers/basic/" + board + "?pagination=false"
	if err := s.http.GetJSONWithHeaders(ctx, u, map[string]string{"Origin": "https://" + board + ".talentstorm.de"}, &offers); err != nil {
		return nil, fmt.Errorf("talentstorm: board %s: %w", board, err)
	}
	if offers == nil {
		return nil, fmt.Errorf("talentstorm: board %s answer carries no offer list", board)
	}
	var jobs []Job
	for _, o := range *offers {
		if j, ok := talentstormJob(board, e.Company, o); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// talentstormJob maps one offer to a Job, dropping an unpublished offer, an open application,
// and one without the id and slug that identify and address it.
func talentstormJob(board, company string, o talentstormOffer) (Job, bool) {
	if !o.IsPublished || o.IDJobtype == talentstormOpenApplication || o.ID == 0 || strings.TrimSpace(o.Slug) == "" {
		return Job{}, false
	}
	var places, countries []string
	for _, l := range o.Locations {
		if p := joinNonEmpty(strings.TrimSpace(l.Location.City), strings.TrimSpace(l.Location.Region)); p != "" && !slices.Contains(places, p) {
			places = append(places, p)
		}
		if c := location.NormalizeCountry(l.Location.Country.Code); c != "" && !slices.Contains(countries, c) {
			countries = append(countries, c)
		}
	}
	j := Job{
		ExternalID:     fmt.Sprint(o.ID),
		URL:            "https://" + board + ".talentstorm.de/stellenangebote/" + strings.TrimSpace(o.Slug),
		Title:          strings.TrimSpace(o.Label),
		Company:        company,
		Location:       strings.Join(places, "; "),
		Description:    sanitizeHTML(o.Description),
		EmploymentType: talentstormEmploymentType(o.IDJobtype, o.Employment.Code),
		PostedAt:       parseRFC3339(strings.TrimSpace(o.CreationDate)),
		Countries:      countries,
	}
	talentstormSalary(&j, o, countries)
	return j, true
}

// talentstormEmploymentType maps the offer type and the employment. Measured across 1,355
// offers on 122 sites (2026-10-25): the offer types are Stellenangebot (1), Ausbildung (2),
// Duales Studium (3), Praktikum (4), Studien-/Abschlussarbeit (5), Initiativbewerbung (6) and
// Freelancer (7); the employments ft (Vollzeit), pt (Teilzeit), mj (Minijob/Studentenjob) and hj
// (Ferienjob). An internship and a freelance offer say what the arrangement is whatever the
// hours; a Minijob and a Ferienjob have no value in the vocabulary and stay "", as do the
// apprenticeship and study types. Talentstorm records no work arrangement, so WorkMode is the
// parser's.
func talentstormEmploymentType(jobtype int, code string) string {
	switch jobtype {
	case 4:
		return "internship"
	case 7:
		return "contract"
	case 1:
		switch strings.ToLower(strings.TrimSpace(code)) {
		case "ft":
			return "full_time"
		case "pt":
			return "part_time"
		}
	}
	return ""
}

// talentstormSalaryPeriods maps the pay-transparency wage type onto the salary vocabulary.
var talentstormSalaryPeriods = map[string]string{
	"annual_salary":  "year",
	"monthly_salary": "month",
	"hourly_wage":    "hour",
}

// talentstormSalary sets the stated pay range. The API states no currency, so a range is kept
// only where every workplace is in Germany or Austria — a pay-transparency statement there is
// in euro — and dropped elsewhere rather than guessed at.
func talentstormSalary(j *Job, o talentstormOffer, countries []string) {
	if o.Pay == nil || o.Pay.NoSalary || o.Pay.WageMin == nil || len(countries) == 0 {
		return
	}
	period := talentstormSalaryPeriods[o.Pay.WageType]
	if period == "" {
		return
	}
	for _, c := range countries {
		if c != "de" && c != "at" {
			return
		}
	}
	lo := int(math.Round(*o.Pay.WageMin))
	j.SalaryMin = &lo
	if o.Pay.WageMax != nil && *o.Pay.WageMax >= *o.Pay.WageMin {
		hi := int(math.Round(*o.Pay.WageMax))
		j.SalaryMax = &hi
	}
	j.SalaryCurrency, j.SalaryPeriod = "EUR", period
}
