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
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

const beehireHost = "app.beehire.com"

var beehireBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

type beehireHTTP interface {
	JSONGetter
}

type beehire struct {
	http beehireHTTP
}

func NewBeehire(c beehireHTTP) Source { return beehire{http: c} }

func (beehire) Provider() string { return "beehire" }

func (beehire) fullBoardListing() {}

func (beehire) emptyBoardListing() {}

func (s beehire) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !beehireBoard.MatchString(board) {
		return nil, fmt.Errorf("beehire: board %q is not a career-page slug", e.Board)
	}
	var body beehirePage
	u := "https://" + beehireHost + "/users/getPublicCampaigns/" + url.PathEscape(board)
	if err := s.http.GetJSON(ctx, u, &body); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return nil, fmt.Errorf("beehire: board %q: %w", board, ErrBoardGone)
		}
		return nil, fmt.Errorf("beehire: board %s: %w", board, err)
	}
	if body.Campaigns == nil {
		return nil, errListingAbsent("beehire", board, "campaigns")
	}
	seen := map[string]struct{}{}
	jobs := make([]Job, 0, len(body.Campaigns))
	for _, item := range body.Campaigns {
		job, err := beehireJob(e.Company, item)
		if err != nil {
			return nil, fmt.Errorf("beehire: board %s: %w", board, err)
		}
		if _, ok := seen[job.ExternalID]; ok {
			continue
		}
		seen[job.ExternalID] = struct{}{}
		jobs = append(jobs, job)
	}
	if len(jobs) != len(body.Campaigns) {
		return nil, fmt.Errorf("beehire: board %s: read %d postings, source listed %d", board, len(jobs), len(body.Campaigns))
	}
	return jobs, nil
}

func beehireJob(company string, item beehireCampaign) (Job, error) {
	id := strings.TrimSpace(item.ID)
	lang := string(item.Language)
	title := beehirePick(item.Title, lang)
	if id == "" || title == "" {
		return Job{}, fmt.Errorf("posting %q has no id or title", item.ID)
	}
	page, err := beehirePageURL(item)
	if err != nil {
		return Job{}, err
	}
	body := beehirePick(item.FullDescription, lang)
	if body == "" {
		body = beehirePick(item.Description, lang)
	}
	job := Job{
		ExternalID:     id,
		URL:            page,
		Title:          title,
		Company:        company,
		Location:       beehireLocation(item.Location),
		Description:    sanitizeHTML(beehireBody(body)),
		EmploymentType: beehireEmployment(item.Details.Contract),
		WorkMode:       beehireWorkMode[strings.ToLower(strings.TrimSpace(item.Details.Contract.Remote))],
	}
	if code := location.NormalizeCountry(item.Location.Country); code != "" {
		job.Countries = []string{code}
	}
	return job, nil
}

func beehirePageURL(item beehireCampaign) (string, error) {
	raw := strings.TrimSpace(item.InviteLink)
	if raw == "" {
		key := strings.TrimSpace(item.InviteKey)
		if key != "" {
			raw = "https://" + beehireHost + "/invite/" + url.PathEscape(key)
		}
	}
	page, err := url.Parse(raw)
	if err != nil || page.Scheme != "https" || !strings.EqualFold(page.Hostname(), beehireHost) {
		return "", fmt.Errorf("posting %s url %q is not on %s", strings.TrimSpace(item.ID), item.InviteLink, beehireHost)
	}
	return page.String(), nil
}

func beehirePick(text beehireLocalized, lang string) string {
	if s := strings.TrimSpace(text[lang]); lang != "" && s != "" {
		return s
	}
	if s := strings.TrimSpace(text["0"]); s != "" {
		return s
	}
	keys := make([]string, 0, len(text))
	for key := range text {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if s := strings.TrimSpace(text[key]); s != "" {
			return s
		}
	}
	return ""
}

func beehireLocation(loc beehirePlace) string {
	if name := strings.TrimSpace(loc.Name); name != "" {
		return name
	}
	return joinNonEmpty(loc.City, loc.State, loc.Country)
}

func beehireBody(s string) string {
	if strings.Contains(s, "<") {
		return s
	}
	return plainTextToHTML(s)
}

func beehireEmployment(c beehireContract) string {
	kind := strings.ToLower(strings.TrimSpace(c.Type))
	span := strings.ToLower(strings.TrimSpace(c.Duration))
	switch kind {
	case "contracttype_internship", "contracttype_apprenticeship", "contracttype_student":
		return "internship"
	}
	switch span {
	case "contractduration_parttime", "contractduration_10h", "contractduration_20h":
		return "part_time"
	}
	switch kind {
	case "contracttype_permanent":
		return "full_time"
	case "contracttype_fixedterm", "contracttype_freelance", "contracttype_interim", "contracttype_replacement":
		return "contract"
	}
	if span == "contractduration_fulltime" {
		return "full_time"
	}
	return ""
}

var beehireWorkMode = map[string]string{
	"remotework_none":    "onsite",
	"remotework_full":    "remote",
	"remotework_partial": "hybrid",
}

type beehirePage struct {
	Campaigns []beehireCampaign `json:"campaigns"`
}

type beehireCampaign struct {
	ID              string            `json:"_id"`
	InviteLink      string            `json:"inviteLink"`
	InviteKey       string            `json:"inviteKey"`
	Language        beehireFlexString `json:"language"`
	Title           beehireLocalized  `json:"title"`
	Description     beehireLocalized  `json:"description"`
	FullDescription beehireLocalized  `json:"fullDescription"`
	Location        beehirePlace      `json:"location"`
	Details         struct {
		Contract beehireContract `json:"contract"`
	} `json:"details"`
}

type beehirePlace struct {
	Name    string `json:"name"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
}

type beehireContract struct {
	Type     string `json:"type"`
	Duration string `json:"duration"`
	Remote   string `json:"remote"`
}

type beehireFlexString string

func (s *beehireFlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = beehireFlexString(v)
		return nil
	}
	*s = beehireFlexString(b)
	return nil
}

type beehireLocalized map[string]string

func (m *beehireLocalized) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		if strings.TrimSpace(v) == "" {
			*m = nil
			return nil
		}
		*m = beehireLocalized{"0": v}
		return nil
	}
	var raw map[string]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*m = raw
	return nil
}
