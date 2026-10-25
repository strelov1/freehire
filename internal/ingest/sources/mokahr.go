package sources

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// mokahr adapts MokaHR (Moka), a Chinese multi-tenant ATS. A board is one tenant careers SITE,
// "<orgId>/<siteId>" (DeepSeek is high-flyer/140576), on app.mokahr.com — or, with region
// "r1", on hire-r1.mokahr.com, Moka's international deployment, where a tenant of the other
// host is unknown. Every tenant site calls the same keyless JSON listing. On some tenants a row
// carries the whole posting with its HTML body; on others (SHEIN, Tesla, Zhihu — 45 of 81 sites
// checked on 2026-10-25) the row has no body and no job function, and the detail endpoint
// supplies both. So the adapter is a HydratingSource: a body-less row is fetched only when the
// catalogue does not hold it yet.
//
// app.mokahr.com answers that listing ENCRYPTED: {"data": base64 AES-128-CBC ciphertext,
// "necromancer": the 16-byte key}, under an IV every tenant's own page publishes and every
// tenant checked shares. hire-r1 answers the same body as plain JSON. Both are read.
//
// The whole tenant is read: a tenant's job functions (zhineng) are its own vocabulary, and on
// Zhipu 117 of 134 postings sit under "其它" (other), so no function filter could scope the crawl
// to technology. Our title dictionary cannot read Chinese titles either (classify resolved a
// category for 24 of 271 titles on the three mainland tenants checked), so IsTechHint is set from
// the tenant's function where its NAME is unambiguously technical (mokahrTechFunctions).
//
// fullBoardListing: there is no total (jobStats.total is always 0), so the walk reads offsets
// until a short page — the source's natural end — and ANY page failing fails the board; reaching
// mokahrMaxPages without that end fails it too.
type mokahr struct {
	http JSONPoster
}

const (
	mokahrPath       = "/api/outer/ats-apply/website/jobs/v2"
	mokahrDetailPath = "/api/outer/ats-apply/website/job"
	// mokahrJobGone is the detail endpoint's answer for a posting that is closed or unknown.
	mokahrJobGone = 703015
	// mokahrPageSize is the listing's hard ceiling: 51 and above answer code 102.
	mokahrPageSize = 50
	// mokahrMaxPages bounds a tenant at 5,000 postings; reaching it fails the crawl.
	mokahrMaxPages = 100
)

// mokahrIV is the AES IV app.mokahr.com encrypts the listing under: the "aesIv" of each tenant
// page's init-data, identical on every tenant checked.
var mokahrIV = []byte("de7c21ed8d6f50fe")

var mokahrBoardRE = regexp.MustCompile(`^([A-Za-z0-9_-]+)/([1-9][0-9]*)$`)

// mokahrRobotsDisallowed are the tenant sites app.mokahr.com's robots.txt excludes.
var mokahrRobotsDisallowed = map[string]bool{"lingjuninvest/46355": true, "shopee/74378": true}

// mokahrTechFunctions are tenant job-function (zhineng) names that state technical work outright,
// as the tenants checked on 2026-10-15 and 2026-10-25 spell them, whitespace removed. A function
// not listed — "其它" (other), a bare "技术类"/"研发类" (technical, R&D: a pharma, a property or a
// battery tenant files lab and building work there too), a data, product or hardware function, a
// name not yet seen — sets no hint and leaves the posting to the title dictionary. The names are
// one map across tenants, so a name earns a place only if it reads as software or IT wherever it
// appears. Adding a tenant means reading its function names first.
var mokahrTechFunctions = map[string]bool{
	"技术类/Technical": true, "算法类/AI": true, // Moonshot
	"工程研发": true, "算法研究": true, // Zhipu
	"AI核心系统研发": true, "全栈开发/算法": true, "深度学习研究员": true, "运维": true, // DeepSeek
	"Engineering": true, // RedotPay
	"信息技术类":       true, "IT/互联网类": true, "社招-IT": true, "企业IT与软件开发": true, "信息科技/数智化": true,
	"信息化": true, "信息类": true, "数字化建设": true, "信息安全": true, // IT departments
	"软件": true, "软件类": true, "软件开发": true, "软件开发类": true, "SW/软件开发": true,
	"技术研发-软件开发": true, "技术开发类": true, "嵌入式": true, "嵌入式开发类": true, // software
	"算法": true, "算法类": true, "AI人工智能": true, "AI/人工智能": true, // algorithms, AI
	"大前端": true, "服务端": true, "客户端": true, "服务器": true, "C++服务器端开发": true, "U3D客户端": true,
	"程序类": true, "程序技术类": true, "游戏程序类": true, "互联网程序类": true, // engineering, games
	"测试及测试开发": true, "自动化测试/白盒测试": true, "运维及运维开发": true, // QA, operations
	"智能驾驶类": true, "智能智驾类": true, "智能网联类": true, // automotive software
}

