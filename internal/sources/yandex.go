package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// yandex adapts Yandex's public jobs API (yandex.ru/jobs and yandex.com/jobs). It is a
// single-company boardless adapter: its optional board field selects the host and language
// segment ("ru" or "com") and defaults to "ru" when unset, so a boardless config entry
// is well-formed and targets the primary market. The list endpoint paginates by an opaque
// cursor and carries no body, so each kept publication's description comes from its own
// detail request, fanned out like SmartRecruiters. List items that redirect out
// (redirect_url) or belong to a hiring event (fast_track) are not real vacancies and are
// dropped.
type yandex struct {
	http JSONGetter
}

func (yandex) boardless() {}

const (
	yandexListURL    = "https://yandex.%s/jobs/api/publications"
	yandexDetailURL  = "https://yandex.%s/jobs/api/publications/%d"
	yandexVacancyURL = "https://yandex.%s/jobs/vacancies/%s"
)

// NewYandex builds the Yandex adapter over the given HTTP client.
func NewYandex(c JSONGetter) Source { return yandex{http: c} }

func (yandex) Provider() string { return "yandex" }

// yandexItem is one publication from the list response (no description here). A non-null
// RedirectURL (links out) or FastTrack (hiring event) marks a non-vacancy, which the crawl
// skips. Cities and work modes drive location and the remote flag.
type yandexItem struct {
	ID                 int64           `json:"id"`
	PublicationSlugURL string          `json:"publication_slug_url"`
	Title              string          `json:"title"`
	RedirectURL        json.RawMessage `json:"redirect_url"`
	FastTrack          json.RawMessage `json:"fast_track"`
	Vacancy            struct {
		Cities []struct {
			Name string `json:"name"`
		} `json:"cities"`
		WorkModes []struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"work_modes"`
	} `json:"vacancy"`
}

func (y yandex) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// board selects the host/language segment ("ru" or "com"); default to the
	// primary (ru) market when unset, so a boardless yandex entry is well-formed.
	if e.Board == "" {
		e.Board = "ru"
	}
	items, err := y.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	open := make([]yandexItem, 0, len(items))
	for _, it := range items {
		if isJSONNull(it.RedirectURL) && isJSONNull(it.FastTrack) {
			open = append(open, it)
		}
	}

	return fetchDetails(open, defaultDetailWorkers, func(it yandexItem) (Job, bool) {
		return y.detail(ctx, e, it)
	}), nil
}

// yandexMaxListPages bounds the cursor pagination. Yandex's API tarpits datacenter IPs and
// under throttling can return a "next" cursor that never empties; without a cap the loop
// runs forever (it hung a prod custom.yml ingest for ~4h, holding its flock). The bound sits
// far above the real page count (~1250 jobs total) so it never truncates a legitimate crawl.
const yandexMaxListPages = 500

// list walks the cursor-paginated publication list. The first request hits the bare
// endpoint; each response's "next" is an absolute URL on Yandex's internal host carrying a
// ?cursor= query, which list re-issues against the public host until next is empty. A
// repeated cursor (the upstream cycling under throttling) or the page cap ends the walk, so
// a misbehaving feed can never spin the loop forever.
func (y yandex) list(ctx context.Context, board string) ([]yandexItem, error) {
	base := fmt.Sprintf(yandexListURL, board)
	url := base
	var all []yandexItem
	seen := make(map[string]bool)
	for page := 0; url != ""; page++ {
		if page >= yandexMaxListPages {
			return nil, fmt.Errorf("yandex: list board %s: exceeded %d pages (runaway cursor)", board, yandexMaxListPages)
		}
		var resp struct {
			Results []yandexItem `json:"results"`
			Next    string       `json:"next"`
		}
		if err := y.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("yandex: list board %s: %w", board, err)
		}
		all = append(all, resp.Results...)

		cursor := cursorParam(resp.Next)
		if cursor == "" || seen[cursor] {
			break
		}
		seen[cursor] = true
		url = base + "?cursor=" + cursor
	}
	return all, nil
}

// detail fetches one publication's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller skips just that publication.
func (y yandex) detail(ctx context.Context, e CompanyEntry, it yandexItem) (Job, bool) {
	var d struct {
		ShortSummary           string `json:"short_summary"`
		Duties                 string `json:"duties"`
		KeyQualifications      string `json:"key_qualifications"`
		AdditionalRequirements string `json:"additional_requirements"`
		Conditions             string `json:"conditions"`
		Modified               string `json:"modified"`
	}
	if err := y.http.GetJSON(ctx, fmt.Sprintf(yandexDetailURL, e.Board, it.ID), &d); err != nil {
		return Job{}, false
	}

	cities := make([]string, 0, len(it.Vacancy.Cities))
	for _, c := range it.Vacancy.Cities {
		cities = append(cities, c.Name)
	}
	location := joinNonEmpty(cities...)

	body := d.ShortSummary + d.Duties + d.KeyQualifications + d.AdditionalRequirements + d.Conditions

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(yandexVacancyURL, e.Board, it.PublicationSlugURL),
		Title:       it.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(body),
		Remote:      isRemote(location) || hasRemoteWorkMode(it),
		PostedAt:    parseRFC3339(d.Modified),
	}, true
}

// hasRemoteWorkMode reports whether any of the publication's work modes signals remote work,
// by either its slug or its (localized) name matching the remote heuristic.
func hasRemoteWorkMode(it yandexItem) bool {
	for _, m := range it.Vacancy.WorkModes {
		if strings.Contains(strings.ToLower(m.Slug), "remote") || isRemote(m.Name) {
			return true
		}
	}
	return false
}

// cursorParam extracts the "cursor" query parameter from a next-page URL, returning "" when
// the URL is empty or unparseable or carries no cursor (the end of pagination). The next URL
// points at an internal host; only its cursor is reused, against the public host.
func cursorParam(next string) string {
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return ""
	}
	return u.Query().Get("cursor")
}

// isJSONNull reports whether a raw JSON field is absent or the literal null, so a nullable
// list field (redirect_url, fast_track) can be tested without decoding its shape.
func isJSONNull(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null"
}
