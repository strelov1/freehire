package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"golang.org/x/net/html"
)

const (
	inploiHost      = "www.inploi.com"
	inploiAPI       = "https://api.inploi.com/search/results"
	inploiPageSize  = 100
	inploiMaxTotal  = 50000
	inploiMaxNew    = 400
	inploiKeyHeader = "x-publishable-key"
)

var (
	inploiBoard   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)
	inploiKey     = regexp.MustCompile(`\bpk_[A-Za-z0-9_-]{12,}\b`)
	inploiSegment = regexp.MustCompile(`(?i)\\?"segment_ids\\?"\s*,\s*\\?"segment\\?"\s*,\s*\\?"(\d+)\\?"`)
)

type inploiHTTP interface {
	HTMLResolvedGetter
	HTMLGetter
	HeaderJSONGetter
}

type inploi struct {
	http     inploiHTTP
	pageSize int
	maxNew   int
}

func NewInploi(c inploiHTTP) Source {
	return inploi{http: c, pageSize: inploiPageSize, maxNew: inploiMaxNew}
}

func (inploi) Provider() string { return "inploi" }

func (inploi) fullBoardListing() {}

func (s inploi) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s inploi) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	var fresh []inploiPosting
	var refresh []Job
	for _, p := range postings {
		if seen(p.ID) {
			refresh = append(refresh, Job{
				ExternalID:  p.ID,
				URL:         p.URL,
				Title:       p.Title,
				Company:     p.Company,
				SeenRefresh: true,
			})
			continue
		}
		fresh = append(fresh, p)
	}
	// Keep part of the budget moving through the backlog: rejected or unreadable
	// postings stay unseen and must not consume every crawl's detail requests.
	head := s.maxNew / 2
	fresh = hydrationLanes(fresh, head, s.maxNew-head, time.Now())
	return append(fetchDetails(fresh, defaultDetailWorkers, func(p inploiPosting) (Job, bool) {
		return s.detail(ctx, p)
	}), refresh...), nil
}

func (s inploi) list(ctx context.Context, e CompanyEntry) ([]inploiPosting, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !inploiBoard.MatchString(board) {
		return nil, fmt.Errorf("inploi: board %q is not a company slug", e.Board)
	}
	key, segment, host, err := s.creds(ctx, board)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var postings []inploiPosting
	var total, last int
	for page := 1; ; page++ {
		if page > (inploiMaxTotal+s.pageSize-1)/s.pageSize {
			return nil, fmt.Errorf("inploi: board %s: page %d is past the listing cap", board, page)
		}
		body, err := s.page(ctx, board, key, segment, page)
		if err != nil {
			return nil, err
		}
		rows, pageTotal, pageLast, err := inploiPageRows(body, page, s.pageSize)
		if err != nil {
			return nil, fmt.Errorf("inploi: board %s: %w", board, err)
		}
		if page == 1 {
			total, last = pageTotal, pageLast
			if total > inploiMaxTotal {
				return nil, fmt.Errorf("inploi: board %s: source lists %d postings, past %d", board, total, inploiMaxTotal)
			}
		} else if pageTotal != total || pageLast != last {
			return nil, fmt.Errorf("inploi: board %s: pagination changed during the walk", board)
		}
		for _, item := range rows {
			p, err := inploiPostingFrom(e.Company, host, item)
			if err != nil {
				return nil, fmt.Errorf("inploi: board %s: %w", board, err)
			}
			if _, ok := seen[p.ID]; ok {
				continue
			}
			seen[p.ID] = struct{}{}
			postings = append(postings, p)
		}
		if page >= last {
			break
		}
	}
	if len(postings) != total {
		return nil, fmt.Errorf("inploi: board %s: read %d postings, source listed %d", board, len(postings), total)
	}
	return postings, nil
}