// mokahrCities maps the city part of a location's administrative code (its first four digits)
// to the city in English; the row names only a district ("海淀区"), and our location dictionary
// reads no Chinese.
var mokahrCities = map[string]string{
	"1101": "Beijing", "3101": "Shanghai", "1201": "Tianjin", "5001": "Chongqing",
	"3301": "Hangzhou", "4403": "Shenzhen", "4401": "Guangzhou", "5101": "Chengdu",
	"3502": "Xiamen", "3201": "Nanjing", "4201": "Wuhan", "6101": "Xi'an", "3205": "Suzhou",
	"8100": "Hong Kong",
}

// mokahrCountries maps the Chinese country names a location states to alpha-2 codes. Hong
// Kong is stated as country 中国, province 香港, and is placed by its province instead.
var mokahrCountries = map[string]string{
	"中国": "cn", "美国": "us", "新加坡": "sg", "马来西亚": "my", "尼日利亚": "ng",
	"巴基斯坦": "pk", "英国": "gb", "巴西": "br", "日本": "jp", "韩国": "kr",
}

var mokahrSARs = map[string]string{"香港": "hk", "澳门": "mo", "台湾": "tw"}

// NewMokaHR builds the MokaHR adapter over the given HTTP client.
func NewMokaHR(c JSONPoster) Source { return mokahr{http: c} }

func (mokahr) Provider() string { return "mokahr" }

var _ HydratingSource = mokahr{}

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (mokahr) fullBoardListing() {}

