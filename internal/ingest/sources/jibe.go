package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// jibe adapts Jibe career sites (the career-site product fronting iCIMS, e.g.
// github.careers). The board is the site's public host (e.g. "www.github.careers"),
// forming the JSON listing API "https://<board>/api/jobs". That list endpoint already
// carries the full posting HTML in "description", so there is no per-job detail fetch —
// the separate responsibilities/qualifications fields are redundant re-extractions of
// the same body (verified: every posting's description already contains them) and are
// intentionally not appended.

// jibePageLimit is the listing page size. The API reports the catalogue total, so the
// crawl pages until every posting is collected.
const jibePageLimit = 100

// jibeDateLayout is Jibe's posted_date format: RFC3339 with a numeric, colon-less zone
// offset ("2026-06-16T20:41:00+0000"), which time.RFC3339 cannot parse.
const jibeDateLayout = "2006-01-02T15:04:05-0700"

type jibe struct {
	http JSONGetter
}

// NewJibe builds the Jibe adapter over the given HTTP client.
func NewJibe(c JSONGetter) Source { return jibe{http: c} }

func (jibe) Provider() string { return "jibe" }

// jibeListing is one /api/jobs page: the postings plus the catalogue total used as the
// stop condition.
type jibeListing struct {
	Jobs []struct {
		Data jibePosting `json:"data"`
	} `json:"jobs"`
	TotalCount int `json:"totalCount"`
}

// jibePosting is one posting's "data" object; only the fields the Job shape needs are
// decoded.
type jibePosting struct {
	Slug               string `json:"slug"`
	ReqID              string `json:"req_id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	LocationName       string `json:"location_name"`
	FullLocation       string `json:"full_location"`
	HiringOrganization string `json:"hiring_organization"`
	PostedDate         string `json:"posted_date"`
	// Tags holds the values of every tag slot ("tags", "tags1" … "tags9"), one slice per slot.
	// The slots are the tenant's own free fields, so they are collected generically.
	Tags [][]string `json:"-"`
}

// jibeTagKey matches a tag slot's key.
var jibeTagKey = regexp.MustCompile(`^tags[0-9]*$`)

// UnmarshalJSON decodes the named fields as usual, then collects every tag slot, whose keys
// vary by tenant and cannot be named in the struct.
func (p *jibePosting) UnmarshalJSON(b []byte) error {
	type plain jibePosting
	if err := json.Unmarshal(b, (*plain)(p)); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		var values []string
		if jibeTagKey.MatchString(k) && json.Unmarshal(v, &values) == nil {
			p.Tags = append(p.Tags, values)
		}
	}
	return nil
}

// jibeWorkplaceTags maps a tag value, lowercased, to a work mode. Jibe has no workplace field
// of its own — the record's location_type is a geocoding setting ("LAT_LNG", "ANY") on every
// sampled posting — so a tenant that states the arrangement puts it in one of its free tag
// slots, and which slot differs per tenant. Every entry is a label a live board publishes (189
// boards, 2026-10-30; about a quarter of postings carry one). Labels that offer a choice
// ("Onsite or Hybrid Required", "Remote / Hybrid / In-Office", "Remote Optional") or a
// permission ("Virtual Permitted") are absent, so they state nothing.
var jibeWorkplaceTags = map[string]string{
	"remote": "remote", "fully remote": "remote", "remote - us": "remote",
	"remote within the united states": "remote", "remote within location state only": "remote",
	"us-remote": "remote", "uk-remote": "remote", "de-remote": "remote", "in-remote": "remote",
	"au-remote": "remote", "nl-remote": "remote",

	"hybrid": "hybrid", "hybride": "hybrid", "#li-hybrid": "hybrid",
	"hybrid - 2 or less days in office": "hybrid", "hybrid (3 days in office)": "hybrid",
	"hybrid (remote / on site)":                              "hybrid",
	"hybrid (work from home with ability to report on-site)": "hybrid",

	"onsite": "onsite", "on-site": "onsite", "fully onsite": "onsite", "onsite 100%": "onsite",
	"onsite with travel": "onsite", "in-office": "onsite", "in office": "onsite",
	"in office w/field work": "onsite", "#li-onsite": "onsite", "not remote": "onsite",
	"no remote work": "onsite", "in office (not remote)": "onsite",
}

// jibeWorkMode reads a posting's work mode from its tag slots, "" when no tag states one or
// two tags disagree (a "Hybrid" slot beside a "#LI-Onsite" one is not a statement).
func jibeWorkMode(tags [][]string) string {
	mode := ""
	for _, slot := range tags {
		for _, v := range slot {
			m := jibeWorkplaceTags[strings.ToLower(strings.TrimSpace(v))]
			switch {
			case m == "":
			case mode == "":
				mode = m
			case mode != m:
				return ""
			}
		}
	}
	return mode
}

func (s jibe) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		url := fmt.Sprintf("https://%s/api/jobs?page=%d&limit=%d", e.Board, page, jibePageLimit)
		var listing jibeListing
		if err := s.http.GetJSON(ctx, url, &listing); err != nil {
			return nil, fmt.Errorf("jibe: list board %s: %w", e.Board, err)
		}
		if len(listing.Jobs) == 0 {
			break
		}
		for _, item := range listing.Jobs {
			if job, ok := s.toJob(e, item.Data); ok {
				jobs = append(jobs, job)
			}
		}
		// Stop once the catalogue total is reached; the empty-page check above is the
		// fallback if totalCount is ever absent or wrong.
		if len(jobs) >= listing.TotalCount {
			break
		}
	}
	return jobs, nil
}

// toJob maps one Jibe posting to the normalized Job shape. The id is the posting slug
// (the public job-page path segment), falling back to req_id. A posting with neither has
// no dedup key — it would collide on a bare ".../jobs/" URL — so it is dropped (ok=false).
func (s jibe) toJob(e CompanyEntry, p jibePosting) (Job, bool) {
	id := firstNonEmpty(p.Slug, p.ReqID)
	if id == "" {
		return Job{}, false
	}
	location := firstNonEmpty(p.LocationName, p.FullLocation)
	workMode := jibeWorkMode(p.Tags)
	return Job{
		ExternalID:  id,
		URL:         fmt.Sprintf("https://%s/careers-home/jobs/%s", e.Board, id),
		Title:       strings.TrimSpace(p.Title),
		Company:     firstNonEmpty(p.HiringOrganization, e.Company),
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseLayout(jibeDateLayout, p.PostedDate),
	}, true
}
