package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// dvinci adapts d.vinci (dvinci.de), a German ATS. Each tenant's careers site serves its whole
// board from one unpaginated, keyless JSON document, https://<host>/jobPublication/list.json,
// carrying every live publication with its full body, places, salary and employer — no detail
// request. The board is the careers HOST: the vendor's two domains (<tenant>.dvinci-hr.com for
// the full product, <tenant>.dvinci-easy.com for the small-business one) serve the same
// application, and so does an employer's own domain (career.dertour-jobs.com), which the same
// path reaches unchanged.
//
// One vacancy (jobOpening) may be published several times, each a publication with its own id
// and URL; see dvinciCollapseTranslations for which of those are one job.
type dvinci struct {
	http JSONGetter
}

// NewDvinci builds the d.vinci adapter over the given HTTP client.
func NewDvinci(c JSONGetter) Source { return dvinci{http: c} }

func (dvinci) Provider() string { return "dvinci" }

// fullBoardListing: the board document is one unpaginated answer carrying every live
// publication — no loop that could stop early.
func (dvinci) fullBoardListing() {}

// dvinciBoard is the shape of a careers host: lowercase DNS labels, at least one dot.
var dvinciBoard = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// dvinciPublication is one publication of a vacancy on the careers site.
type dvinciPublication struct {
	ID                int64                 `json:"id"`
	Language          string                `json:"language"`
	Position          string                `json:"position"`
	JobPublicationURL string                `json:"jobPublicationURL"`
	StartDate         string                `json:"startDate"`
	Introduction      string                `json:"introduction"`
	Tasks             string                `json:"tasks"`
	Profile           string                `json:"profile"`
	WeOffer           string                `json:"weOffer"`
	ClosingText       string                `json:"closingText"`
	RenderedContent   map[string]dvinciPart `json:"renderedContent"`
	JobOpening        *dvinciOpening        `json:"jobOpening"`
}

// dvinciPart is one section of the rendered posting page.
type dvinciPart struct {
	Headline string `json:"headline"`
	Body     string `json:"body"`
}

