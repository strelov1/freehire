package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// easycruit adapts Visma EasyCruit (easycruit.com), an ATS used across the Netherlands and the
// Nordics — healthcare, municipalities and housing corporations in the first, municipalities and
// industry in the second. A customer's career site lives at https://<board>.easycruit.com, and
// every one publishes two open XML exports beside it:
//
//	/export/xml/vacancy/list.xml   every open vacancy: title, location, country, categories, no body
//	/export/xml/vacancy/<id>.xml   one vacancy with its body (Description, CandidateProfile, JobOffer)
//
// So this is a HydratingSource. The career site's own HTML pages sit behind an AWS WAF browser
// challenge (HTTP 202 and a script); the exports do not, and the vacancy page a visitor opens is
// the VacancyURL the export names.
//
// An unknown label answers the list with a plain 404 "Page not found" (every label resolves, so
// DNS proves nothing), read as ErrBoardGone. A site with no openings answers an empty
// <VacancyList/>.
type easycruit struct {
	http XMLGetter
}

// NewEasyCruit builds the EasyCruit adapter over the given HTTP client.
func NewEasyCruit(c XMLGetter) Source { return easycruit{http: c} }

func (easycruit) Provider() string { return "easycruit" }

// fullBoardListing: the list export is one unpaginated document carrying every open vacancy.
func (easycruit) fullBoardListing() {}

// easycruitBoard is the shape of a career-site label.
var easycruitBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// easycruitList is the list export. XMLName is checked so an answer that is not the export is
// told apart from a site with no openings, which the platform serves as an empty VacancyList.
type easycruitList struct {
	XMLName   xml.Name           `xml:"VacancyList"`
	Vacancies []easycruitVacancy `xml:"Vacancy"`
}

// easycruitVacancy is a vacancy as both exports carry it; only the detail fills the body fields.
// Versions and Departments are read as direct children: a department carries Versions of its
// own (its "about" text), which must not be taken for the vacancy's.
type easycruitVacancy struct {
	ID          string             `xml:"id,attr"`
	DateStart   string             `xml:"date_start,attr"`
	Versions    []easycruitVersion `xml:"Versions>Version"`
	Departments []struct {
		ID         string `xml:"id,attr"`
		VacancyURL string `xml:"VacancyURL"`
	} `xml:"Departments>Department"`
}

type easycruitVersion struct {
	Title                  string `xml:"Title"`
	Appetizer              string `xml:"Appetizer"`
	AlternativeCompanyName string `xml:"AlternativeCompanyName"`
	Location               string `xml:"Location"`
	Description            string `xml:"Description"`
	CandidateProfile       string `xml:"CandidateProfile"`
	JobOffer               string `xml:"JobOffer"`
	Countries              []struct {
		Name     string   `xml:"name,attr"`
		Counties []string `xml:"County"`
	} `xml:"Region>Country"`
	Categories []easycruitCategory `xml:"Categories>Item"`
}

type easycruitCategory struct {
	Type string `xml:"type,attr"`
	ID   string `xml:"id,attr"`
}

// version is the vacancy's text. 1,834 of 1,837 vacancies across 158 sites (2026-10-25) carry
// exactly one; the three that carry two are one opening in two languages, and the first is kept.
func (v easycruitVacancy) version() easycruitVersion {
	if len(v.Versions) == 0 {
		return easycruitVersion{}
	}
	return v.Versions[0]
}

// pageURL is the vacancy page. A vacancy published under several departments has one URL per
// department, all showing the same opening, and the export lists them in no stable order (two
// reads of jeugdzorg's list a few hours apart swapped them), so the lowest department id's page
// is kept and the URL does not flip between crawls.
func (v easycruitVacancy) pageURL() string {
	var id, page string
	for _, d := range v.Departments {
		u, did := strings.TrimSpace(d.VacancyURL), strings.TrimSpace(d.ID)
		if u == "" {
			continue
		}
		if page == "" || len(did) < len(id) || (len(did) == len(id) && did < id) {
			id, page = did, u
		}
	}
	return page
}

func easycruitExport(board string) string {
	return "https://" + board + ".easycruit.com/export/xml/vacancy/"
}

// Fetch reads the list export and every vacancy's detail.
func (s easycruit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board, vacancies, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v easycruitVacancy) (Job, bool) {
		return s.detail(ctx, e, board, v)
	}), nil
}

// FetchNew is Fetch that spends a detail request only on vacancies the catalogue does not hold;
// a seen vacancy is re-listed as a SeenRefresh, the list export proving it is still open.
func (s easycruit) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, vacancies, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v easycruitVacancy) (Job, bool) {
		if seen(v.ID) {
			return Job{ExternalID: v.ID, URL: v.pageURL(), Title: strings.TrimSpace(v.version().Title),
				Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, board, v)
	}), nil
}

