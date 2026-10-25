package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// engage adapts en-gage.net, エン・ジャパン's free ATS, through the cross-tenant job search its own
// site runs on (/user/api/search/result_work_list/). Board-based aggregator: a board is one of the
// search's OCCUPATION CATEGORY codes (the `job` parameter, e.g. 400000 for IT), so one board
// reaches every tenant's postings in that category — not one tenant's careers page, which caps
// each employment type at 100 postings and would need a roster of tenant slugs to reach anyone.
//
// A search row carries the whole posting — the body's sections, salary, address, employment
// type — so the crawl makes no detail requests. See the engage traps in AGENTS.md.
type engage struct{ http JSONGetter }

const (
	engageBase = "https://en-gage.net"
	// engageMaxPages is a safety ceiling, not the platform's: 400000 held 17,152 postings (286
	// pages of 60) on 2026-10-25. A walk that reaches it is logged.
	engageMaxPages = 1000
	// engageRequestInterval paces the walk to one page a second; a page answers in ~3 s anyway.
	engageRequestInterval = time.Second
	engageRequestBurst    = 1
)

// engageBoardRE is a six-digit occupation category code.
var engageBoardRE = regexp.MustCompile(`^[1-9][0-9]{5}$`)

// engageTechBoards are the categories whose postings are technical by the site's own filing.
// 350000 (creative and web) mixes UI engineers with video and print staff, so it is left to the
// title dictionary.
var engageTechBoards = map[string]bool{"400000": true}

// NewEngage builds the engage adapter over the given HTTP client.
func NewEngage(c JSONGetter) Source { return engage{http: c} }

// pacedEngageGetter holds every request of a run to one limiter.
func pacedEngageGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(engageRequestInterval), engageRequestBurst),
	}
}

func (engage) Provider() string { return "engage" }

// Every row names its own employer, so the source is an aggregator over its tenants.
func (engage) aggregator() {}

// engageRow is one search row, reduced to what the adapter reads. Rows come in two shapes that
// disagree on scalar types (a number in one is a string in the other), so every scalar the
// adapter reads tolerates both.
type engageRow struct {
	WorkID      json.RawMessage `json:"work_id"`
	IsKBWork    flexjson.Bool   `json:"is_kb_work"`
	Slug        string          `json:"company_url_root_dir"`
	CompanyName string          `json:"company_name"`
	Official    string          `json:"official_corporate_name"`
	Occupation  string          `json:"official_occupation_name"`
	Catch       string          `json:"occupation_name"`
	Employment  string          `json:"employment_status"`
	Published   string          `json:"publish_start_datetime"`
	SalaryType  string          `json:"salary_type"`
	SalaryFrom  flexjson.Int64  `json:"salary_amount_from"`
	SalaryTo    flexjson.Int64  `json:"salary_amount_to"`
	Address     string          `json:"address_1"`
	Offices     string          `json:"work_office_addresses"`
	RemoteAddr  string          `json:"full_remote_office_address"`
	ForeignAddr string          `json:"foreign_address"`

	Contents      string `json:"work_contents"`
	Qualification string `json:"qualification"`
	Background    string `json:"recruitment_bg"`
	SalaryText    string `json:"salary_amount"`
	SalaryNote    string `json:"salary_note"`
	WorkLocation  string `json:"work_location"`
	Access        string `json:"access"`
	OfficeHours   string `json:"office_hours"`
	Holiday       string `json:"holiday"`
	Treatment     string `json:"treatment"`
}

type engagePage struct {
	Result       string      `json:"result"`
	ErrorMessage string      `json:"error_message"`
	TotalCount   int         `json:"totalCount"`
	SearchResult []engageRow `json:"searchResult"`
	Paginator    struct {
		HasMorePages bool `json:"hasMorePages"`
	} `json:"paginator"`
}

// engagePageURL builds one search page. Paging is a stitch, not an offset: page 1 needs f_t=0,
// and every later page needs p_t set to the total the previous page reported — without it the
// search answers success with nothing in it.
func engagePageURL(board string, page, prevTotal int) string {
	q := url.Values{}
	q.Set("p_t", strconv.Itoa(prevTotal))
	q.Set("f_t", "0")
	q.Set("page", strconv.Itoa(page))
	q.Set("job", board)
	return engageBase + "/user/api/search/result_work_list/?" + q.Encode()
}

// Fetch walks the category to its end. The first page failing fails the board; a later page
// failing ends the walk with what was read. A page the search itself answers with result
// "error" is a failed page, never an empty one.
func (s engage) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !engageBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("engage: board %q is not an occupation category code", e.Board)
	}
	tech := engageTechBoards[e.Board]
	var jobs []Job
	seen := map[string]bool{}
	total := 0
	for page := 1; page <= engageMaxPages; page++ {
		var p engagePage
		err := s.http.GetJSON(ctx, engagePageURL(e.Board, page, total), &p)
		if err == nil && p.Result != "success" {
			err = fmt.Errorf("search answered %q: %s", p.Result, p.ErrorMessage)
		}
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("engage: category %s: %w", e.Board, err)
			}
			log.Printf("engage: category %s: page %d failed; keeping %d postings read: %v", e.Board, page, len(jobs), err)
			break
		}
		total = p.TotalCount
		for _, r := range p.SearchResult {
			j, ok := engageJob(r, tech)
			if !ok || seen[j.ExternalID] {
				continue
			}
			seen[j.ExternalID] = true
			jobs = append(jobs, j)
		}
		if len(p.SearchResult) == 0 || !p.Paginator.HasMorePages {
			break
		}
		if page == engageMaxPages {
			log.Printf("engage: category %s: stopped at the %d-page ceiling with %d of %d postings read",
				e.Board, engageMaxPages, len(jobs), total)
		}
	}
	return jobs, nil
}

