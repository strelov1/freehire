package collections

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Venture portfolios, read from each firm's own site. Every parser here drops the
// companies the firm records as acquired or shut down: an acquired company now hires
// under its acquirer's name, so the tag would either match nothing or — worse — match
// an unrelated company that kept the name. A company that went public stays: it is
// still the firm's portfolio company and still hires under its own name.
//
// Every record carries the member's website where the firm publishes one, which is
// what lets Members refuse a same-named company running a different site.

// Sanity is the CMS behind the Greylock, Union Square Ventures and Accel sites. Its
// read API is public — no token — and answers a GROQ query with {"result": [...]},
// so each firm is one pinned URL. These are the same reads the firms' own pages make.
const (
	greylockDatasetURL = `https://uz672vkd.apicdn.sanity.io/v2021-10-21/data/query/production?query=*%5B_type%3D%3D%22portfolioCompany%22%5D%7Bname%2Cwebsite%2Cstatus%7D`
	usvDatasetURL      = `https://2r1amp8c.apicdn.sanity.io/v2021-10-21/data/query/production?query=*%5B_type%3D%3D%22company%22%5D%7Bname%2Cwebsite%2Curl%2CcurrentStage%2C%22profileWebsite%22%3AsiteProfile.website%7D`
	accelDatasetURL    = `https://458oembh.apicdn.sanity.io/v2021-10-21/data/query/production?query=*%5B_type%3D%3D%22company%22%5D%7Bname%2CwebsiteUrl%2Carchived%7D`
	bondDatasetURL     = "https://www.bondcap.com/assets/data/investments/index.json"
	a16zPortfolioURL   = "https://a16z.com/portfolio/"
	sequoiaCompanyURL  = "https://www.sequoiacap.com/our-companies/"
)

func record(name, website string) Record {
	return Record{Name: strings.TrimSpace(name), Meta: map[string]string{MetaWebsite: strings.TrimSpace(website)}}
}

func parseSanity[T any](data []byte, source string) ([]T, error) {
	var payload struct {
		Result []T `json:"result"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("collections: parse %s portfolio: %w", source, err)
	}
	return payload.Result, nil
}

// ParseGreylock reads Greylock's portfolio: status is Active, Public or Acquired.
func ParseGreylock(data []byte) ([]Record, error) {
	rows, err := parseSanity[struct {
		Name    string `json:"name"`
		Website string `json:"website"`
		Status  string `json:"status"`
	}](data, "greylock")
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, r := range rows {
		if r.Name == "" || strings.EqualFold(r.Status, "acquired") {
			continue
		}
		out = append(out, record(r.Name, r.Website))
	}
	return out, nil
}

// ParseUSV reads Union Square Ventures' portfolio: currentStage is early, growth,
// public, acquired or inactive. The website sits in one of three fields depending on
// how old the entry is.
func ParseUSV(data []byte) ([]Record, error) {
	rows, err := parseSanity[struct {
		Name           string `json:"name"`
		Website        string `json:"website"`
		URL            string `json:"url"`
		ProfileWebsite string `json:"profileWebsite"`
		Stage          string `json:"currentStage"`
	}](data, "usv")
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, r := range rows {
		if r.Name == "" || r.Stage == "acquired" || r.Stage == "inactive" {
			continue
		}
		out = append(out, record(r.Name, firstNonEmpty(r.Website, r.ProfileWebsite, r.URL)))
	}
	return out, nil
}

// ParseAccel reads Accel's portfolio. Accel marks no exit status the page explains —
// its currentStatus flag splits current from past investments, and a past investment
// may well be a public company still hiring — so only the entries Accel itself
// archived are dropped.
func ParseAccel(data []byte) ([]Record, error) {
	rows, err := parseSanity[struct {
		Name     string `json:"name"`
		Website  string `json:"websiteUrl"`
		Archived bool   `json:"archived"`
	}](data, "accel")
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, r := range rows {
		if r.Name == "" || r.Archived {
			continue
		}
		out = append(out, record(r.Name, r.Website))
	}
	return out, nil
}

// bondNote is the parenthetical BOND appends to a name to record an exit:
// "CLEAR (NYSE: YOU)", "Weights & Biases (CoreWeave acquired)".
var bondNote = regexp.MustCompile(`\s*\(([^)]*)\)\s*$`)

// ParseBond reads BOND's investments file. It also lists the legacy Kleiner Perkins
// Digital Growth fund under a separate key, which is not BOND's portfolio and is not
// read. An exit is only recorded in the name's parenthetical, so that is where an
// acquisition is detected and stripped.
func ParseBond(data []byte) ([]Record, error) {
	var payload struct {
		Data struct {
			Investments struct {
				Edges []struct {
					Node struct {
						Name string `json:"name"`
						URL  string `json:"url"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"investments_bond"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("collections: parse bond portfolio: %w", err)
	}
	var out []Record
	for _, e := range payload.Data.Investments.Edges {
		name := e.Node.Name
		if m := bondNote.FindStringSubmatch(name); m != nil {
			if strings.Contains(strings.ToLower(m[1]), "acquired") {
				continue
			}
			name = strings.TrimSpace(name[:len(name)-len(m[0])])
		}
		if name == "" {
			continue
		}
		out = append(out, record(name, e.Node.URL))
	}
	return out, nil
}

