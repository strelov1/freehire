package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// infojobsBR adapts InfoJobs Brasil (www.infojobs.com.br), a general Brazilian job board. It is
// not infojobs.net: the Spanish site is the `infojobs` adapter, a different catalogue on a
// different stack, so this one is keyed `infojobsbr`. See the "InfoJobs Brasil traps" section
// of AGENTS.md for the measurements behind each rule.
//
// A board is a federative unit ("sp", "ac", ...) or "todo-brasil", the postings that name no
// place. There is no sitemap; the crawl walks the site's own state listings, cutting a listing
// the site would truncate into category and filter slices, and hydrates each new posting from
// its page. It is an aggregator — the employer is the company each card names — and much of
// it is the employers' own Pandapé career sites republished, which the coverage gate drops.
type infojobsBR struct {
	http           infojobsBRHTTP
	limiter        waiter
	maxNewPerBoard int
}

// infojobsBRHTTP is the transport: listing and posting pages, and the JSON pager. It is a
// cookie-jar client in production — the pager answers page 1 again to a request that carries
// no session cookie.
type infojobsBRHTTP interface {
	HTMLGetter
	JSONGetter
}

const (
	infojobsBRBase = "https://www.infojobs.com.br"
	// infojobsBRPager is the "load more" endpoint the listing's own script calls with the
	// listing URL plus page=N.
	infojobsBRPager = infojobsBRBase + "/mf-publicarea/VacancyList/GetVacancyListFragment?url="
	// infojobsBRNationalBoard holds the postings whose place is "Todo Brasil".
	infojobsBRNationalBoard = "todo-brasil"
	// infojobsBRNationalListing is the listing the national board is read from. Every state
	// listing pads its own postings with the same ~1.9k others (~930 from other states and ~840
	// "Todo Brasil"); Acre's is the shortest walk to them (54 of its own ahead of them on
	// 2026-10-24).
	infojobsBRNationalListing = "acre"
	// infojobsBRHiddenEmployer is the card's label for an employer that chose not to be named.
	infojobsBRHiddenEmployer = "Empresa confidencial"
)

// infojobsBRSliceMax is the largest listing walked whole. A listing stops serving new results
// past ~10,000 (page ~490 answers the same page over and over); a listing stating more than this
// is cut into slices instead. The stated count includes the ~1.9k padding appended to every
// state listing, so the margin is deliberate.
const infojobsBRSliceMax = 9000

// infojobsBRMaxPages is the walk's safety ceiling, 20 cards a page. A slice under
// infojobsBRSliceMax ends well before it; reaching it fails the board.
const infojobsBRMaxPages = 480

// infojobsBRCuts are the listing filters a too-large slice is cut by, in order: the category
// (a path), then level, contract and hours (query parameters). Each slice's own page lists the
// options it can be cut into, so the cut reads them rather than restating them.
var infojobsBRCuts = []string{"categoria", "im", "tipocontrato", "wo"}

// infojobsBRRequestInterval paces every request of a run through one limiter. Measured
// 2026-10-24 from the fsn host: two walkers at ~3.2 requests a second for ~1,000 listing
// requests drew no refusal, and neither did a full listing pass at this rate (~6k requests). The origin is the same Azure stack as Pandapé, which answers a
// burst with 403 (~6-7 a second), so this is the rate observed to pass, not a limit we know.
const infojobsBRRequestInterval = 333 * time.Millisecond

// infojobsBRSliceWorkers walks a board's slices in parallel: one walk is sequential and a page
// takes ~0.6 s, so a single walker cannot reach the limiter's rate.
const infojobsBRSliceWorkers = 3

// infojobsBRMaxNewPerBoard bounds how many unseen postings one board's crawl hydrates. A full
// listing pass of all 28 boards, eight at a time, read 115,140 postings in 36.6 minutes from the
// fsn host (2026-10-24; São Paulo alone 60,139 in 26 minutes). 80 new postings on each board is
// at most ~2.2k detail requests (~12 minutes at the run's pace) and ~5 minutes of writes at the
// ~7 a second a new posting measured from the crawl host, so a run needs a 4500 s budget, not
// the default 3000 s. The rest wait for later runs.
const infojobsBRMaxNewPerBoard = 80