var engageDigitsRE = regexp.MustCompile(`^[0-9]+$`)

// engageWorkID reads a row's posting id. engage's own postings carry a numeric work id; the
// search also mixes in rows syndicated from another job site (is_kb_work), whose id is a hex
// token and whose page is not on en-gage.net, so anything but digits is no posting of ours.
func engageWorkID(raw json.RawMessage) string {
	id := strings.Trim(string(bytes.TrimSpace(raw)), `"`)
	if !engageDigitsRE.MatchString(id) {
		return ""
	}
	return id
}

// engageJob maps one search row to a Job, or reports it is not an engage posting.
func engageJob(r engageRow, tech bool) (Job, bool) {
	id := engageWorkID(r.WorkID)
	slug := strings.TrimSpace(r.Slug)
	company := engageCompany(r)
	title := strings.Join(strings.Fields(norm.NFKC.String(firstNonEmpty(r.Occupation, r.Catch))), " ")
	if bool(r.IsKBWork) || id == "" || slug == "" || company == "" || title == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     id,
		URL:            engageBase + "/" + url.PathEscape(slug) + "/work_" + id + "/",
		Title:          title,
		Company:        company,
		IsTechHint:     tech,
		EmploymentType: engageEmploymentType(r.Employment),
		Description:    engageBody(r),
	}
	switch {
	case strings.TrimSpace(r.RemoteAddr) != "":
		// The site's fully remote postings: address_1 reads フルリモート（完全在宅） and the
		// remote-office field holds the employer's office.
		j.WorkMode, j.Remote = "remote", true
		j.Countries = []string{"jp"}
	case strings.TrimSpace(r.ForeignAddr) != "":
		j.Location = strings.TrimSpace(r.ForeignAddr)
	default:
		j.Location = strings.TrimSpace(firstNonEmpty(r.Address, r.Offices))
		j.Countries = []string{"jp"}
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05.000", strings.TrimSpace(r.Published), engageJST); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	engageSalary(&j, r)
	return j, true
}

var engageJST = time.FixedZone("JST", 9*60*60)

// engageCompany is the employer's registered name, width-normalized: the display name often
// carries the hiring office ("ポールトゥウィン株式会社　仙台センター"), and the registered name is
// sometimes typed in full-width letters ("株式会社Ｗｉｄｓｌｅｙ").
func engageCompany(r engageRow) string {
	return strings.Join(strings.Fields(norm.NFKC.String(firstNonEmpty(r.Official, r.CompanyName))), " ")
}

// engageEmploymentType maps the site's employment labels onto vocab.EmploymentTypeValues. A
// permanent dispatch contract (正社員（無期雇用派遣）) is still permanent full-time employment;
// fixed-term dispatch (派遣社員), freelance (業務委託) and new-graduate hiring (新卒) have no exact
// equivalent and are left to the pipeline.
func engageEmploymentType(label string) string {
	switch l := strings.TrimSpace(label); {
	case strings.HasPrefix(l, "正社員"):
		return "full_time"
	case l == "契約社員":
		return "contract"
	case strings.Contains(l, "アルバイト") || strings.Contains(l, "パート"):
		return "part_time"
	}
	return ""
}

// engageSalary copies the structured salary. salary_amount_from/to are the search's own figures:
// annual yen for a monthly (月給) or annual (年俸, 年収) salary — the monthly amounts sit in their
// own fields — and the hourly or daily wage itself for 時給 and 日給. A 0 ceiling is open-ended.
func engageSalary(j *Job, r engageRow) {
	period := map[string]string{"月給": "year", "年俸": "year", "年収": "year", "時給": "hour", "日給": "day"}[strings.TrimSpace(r.SalaryType)]
	lo, hi := int(r.SalaryFrom), int(r.SalaryTo)
	if period == "" || lo <= 0 {
		return
	}
	j.SalaryMin = &lo
	if hi >= lo {
		j.SalaryMax = &hi
	}
	j.SalaryCurrency = "JPY"
	j.SalaryPeriod = period
}

// engageBody renders the row's posting sections as HTML, under the headings the site's own page
// uses. The fields are plain text with newlines.
func engageBody(r engageRow) string {
	text := func(parts ...string) string {
		var out []string
		for _, p := range parts {
			if p = strings.TrimSpace(strings.ReplaceAll(p, "\r\n", "\n")); p != "" {
				out = append(out, strings.ReplaceAll(html.EscapeString(p), "\n", "<br>"))
			}
		}
		return strings.Join(out, "<br><br>")
	}
	return sanitizeHTML(
		headedSection("仕事内容", text(r.Contents)) +
			headedSection("応募資格", text(r.Qualification)) +
			headedSection("募集背景", text(r.Background)) +
			headedSection("勤務地・交通", text(r.Offices, r.WorkLocation, r.Access)) +
			headedSection("勤務時間", text(r.OfficeHours)) +
			headedSection("給与", text(r.SalaryText, r.SalaryNote)) +
			headedSection("休日休暇", text(r.Holiday)) +
			headedSection("待遇・福利厚生", text(r.Treatment)))
}