type mokahrQuery struct {
	SiteID int    `json:"siteId"`
	OrgID  string `json:"orgId"`
	Locale string `json:"locale"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// mokahrEnvelope is app.mokahr.com's encrypted answer: data is the ciphertext of the body.
type mokahrEnvelope struct {
	Data        json.RawMessage `json:"data"`
	Necromancer string          `json:"necromancer"`
}

type mokahrBody struct {
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

type mokahrDetailQuery struct {
	SiteID int    `json:"siteId"`
	OrgID  string `json:"orgId"`
	JobID  string `json:"jobId"`
	Locale string `json:"locale"`
}

type mokahrJob struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Description string `json:"jobDescription"`
	Commitment  string `json:"commitment"`
	OpenedAt    string `json:"openedAt"`
	Zhineng     *struct {
		Name string `json:"name"`
	} `json:"zhineng"`
	Locations []struct {
		CityID       int64  `json:"cityId"`
		CityName     string `json:"cityName"`
		ProvinceName string `json:"provinceName"`
		Country      string `json:"country"`
	} `json:"locations"`
}

// mokahrSite is a parsed board: the host it lives on and its tenant and site ids.
type mokahrSite struct {
	host, board, org string
	siteID           int
}

func (s mokahr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the whole listing, then maps each row: a row with a body is the posting; a
// body-less row the catalogue already holds is a SeenRefresh; a new body-less row is hydrated.
// A failed detail defers that posting rather than storing it body-less, which once seen would
// never be hydrated again; the listing already proved the board, so it does not fail the crawl
// unless nothing at all came of it.
func (s mokahr) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	site, err := parseMokahrBoard(e)
	if err != nil {
		return nil, err
	}
	rows, err := s.list(ctx, site)
	if err != nil {
		return nil, err
	}
	var failed atomic.Int64
	jobs := fetchDetails(rows, defaultDetailWorkers, func(r mokahrJob) (Job, bool) {
		if strings.TrimSpace(r.Description) != "" {
			return r.toJob(site.host, site.board, e.Company)
		}
		if r.Status != "open" {
			return Job{}, false
		}
		if seen(r.ID) {
			return Job{ExternalID: r.ID, URL: mokahrJobURL(site.host, site.board, r.ID), SeenRefresh: true}, true
		}
		d, err := s.detail(ctx, site, r.ID)
		if err != nil {
			failed.Add(1)
			log.Printf("mokahr: board %q, detail %s failed; deferring: %v", site.board, r.ID, err)
			return Job{}, false
		}
		if d == nil {
			return Job{}, false // closed between the listing and the detail
		}
		return d.toJob(site.host, site.board, e.Company)
	})
	// A crawl that yields nothing because every detail failed did not read the board; returning
	// an empty board would record it healthy.
	if len(jobs) == 0 && failed.Load() > 0 {
		return nil, fmt.Errorf("mokahr: board %q: all %d detail requests failed", site.board, failed.Load())
	}
	return jobs, nil
}

func parseMokahrBoard(e CompanyEntry) (mokahrSite, error) {
	m := mokahrBoardRE.FindStringSubmatch(strings.TrimSpace(e.Board))
	if m == nil {
		return mokahrSite{}, fmt.Errorf("mokahr: board %q is not <orgId>/<siteId>", e.Board)
	}
	if mokahrRobotsDisallowed[m[0]] {
		return mokahrSite{}, fmt.Errorf("mokahr: board %q is disallowed by robots.txt", m[0])
	}
	site := mokahrSite{host: "app.mokahr.com", board: m[0], org: m[1]}
	switch e.Region {
	case "":
	case "r1":
		site.host = "hire-r1.mokahr.com"
	default:
		return mokahrSite{}, fmt.Errorf("mokahr: unknown region %q", e.Region)
	}
	site.siteID, _ = strconv.Atoi(m[2])
	return site, nil
}

// list walks the listing to its natural end and returns every distinct row.
func (s mokahr) list(ctx context.Context, site mokahrSite) ([]mokahrJob, error) {
	var (
		rows []mokahrJob
		seen = map[string]bool{}
	)
	for page := 0; ; page++ {
		if page == mokahrMaxPages {
			return nil, fmt.Errorf("mokahr: board %q: %d pages without reaching the end", site.board, mokahrMaxPages)
		}
		var data struct {
			Jobs []mokahrJob `json:"jobs"`
		}
		err := s.call(ctx, site.host, mokahrPath, mokahrQuery{SiteID: site.siteID, OrgID: site.org, Locale: "zh-CN",
			Limit: mokahrPageSize, Offset: page * mokahrPageSize}, &data)
		if err != nil {
			return nil, fmt.Errorf("mokahr: board %q, offset %d: %w", site.board, page*mokahrPageSize, err)
		}
		for _, r := range data.Jobs {
			if r.ID == "" || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			rows = append(rows, r)
		}
		// The raw page length, not the kept count, decides the end: a dropped row must not
		// read as the last page.
		if len(data.Jobs) < mokahrPageSize {
			break
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("mokahr: board %q lists no postings", site.board)
	}
	return rows, nil
}

// detail reads one posting. A nil posting with no error is the platform saying it is gone.
func (s mokahr) detail(ctx context.Context, site mokahrSite, id string) (*mokahrJob, error) {
	var j mokahrJob
	err := s.call(ctx, site.host, mokahrDetailPath, mokahrDetailQuery{SiteID: site.siteID, OrgID: site.org, JobID: id, Locale: "zh-CN"}, &j)
	var code mokahrCodeError
	if errors.As(err, &code) && code.code == mokahrJobGone {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if j.ID != id {
		return nil, fmt.Errorf("answered posting %q", j.ID)
	}
	return &j, nil
}

func mokahrJobURL(host, board, id string) string {
	return "https://" + host + "/social-recruitment/" + board + "#/job/" + id
}

// mokahrCodeError is an unsuccessful answer: every MokaHR outcome is HTTP 200, told apart by code.
type mokahrCodeError struct {
	code int
	msg  string
}

func (e mokahrCodeError) Error() string { return fmt.Sprintf("code %d %s", e.code, e.msg) }

// call POSTs q to one of the tenant endpoints and decodes the successful answer's data into v.
func (s mokahr) call(ctx context.Context, host, path string, q, v any) error {
	var raw json.RawMessage
	if err := s.http.PostJSON(ctx, "https://"+host+path, q, &raw); err != nil {
		return err
	}
	var env mokahrEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	// A string "data" is the ciphertext of the whole body; otherwise the answer IS the body.
	plain := []byte(raw)
	if len(env.Data) > 0 && env.Data[0] == '"' {
		var err error
		if plain, err = mokahrDecrypt(env); err != nil {
			return err
		}
	}
	var body mokahrBody
	if err := json.Unmarshal(plain, &body); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if !body.Success || len(body.Data) == 0 || string(body.Data) == "null" {
		return mokahrCodeError{code: body.Code, msg: body.Msg}
	}
	if err := json.Unmarshal(body.Data, v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// mokahrDecrypt opens app.mokahr.com's encrypted listing: AES-128-CBC under the response's own
// key and the shared IV, PKCS#7-padded.
func mokahrDecrypt(env mokahrEnvelope) ([]byte, error) {
	var b64 string
	if err := json.Unmarshal(env.Data, &b64); err != nil {
		return nil, fmt.Errorf("ciphertext: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("ciphertext: %w", err)
	}
	block, err := aes.NewCipher([]byte(env.Necromancer))
	if err != nil {
		return nil, fmt.Errorf("key: %w", err)
	}
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is %d bytes, not whole blocks", len(ct))
	}
	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, mokahrIV).CryptBlocks(plain, ct)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(plain[len(plain)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, fmt.Errorf("bad padding: the IV or key no longer matches")
	}
	return plain[:len(plain)-pad], nil
}

// mokahrChina is the zone a tenant's zone-less timestamps are written in.
var mokahrChina = time.FixedZone("CST", 8*3600)

func (r mokahrJob) toJob(host, board, company string) (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	// A paused posting is listed but takes no applications.
	if r.Status != "open" || title == "" || company == "" || strings.TrimSpace(r.Description) == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:  r.ID,
		URL:         mokahrJobURL(host, board, r.ID),
		Title:       title,
		Company:     company,
		Description: sanitizeHTML(r.Description),
		IsTechHint:  r.Zhineng != nil && mokahrTechFunctions[strings.Join(strings.Fields(r.Zhineng.Name), "")],
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", r.OpenedAt, mokahrChina); err == nil {
		job.PostedAt = &t
	}
	switch r.Commitment {
	case "全职":
		job.EmploymentType = "full_time"
	case "兼职":
		job.EmploymentType = "part_time"
	case "实习":
		job.EmploymentType, job.Seniority = "internship", "intern"
	}
	var places []string
	seen := map[string]bool{}
	for _, l := range r.Locations {
		code := mokahrSARs[l.ProvinceName]
		if code == "" {
			code = mokahrCountries[l.Country]
		}
		if code != "" && !seen[code] {
			seen[code] = true
			job.Countries = append(job.Countries, code)
		}
		city := ""
		if code := strconv.FormatInt(l.CityID, 10); len(code) == 6 {
			city = mokahrCities[code[:4]]
		}
		if city == "" {
			city = joinNonEmpty(l.CityName, l.ProvinceName)
		}
		places = append(places, city)
	}
	job.Location = distinctJoin(places, "; ", strings.TrimSpace)
	return job, true
}
