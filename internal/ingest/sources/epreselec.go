package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// epreselec adapts ePreselec, the ATS of Adevinta (InfoJobs' owner) that Spanish employers —
// FCC, Ineco, Ilunion, Paradores, Applus+, Compass Group — run their careers sites on, one
// ASP.NET site per customer at https://<board>.epreselec.com. The board is that label.
//
//	/Ofertas/Ofertas.aspx                 every open offer, in one unpaginated list
//	/Ofertas/Ofertas.aspx?id_oferta=<id>  the same page with that offer's detail panel filled in
//
// A list entry is an anchor carrying data_idOferta with the title, the employer brand (a group
// such as FCC lists Aqualia and FCC Infraestructuras), "Province, Country" and a Spanish date;
// the page states the count ("Total ofertas: 259"), which the walk is checked against. The detail
// page repeats the whole list beside the panel, so a body costs 50-700 KB; hence a HydratingSource
// with a per-board budget.
//
// The edge is Imperva's: to the crawl host and to a single proxy exit it answers "Pardon Our
// Interruption" (HTTP 405) after a handful of requests, and through the rotating exits about
// half the requests pass. The provider is therefore routed through the Scrapling tier's
// impersonating fetcher over the rotating proxy (scraplingtier.go), and the adapter retries a 405
// several times — each retry leaves on a new exit.
//
// An unknown or switched-off label answers 200 with the platform's "404 - Page not found" page;
// that is a board-level error, so the chronic-board close retires the board.
type epreselec struct {
	http         HTMLGetter
	maxNewPerRun int
	retryWait    time.Duration
}

// NewEPreselec builds the ePreselec adapter over the given HTML client.
func NewEPreselec(c HTMLGetter) Source {
	return epreselec{http: c, maxNewPerRun: epreselecMaxNewPerBoard, retryWait: 5 * time.Second}
}

func (epreselec) Provider() string { return "epreselec" }

// fullBoardListing: the list is one unpaginated page, and its length is checked against the
// count the page states.
func (epreselec) fullBoardListing() {}

const (
	// epreselecMaxNewPerBoard bounds the bodies one board buys in a run. The largest sites hold
	// ~300 offers and each body is a page carrying the whole list again; the rest wait for the
	// next run, and a stored posting never costs a body again.
	epreselecMaxNewPerBoard = 120
	// epreselecAttempts is how many times a request refused by the edge (405) is sent. With four,
	// the first crawl (2026-10-27) still lost 498 of 1,211 bodies and ten lists: the refusals
	// come in runs, not independently. Each retry leaves on a new rotating exit.
	epreselecAttempts = 8
	// epreselecDetailWorkers keeps the per-board fan-out low: every request goes through the
	// crawl host's Scrapling service.
	epreselecDetailWorkers = 2
)

// epreselecBoardPattern is the shape of a site label.
var epreselecBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var epreselecTotal = regexp.MustCompile(`Total ofertas:\s*(\d+)`)

// epreselecOffer is one entry of the list.
type epreselecOffer struct {
	id, title, company, place, date string
}

func epreselecListURL(board string) string {
	return "https://" + board + ".epreselec.com/Ofertas/Ofertas.aspx"
}

func epreselecOfferURL(board, id string) string {
	return epreselecListURL(board) + "?id_oferta=" + id
}

// Fetch reads the list and every offer's detail.
func (s epreselec) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the list and buys a body only for offers the catalogue does not hold, at most
// maxNewPerRun of them; a seen offer is re-listed as a SeenRefresh, the list proving it is still
// open, and an unseen offer past the budget is left for a later run.
func (s epreselec) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, offers, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []epreselecOffer
	deferred := 0
	for _, o := range offers {
		switch {
		case seen(o.id):
			jobs = append(jobs, Job{ExternalID: o.id, URL: epreselecOfferURL(board, o.id), Title: o.title,
				Company: firstNonEmpty(o.company, e.Company), SeenRefresh: true})
		case len(fresh) < s.maxNewPerRun:
			fresh = append(fresh, o)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("epreselec: board %q: hydration budget %d reached; %d new offers deferred to a later run",
			board, s.maxNewPerRun, deferred)
	}
	return append(jobs, fetchDetails(fresh, epreselecDetailWorkers, func(o epreselecOffer) (Job, bool) {
		return s.detail(ctx, e, board, o)
	})...), nil
}

// get fetches one page, sending it again while the edge refuses it with a 405.
func (s epreselec) get(ctx context.Context, url string) (*html.Node, error) {
	var err error
	for attempt := 1; attempt <= epreselecAttempts; attempt++ {
		var root *html.Node
		root, err = s.http.GetHTML(ctx, url)
		if err == nil {
			return root, nil
		}
		var se *StatusError
		if !errors.As(err, &se) || se.Code != http.StatusMethodNotAllowed || attempt == epreselecAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.retryWait):
		}
	}
	return nil, err
}