func (s inploi) creds(ctx context.Context, board string) (key, segment, host string, err error) {
	root, final, err := s.http.GetHTMLResolved(ctx, inploiSearchURL(board))
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return "", "", "", fmt.Errorf("inploi: board %q: %w", board, ErrBoardGone)
		}
		return "", "", "", fmt.Errorf("inploi: board %s: %w", board, err)
	}
	host, err = inploiCareerHost(final)
	if err != nil {
		return "", "", "", fmt.Errorf("inploi: board %s: %w", board, err)
	}
	key, segment = inploiCreds(inploiNodeText(root))
	if key != "" && segment != "" {
		return key, segment, host, nil
	}
	if path, err := url.Parse(final); err == nil && !inploiSearchPath(path.Path) {
		root, err = s.http.GetHTML(ctx, "https://"+host+"/search")
		if err != nil {
			return "", "", "", fmt.Errorf("inploi: board %s: %w", board, err)
		}
		key, segment = inploiCreds(inploiNodeText(root))
	}
	if key == "" || segment == "" {
		return "", "", "", fmt.Errorf("inploi: board %s: search page did not name its key and segment", board)
	}
	return key, segment, host, nil
}

func inploiSearchURL(board string) string {
	return "https://" + inploiHost + "/company/" + url.PathEscape(board) + "/search"
}

func inploiSearchPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return strings.HasSuffix(path, "/search")
}

func inploiCareerHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", fmt.Errorf("career page %q is not an https host", raw)
	}
	return u.Hostname(), nil
}

func inploiCreds(text string) (key, segment string) {
	if m := inploiKey.FindString(text); m != "" {
		key = m
	}
	if m := inploiSegment.FindStringSubmatch(text); len(m) == 2 {
		segment = m[1]
	}
	return key, segment
}

func inploiNodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func (s inploi) page(ctx context.Context, board, key, segment string, page int) (inploiEnvelope, error) {
	u := fmt.Sprintf("%s?filters[segment_ids][0]=%s&query=&page=%d&per_page=%d",
		inploiAPI, url.QueryEscape(segment), page, s.pageSize)
	var body inploiEnvelope
	err := s.http.GetJSONWithHeaders(ctx, u, map[string]string{inploiKeyHeader: key}, &body)
	if err != nil {
		return inploiEnvelope{}, fmt.Errorf("inploi: board %s: %w", board, err)
	}
	return body, nil
}

func inploiPageRows(body inploiEnvelope, page, pageSize int) ([]inploiItem, int, int, error) {
	if len(body.Data) == 0 || bytes.Equal(bytes.TrimSpace(body.Data), []byte("null")) {
		return nil, 0, 0, fmt.Errorf("the listing response carries no %q collection", "data")
	}
	if body.Pagination == nil {
		return nil, 0, 0, fmt.Errorf("the listing response carries no pagination")
	}
	var rows []inploiItem
	if err := json.Unmarshal(body.Data, &rows); err != nil {
		return nil, 0, 0, fmt.Errorf("the listing response is not a job list")
	}
	p := body.Pagination
	if p.Total < 0 || p.CurrentPage != page || p.LastPage < 1 {
		return nil, 0, 0, fmt.Errorf("the listing response has invalid pagination")
	}
	expectedLast := max(1, (p.Total+pageSize-1)/pageSize)
	expectedRows := min(pageSize, max(p.Total-(page-1)*pageSize, 0))
	if p.LastPage != expectedLast || len(rows) != expectedRows {
		return nil, 0, 0, fmt.Errorf("read %d rows on page %d, source listed %d of %d", len(rows), page, expectedRows, p.Total)
	}
	return rows, p.Total, p.LastPage, nil
}

func inploiPostingFrom(company, host string, item inploiItem) (inploiPosting, error) {
	id := strings.TrimSpace(item.ID.String())
	title := strings.TrimSpace(item.Title)
	if id == "" || title == "" || id == "0" {
		return inploiPosting{}, fmt.Errorf("posting %q has no id or title", item.ID.String())
	}
	name := strings.TrimSpace(item.CompanyName)
	if name == "" {
		name = company
	}
	p := inploiPosting{
		ID:             id,
		URL:            "https://" + host + "/job/" + url.PathEscape(id),
		Title:          title,
		Company:        name,
		Location:       joinNonEmpty(item.Town, item.City, item.Country),
		EmploymentType: inploiEmployment(item.EmploymentType, item.ContractType),
		WorkMode:       inploiWorkMode(item.LocationType),
		PostedAt:       parseRFC3339(item.CreatedAt),
	}
	if code := location.NormalizeCountry(item.Country); code != "" {
		p.Country = code
	}
	p.SalaryMin, p.SalaryMax, p.SalaryCurrency, p.SalaryPeriod = inploiSalary(item)
	return p, nil
}