// infojobsBRStates maps a board to the state's listing slug. Rio de Janeiro's slug drops "de";
// "rio-de-janeiro" answers São Paulo's listing.
var infojobsBRStates = map[string]string{
	"ac": "acre", "al": "alagoas", "ap": "amapa", "am": "amazonas", "ba": "bahia",
	"ce": "ceara", "df": "distrito-federal", "es": "espirito-santo", "go": "goias",
	"ma": "maranhao", "mt": "mato-grosso", "ms": "mato-grosso-do-sul", "mg": "minas-gerais",
	"pa": "para", "pb": "paraiba", "pr": "parana", "pe": "pernambuco", "pi": "piaui",
	"rj": "rio-janeiro", "rn": "rio-grande-do-norte", "rs": "rio-grande-do-sul",
	"ro": "rondonia", "rr": "roraima", "sc": "santa-catarina", "sp": "sao-paulo",
	"se": "sergipe", "to": "tocantins",
}

// infojobsBRZone is the clock datePosted is written in: Brasília time, UTC-3 all year.
var infojobsBRZone = time.FixedZone("BRT", -3*3600)

// NewInfoJobsBR builds the adapter over a cookie-jar client (see cookieSessionSource).
func NewInfoJobsBR(c infojobsBRHTTP) Source {
	return infojobsBR{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(infojobsBRRequestInterval), 1),
		maxNewPerBoard: infojobsBRMaxNewPerBoard,
	}
}

func (infojobsBR) Provider() string { return "infojobsbr" }

// Thousands of employers post here, each named per card.
func (infojobsBR) aggregator() {}

// infojobsBRCard is one listing card: everything the listing states about a posting.
type infojobsBRCard struct {
	ID, Path, Title, Company, Location, Salary, WorkMode string
}

// state is the card's federative unit ("AC" from "Rio Branco - AC"), "" for "Todo Brasil".
func (c infojobsBRCard) state() string {
	i := strings.LastIndex(c.Location, " - ")
	if i < 0 {
		return ""
	}
	return c.Location[i+3:]
}

// keeps reports whether a card belongs to the board: its own state's, or the national one's.
func infojobsBRKeeps(board string, c infojobsBRCard) bool {
	if board == infojobsBRNationalBoard {
		return c.Location == "Todo Brasil"
	}
	return strings.EqualFold(c.state(), board)
}

