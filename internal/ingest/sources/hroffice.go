package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// hroffice adapts HR Office (hroffice.eu), a Dutch HR suite whose recruiting module hosts a
// career site at https://<board>.inhroffice.com — municipalities, healthcare, wholesale and
// manufacturing. The site is a small app over one open JSON listing on its own host:
//
//	/api/nl/vacancies   every open vacancy: id, title, city, country (in Dutch), publication date
//
// The listing carries no body; each vacancy's page, /nl/job/<id>, is server-rendered with the
// body in its main column, so this is a HydratingSource.
//
// Many employers show the same vacancies on their own domain (careers.fitforme.com,
// werkenbij.jaarbeurs.nl); those sites pull from HR Office and do not serve its API, so the
// board is always the inhroffice.com label. An unknown or switched-off label answers the listing
// with HTTP 500 and a "MiniJobPortalNotFoundException" body — a status the shared client cannot
// tell from a real server error, so it stays an ordinary failure and the chronic-board close
// retires such a board.
type hroffice struct {
	http hrofficeGetter
}

// hrofficeGetter is the transport the adapter needs: the listing as JSON, each page as HTML.
type hrofficeGetter interface {
	JSONGetter
	HTMLGetter
}

// NewHROffice builds the HR Office adapter over the given HTTP client.
func NewHROffice(c hrofficeGetter) Source { return hroffice{http: c} }

func (hroffice) Provider() string { return "hroffice" }

// fullBoardListing: the listing is one unpaginated answer carrying every open vacancy.
func (hroffice) fullBoardListing() {}

// hrofficeBoard is the shape of a career-site label.
var hrofficeBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// hrofficeVacancy is one listing entry.
type hrofficeVacancy struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	City        *string `json:"city"`
	Country     string  `json:"country"`
	PublishDate string  `json:"publishDate"`
}

func (v hrofficeVacancy) externalID() string { return strconv.FormatInt(v.ID, 10) }

func hrofficePage(board string, id string) string {
	return "https://" + board + ".inhroffice.com/nl/job/" + id
}

// Fetch reads the listing and every vacancy's page.
func (s hroffice) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board, vacancies, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v hrofficeVacancy) (Job, bool) {
		return s.detail(ctx, e, board, v)
	}), nil
}

// FetchNew is Fetch that spends a page request only on vacancies the catalogue does not hold; a
// seen vacancy is re-listed as a SeenRefresh, the listing proving it is still open.
func (s hroffice) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, vacancies, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(vacancies, defaultDetailWorkers, func(v hrofficeVacancy) (Job, bool) {
		if id := v.externalID(); seen(id) {
			return Job{ExternalID: id, URL: hrofficePage(board, id), Title: strings.TrimSpace(v.Title),
				Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, board, v)
	}), nil
}

// list reads the site's listing. The answer is a bare array, so a reshaped one fails the decode
// rather than reading as an empty site; an entry without an id is dropped, since no close could
// reach it.
func (s hroffice) list(ctx context.Context, rawBoard string) (string, []hrofficeVacancy, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !hrofficeBoard.MatchString(board) {
		return "", nil, fmt.Errorf("hroffice: board %q is not a career-site label", rawBoard)
	}
	var vacancies []hrofficeVacancy
	if err := s.http.GetJSON(ctx, "https://"+board+".inhroffice.com/api/nl/vacancies", &vacancies); err != nil {
		return "", nil, fmt.Errorf("hroffice: board %s listing: %w", board, err)
	}
	if vacancies == nil {
		return "", nil, fmt.Errorf("hroffice: board %s listing is not an array", board)
	}
	out := vacancies[:0]
	seen := map[int64]bool{}
	for _, v := range vacancies {
		if v.ID > 0 && !seen[v.ID] {
			seen[v.ID] = true
			out = append(out, v)
		}
	}
	return board, out, nil
}

// detail fetches one vacancy's page and maps it, with its listing entry, to a Job. A 404 is the
// site's own "no such vacancy" and drops it; any other failure, or a page without its main
// column, is an unreadableDetail marker — the page is the only source of the body.
func (s hroffice) detail(ctx context.Context, e CompanyEntry, board string, v hrofficeVacancy) (Job, bool) {
	id := v.externalID()
	pageURL := hrofficePage(board, id)
	root, err := s.http.GetHTML(ctx, pageURL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, pageURL, e.Company), true
		}
		return Job{}, false
	}
	main := firstByClass(root, "vacancy-main-container")
	if main == nil {
		return unreadableDetail(id, pageURL, e.Company), true
	}
	body := main
	if sec := firstByTag(main, "section"); sec != nil {
		body = sec
	}
	dropApplyButtons(body)
	city := ""
	if v.City != nil {
		city = strings.TrimSpace(*v.City)
	}
	j := Job{
		ExternalID:  id,
		URL:         pageURL,
		Title:       strings.TrimSpace(v.Title),
		Company:     e.Company,
		Location:    joinNonEmpty(city, strings.TrimSpace(v.Country)), // as the page shows it: "Schipluiden, Nederland"
		Description: sanitizeHTML(innerHTML(body)),
		PostedAt:    parseLayout("02-01-2006", strings.TrimSpace(v.PublishDate)),
	}
	// The country is a dedicated field, written in Dutch ("Nederland", "België", "Duitsland").
	if c := location.NormalizeCountry(v.Country); c != "" {
		j.Countries = []string{c}
	}
	return j, true
}

// dropApplyButtons removes the "Solliciteren" button block the page closes its body with — a
// direct child holding a button-styled link — so the body is the vacancy's own text.
func dropApplyButtons(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			hasButton := false
			walk(c, func(m *html.Node) bool {
				if m.Type == html.ElementNode && m.Data == "a" && HasClass(m, "btn") {
					hasButton = true
				}
				return !hasButton
			})
			if hasButton {
				n.RemoveChild(c)
			}
		}
		c = next
	}
}
