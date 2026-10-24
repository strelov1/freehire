package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// flatchr adapts Flatchr (flatchr.io), a French ATS whose hosted careers sites live at
// https://careers.flatchr.io/<lang>/company/<board>. The site is a client-rendered Next.js app
// that reads the whole board from one unpaginated JSON document,
// https://careers.flatchr.io/company/<board>.json, which carries every published vacancy with
// its full body (description, mission, profile) and structured fields — no detail request.
//
// A board can be a GROUP: a holding's careers site lists its subsidiaries' vacancies beside its
// own (additi lists medialex's and regieradioregions'), so the employer is read per vacancy.
//
// An unknown board answers 404 with a JSON error; a board whose careers site was switched off
// answers HTTP 200 with an HTML "Votre recherche est tombée à l'eau" fragment instead of JSON.
type flatchr struct {
	http flatchrGetter
}

// flatchrGetter is the transport the adapter needs: the board document as JSON, and — only
// when that fails to decode — the same address as text, to recognise the switched-off page.
// The document cannot be read as text in the first place: one board (albconciergerie, 2,264
// vacancies) is 22 MB, past GetText's cap.
type flatchrGetter interface {
	JSONGetter
	TextGetter
}

// NewFlatchr builds the Flatchr adapter over the given HTTP client.
func NewFlatchr(c flatchrGetter) Source { return flatchr{http: c} }

func (flatchr) Provider() string { return "flatchr" }

// fullBoardListing: the board document is one unpaginated answer carrying every published
// vacancy — no loop that could stop early.
func (flatchr) fullBoardListing() {}

// flatchrBoard is the shape of a board slug.
var flatchrBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// flatchrDocument is the board document. Items is a pointer so a reshaped answer is told apart
// from a board with no vacancies, which the platform serves as an empty array.
type flatchrDocument struct {
	Items *[]flatchrItem `json:"items"`
}

// flatchrItem is one publication of a vacancy on the careers site.
type flatchrItem struct {
	Status    string         `json:"status"`
	Published bool           `json:"published"`
	CreatedAt string         `json:"created_at"`
	Vacancy   flatchrVacancy `json:"vacancy"`
}

type flatchrVacancy struct {
	ID               string `json:"id"`
	Slug             string `json:"slug"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Mission          string `json:"mission"`
	Profile          string `json:"profile"`
	Language         string `json:"language"`
	Remote           string `json:"remote"`
	Partial          bool   `json:"partial"`
	ContractType     string `json:"contract_type"`
	AddressFormatted string `json:"addressFormatted"`
	Company          struct {
		Name string `json:"name"`
	} `json:"company"`
}

func (s flatchr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !flatchrBoard.MatchString(board) {
		return nil, fmt.Errorf("flatchr: board %q is not a company slug", e.Board)
	}
	docURL := "https://careers.flatchr.io/company/" + board + ".json"
	var doc flatchrDocument
	if err := s.http.GetJSON(ctx, docURL, &doc); err != nil {
		if s.gone(ctx, docURL, err) {
			return nil, fmt.Errorf("flatchr: board %q: %w", board, ErrBoardGone)
		}
		return nil, fmt.Errorf("flatchr: board %s: %w", board, err)
	}
	if doc.Items == nil {
		return nil, fmt.Errorf("flatchr: board %s carries no items", board)
	}
	var jobs []Job
	for _, it := range *doc.Items {
		if !it.Published || it.Status != "published" {
			continue
		}
		if j, ok := flatchrJob(board, e, it); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// gone reports whether a failed board read is the platform saying the board does not exist:
// a 404, or the switched-off careers site's HTML fragment in place of the document. The second
// costs one more request, spent only on a document that did not decode.
func (s flatchr) gone(ctx context.Context, docURL string, err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusNotFound
	}
	body, terr := s.http.GetText(ctx, docURL)
	return terr == nil && strings.Contains(body, `class="notFound"`)
}

func flatchrJob(board string, e CompanyEntry, it flatchrItem) (Job, bool) {
	v := it.Vacancy
	id := strings.TrimSpace(v.ID)
	slug := strings.TrimSpace(v.Slug)
	if id == "" || slug == "" {
		return Job{}, false // no id → nothing a close could reach; no slug → no address
	}
	var parts []string
	for _, h := range []string{v.Description, v.Mission, v.Profile} {
		if h = strings.TrimSpace(h); h != "" {
			parts = append(parts, h)
		}
	}
	workMode := flatchrWorkMode(v.Remote)
	return Job{
		ExternalID:     id,
		URL:            "https://careers.flatchr.io/" + flatchrLang(v.Language) + "/company/" + board + "/vacancy/" + slug + "/",
		Title:          strings.TrimSpace(v.Title),
		Company:        firstNonEmpty(strings.TrimSpace(v.Company.Name), e.Company),
		Location:       strings.TrimSpace(v.AddressFormatted),
		Description:    sanitizeHTML(strings.Join(parts, "\n")),
		Remote:         workMode == "remote",
		WorkMode:       workMode,
		EmploymentType: flatchrEmploymentType(v.ContractType, v.Partial),
		PostedAt:       parseRFC3339(it.CreatedAt),
	}, true
}

// flatchrLang is the careers-site path prefix for a vacancy's language ("fr_FR" → "fr").
func flatchrLang(language string) string {
	if l, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(language)), "_"); len(l) == 2 {
		return l
	}
	return "fr"
}

// flatchrWorkMode maps the telework enum: full, partial or no telework.
func flatchrWorkMode(remote string) string {
	switch strings.ToLower(strings.TrimSpace(remote)) {
	case "fulltime":
		return "remote"
	case "parttime":
		return "hybrid"
	case "notime":
		return "onsite"
	}
	return ""
}

// flatchrEmploymentType maps the French contract type onto vocab.EmploymentTypeValues, the way
// wttj reads the same contracts: a CDD (fixed-term), freelance, interim, seasonal or
// contractual post is "contract" and a Stage "internship". A CDI is a permanent contract, so
// its hours come from the part-time flag. The public-service statuses (Statutaire,
// Titulaire), work-study (Alternance, Professionnalisation) and VIE have no value in the
// vocabulary and stay "" unless flagged part-time.
func flatchrEmploymentType(contract string, partial bool) string {
	switch strings.ToLower(strings.TrimSpace(contract)) {
	case "freelance", "cdd", "interim", "saisonnier", "contractuel":
		return "contract"
	case "stage":
		return "internship"
	}
	if partial {
		return "part_time"
	}
	if strings.EqualFold(strings.TrimSpace(contract), "cdi") {
		return "full_time"
	}
	return ""
}