type dvinciOpening struct {
	ID           int64  `json:"id"`
	CreatedDate  string `json:"createdDate"`
	WorkingTimes []struct {
		InternalName string `json:"internalName"`
	} `json:"workingTimes"`
	Locations []struct {
		Name    string `json:"name"`
		Country *struct {
			Name  string `json:"name"`
			IsoA2 string `json:"isoA2"`
		} `json:"country"`
		Address *struct {
			City string `json:"city"`
		} `json:"address"`
	} `json:"locations"`
	Company *struct {
		Name string `json:"name"`
	} `json:"company"`
	Salary *struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"salary"`
}

func (s dvinci) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !dvinciBoard.MatchString(board) {
		return nil, fmt.Errorf("dvinci: board %q is not a careers host", e.Board)
	}
	// A pointer, so a document that decodes to JSON null is told apart from a board with no
	// publications, which the platform serves as an empty array.
	var doc *[]dvinciPublication
	if err := s.http.GetJSON(ctx, "https://"+board+"/jobPublication/list.json", &doc); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return nil, fmt.Errorf("dvinci: board %s: %w", board, ErrBoardGone)
		}
		return nil, fmt.Errorf("dvinci: board %s: %w", board, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("dvinci: board %s answered no publication list", board)
	}
	jobs := make([]Job, 0, len(*doc))
	for _, p := range dvinciCollapseTranslations(*doc) {
		if j, ok := dvinciJob(e, p); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// dvinciCollapseTranslations keeps one publication of an opening published once per language
// (weig: "Ausbildung zum Papiertechnologen" beside "Training as a paper technologist") — the
// lowest publication id, which stays the same while the opening runs. An opening published
// several times in ONE language is left alone: those are differently titled postings of one
// vacancy (cadenas runs "(Senior) IT Administrator" and "DevOps Engineer" on one opening), each
// with its own page, and dropping one would hide a title the employer chose to advertise.
// Measured 2026-10-25 over 75 boards: 19 translation groups, 44 same-language ones.
func dvinciCollapseTranslations(pubs []dvinciPublication) []dvinciPublication {
	groups := map[int64][]int{}
	for i, p := range pubs {
		if p.JobOpening != nil && p.JobOpening.ID != 0 {
			groups[p.JobOpening.ID] = append(groups[p.JobOpening.ID], i)
		}
	}
	drop := map[int]bool{}
	for _, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		langs := map[string]bool{}
		keep := idx[0]
		for _, i := range idx {
			langs[strings.ToLower(strings.TrimSpace(pubs[i].Language))] = true
			if pubs[i].ID < pubs[keep].ID {
				keep = i
			}
		}
		if len(langs) != len(idx) {
			continue // a language repeats: distinct postings, not translations
		}
		for _, i := range idx {
			if i != keep {
				drop[i] = true
			}
		}
	}
	out := make([]dvinciPublication, 0, len(pubs))
	for i, p := range pubs {
		if !drop[i] {
			out = append(out, p)
		}
	}
	return out
}

func dvinciJob(e CompanyEntry, p dvinciPublication) (Job, bool) {
	url := strings.TrimSpace(p.JobPublicationURL)
	if p.ID == 0 || url == "" || p.JobOpening == nil {
		return Job{}, false // no id → nothing a close could reach; no URL → nothing to link
	}
	o := p.JobOpening
	j := Job{
		ExternalID:     strconv.FormatInt(p.ID, 10),
		URL:            url,
		Title:          strings.TrimSpace(p.Position),
		Company:        e.Company,
		Description:    sanitizeHTML(dvinciBody(p)),
		EmploymentType: dvinciEmploymentType(o),
		PostedAt:       dvinciPostedAt(p),
	}
	if o.Company != nil {
		j.Company = firstNonEmpty(strings.TrimSpace(o.Company.Name), e.Company)
	}
	var places []string
	seen := map[string]bool{}
	for _, l := range o.Locations {
		name := strings.TrimSpace(l.Name)
		if name == "" && l.Address != nil {
			name = strings.TrimSpace(l.Address.City)
		}
		if l.Country != nil {
			if c := location.NormalizeCountry(l.Country.IsoA2); c != "" && !seen[c] {
				seen[c] = true
				j.Countries = append(j.Countries, c)
			}
			if country := strings.TrimSpace(l.Country.Name); country != "" && name != "" {
				name += ", " + country
			}
		}
		if name != "" {
			places = append(places, name)
		}
	}
	j.Location = strings.Join(places, "; ")
	if sal := o.Salary; sal != nil {
		if period := schemaSalaryPeriod(sal.Value.UnitText); period != "" && sal.Currency != "" {
			j.SalaryMin, j.SalaryMax = roundSalaryPart(sal.Value.MinValue), roundSalaryPart(sal.Value.MaxValue)
			if j.SalaryMin != nil || j.SalaryMax != nil {
				j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(strings.TrimSpace(sal.Currency)), period
			}
		}
	}
	return j, true
}

// dvinciSections is the order the posting page renders its sections in. "position" is the
// page's header (title, place, hours — already structured fields) and "contact" names the
// recruiter with a phone number; neither is part of the posting's body.
var dvinciSections = []string{"introduction", "tasks", "profile", "weOffer", "additionalInformation"}

// dvinciBody is the posting's body. The rendered sections are preferred: a tenant whose page
// template adds sections fills them there, and on ~30% of publications they are the longer
// text. A tenant on the plain template (no renderedContent) has only the raw fields.
func dvinciBody(p dvinciPublication) string {
	var parts []string
	for _, k := range dvinciSections {
		part := p.RenderedContent[k]
		if strings.TrimSpace(textFromHTML(part.Body)) == "" {
			continue
		}
		if h := strings.TrimSpace(part.Headline); h != "" {
			parts = append(parts, "<h3>"+h+"</h3>")
		}
		parts = append(parts, part.Body)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	for _, h := range []string{p.Introduction, p.Tasks, p.Profile, p.WeOffer, p.ClosingText} {
		if strings.TrimSpace(h) != "" {
			parts = append(parts, h)
		}
	}
	return strings.Join(parts, "\n")
}

// dvinciEmploymentType maps the working-time picklist. Tenants may add their own values
// (VOLLZEIT, TEILZEIT_5075, VOLL_ODER_TEILZEIT_NACH_VEREINBARUNG), so only the platform's own
// two, and their German twins, are read — and only when the posting states exactly one.
func dvinciEmploymentType(o *dvinciOpening) string {
	if len(o.WorkingTimes) != 1 {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(o.WorkingTimes[0].InternalName)) {
	case "FULL_TIME", "VOLLZEIT":
		return "full_time"
	case "PART_TIME", "TEILZEIT":
		return "part_time"
	}
	return ""
}

// dvinciPostedAt is the publication's start, or the opening's creation when it states none.
func dvinciPostedAt(p dvinciPublication) *time.Time {
	if t := dvinciTime(p.StartDate); t != nil {
		return t
	}
	return dvinciTime(p.JobOpening.CreatedDate)
}

// dvinciTime parses the document's timestamps: createdDate carries milliseconds and a Z
// ("2026-08-13T09:42:58.324Z"); a publication's startDate has no seconds ("2018-09-10T22:00Z").
func dvinciTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseLayout("2006-01-02T15:04Z07:00", s)
}
