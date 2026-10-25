package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// kenjo adapts Kenjo (kenjo.io), a German HR suite for small and mid-sized companies whose
// recruiting module hosts a career site at https://<board>.kenjo.io. The site is an Angular app
// that reads one JSON API on its own host:
//
//	/api/controller/career-site/public/<board>/positions              the site and its open positions
//	/api/controller/career-site/public/<board>/positions/<customUrl>  one position, with its body
//
// The listing is unpaginated and carries every open position's id, title and office name but no
// body, so this is a HydratingSource; a position's public page is https://<board>.kenjo.io/
// <customUrl>. An unknown board answers 404 "Company career site was not found." (the platform
// serves every label, so DNS proves nothing). The same API on api.kenjo.io refuses a request
// that names no Kenjo origin; on the tenant's own host it answers a plain request.
type kenjo struct {
	http JSONGetter
}

// NewKenjo builds the Kenjo adapter over the given HTTP client.
func NewKenjo(c JSONGetter) Source { return kenjo{http: c} }

func (kenjo) Provider() string { return "kenjo" }

// fullBoardListing: the listing is one unpaginated answer carrying every open position.
func (kenjo) fullBoardListing() {}

// kenjoBoard is the shape of a career-site label.
var kenjoBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// kenjoSite is the listing answer. ActivePositions is a pointer so a reshaped answer is told
// apart from a site with no openings.
type kenjoSite struct {
	CompanyName     string           `json:"companyName"`
	ActivePositions *[]kenjoPosition `json:"activePositions"`
}

// kenjoPosition is a position as the listing and the detail both carry it; only the detail
// fills JobDescription, OfficeAddress and DatePosted.
type kenjoPosition struct {
	ID             string `json:"_id"`
	JobTitle       string `json:"jobTitle"`
	CustomURL      string `json:"customUrl"`
	PositionType   string `json:"positionType"`
	CompanyName    string `json:"companyName"`
	OfficeName     string `json:"officeName"`
	DatePosted     string `json:"datePosted"`
	JobDescription struct {
		HTML string `json:"html"`
	} `json:"jobDescription"`
	OfficeAddress struct {
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"officeAddress"`
}

func kenjoAPI(board string) string {
	return "https://" + board + ".kenjo.io/api/controller/career-site/public/" + board + "/positions"
}

// Fetch reads the listing and every position's detail.
func (s kenjo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board, positions, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(positions, defaultDetailWorkers, func(p kenjoPosition) (Job, bool) {
		return s.detail(ctx, e, board, p)
	}), nil
}

// FetchNew is Fetch that spends a detail request only on positions the catalogue does not
// hold; a seen position is re-listed as a SeenRefresh, the listing proving it is still open.
func (s kenjo) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, positions, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(positions, defaultDetailWorkers, func(p kenjoPosition) (Job, bool) {
		if seen(p.ID) {
			return Job{ExternalID: p.ID, URL: kenjoPageURL(board, p.CustomURL), Title: strings.TrimSpace(p.JobTitle),
				Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, board, p)
	}), nil
}

// list reads the site's open positions, dropping any without the id a close needs or the
// customUrl that addresses its detail.
func (s kenjo) list(ctx context.Context, rawBoard string) (string, []kenjoPosition, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !kenjoBoard.MatchString(board) {
		return "", nil, fmt.Errorf("kenjo: board %q is not a career-site label", rawBoard)
	}
	var site kenjoSite
	if err := s.http.GetJSON(ctx, kenjoAPI(board), &site); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", nil, fmt.Errorf("kenjo: board %q: %w", board, ErrBoardGone)
		}
		return "", nil, fmt.Errorf("kenjo: board %s: %w", board, err)
	}
	if site.ActivePositions == nil {
		return "", nil, fmt.Errorf("kenjo: board %s answer carries no activePositions", board)
	}
	var out []kenjoPosition
	for _, p := range *site.ActivePositions {
		if strings.TrimSpace(p.ID) != "" && strings.TrimSpace(p.CustomURL) != "" {
			out = append(out, p)
		}
	}
	return board, out, nil
}

func kenjoPageURL(board, customURL string) string {
	return "https://" + board + ".kenjo.io/" + url.PathEscape(strings.TrimSpace(customURL))
}

// detail fetches one position and maps it to a Job. A 404 is the platform's own "Position was
// not found." and drops it; any other failure, or a detail without a body, is an
// unreadableDetail marker — the detail is the only source of the body.
func (s kenjo) detail(ctx context.Context, e CompanyEntry, board string, p kenjoPosition) (Job, bool) {
	pageURL := kenjoPageURL(board, p.CustomURL)
	var d kenjoPosition
	if err := s.http.GetJSON(ctx, kenjoAPI(board)+"/"+url.PathEscape(strings.TrimSpace(p.CustomURL)), &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.ID, pageURL, e.Company), true
		}
		return Job{}, false
	}
	if strings.TrimSpace(d.JobDescription.HTML) == "" {
		return unreadableDetail(p.ID, pageURL, e.Company), true
	}
	j := Job{
		ExternalID:     p.ID,
		URL:            pageURL,
		Title:          firstNonEmpty(strings.TrimSpace(d.JobTitle), strings.TrimSpace(p.JobTitle)),
		Company:        firstNonEmpty(strings.TrimSpace(d.CompanyName), strings.TrimSpace(p.CompanyName), e.Company),
		Location:       firstNonEmpty(strings.TrimSpace(d.OfficeAddress.City), strings.TrimSpace(d.OfficeName), strings.TrimSpace(p.OfficeName)),
		Description:    sanitizeHTML(d.JobDescription.HTML),
		EmploymentType: kenjoEmploymentType(firstNonEmpty(d.PositionType, p.PositionType)),
		PostedAt:       parseRFC3339(strings.TrimSpace(d.DatePosted)),
	}
	if c := location.NormalizeCountry(d.OfficeAddress.Country); c != "" {
		j.Countries = []string{c}
	}
	return j, true
}

// kenjoEmploymentType maps the position type. The listing localises it ("Vollzeit"); the
// detail states the picklist's own key, which is what is mapped — measured across 177 positions
// on 55 sites: Full-time, Flexible, Freelance, Apprentice, Intern, Student, and none at all.
// "Flexible", "Apprentice" and "Student" (Werkstudent) have no value in the vocabulary and stay
// "", as does no work arrangement at all: Kenjo records none, so WorkMode is the parser's.
func kenjoEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time", "vollzeit":
		return "full_time"
	case "part-time", "teilzeit":
		return "part_time"
	case "freelance", "temporary", "befristet":
		return "contract"
	case "intern", "internship", "praktikant":
		return "internship"
	}
	return ""
}