// a16zCompanies is the attribute the a16z portfolio page embeds its full company list
// in, as HTML-escaped JSON. The page's visible cards carry only a few dozen of them.
var a16zCompanies = regexp.MustCompile(`data-companies="([^"]*)"`)

// ParseA16zPortfolio reads the a16z portfolio page. An exit is listed with the stages
// it went through, so "M&A" (an acquisition) and the bare "EXIT" are dropped while an
// IPO, SPAC or direct listing is kept.
func ParseA16zPortfolio(data []byte) ([]Record, error) {
	m := a16zCompanies.FindSubmatch(data)
	if m == nil {
		return nil, fmt.Errorf("collections: a16z portfolio: no data-companies attribute")
	}
	var rows []struct {
		Name        string    `json:"name"`
		ExternalURL string    `json:"external_url"`
		Stages      looseList `json:"stages"`
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(string(m[1]))), &rows); err != nil {
		return nil, fmt.Errorf("collections: parse a16z portfolio: %w", err)
	}
	var out []Record
	for _, r := range rows {
		if r.Name == "" || acquired(r.Stages) {
			continue
		}
		out = append(out, record(html.UnescapeString(r.Name), r.ExternalURL))
	}
	return out, nil
}

// looseList reads a field the a16z page writes as a list of strings or, when it is
// empty, as the bare string "".
type looseList []string

func (l *looseList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		if one != "" {
			*l = looseList{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*l = many
	return nil
}

func acquired(stages []string) bool {
	for _, s := range stages {
		switch html.UnescapeString(s) {
		case "M&A", "EXIT":
			return true
		}
	}
	return false
}

// sequoiaIndex is the search index the Sequoia site (built on Framer) publishes. Its
// file name carries a content hash that changes on every publish, so it is found on
// the companies page rather than pinned. Only the site id and the hash are read from
// the page; the address is rebuilt on Framer's own host, so nothing the page says can
// point the fetch anywhere else.
var sequoiaIndex = regexp.MustCompile(`sites/([A-Za-z0-9]+)/searchIndex-([A-Za-z0-9]+)\.json`)

const sequoiaIndexURL = "https://framerusercontent.com/sites/%s/searchIndex-%s.json"

// resolveSequoiaIndex discovers the current search-index address from the page.
func resolveSequoiaIndex(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sequoiaCompanyURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("collections: fetch sequoia companies page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("collections: sequoia companies page: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	found := sequoiaIndex.FindSubmatch(body)
	if found == nil {
		return "", fmt.Errorf("collections: sequoia companies page names no search index")
	}
	return fmt.Sprintf(sequoiaIndexURL, found[1], found[2]), nil
}

var (
	sequoiaCompanyPath = regexp.MustCompile(`^/companies/[^/]+/?$`)
	sequoiaAcquired    = regexp.MustCompile(`^Acquired \d{4}$`)
	// A bare hostname in the page's paragraph list is the company's website: "n.xyz",
	// "evy.eu". Prose never matches — it has spaces.
	sequoiaHost = regexp.MustCompile(`^(?i)[a-z0-9-]+(\.[a-z0-9-]+)+(/\S*)?$`)
)

// ParseSequoia reads the Sequoia site's search index: one entry per page, of which the
// /companies/<slug> ones are the portfolio. A company page titles itself
// "<Name> | Sequoia Capital" and lists its website and milestones ("Acquired 2023",
// "IPO 2019") among its paragraphs.
func ParseSequoia(data []byte) ([]Record, error) {
	var index map[string]struct {
		Title string   `json:"title"`
		P     []string `json:"p"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("collections: parse sequoia search index: %w", err)
	}
	var out []Record
	for path, page := range index {
		if !sequoiaCompanyPath.MatchString(path) {
			continue
		}
		name, _, _ := strings.Cut(page.Title, " | ")
		if strings.TrimSpace(name) == "" {
			continue
		}
		website, exited := "", false
		for _, p := range page.P {
			p = strings.TrimSpace(p)
			if sequoiaAcquired.MatchString(p) {
				exited = true
			}
			if website == "" && sequoiaHost.MatchString(p) {
				website = p
			}
		}
		if !exited {
			out = append(out, record(name, website))
		}
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
