package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Observations are audit evidence, not another ingestion adapter. Fields record presence,
// not value equivalence; in particular a populated feed enum has not been normalized.
type auditPosting struct {
	ID, URL, Title, Body string
	Fields               map[string]bool
}

type feedObservation struct {
	URL, Schema string
	Rows        int
	Jobs        map[string]auditPosting
}

var scalisAuditID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var teamtailorAuditPath = regexp.MustCompile(`^/jobs/(\d+)(?:-[^/]+)?/?$`)
var auditTenant = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func auditFeedURL(provider, board string) (string, error) {
	switch provider {
	case "scalis":
		if !auditTenant.MatchString(board) {
			return "", fmt.Errorf("invalid scalis board %q", board)
		}
		return "https://" + board + ".scalis.ai/api/job-listings/feed", nil
	case "teamtailor":
		u, err := url.Parse("https://" + board)
		if err != nil || u.Host != board || u.Path != "" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf("invalid teamtailor host %q", board)
		}
		if !strings.Contains(board, ".") {
			return "", fmt.Errorf("invalid teamtailor host %q", board)
		}
		return u.String() + "/jobs.rss", nil
	default:
		return "", fmt.Errorf("feed audit is not supported for %q (supported: scalis, teamtailor)", provider)
	}
}

func readAuditFeed(ctx context.Context, c httpClient, provider, board string) (feedObservation, error) {
	endpoint, err := auditFeedURL(provider, board)
	if err != nil {
		return feedObservation{}, err
	}
	if provider == "scalis" {
		return readScalisAudit(ctx, c, endpoint)
	}
	return readTeamtailorAudit(ctx, c, endpoint)
}

type scalisAuditFeed struct {
	XMLName xml.Name `xml:"jobs"`
	Jobs    []struct {
		Info struct {
			Title   string `xml:"title"`
			Body    string `xml:"job_description"`
			City    string `xml:"job_location_name"`
			Country string `xml:"country"`
			Remote  string `xml:"remote"`
		} `xml:"jobinformation"`
		Mapping struct {
			Type     string `xml:"job_type"`
			Min      string `xml:"salary_min"`
			Max      string `xml:"salary_max"`
			Currency string `xml:"salary_currency"`
			Period   string `xml:"salary_compensation_type"`
		} `xml:"mapping_information"`
		URL    string   `xml:"contact_information>vacancy_url"`
		Skills []string `xml:"skills>skill"`
	} `xml:"job"`
}

func readScalisAudit(ctx context.Context, c httpClient, endpoint string) (feedObservation, error) {
	out := feedObservation{URL: endpoint, Schema: "scalis-xml", Jobs: map[string]auditPosting{}}
	var feed scalisAuditFeed
	if err := c.GetXML(ctx, endpoint, &feed); err != nil {
		return out, err
	}
	out.Rows = len(feed.Jobs)
	for _, row := range feed.Jobs {
		id, canonical, err := auditPostingURL(endpoint, row.URL, "scalis")
		if err != nil {
			return out, err
		}
		j := auditPosting{
			ID: id, URL: canonical, Title: strings.TrimSpace(row.Info.Title),
			Body: auditBody(row.Info.Body),
			Fields: map[string]bool{
				"location":        strings.TrimSpace(row.Info.City+row.Info.Country) != "",
				"country":         strings.TrimSpace(row.Info.Country) != "",
				"work_mode":       strings.TrimSpace(row.Info.Remote) != "",
				"employment_type": strings.TrimSpace(row.Mapping.Type) != "",
				"skills":          len(row.Skills) > 0,
				"salary":          strings.TrimSpace(row.Mapping.Min+row.Mapping.Max) != "",
				"salary_currency": strings.TrimSpace(row.Mapping.Currency) != "",
				"salary_period":   strings.TrimSpace(row.Mapping.Period) != "",
			},
		}
		if j.Title == "" {
			return out, fmt.Errorf("scalis job %s has no title", id)
		}
		if previous, exists := out.Jobs[id]; exists {
			if previous.Body != j.Body || previous.Title != j.Title {
				return out, fmt.Errorf("scalis job %s has conflicting location rows", id)
			}
			for field, present := range previous.Fields {
				j.Fields[field] = j.Fields[field] || present
			}
		}
		out.Jobs[id] = j
	}
	return out, nil
}

type teamtailorAuditFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel *struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Date        string `xml:"pubDate"`
			Remote      string `xml:"remoteStatus"`
			Locations   []struct {
				City    string `xml:"city"`
				Country string `xml:"country"`
				Name    string `xml:"name"`
			} `xml:"locations>location"`
		} `xml:"item"`
	} `xml:"channel"`
}

func readTeamtailorAudit(ctx context.Context, c httpClient, endpoint string) (feedObservation, error) {
	out := feedObservation{URL: endpoint, Schema: "teamtailor-rss", Jobs: map[string]auditPosting{}}
	const pageSize = 500
	const maxPages = 20
	for page := 0; page < maxPages; page++ {
		u := fmt.Sprintf("%s?offset=%d&per_page=%d", endpoint, page*pageSize, pageSize)
		var feed teamtailorAuditFeed
		if err := c.GetXML(ctx, u, &feed); err != nil {
			return out, err
		}
		if feed.Channel == nil {
			return out, fmt.Errorf("teamtailor rss has no channel")
		}
		for _, item := range feed.Channel.Items {
			id, canonical, err := auditPostingURL(endpoint, item.Link, "teamtailor")
			if err != nil {
				return out, err
			}
			if _, exists := out.Jobs[id]; exists {
				return out, fmt.Errorf("teamtailor rss repeats job %s; pagination is unverified", id)
			}
			j := auditPosting{
				ID: id, URL: canonical, Title: strings.TrimSpace(item.Title),
				Body: auditBody(item.Description),
				Fields: map[string]bool{
					"posted_at": strings.TrimSpace(item.Date) != "",
					"work_mode": item.Remote != "" && item.Remote != "none",
				},
			}
			if j.Title == "" {
				return out, fmt.Errorf("teamtailor job %s has no title", id)
			}
			for _, loc := range item.Locations {
				j.Fields["location"] = j.Fields["location"] || strings.TrimSpace(loc.City+loc.Country+loc.Name) != ""
				j.Fields["country"] = j.Fields["country"] || strings.TrimSpace(loc.Country) != ""
			}
			out.Jobs[id] = j
			out.Rows++
		}
		if len(feed.Channel.Items) < pageSize {
			return out, nil
		}
	}
	return out, fmt.Errorf("teamtailor rss reached %d pages without proving the end", maxPages)
}

// A Scalis ID names a posting only within its tenant, so its origin must be the board's:
// a feed carrying another customer's postings must not pass for the requested board. A
// Teamtailor ID is platform-wide, and a site on its own domain links every item there
// rather than to the board host (ascom.teamtailor.com -> career.ascom.com), so only the
// ID is compared; a posting of another board shows up as only_in_feed.
func auditPostingURL(endpoint, raw, provider string) (string, string, error) {
	origin, _ := url.Parse(endpoint)
	u, err := url.Parse(strings.TrimSpace(raw))
	foreign := provider == "scalis" && err == nil && !strings.EqualFold(u.Host, origin.Host)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || foreign {
		return "", "", fmt.Errorf("invalid or foreign posting url %q", raw)
	}
	var id string
	switch provider {
	case "scalis":
		id = strings.TrimPrefix(u.Path, "/job/")
		if !strings.HasPrefix(u.Path, "/job/") || !scalisAuditID.MatchString(id) {
			return "", "", fmt.Errorf("invalid scalis posting path %q", u.Path)
		}
	case "teamtailor":
		match := teamtailorAuditPath.FindStringSubmatch(u.Path)
		if match == nil {
			return "", "", fmt.Errorf("invalid teamtailor posting path %q", u.Path)
		}
		id = match[1]
	}
	u.RawQuery = ""
	u.Fragment = ""
	return id, u.String(), nil
}

// Tokenization preserves word boundaries between block tags. Neither source markup nor
// recruiter contact fields are written to the report.
func auditBody(body string) string {
	z := html.NewTokenizer(strings.NewReader(body))
	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case html.TextToken:
			b.Write(z.Text())
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "p", "br", "div", "li", "ul", "ol", "h1", "h2", "h3", "h4", "section", "table", "tr", "td":
				b.WriteByte(' ')
			}
		}
	}
}
