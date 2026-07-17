package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// hurma adapts Hurma career sites. Hurma hosts each employer on its own subdomain
// (<board>.hurma.work); the board is that subdomain (e.g. "scrumlaunch"). The public
// job board is a client-rendered Vue app backed by a paginated JSON API whose list
// endpoint omits the description, so the body comes from a per-vacancy detail fetch
// (bounded-concurrency), like the other detail adapters.
type hurma struct {
	http HeaderJSONGetter
}

// NewHurma builds the Hurma adapter over the given HTTP client.
func NewHurma(c HeaderJSONGetter) Source { return hurma{http: c} }

func (hurma) Provider() string { return "hurma" }

const (
	// hurmaPerPage is the listing page size; Hurma paginates and reports last_page.
	hurmaPerPage = 100
	// hurmaMaxPages bounds pagination so a board that never reports its last page
	// cannot loop forever.
	hurmaMaxPages = 50
)

// hurmaXHRHeaders is the header the Vue app's fetch carries. Hurma's Laravel API gates
// the public-vacancies JSON behind it — a bare GET returns an empty {"message":""} — so
// every request must present it. The standard Accept: application/json still wins.
var hurmaXHRHeaders = map[string]string{"X-Requested-With": "XMLHttpRequest"}

// hurmaListItem is one entry of the /public-vacancies list page. The list omits the
// description; residence/work_types carry the location and employment-type signal.
type hurmaListItem struct {
	Name      string `json:"name"`
	PublicURL string `json:"public_url"`
	Residence string `json:"residence"`
	WorkTypes string `json:"work_types"`
}

// hurmaList is the paginated list response: data plus Laravel's pagination meta.
type hurmaList struct {
	Data []hurmaListItem `json:"data"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
}

// hurmaDetail is one vacancy's full record. The description is split across several
// HTML section fields; hurmaDescription stitches the present ones together.
type hurmaDetail struct {
	Name              string `json:"name"`
	Residence         string `json:"residence"`
	WorkTypes         string `json:"work_types"`
	Description       string `json:"description"`
	Demand            string `json:"demand"`
	Responsibility    string `json:"responsibility"`
	WorkingConditions string `json:"working_conditions"`
	Addition          string `json:"addition"`
}

func (h hurma) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base := fmt.Sprintf("https://%s.hurma.work", e.Board)

	var items []hurmaListItem
	for page := 1; page <= hurmaMaxPages; page++ {
		listURL := fmt.Sprintf("%s/api/v1/public-vacancies?page=%d&per_page=%d", base, page, hurmaPerPage)
		var list hurmaList
		if err := h.http.GetJSONWithHeaders(ctx, listURL, hurmaXHRHeaders, &list); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("hurma: fetch board %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the vacancies gathered so far
		}
		items = append(items, list.Data...)
		// Stop at the reported last page; fall back to a short/empty page when meta is absent.
		if list.Meta.LastPage > 0 {
			if page >= list.Meta.LastPage {
				break
			}
		} else if len(list.Data) < hurmaPerPage {
			break
		}
	}

	// Each vacancy's description comes from its own detail fetch, fanned out under a
	// bounded pool.
	return fetchDetails(items, defaultDetailWorkers, func(it hurmaListItem) (Job, bool) {
		return h.detail(ctx, e, base, it)
	}), nil
}

// detail fetches one vacancy's full record and maps it to a Job, returning ok=false when
// the URL carries no parseable id or the fetch fails, so the caller skips just that vacancy.
func (h hurma) detail(ctx context.Context, e CompanyEntry, base string, it hurmaListItem) (Job, bool) {
	id := hurmaVacancyID(it.PublicURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	var resp struct {
		Data hurmaDetail `json:"data"`
	}
	detailURL := fmt.Sprintf("%s/api/v1/public-vacancies/%s", base, id)
	if err := h.http.GetJSONWithHeaders(ctx, detailURL, hurmaXHRHeaders, &resp); err != nil {
		return Job{}, false
	}
	d := resp.Data

	location := firstNonEmpty(d.Residence, it.Residence)
	workTypes := firstNonEmpty(d.WorkTypes, it.WorkTypes)
	return Job{
		ExternalID:  id,
		URL:         it.PublicURL,
		Title:       hurmaTitle(firstNonEmpty(d.Name, it.Name)),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(hurmaDescription(d)),
		// residence and work_types are free text ("remote", "Full-time, Remote work"),
		// so the remote flag is a text heuristic; WorkMode is left for the parser.
		Remote:         isRemote(location) || isRemote(workTypes),
		EmploymentType: hurmaEmploymentType(workTypes),
	}, true
}

// hurmaVacancyIDPattern captures the numeric vacancy id from a public_url's
// /public-vacancies/<id> segment.
var hurmaVacancyIDPattern = regexp.MustCompile(`/public-vacancies/(\d+)`)

// hurmaVacancyID extracts the native numeric vacancy id from a vacancy's public URL.
func hurmaVacancyID(u string) string {
	return firstSubmatch(hurmaVacancyIDPattern, u)
}

// hurmaTitlePrefix matches the "#<id> " tag Hurma prepends to a vacancy name.
var hurmaTitlePrefix = regexp.MustCompile(`^#\d+\s+`)

// hurmaTitle strips the leading "#<id> " tag from a vacancy name, leaving the plain title.
func hurmaTitle(name string) string {
	return strings.TrimSpace(hurmaTitlePrefix.ReplaceAllString(name, ""))
}

// hurmaDescription stitches the present HTML sections of a vacancy into one body, tagging
// the requirements/responsibilities/conditions sections with a heading and dropping empties.
func hurmaDescription(d hurmaDetail) string {
	var b strings.Builder
	section := func(heading, body string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		if heading != "" {
			fmt.Fprintf(&b, "<h3>%s</h3>", heading)
		}
		b.WriteString(body)
	}
	section("", d.Description)
	section("Requirements", d.Demand)
	section("Responsibilities", d.Responsibility)
	section("Working conditions", d.WorkingConditions)
	section("", d.Addition)
	return b.String()
}

// hurmaEmploymentType maps Hurma's free-text work_types ("Full-time, Remote work") onto the
// freehire vocabulary from its first employment token, returning "" for an unknown/absent
// value so the description parser decides.
func hurmaEmploymentType(workTypes string) string {
	for _, part := range strings.Split(workTypes, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "full-time", "full time":
			return "full_time"
		case "part-time", "part time":
			return "part_time"
		case "contract", "freelance":
			return "contract"
		case "internship", "intern":
			return "internship"
		}
	}
	return ""
}
