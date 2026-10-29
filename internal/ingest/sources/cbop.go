package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// cbop adapts the Centralna Baza Ofert Pracy (oferty.praca.gov.pl), Poland's public job board:
// the offers the country's labour offices (PUP/WUP) and youth labour corps (OHP) register on
// behalf of employers, ~18k open, every trade. It is boardless and an aggregator — the employer
// is named per offer. See the "CBOP traps" section of AGENTS.md for the measurements behind each
// rule.
//
// The listing is the portal's own search API, whose rows carry the whole offer (duties,
// requirements, contract, pay as text), so a crawl buys no detail page. The portal's HTML sits
// behind an F5 JavaScript challenge; its API does not.
type cbop struct {
	http    JSONPoster
	limiter waiter
}

const (
	cbopSearchURL = "https://oferty.praca.gov.pl/portal-api/v3/oferta/wyszukiwanie?page=%d&size=%d"
	cbopOfferURL  = "https://oferty.praca.gov.pl/portal/lista-ofert/szczegoly-oferty/"
	// cbopPageSize is the listing page; a full walk is ~185 pages of ~250 KB.
	cbopPageSize = 100
	// cbopSplit is how a refused page is re-read: as cbopSplit pages of a tenth the size, and a
	// refused one of those as single rows (see list).
	cbopSplit = 10
	// cbopMaxPages is a safety ceiling: the catalogue filled 183 pages on 2026-10-29.
	cbopMaxPages = 1000
	// cbopMaxUnreadShare bounds the rows a walk may lose to refused single-row reads before the
	// listing is no longer the whole catalogue: 5 of 18,290 were on 2026-10-29.
	cbopMaxUnreadShare = 0.01
	// cbopMaxShortShare bounds how far short of page 0's stated total a walk may end before it
	// is taken for a page that answered short, not for offers withdrawn mid-walk.
	cbopMaxShortShare = 0.02
	// cbopRequestInterval paces every request — a courtesy ceiling, not a measured limit.
	cbopRequestInterval = time.Second
)

// NewCBOP builds the adapter over the given client.
func NewCBOP(c JSONPoster) Source {
	return cbop{http: c, limiter: rate.NewLimiter(rate.Every(cbopRequestInterval), 1)}
}

func (cbop) Provider() string { return "cbop" }

// One national board, no per-tenant id.
func (cbop) boardless() {}

// Thousands of employers, each named per offer.
func (cbop) aggregator() {}

// cbopResponse is one search answer. A row the service cannot render fails the whole page with
// "status": 500 and a null payload under HTTP 200.
type cbopResponse struct {
	Status  int    `json:"status"`
	Msg     string `json:"msg"`
	Payload *struct {
		Page struct {
			Content       []cbopOffer `json:"content"`
			TotalElements int         `json:"totalElements"`
		} `json:"ofertyPracyPage"`
	} `json:"payload"`
}

// cbopOffer is the part of a search row the adapter reads.
type cbopOffer struct {
	ID           string `json:"id"`
	Title        string `json:"stanowisko"`
	Employer     string `json:"pracodawca"`
	Place        string `json:"miejscePracy"`
	WholePoland  bool   `json:"calaPolska"`
	Duties       string `json:"zakresObowiazkow"`
	Requirements string `json:"wymagania"`
	Contract     string `json:"rodzajUmowy"`
	Hours        string `json:"wymiarZatrud"`
	Pay          string `json:"wynagrodzenie"`
	Kind         string `json:"typOfertyEnum"`
	Added        string `json:"dataDodaniaCbop"`
}