func inploiEmployment(kind, contract string) string {
	if et := schemaEmploymentType(kind); et != "" {
		return et
	}
	return schemaEmploymentType(contract)
}

func inploiWorkMode(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "remote", "fully_remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "onsite", "on-site", "office":
		return "onsite"
	default:
		return ""
	}
}

func inploiSalary(item inploiItem) (min, max *int, currency, period string) {
	if item.masked() {
		return nil, nil, "", ""
	}
	period = inploiPayPeriod[strings.ToUpper(strings.TrimSpace(item.PayType))]
	currency = strings.ToUpper(strings.TrimSpace(item.PayCurrency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(item.CurrencyCode))
	}
	if period == "" || len(currency) != 3 {
		return nil, nil, "", ""
	}
	lo, hi := roundSalaryPart(float64(item.PayMin)), roundSalaryPart(float64(item.PayMax))
	if lo == nil && hi == nil {
		lo = roundSalaryPart(float64(item.Pay))
		hi = lo
	}
	if lo == nil && hi == nil {
		return nil, nil, "", ""
	}
	return lo, hi, currency, period
}

var inploiPayPeriod = map[string]string{
	"ANNUALLY": "year",
	"MONTHLY":  "month",
	"DAILY":    "day",
	"HOURLY":   "hour",
}

func (s inploi) detail(ctx context.Context, p inploiPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.ID, p.URL, p.Company), true
		}
		return Job{}, false
	}
	var posting struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(root, &posting) || strings.TrimSpace(posting.Description) == "" {
		return unreadableDetail(p.ID, p.URL, p.Company), true
	}
	body := strings.TrimSpace(posting.Description)
	if !strings.Contains(body, "<") {
		body = plainTextToHTML(body)
	}
	job := Job{
		ExternalID:     p.ID,
		URL:            p.URL,
		Title:          p.Title,
		Company:        p.Company,
		Location:       p.Location,
		Description:    sanitizeHTML(body),
		WorkMode:       p.WorkMode,
		EmploymentType: p.EmploymentType,
		PostedAt:       p.PostedAt,
		SalaryMin:      p.SalaryMin,
		SalaryMax:      p.SalaryMax,
		SalaryCurrency: p.SalaryCurrency,
		SalaryPeriod:   p.SalaryPeriod,
	}
	if p.Country != "" {
		job.Countries = []string{p.Country}
	}
	return job, true
}

type inploiPosting struct {
	ID             string
	URL            string
	Title          string
	Company        string
	Location       string
	EmploymentType string
	WorkMode       string
	PostedAt       *time.Time
	Country        string
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

type inploiEnvelope struct {
	Data       json.RawMessage `json:"data"`
	Pagination *struct {
		Total       int `json:"total"`
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"pagination"`
}

type inploiItem struct {
	ID             inploiID     `json:"id"`
	Title          string       `json:"title"`
	CompanyName    string       `json:"company_name"`
	EmploymentType string       `json:"employment_type"`
	ContractType   string       `json:"contract_type"`
	LocationType   string       `json:"location_type"`
	Town           string       `json:"town"`
	City           string       `json:"city"`
	Country        string       `json:"country"`
	CreatedAt      string       `json:"created_at"`
	Pay            inploiAmount `json:"pay"`
	PayMin         inploiAmount `json:"pay_min"`
	PayMax         inploiAmount `json:"pay_max"`
	PayType        string       `json:"pay_type"`
	PayCurrency    string       `json:"pay_currency"`
	CurrencyCode   string       `json:"currency_code"`
	PayDisplay     *bool        `json:"pay_display"`
	PayMask        *bool        `json:"pay_mask"`
}

func (item inploiItem) masked() bool {
	if item.PayMask != nil && *item.PayMask {
		return true
	}
	return item.PayDisplay != nil && !*item.PayDisplay
}

type inploiID string

func (id inploiID) String() string { return string(id) }

func (id *inploiID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*id = inploiID(s)
		return nil
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}
	*id = inploiID(strconv.FormatInt(n, 10))
	return nil
}

type inploiAmount float64

func (a *inploiAmount) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*a = inploiAmount(f)
		return nil
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*a = inploiAmount(f)
	return nil
}