// list reads the site's offer list and checks it against the count the page states. A page that
// states no count is not the list (the platform's 404 page, or a reshaped site).
func (s epreselec) list(ctx context.Context, rawBoard string) (string, []epreselecOffer, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !epreselecBoardPattern.MatchString(board) {
		return "", nil, fmt.Errorf("epreselec: board %q is not a site label", rawBoard)
	}
	root, err := s.get(ctx, epreselecListURL(board))
	if err != nil {
		return "", nil, fmt.Errorf("epreselec: board %s list: %w", board, err)
	}
	total := -1
	var offers []epreselecOffer
	seen := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		if total < 0 && HasClass(n, "total-vacancies") {
			if m := epreselecTotal.FindStringSubmatch(textContent(n)); m != nil {
				total, _ = strconv.Atoi(m[1])
			}
			return false
		}
		id := strings.TrimSpace(Attr(n, "data_idoferta"))
		if n.Data != "a" || id == "" {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		o := epreselecOffer{id: id}
		walk(n, func(m *html.Node) bool {
			if m.Type != html.ElementNode || m.Data != "span" {
				return true
			}
			v := epreselecText(textContent(m))
			switch {
			case HasClass(m, "op-titulo"):
				o.title = v
			case HasClass(m, "op-perfil"):
				o.company = v
			case HasClass(m, "op-provincia"):
				o.place = v
			case HasClass(m, "op-fecha"):
				o.date = v
			}
			return false
		})
		offers = append(offers, o)
		return false
	})
	if total < 0 {
		if firstByID(root, "ctl00_CPH_Body_pnlVacanciesList") == nil {
			return "", nil, fmt.Errorf("epreselec: board %s: no offer list on the page (unknown or closed site)", board)
		}
		total = 0 // the list panel without a count: the site's "no offers" state
	}
	if len(offers) != total {
		return "", nil, fmt.Errorf("epreselec: board %s: read %d offers, the page states %d", board, len(offers), total)
	}
	return board, offers, nil
}

// detail fetches one offer's page and maps its detail panel, with the list entry, to a Job. The
// panel is a run of h3 headings (Funciones, Requisitos, Se ofrece …) each followed by a paragraph;
// a page whose panel names no title is the platform dropping an id it no longer serves.
func (s epreselec) detail(ctx context.Context, e CompanyEntry, board string, o epreselecOffer) (Job, bool) {
	pageURL := epreselecOfferURL(board, o.id)
	company := firstNonEmpty(o.company, e.Company)
	root, err := s.get(ctx, pageURL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(o.id, pageURL, company), true
		}
		return Job{}, false
	}
	panel := firstByClass(root, "onepage-oferta")
	if panel == nil {
		return unreadableDetail(o.id, pageURL, company), true
	}
	field := func(id string) string {
		if n := firstByID(panel, "ctl00_CPH_Body_"+id); n != nil {
			return epreselecText(textContent(n))
		}
		return ""
	}
	if field("lDescripcion") == "" {
		return Job{}, false
	}
	var body strings.Builder
	var heading string
	walk(panel, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "h3":
			heading = epreselecText(textContent(n))
			return false
		case "p":
			if text := strings.TrimSpace(innerHTML(n)); text != "" && epreselecText(textContent(n)) != "" {
				if heading != "" {
					body.WriteString("<h3>" + html.EscapeString(heading) + "</h3>")
				}
				body.WriteString("<p>" + text + "</p>")
			}
			heading = ""
			return false
		}
		return true
	})
	if body.Len() == 0 {
		return unreadableDetail(o.id, pageURL, company), true
	}
	city, province, country := field("lLocalidadText"), field("lProvinciaText"), field("lPaisText")
	if province == "" && country == "" {
		province, country, _ = strings.Cut(o.place, ",")
	}
	country = strings.TrimSpace(firstNonEmpty(country, "España"))
	j := Job{
		ExternalID:  o.id,
		URL:         pageURL,
		Title:       firstNonEmpty(field("lDescripcion"), o.title),
		Company:     firstNonEmpty(field("lEmpresaText"), company),
		Location:    joinNonEmpty(city, strings.TrimSpace(province), country),
		Description: sanitizeHTML(body.String()),
		PostedAt:    epreselecDate(o.date),
	}
	if c := location.NormalizeCountry(country); c != "" {
		j.Countries = []string{c}
	}
	return j, true
}

// epreselecMonths are the Spanish month names of the list's dates ("27 de octubre, 2026").
var epreselecMonths = map[string]time.Month{
	"enero": time.January, "febrero": time.February, "marzo": time.March, "abril": time.April,
	"mayo": time.May, "junio": time.June, "julio": time.July, "agosto": time.August,
	"septiembre": time.September, "setiembre": time.September, "octubre": time.October,
	"noviembre": time.November, "diciembre": time.December,
}

var epreselecDatePattern = regexp.MustCompile(`^(\d{1,2}) de ([a-záéíóú]+),? (\d{4})$`)

// epreselecDate parses a list date, or returns nil for any other text.
func epreselecDate(s string) *time.Time {
	m := epreselecDatePattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return nil
	}
	month, ok := epreselecMonths[m[2]]
	if !ok {
		return nil
	}
	day, _ := strconv.Atoi(m[1])
	year, _ := strconv.Atoi(m[3])
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &t
}

// epreselecText collapses the whitespace the templates indent their text with.
func epreselecText(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}