// list reads the site's list export, dropping any vacancy without the id a close needs or the
// page a visitor would open.
func (s easycruit) list(ctx context.Context, rawBoard string) (string, []easycruitVacancy, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !easycruitBoard.MatchString(board) {
		return "", nil, fmt.Errorf("easycruit: board %q is not a career-site label", rawBoard)
	}
	var doc easycruitList
	if err := s.http.GetXML(ctx, easycruitExport(board)+"list.xml", &doc); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", nil, fmt.Errorf("easycruit: board %q: %w", board, ErrBoardGone)
		}
		return "", nil, fmt.Errorf("easycruit: board %s list: %w", board, err)
	}
	var out []easycruitVacancy
	seen := map[string]bool{}
	for _, v := range doc.Vacancies {
		v.ID = strings.TrimSpace(v.ID)
		if v.ID == "" || v.pageURL() == "" || seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		out = append(out, v)
	}
	return board, out, nil
}

// detail fetches one vacancy's export and maps it, with its list entry, to a Job. A 404 is the
// platform's own "Page not found" and drops it; any other failure, or an answer that is not this
// vacancy, is an unreadableDetail marker — the detail is the only source of the body.
func (s easycruit) detail(ctx context.Context, e CompanyEntry, board string, v easycruitVacancy) (Job, bool) {
	pageURL := v.pageURL()
	var d easycruitVacancy
	if err := s.http.GetXML(ctx, easycruitExport(board)+v.ID+".xml", &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(v.ID, pageURL, e.Company), true
		}
		return Job{}, false
	}
	if strings.TrimSpace(d.ID) != v.ID || len(d.Versions) == 0 {
		return unreadableDetail(v.ID, pageURL, e.Company), true
	}
	dv := d.version()
	return Job{
		ExternalID:     v.ID,
		URL:            pageURL,
		Title:          firstNonEmpty(strings.TrimSpace(dv.Title), strings.TrimSpace(v.version().Title)),
		Company:        firstNonEmpty(strings.TrimSpace(dv.AlternativeCompanyName), e.Company),
		Location:       easycruitLocation(dv),
		Description:    easycruitBody(dv),
		EmploymentType: easycruitEmploymentType(dv.Categories),
		PostedAt:       parseDate(strings.TrimSpace(v.DateStart)),
		Countries:      easycruitCountries(dv),
	}, true
}

// easycruitBody joins the vacancy's three HTML sections. A vacancy that writes none of them
// still carries its teaser, which is then the body.
func easycruitBody(v easycruitVersion) string {
	var parts []string
	for _, s := range []string{v.Description, v.CandidateProfile, v.JobOffer} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		if a := strings.TrimSpace(v.Appetizer); a != "" {
			parts = append(parts, "<p>"+a+"</p>")
		}
	}
	return sanitizeHTML(strings.Join(parts, "\n"))
}

// easycruitLocation is the vacancy's free-text location beside the county (province, fylke)
// its region names. The country is left out of the text: it travels in Countries instead.
func easycruitLocation(v easycruitVersion) string {
	parts := []string{strings.TrimSpace(v.Location)}
	for _, c := range v.Countries {
		for _, county := range c.Counties {
			if county = strings.TrimSpace(county); county != "" && !slices.Contains(parts, county) {
				parts = append(parts, county)
			}
		}
	}
	return joinNonEmpty(parts...)
}

// easycruitCountries reads the region's countries. EasyCruit names them in the site's language
// ("Nederland", "Norge", "Noreg", "Sverige"); the numeric id beside the name is the platform's
// own and is not an ISO code.
func easycruitCountries(v easycruitVersion) []string {
	var out []string
	for _, c := range v.Countries {
		if code := location.NormalizeCountry(c.Name); code != "" && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}

// easycruitEmploymentType maps the two picklists EasyCruit keys in English whatever language the
// site writes their labels in, measured across 1,837 vacancies: "duration" (permanent, contract,
// temporary, fixed-term-contract, extra-help, seasonal, probation, internship, trainee) and
// "extent" (fulltime, parttime, casual-work, internship, project-based, volunteer). A training
// placement is an internship whatever its extent; a fixed or temporary engagement is a contract,
// as the schema.org TEMPORARY a JobPosting would state; otherwise the extent decides, and only
// when it names one arrangement — a vacancy offering both full and part time states neither.
func easycruitEmploymentType(items []easycruitCategory) string {
	var durations, extents []string
	for _, c := range items {
		switch strings.TrimSpace(c.Type) {
		case "duration":
			durations = append(durations, strings.TrimSpace(c.ID))
		case "extent":
			extents = append(extents, strings.TrimSpace(c.ID))
		}
	}
	has := func(set []string, ids ...string) bool {
		return slices.ContainsFunc(set, func(s string) bool { return slices.Contains(ids, s) })
	}
	switch {
	case has(durations, "internship", "trainee") || has(extents, "internship"):
		return "internship"
	case has(durations, "contract", "temporary", "fixed-term-contract", "extra-help", "seasonal"):
		return "contract"
	case len(extents) == 1 && extents[0] == "fulltime":
		return "full_time"
	case len(extents) == 1 && extents[0] == "parttime":
		return "part_time"
	}
	return ""
}