func (s infojobsBR) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s infojobsBR) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists the board, refreshes the postings already stored and hydrates up to
// maxNewPerBoard of the rest — skipping the body of a posting the pipeline is going to drop
// anyway (its company already covered by a first-party source, or its title out of the
// catalogue), which is still yielded, body-less, for the pipeline to count.
func (s infojobsBR) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if _, ok := infojobsBRStates[board]; !ok && board != infojobsBRNationalBoard {
		return nil, fmt.Errorf("infojobsbr: board %q is not a state code or %q", e.Board, infojobsBRNationalBoard)
	}
	cards, err := s.list(ctx, board)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		names := make([]string, 0, len(cards))
		for _, c := range cards {
			names = append(names, c.Company)
		}
		covered = gate.Covered(names)
	}
	var refreshed, skipped []Job
	var unseen []infojobsBRCard
	for _, c := range cards {
		base := Job{ExternalID: c.ID, URL: infojobsBRBase + c.Path, Title: c.Title, Company: c.Company}
		switch {
		case seen(c.ID):
			base.SeenRefresh = true
			refreshed = append(refreshed, base)
		case covered[c.Company] || (gate.Rejected != nil && gate.Rejected(base)):
			skipped = append(skipped, base)
		case len(unseen) < s.maxNewPerBoard:
			unseen = append(unseen, c)
		}
	}
	var read, gone atomic.Int64
	jobs := fetchDetails(unseen, defaultDetailWorkers, func(c infojobsBRCard) (Job, bool) {
		link := infojobsBRBase + c.Path
		if err := s.limiter.Wait(ctx); err != nil {
			return Job{}, false
		}
		root, err := s.http.GetHTML(ctx, link)
		if err != nil {
			log.Printf("infojobsbr: %s: %v; deferring", link, err)
			return Job{}, false
		}
		// A posting closed since the listing was read answers 200 with "Esta vaga não está mais
		// disponível" — the site's own answer that it is gone, so it is not stored. It was never
		// stored before either (it is unseen), so there is nothing for a close to reach.
		if firstByClass(root, "js_VacancyNotFound") != nil {
			gone.Add(1)
			return Job{}, false
		}
		job, ok := infojobsBRJob(root, c, link)
		if !ok {
			log.Printf("infojobsbr: %s is not a posting page; deferring", link)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A page that answered "gone" is an answer, so only a crawl that got NO answer fails.
	if len(unseen) > 0 && read.Load() == 0 && gone.Load() == 0 {
		return nil, fmt.Errorf("infojobsbr: listed %d new postings and read none of their pages", len(unseen))
	}
	return append(append(jobs, refreshed...), skipped...), nil
}

// list reads every card that belongs to the board, from the board's listing cut into slices
// small enough for the site to serve whole. A slice failing fails the board.
func (s infojobsBR) list(ctx context.Context, board string) ([]infojobsBRCard, error) {
	slug := infojobsBRNationalListing
	if board != infojobsBRNationalBoard {
		slug = infojobsBRStates[board]
	}
	parts, err := s.slices(ctx, infojobsBRBase+"/empregos-em-"+slug+".aspx", 0)
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		cards []infojobsBRCard
		ids   = map[string]bool{}
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(infojobsBRSliceWorkers)
	for _, sl := range parts {
		g.Go(func() error {
			got, err := s.walk(gctx, board, sl)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			for _, c := range got {
				if !ids[c.ID] {
					ids[c.ID] = true
					cards = append(cards, c)
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return cards, nil
}

// infojobsBRSlice is one listing small enough to walk whole, with its first page in hand.
type infojobsBRSlice struct {
	url   string
	first *xhtml.Node
}

// slices fetches a listing's first page and returns it as one slice when the site serves it
// whole, or cuts it by the next filter in infojobsBRCuts and recurses.
func (s infojobsBR) slices(ctx context.Context, listing string, cut int) ([]infojobsBRSlice, error) {
	root, err := s.get(ctx, listing)
	if err != nil {
		return nil, fmt.Errorf("infojobsbr: listing %s: %w", listing, err)
	}
	total, ok := infojobsBRTotal(root)
	if !ok {
		return nil, fmt.Errorf("infojobsbr: listing %s is not a listing page", listing)
	}
	if total == 0 {
		return nil, nil
	}
	if total <= infojobsBRSliceMax {
		return []infojobsBRSlice{{url: listing, first: root}}, nil
	}
	if cut >= len(infojobsBRCuts) {
		return nil, fmt.Errorf("infojobsbr: listing %s holds %d postings and no filter is left to cut it by", listing, total)
	}
	children := infojobsBRFilterURLs(root, infojobsBRCuts[cut], listing)
	if len(children) == 0 {
		return s.slices(ctx, listing, cut+1)
	}
	var out []infojobsBRSlice
	for _, child := range children {
		got, err := s.slices(ctx, child, cut+1)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	return out, nil
}

// walk pages a slice through the site's own pager. It ends on eof; on a state board it also
// ends at the first page carrying none of the state's postings, since a state listing serves
// its own postings first and the national ones after. The first page failing fails the slice
// (it was already read); a later page failing fails it too, because nothing says how much of
// the slice is left.
func (s infojobsBR) walk(ctx context.Context, board string, sl infojobsBRSlice) ([]infojobsBRCard, error) {
	var kept []infojobsBRCard
	keep := func(cards []infojobsBRCard) int {
		n := 0
		for _, c := range cards {
			if infojobsBRKeeps(board, c) {
				kept = append(kept, c)
				n++
			}
		}
		return n
	}
	keep(parseInfojobsBRCards(sl.first))
	for page := 2; ; page++ {
		if page > infojobsBRMaxPages {
			return nil, fmt.Errorf("infojobsbr: %s still paging after %d pages", sl.url, infojobsBRMaxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var resp struct {
			EOF  bool   `json:"eof"`
			HTML string `json:"listFragmentHTML"`
		}
		pageURL := sl.url + infojobsBRPageSep(sl.url) + "page=" + strconv.Itoa(page)
		if err := s.http.GetJSON(ctx, infojobsBRPager+url.QueryEscape(pageURL), &resp); err != nil {
			return nil, fmt.Errorf("infojobsbr: %s page %d: %w", sl.url, page, err)
		}
		root, err := xhtml.Parse(strings.NewReader(resp.HTML))
		if err != nil {
			return nil, fmt.Errorf("infojobsbr: %s page %d: %w", sl.url, page, err)
		}
		n := keep(parseInfojobsBRCards(root))
		if resp.EOF || (board != infojobsBRNationalBoard && n == 0) {
			return kept, nil
		}
	}
}

func infojobsBRPageSep(u string) string {
	if strings.Contains(u, "?") {
		return "&"
	}
	return "?"
}

// get fetches a page through the run's limiter.
func (s infojobsBR) get(ctx context.Context, u string) (*xhtml.Node, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return s.http.GetHTML(ctx, u)
}

// infojobsBRCountPattern reads the listing's count from its meta description ("1.969 Vagas de
// Emprego em Acre. ..."). The counter beside the heading is a different, smaller figure. A
// listing with no result states no number at all ("Vagas de Emprego de Comercial, Vendas em São
// Paulo. ...") and shows recommendations instead, which are not its postings.
var infojobsBRCountPattern = regexp.MustCompile(`^([\d.]+) Vagas? de Emprego`)

// infojobsBRTotal returns the listing's count, 0 for a listing with no result, and ok=false for
// a page that carries no meta description — not a listing at all.
func infojobsBRTotal(root *xhtml.Node) (total int, ok bool) {
	walk(root, func(n *xhtml.Node) bool {
		if ok || n.Type != xhtml.ElementNode || n.Data != "meta" || Attr(n, "name") != "description" {
			return !ok
		}
		ok = true
		if m := infojobsBRCountPattern.FindStringSubmatch(Attr(n, "content")); m != nil {
			total, _ = strconv.Atoi(strings.ReplaceAll(m[1], ".", ""))
		}
		return false
	})
	return total, ok
}

// infojobsBRFilterURLs lists the slices a listing's filter of the given key cuts it into: each
// option's data-url, resolved against the listing.
func infojobsBRFilterURLs(root *xhtml.Node, key, listing string) []string {
	base, err := url.Parse(listing)
	if err != nil {
		return nil
	}
	var out []string
	walk(root, func(n *xhtml.Node) bool {
		if n.Type == xhtml.ElementNode && n.Data == "input" && Attr(n, "data-key") == key {
			if ref, err := url.Parse(Attr(n, "data-url")); err == nil && Attr(n, "data-url") != "" {
				out = append(out, base.ResolveReference(ref).String())
			}
		}
		return true
	})
	return out
}

// parseInfojobsBRCards reads every card on a listing page or pager fragment. A card renders its
// facts as an icon followed by text; the icon names the fact. A posting can appear twice on a
// page (a "similar vacancies" group repeats its head), which the caller's id set absorbs.
func parseInfojobsBRCards(root *xhtml.Node) []infojobsBRCard {
	var out []infojobsBRCard
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "div" || !HasClass(n, "js_vacancyLoad") {
			return true
		}
		c := infojobsBRCard{ID: Attr(n, "data-id"), Path: Attr(n, "data-href")}
		if t := firstByClass(n, "js_vacancyTitle"); t != nil {
			c.Title = strings.Join(strings.Fields(textContent(t)), " ")
		}
		walk(n, func(d *xhtml.Node) bool {
			if d.Type != xhtml.ElementNode || d.Data != "div" {
				return true
			}
			switch {
			case c.Company == "" && HasClass(d, "text-body") && !HasClass(d, "js_vacancyTitle"):
				c.Company = strings.Join(strings.Fields(textContent(d)), " ")
			case c.Location == "" && HasClass(d, "mb-8") && d.FirstChild != nil && d.FirstChild.Type == xhtml.TextNode:
				// The first text node only: a hidden span after it carries the distance.
				c.Location = strings.Join(strings.Fields(d.FirstChild.Data), " ")
			}
			return true
		})
		walk(n, func(i *xhtml.Node) bool {
			if i.Type != xhtml.ElementNode || i.Data != "svg" || i.Parent == nil {
				return true
			}
			text := strings.Join(strings.Fields(textContent(i.Parent)), " ")
			switch {
			case HasClass(i, "icon-money"):
				c.Salary = text
			case HasClass(i, "icon-buildings"), HasClass(i, "icon-house-and-building"), HasClass(i, "icon-user-home"):
				c.WorkMode = text
			}
			return false
		})
		if c.ID != "" && strings.HasPrefix(c.Path, "/") && c.Title != "" && c.Company != "" &&
			!strings.EqualFold(c.Company, infojobsBRHiddenEmployer) {
			out = append(out, c)
		}
		return false
	})
	return out
}

// infojobsBRLD is the part of the posting page's schema.org JobPosting the adapter reads. A
// posting that names no place ("Todo Brasil") carries none, so it is optional.
type infojobsBRLD struct {
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// infojobsBRJob maps a posting page plus its card. ok is false when the page is not a live
// posting: a closed one answers 200 with "Esta vaga não está mais disponível" and no header.
func infojobsBRJob(root *xhtml.Node, c infojobsBRCard, link string) (Job, bool) {
	if firstByClass(root, "js_vacancyHeaderTitle") == nil {
		return Job{}, false
	}
	body := infojobsBRBody(root)
	if body == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:  c.ID,
		URL:         link,
		Title:       c.Title,
		Company:     c.Company,
		Location:    c.Location,
		Description: body,
	}
	var ld infojobsBRLD
	if ldJobPosting(root, &ld) {
		j.PostedAt = infojobsBRPostedAt(ld.DatePosted)
		// The card shouts the name in capitals; the JobPosting spells it as the employer does.
		if name := strings.TrimSpace(ld.HiringOrganization.Name); name != "" {
			j.Company = name
		}
		if len(ld.JobLocation) > 0 {
			a := ld.JobLocation[0].Address
			// UF codes are ISO country codes ("RS" is Serbia), so the country is spelled out.
			if a.AddressLocality != "" {
				j.Location = joinNonEmpty(a.AddressLocality, a.AddressRegion, "Brasil")
			}
			j.Countries = countryFromCode(a.AddressCountry)
		}
	}
	if j.Countries == nil {
		j.FallbackCountries = []string{"br"}
	}
	switch c.WorkMode {
	case "Presencial":
		j.WorkMode = "onsite"
	case "Híbrido":
		j.WorkMode = "hybrid"
	case "Home office":
		j.WorkMode, j.Remote = "remote", true
	}
	j.EmploymentType = infojobsBREmploymentType(infojobsBRField(root, "Tipo de contrato e Jornada:"))
	if lo, hi, ok := infojobsBRSalary(c.Salary); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "BRL", "month"
	}
	return j, true
}

// infojobsBRBody renders the posting's text and the panel after it: the facts ("Número de
// vagas", "Tipo de contrato e Jornada", "Área Profissional") and the "Exigências",
// "Valorizado" and "Habilidades" lists, up to the panel's report form. The text is a
// white-space: pre-line paragraph, so its line breaks are made explicit. The premium-account
// banner in the middle is skipped.
func infojobsBRBody(root *xhtml.Node) string {
	var text *xhtml.Node
	walk(root, func(n *xhtml.Node) bool {
		if text == nil && n.Type == xhtml.ElementNode && n.Data == "p" && HasClass(n, "white-space-pre-line") {
			text = n
		}
		return text == nil
	})
	if text == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(textContent(text))), "\n", "<br>") + "</p>")
	for n := text.NextSibling; n != nil; n = n.NextSibling {
		if n.Type != xhtml.ElementNode || n.Data == "a" {
			continue
		}
		if n.Data == "form" {
			break
		}
		if HasClass(n, "h4") {
			b.WriteString("<h3>" + html.EscapeString(strings.Join(strings.Fields(textContent(n)), " ")) + "</h3>")
			continue
		}
		_ = xhtml.Render(&b, n)
	}
	return sanitizeHTML(b.String())
}

// infojobsBRField reads a labelled fact of the posting page: <p><span>label</span> value</p>.
func infojobsBRField(root *xhtml.Node, label string) string {
	var value string
	walk(root, func(n *xhtml.Node) bool {
		if value != "" || n.Type != xhtml.ElementNode || n.Data != "p" || n.FirstChild == nil {
			return value == ""
		}
		span := n.FirstChild
		for span != nil && span.Type != xhtml.ElementNode {
			span = span.NextSibling
		}
		if span != nil && span.Data == "span" && strings.TrimSpace(textContent(span)) == label {
			value = strings.TrimSpace(strings.TrimPrefix(strings.Join(strings.Fields(textContent(n)), " "), label))
		}
		return false
	})
	return value
}

// infojobsBREmploymentType maps "Tipo de contrato e Jornada" ("Efetivo – CLT - Período
// Integral"): the contract regime says internship or contract, else the hours say full or part
// time. Jovem Aprendiz, Trainee and "Outros" name no type of their own.
func infojobsBREmploymentType(v string) string {
	contract, hours, _ := strings.Cut(v, " - ")
	switch strings.TrimSpace(contract) {
	case "Estágio":
		return "internship"
	case "Prestador de Serviços (PJ)", "Autônomo", "Temporário", "Cooperado":
		return "contract"
	case "Efetivo – CLT":
		switch h := strings.TrimSpace(hours); {
		case h == "Período Integral":
			return "full_time"
		case strings.HasPrefix(h, "Parcial"):
			return "part_time"
		}
	}
	return ""
}

var infojobsBRMoney = regexp.MustCompile(`R\$ ?([\d.]+),\d\d`)

// infojobsBRMonthlyFloor separates a monthly wage from anything else. The card states an amount
// and no period; Brazilian wages are quoted monthly, and the figures on 300 sampled São Paulo
// cards (2026-10-24) cluster at R$ 1.6k-3k. Below the floor nothing is published.
const infojobsBRMonthlyFloor = 1000

// infojobsBRSalary parses the card's "R$ 1.625,00 a R$ 2.000,00" or "R$ 2.120,00"; "A combinar"
// (to be agreed) is no salary.
func infojobsBRSalary(s string) (lo, hi int, ok bool) {
	m := infojobsBRMoney.FindAllStringSubmatch(s, -1)
	if len(m) == 0 || len(m) > 2 {
		return 0, 0, false
	}
	lo, err := strconv.Atoi(strings.ReplaceAll(m[0][1], ".", ""))
	if err != nil {
		return 0, 0, false
	}
	hi, err = strconv.Atoi(strings.ReplaceAll(m[len(m)-1][1], ".", ""))
	if err != nil || lo < infojobsBRMonthlyFloor || hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}

// infojobsBRPostedAt parses datePosted ("2026-10-23T16:56:00.0000000", no offset).
func infojobsBRPostedAt(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04:05.9999999", s, infojobsBRZone)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}
