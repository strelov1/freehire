package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// francetravail adapts France Travail (ex-Pôle emploi), France's public employment service,
// through its official "Offres d'emploi v2" API. It is keyed: the API takes an OAuth2
// client-credentials token issued to an application registered on francetravail.io
// (FRANCE_TRAVAIL_CLIENT_ID / FRANCE_TRAVAIL_CLIENT_SECRET), and the provider is registered only
// when both are set. It is an aggregator — the employer is read per posting — whose board is a
// DÉPARTEMENT code ("75", "2A", "971"): every offer has one place of work, so the départements
// partition the catalogue. See the "France Travail traps" section of AGENTS.md.
//
// The search carries every offer whole, description included, so a crawl issues no detail
// request: it lists the board's offers in creation-date windows narrow enough to fall under the
// API's 1,150-result window, and yields each.
type francetravail struct {
	http    francetravailHTTP
	limiter waiter
	creds   url.Values
	token   *francetravailToken
	now     func() time.Time
	// floor is the earliest creation date a window reaches back to.
	floor time.Time
}

// francetravailHTTP is the transport the adapter needs: a form POST for the token, and a search
// GET that may answer 204 for an empty window.
type francetravailHTTP interface {
	FormJSONPoster
	GetJSONOrNoContent(ctx context.Context, url string, headers map[string]string, v any) (bool, error)
}

const (
	francetravailTokenURL  = "https://entreprise.francetravail.fr/connexion/oauth2/access_token?realm=%2Fpartenaire"
	francetravailSearchURL = "https://api.francetravail.io/partenaire/offresdemploi/v2/offres/search"
	francetravailScope     = "api_offresdemploiv2 o2dsoffre"
	francetravailOfferURL  = "https://candidat.francetravail.fr/offres/recherche/detail/"

	// francetravailPageSize is the most a search page returns; francetravailWindow is the
	// API's result window: a range may start at most at 1,000 and end at most at 1,149.
	francetravailPageSize = 150
	francetravailWindow   = 1150
	// francetravailRequestInterval paces every request at one a second, a tenth of the API's
	// documented 10 calls a second for an application.
	francetravailRequestInterval = time.Second
	// francetravailMinWindow is the narrowest creation-date window the slicing splits down to.
	// More than 1,150 offers created inside one minute in one département would be a burst
	// of bulk uploads; the run reads the first 1,150 and logs the rest.
	francetravailMinWindow = time.Minute
	// francetravailOrigin selects France Travail's own offers. Partner offers (origine 2) are
	// copies from other job boards, several of which freehire crawls itself.
	francetravailOrigin = "1"
)

// francetravailFloor is the earliest creation date a window reaches back to. Offers older than a
// few months are rare; a département whose unwindowed count exceeds what the windows found says
// so in the log.
var francetravailFloor = time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)

// NewFranceTravail builds the adapter over the given client with the application's credentials.
func NewFranceTravail(c francetravailHTTP, clientID, clientSecret string) Source {
	return francetravail{
		http:    c,
		limiter: rate.NewLimiter(rate.Every(francetravailRequestInterval), 1),
		creds: url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {clientID},
			"client_secret": {clientSecret},
			"scope":         {francetravailScope},
		},
		token: &francetravailToken{},
		now:   time.Now,
		floor: francetravailFloor,
	}
}

func (francetravail) Provider() string { return "francetravail" }

// Tens of thousands of employers post here, each named per offer.
func (francetravail) aggregator() {}

// francetravailToken caches the access token across the run's boards; it lives ~25 minutes.
type francetravailToken struct {
	mu      sync.Mutex
	value   string
	expires time.Time
}

type francetravailTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// bearer returns a token valid for at least another minute, fetching one when the cached one is
// missing, expiring, or was refused (stale).
func (s francetravail) bearer(ctx context.Context, stale string) (string, error) {
	t := s.token
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.value != "" && t.value != stale && s.now().Add(time.Minute).Before(t.expires) {
		return t.value, nil
	}
	if err := s.limiter.Wait(ctx); err != nil {
		return "", err
	}
	var r francetravailTokenResponse
	if err := s.http.PostFormJSON(ctx, francetravailTokenURL, s.creds, &r); err != nil {
		return "", fmt.Errorf("francetravail: token: %w", err)
	}
	if r.AccessToken == "" {
		return "", errors.New("francetravail: token response carries no access_token")
	}
	ttl := time.Duration(r.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	t.value, t.expires = r.AccessToken, s.now().Add(ttl)
	return t.value, nil
}

// francetravailDepartement is a board: a département code, metropolitan or overseas.
var francetravailDepartement = regexp.MustCompile(`^(0[1-9]|[1-8][0-9]|9[0-5]|2A|2B|97[1-6])$`)

type francetravailPage struct {
	Results []francetravailOffer `json:"resultats"`
	Filters []struct {
		Filter       string `json:"filtre"`
		Aggregations []struct {
			Count int `json:"nbResultats"`
		} `json:"agregation"`
	} `json:"filtresPossibles"`
}

// total is the number of offers the search matched: the sum of one facet's aggregation, every
// offer having exactly one contract type. ok is false when the page states no such facet.
func (p francetravailPage) total() (int, bool) {
	for _, f := range p.Filters {
		if f.Filter != "typeContrat" {
			continue
		}
		n := 0
		for _, a := range f.Aggregations {
			n += a.Count
		}
		return n, true
	}
	return 0, false
}

type francetravailOffer struct {
	ID          string `json:"id"`
	Title       string `json:"intitule"`
	Description string `json:"description"`
	Created     string `json:"dateCreation"`
	PlaceOfWork struct {
		Label    string `json:"libelle"`
		Postcode string `json:"codePostal"`
	} `json:"lieuTravail"`
	ROMECode string `json:"romeCode"`
	Employer struct {
		Name string `json:"nom"`
	} `json:"entreprise"`
	ContractType   string          `json:"typeContrat"`
	Experience     string          `json:"experienceExige"`
	ExperienceText string          `json:"experienceLibelle"`
	Salary         json.RawMessage `json:"salaire"`
	WorkingTime    string          `json:"dureeTravailLibelleConverti"`
	Origin         struct {
		Origin string `json:"origine"`
		URL    string `json:"urlOrigine"`
	} `json:"origineOffre"`
}

// Fetch lists the département's offers whole and yields each.
func (s francetravail) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	dep := strings.ToUpper(strings.TrimSpace(e.Board))
	if !francetravailDepartement.MatchString(dep) {
		return nil, fmt.Errorf("francetravail: board %q is not a département code", e.Board)
	}
	w := francetravailWalk{s: s, dep: dep, seen: map[string]bool{}}
	total, err := w.window(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	if total > len(w.offers) && total-len(w.offers) > total/100 {
		log.Printf("francetravail: %s: the search matched %d offers and the walk read %d", dep, total, len(w.offers))
	}
	if w.truncated > 0 {
		log.Printf("francetravail: %s: %d offers past the result window of a one-minute slice were not read", dep, w.truncated)
	}
	jobs := make([]Job, 0, len(w.offers))
	for _, o := range w.offers {
		if j, ok := francetravailJob(o); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// francetravailWalk lists one département, window by window.
type francetravailWalk struct {
	s         francetravail
	dep       string
	seen      map[string]bool
	offers    []francetravailOffer
	truncated int
}

// window reads the offers created in [from, to) — the whole département when both are zero —
// and returns how many the search matched. A window matching more than the result window is
// split in two at its midpoint, the unwindowed search into [floor, now].
func (w *francetravailWalk) window(ctx context.Context, from, to time.Time) (int, error) {
	first, found, err := w.page(ctx, from, to, 0)
	if err != nil || !found {
		return 0, err
	}
	total, stated := first.total()
	full := len(first.Results) == francetravailPageSize
	if (stated && total > francetravailWindow) || (!stated && full && w.reachesWindowEnd(ctx, from, to)) {
		if from.IsZero() {
			from, to = w.s.floor, w.s.now().Add(time.Hour)
		}
		if to.Sub(from) > francetravailMinWindow {
			mid := from.Add(to.Sub(from) / 2).Truncate(time.Second)
			if _, err := w.window(ctx, from, mid); err != nil {
				return 0, err
			}
			if _, err := w.window(ctx, mid, to); err != nil {
				return 0, err
			}
			return total, nil
		}
		w.truncated += total - francetravailWindow
	}
	w.keep(first.Results)
	// The window's last page starts at 1,000, not at the next multiple of 150 (1,050), which the
	// API refuses; the overlap with the page before is deduplicated by keep.
	const lastStart = francetravailWindow - francetravailPageSize
	for start := francetravailPageSize; start < francetravailWindow && len(first.Results) == francetravailPageSize; start += francetravailPageSize {
		off := min(start, lastStart)
		p, found, err := w.page(ctx, from, to, off)
		if err != nil {
			return 0, err
		}
		if !found {
			break
		}
		w.keep(p.Results)
		if len(p.Results) < francetravailPageSize || off == lastStart {
			break
		}
	}
	return total, nil
}

// reachesWindowEnd reports, for a page that states no total, whether the window's last page is
// full — the only sign left that the window holds more than the API will show.
func (w *francetravailWalk) reachesWindowEnd(ctx context.Context, from, to time.Time) bool {
	p, found, err := w.page(ctx, from, to, francetravailWindow-francetravailPageSize)
	return err == nil && found && len(p.Results) == francetravailPageSize
}

func (w *francetravailWalk) keep(offers []francetravailOffer) {
	for _, o := range offers {
		if o.ID == "" || w.seen[o.ID] {
			continue
		}
		w.seen[o.ID] = true
		w.offers = append(w.offers, o)
	}
}

// page reads one search page. found is false when the API answered 204 (nothing matched). A 401
// gets one fresh token and one more try.
func (w *francetravailWalk) page(ctx context.Context, from, to time.Time, start int) (francetravailPage, bool, error) {
	q := url.Values{
		"departement":  {w.dep},
		"origineOffre": {francetravailOrigin},
		"sort":         {"1"},
		"range":        {fmt.Sprintf("%d-%d", start, start+francetravailPageSize-1)},
	}
	if !from.IsZero() {
		q.Set("minCreationDate", from.UTC().Format("2006-01-02T15:04:05Z"))
		q.Set("maxCreationDate", to.UTC().Format("2006-01-02T15:04:05Z"))
	}
	u := francetravailSearchURL + "?" + q.Encode()
	stale := ""
	for attempt := 0; ; attempt++ {
		token, err := w.s.bearer(ctx, stale)
		if err != nil {
			return francetravailPage{}, false, err
		}
		if err := w.s.limiter.Wait(ctx); err != nil {
			return francetravailPage{}, false, err
		}
		var p francetravailPage
		found, err := w.s.http.GetJSONOrNoContent(ctx, u, map[string]string{"Authorization": "Bearer " + token}, &p)
		var se *StatusError
		if attempt == 0 && errors.As(err, &se) && se.Code == http.StatusUnauthorized {
			stale = token
			continue
		}
		if err != nil {
			return francetravailPage{}, false, fmt.Errorf("francetravail: %s search: %w", w.dep, err)
		}
		return p, found, nil
	}
}

// francetravailLabelPrefix is the département prefix of a place-of-work label, "75 - PARIS 15".
var francetravailLabelPrefix = regexp.MustCompile(`^\s*(\d{2,3}|2A|2B)\s+-\s+`)

// francetravailJob maps an offer to a Job. ok is false for one with no title, body or named
// employer — an offer may withhold its employer, and filing those under no name would invent one
// employer collecting every anonymous offer.
func francetravailJob(o francetravailOffer) (Job, bool) {
	title := strings.Join(strings.Fields(o.Title), " ")
	company := strings.Join(strings.Fields(o.Employer.Name), " ")
	body := strings.TrimSpace(o.Description)
	if title == "" || company == "" || body == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:         o.ID,
		URL:                francetravailOfferURL + url.PathEscape(o.ID),
		Title:              title,
		Company:            company,
		Description:        sanitizeHTML(plainTextToHTML(body)),
		Location:           francetravailLabelPrefix.ReplaceAllString(strings.TrimSpace(o.PlaceOfWork.Label), ""),
		Countries:          []string{"fr"},
		IsTechHint:         strings.HasPrefix(strings.ToUpper(strings.TrimSpace(o.ROMECode)), "M18"),
		EmploymentType:     francetravailEmploymentType(o.ContractType, o.WorkingTime),
		ExperienceYearsMin: francetravailExperienceYears(o.Experience, o.ExperienceText),
	}
	if t := parseRFC3339(strings.TrimSpace(o.Created)); t != nil {
		j.PostedAt = NotFuture(t)
	}
	if lo, hi, period, ok := francetravailSalary(o.Salary); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "EUR", period
	}
	return j, true
}

// francetravailEmploymentType maps the contract type: a permanent contract is full or part time
// as the offer's working time says; a fixed-term, temporary-agency (MIS) or seasonal (SAI)
// contract is a contract. Freelance and franchise offers (LIB, FRA) and the rest say nothing our
// vocabulary holds.
func francetravailEmploymentType(contract, workingTime string) string {
	partTime := strings.EqualFold(strings.TrimSpace(workingTime), "Temps partiel")
	switch strings.ToUpper(strings.TrimSpace(contract)) {
	case "CDI":
		if partTime {
			return "part_time"
		}
		return "full_time"
	case "CDD", "MIS", "SAI":
		return "contract"
	}
	return ""
}

var francetravailYears = regexp.MustCompile(`(?i)(\d+)\s*an`)

// francetravailExperienceYears reads a REQUIRED experience ("E") stated in years, "3 An(s)". A
// wished-for one ("S") or a beginner-friendly offer ("D") states no minimum, and months round
// down to nothing.
func francetravailExperienceYears(code, text string) *int {
	if !strings.EqualFold(strings.TrimSpace(code), "E") {
		return nil
	}
	m := francetravailYears.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 || n > 40 {
		return nil
	}
	return &n
}

// francetravailSalaryLabel matches the structured salary's label, "Mensuel de 1800.00 Euros à
// 2000.00 Euros sur 12 mois" or "Annuel de 26400,00 Euros sur 12 mois".
var francetravailSalaryLabel = regexp.MustCompile(`(?i)^\s*(Annuel|Mensuel|Horaire)\s+de\s+([\d\s.,]+?)\s*Euros(?:\s+à\s+([\d\s.,]+?)\s*Euros)?`)

// francetravailSalary reads the offer's structured salary. The service writes it as an object
// whose label states the period, the bounds and the currency in a fixed form, or as an empty
// list when the employer stated none; a free-text "commentaire" is never read.
func francetravailSalary(raw json.RawMessage) (lo, hi int, period string, ok bool) {
	var s struct {
		Label string `json:"libelle"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return 0, 0, "", false
	}
	m := francetravailSalaryLabel.FindStringSubmatch(s.Label)
	if m == nil {
		return 0, 0, "", false
	}
	period = map[string]string{"annuel": "year", "mensuel": "month", "horaire": "hour"}[strings.ToLower(m[1])]
	lo, okLo := francetravailAmount(m[2])
	hi, okHi := lo, okLo
	if m[3] != "" {
		hi, okHi = francetravailAmount(m[3])
	}
	if !okLo || !okHi || lo <= 0 || hi < lo {
		return 0, 0, "", false
	}
	return lo, hi, period, true
}

// francetravailAmount reads "26400,00", "1800.00" or "1 800,50", rounded to whole euros.
func francetravailAmount(s string) (int, bool) {
	s = strings.Join(strings.Fields(s), "")
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int(math.Round(f)), true
}