// Fetch lists every offer and maps the ones naming their employer.
func (s cbop) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	offers, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(offers))
	for _, o := range offers {
		if j, ok := o.job(); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// list walks the search newest first and returns each offer once. A page the service refuses
// is re-read in smaller pages so one bad row costs only itself; past cbopMaxUnreadShare of rows
// lost, or on any transport failure, the crawl fails, because a partial listing would let the
// sweep close the offers it did not read.
func (s cbop) list(ctx context.Context) ([]cbopOffer, error) {
	seen := make(map[string]bool)
	var out []cbopOffer
	unread := 0
	add := func(rows []cbopOffer) {
		for _, o := range rows {
			if o.ID != "" && !seen[o.ID] {
				seen[o.ID] = true
				out = append(out, o)
			}
		}
	}
	first, err := s.page(ctx, 0, cbopPageSize)
	if err != nil {
		return nil, fmt.Errorf("cbop: search page 0: %w", err)
	}
	if first == nil {
		return nil, fmt.Errorf("cbop: search page 0 refused")
	}
	total := first.Payload.Page.TotalElements
	read := len(first.Payload.Page.Content)
	add(first.Payload.Page.Content)
	// The walk ends where the service does — at a page shorter than the page size — rather
	// than at page 0's total, which moves while the walk runs.
	for p := 1; read%cbopPageSize == 0 && read > 0; p++ {
		if p >= cbopMaxPages {
			return nil, fmt.Errorf("cbop: search still not exhausted after %d pages", p)
		}
		rows, lost, err := s.readSplit(ctx, p, cbopPageSize)
		if err != nil {
			return nil, err
		}
		unread += lost
		read += len(rows) + lost
		add(rows)
		if len(rows)+lost < cbopPageSize {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cbop: search names no offer")
	}
	// A page that answered short in the middle ends the walk early; the count read against the
	// stated total is what catches it. The total moves by a few dozen during a walk.
	if float64(read) < (1-cbopMaxShortShare)*float64(total) {
		return nil, fmt.Errorf("cbop: walk ended after %d of %d offers", read, total)
	}
	if float64(unread) > cbopMaxUnreadShare*float64(total) {
		return nil, fmt.Errorf("cbop: %d of %d offers could not be read", unread, total)
	}
	if unread > 0 {
		log.Printf("cbop: %d of %d offers refused by the service and skipped", unread, total)
	}
	return out, nil
}

// readSplit reads page p of the given size, and when the service refuses it, the same rows as
// cbopSplit pages of size/cbopSplit, down to single rows. lost counts the rows no read reached.
func (s cbop) readSplit(ctx context.Context, p, size int) (rows []cbopOffer, lost int, err error) {
	resp, err := s.page(ctx, p, size)
	if err != nil {
		return nil, 0, fmt.Errorf("cbop: search page %d (size %d): %w", p, size, err)
	}
	if resp != nil {
		return resp.Payload.Page.Content, 0, nil
	}
	if size == 1 {
		return nil, 1, nil
	}
	sub := size / cbopSplit
	if sub < 1 {
		sub = 1
	}
	for q := p * size / sub; q < (p+1)*size/sub; q++ {
		r, l, err := s.readSplit(ctx, q, sub)
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, r...)
		lost += l
	}
	return rows, lost, nil
}

// page reads one search page; nil with a nil error is the service refusing it.
func (s cbop) page(ctx context.Context, p, size int) (*cbopResponse, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	var resp cbopResponse
	if err := s.http.PostJSON(ctx, fmt.Sprintf(cbopSearchURL, p, size), struct{}{}, &resp); err != nil {
		return nil, err
	}
	if resp.Payload == nil {
		return nil, nil
	}
	return &resp, nil
}

// job maps an offer, or ok=false for one whose employer is withheld ("kontakt przez PUP" — the
// labour office takes the applications; every offer reserved for registered jobseekers is one)
// or that has no title or body.
func (o cbopOffer) job() (Job, bool) {
	title := strings.Join(strings.Fields(o.Title), " ")
	employer := strings.Join(strings.Fields(o.Employer), " ")
	if title == "" || employer == "" || strings.HasPrefix(strings.ToLower(employer), "kontakt przez") {
		return Job{}, false
	}
	var body strings.Builder
	section := func(head, text string) {
		if text = strings.TrimSpace(text); text != "" {
			body.WriteString("<h3>" + head + "</h3>\n" + plainTextToHTML(text) + "\n")
		}
	}
	section("Zakres obowiązków", o.Duties)
	section("Wymagania", o.Requirements)
	if body.Len() == 0 {
		return Job{}, false
	}
	var facts []string
	for _, f := range [][2]string{{"Rodzaj umowy", o.Contract}, {"Wymiar etatu", o.Hours}, {"Wynagrodzenie brutto", o.Pay}} {
		if v := strings.Join(strings.Fields(f[1]), " "); v != "" && !strings.EqualFold(v, "nie dotyczy") {
			facts = append(facts, html.EscapeString(f[0]+": "+v))
		}
	}
	if len(facts) > 0 {
		body.WriteString("<p>" + strings.Join(facts, "<br>") + "</p>\n")
	}
	j := Job{
		ExternalID:        o.ID,
		URL:               cbopOfferURL + o.ID,
		Title:             title,
		Company:           employer,
		Location:          strings.Join(strings.Fields(o.Place), " "),
		Description:       sanitizeHTML(body.String()),
		FallbackCountries: []string{"pl"},
		EmploymentType:    cbopEmploymentType(o),
		PostedAt:          NotFuture(parseLayout("02.01.2006", strings.TrimSpace(o.Added))),
	}
	if o.WholePoland {
		j.Location = "Polska"
		j.Countries = []string{"pl"}
	}
	return j, true
}

// cbopEmploymentType maps the offer kind and the share of a full-time post ("1", "0.5", "3/4";
// "nie dotyczy" for a mandate contract). An internship or traineeship (staż, praktyki) is an
// internship whatever its hours.
func cbopEmploymentType(o cbopOffer) string {
	switch o.Kind {
	case "STAZ", "STAZ_UP", "PRAKTYKI":
		return "internship"
	}
	share, ok := cbopShare(o.Hours)
	switch {
	case !ok:
		return ""
	case share >= 1:
		return "full_time"
	case share > 0:
		return "part_time"
	}
	return ""
}

// cbopShare reads a post share written as a decimal or a fraction.
func cbopShare(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if num, den, isFrac := strings.Cut(s, "/"); isFrac {
		n, err1 := strconv.ParseFloat(num, 64)
		d, err2 := strconv.ParseFloat(den, 64)
		if err1 != nil || err2 != nil || d == 0 {
			return 0, false
		}
		return n / d, true
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	return v, err == nil
}
