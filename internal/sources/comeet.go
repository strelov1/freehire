package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// comeet adapts Comeet career sites. The board is "<slug>/<companyUID>" (taken straight
// from a public job URL, comeet.com/jobs/<slug>/<companyUID>). The company page embeds a
// public read token; with it, one careers-api call returns every position fully populated
// (structured location, a workplace_type enum, and a description split into named HTML
// sections), so no per-posting detail fetch is needed.
type comeetHTTP interface {
	HTMLGetter
	JSONGetter
}

type comeet struct {
	http comeetHTTP
}

// NewComeet builds the Comeet adapter over the given HTTP client.
func NewComeet(c comeetHTTP) Source { return comeet{http: c} }

func (comeet) Provider() string { return "comeet" }

// comeetLocation is a position's structured location.
type comeetLocation struct {
	Name    string `json:"name"`
	Country string `json:"country"`
	City    string `json:"city"`
	State   string `json:"state"`
}

// String renders the location as "City, State", falling back to the country code, then the
// free-text name, then "".
func (l comeetLocation) String() string {
	var parts []string
	for _, p := range []string{l.City, l.State} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	if c := strings.TrimSpace(l.Country); c != "" {
		return c
	}
	return strings.TrimSpace(l.Name)
}

// comeetSection is one named HTML block of a position's description (Description,
// Responsibilities, Requirements, …).
type comeetSection struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type comeetPosition struct {
	UID           string          `json:"uid"`
	Name          string          `json:"name"`
	CompanyName   string          `json:"company_name"`
	URLActivePage string          `json:"url_active_page"`
	WorkplaceType string          `json:"workplace_type"`
	TimeUpdated   string          `json:"time_updated"`
	Location      comeetLocation  `json:"location"`
	Details       []comeetSection `json:"details"`
}

func (c comeet) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	slash := strings.LastIndex(e.Board, "/")
	if slash < 0 {
		return nil, fmt.Errorf("comeet: board %q must be %q", e.Board, "<slug>/<companyUID>")
	}
	uid := e.Board[slash+1:]

	root, err := c.http.GetHTML(ctx, "https://www.comeet.com/jobs/"+e.Board)
	if err != nil {
		return nil, fmt.Errorf("comeet: page %s: %w", e.Board, err)
	}
	token := comeetToken(root)
	if token == "" {
		return nil, fmt.Errorf("comeet: no token found for board %s", e.Board)
	}

	url := fmt.Sprintf("https://www.comeet.co/careers-api/2.0/company/%s/positions?token=%s&details=true", uid, token)
	var positions []comeetPosition
	if err := c.http.GetJSON(ctx, url, &positions); err != nil {
		return nil, fmt.Errorf("comeet: positions %s: %w", uid, err)
	}

	jobs := make([]Job, 0, len(positions))
	for _, p := range positions {
		if j, ok := c.toJob(p, e); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// toJob maps a position to a Job, returning ok=false when it carries no uid (which would
// collide on the dedup key). Company comes from config, falling back to the payload.
func (comeet) toJob(p comeetPosition, e CompanyEntry) (Job, bool) {
	if p.UID == "" {
		return Job{}, false
	}

	var desc strings.Builder
	for _, s := range p.Details {
		if strings.TrimSpace(s.Value) == "" {
			continue
		}
		// The intro section is unlabeled prose; the rest (Responsibilities, Requirements)
		// keep their heading so the assembled body stays readable.
		if s.Name != "" && !strings.EqualFold(s.Name, "Description") {
			desc.WriteString("<h3>" + html.EscapeString(s.Name) + "</h3>")
		}
		desc.WriteString(s.Value)
	}

	company := e.Company
	if company == "" {
		company = p.CompanyName
	}

	mode := workplaceTypeMode(p.WorkplaceType)
	return Job{
		ExternalID:  p.UID,
		URL:         p.URLActivePage,
		Title:       p.Name,
		Company:     company,
		Location:    p.Location.String(),
		Description: sanitizeHTML(desc.String()),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseRFC3339(p.TimeUpdated),
	}, true
}

// comeetTokenPattern captures the public read token Comeet embeds in the careers page.
// The length floor avoids matching shorter, unrelated "token" keys other inline scripts
// (analytics, consent, …) may carry; the real careers token is a long alphanumeric string.
var comeetTokenPattern = regexp.MustCompile(`"token"\s*:\s*"([A-Za-z0-9]{20,})"`)

// comeetToken extracts the careers read token from the page's inline script state.
func comeetToken(root *html.Node) string {
	var token string
	walk(root, func(n *html.Node) bool {
		if token != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			if m := comeetTokenPattern.FindStringSubmatch(textContent(n)); m != nil {
				token = m[1]
			}
		}
		return true
	})
	return token
}
