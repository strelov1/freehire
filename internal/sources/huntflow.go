package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// huntflow adapts the public career sites Huntflow hosts at <board>.huntflow.io. Those
// pages are a Nuxt SSR app, so the vacancy data is served as a devalue-encoded
// _payload.json (a flat array where object/array fields are indices into it). The list
// payload carries no description, so each open vacancy's body comes from its own detail
// payload, fanned out like SmartRecruiters.
type huntflow struct {
	http JSONGetter
}

const (
	huntflowListURL    = "https://%s.huntflow.io/_payload.json"
	huntflowDetailURL  = "https://%s.huntflow.io/vacancy/%s/_payload.json"
	huntflowVacancyURL = "https://%s.huntflow.io/vacancy/%s"
)

// huntflowHubFolder is the division-breadcrumb folder a community hub nests every partner
// company under (see companyFromDivision).
const huntflowHubFolder = "Partners"

// NewHuntflow builds the Huntflow adapter over the given HTTP client.
func NewHuntflow(c JSONGetter) Source { return huntflow{http: c} }

func (huntflow) Provider() string { return "huntflow" }

// hfItem is one vacancy from the list payload (no description here). A non-nil
// ArchivedAt marks a closed vacancy, which the crawl skips.
type hfItem struct {
	ID         int64   `json:"id"`
	Slug       string  `json:"slug"`
	Position   string  `json:"position"`
	ArchivedAt *string `json:"archived_at"`
}

func (h huntflow) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := h.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	open := make([]hfItem, 0, len(items))
	for _, it := range items {
		if it.ArchivedAt == nil {
			open = append(open, it)
		}
	}

	return fetchDetails(open, defaultDetailWorkers, func(it hfItem) (Job, bool) {
		return h.detail(ctx, e, it)
	}), nil
}

// list fetches a board's vacancy list from its _payload.json.
func (h huntflow) list(ctx context.Context, board string) ([]hfItem, error) {
	node, err := h.payload(ctx, fmt.Sprintf(huntflowListURL, board))
	if err != nil {
		return nil, fmt.Errorf("huntflow: list board %s: %w", board, err)
	}
	var lp struct {
		Data struct {
			Vacancies struct {
				Items []hfItem `json:"items"`
			} `json:"vacancies"`
		} `json:"data"`
	}
	if err := remarshal(node, &lp); err != nil {
		return nil, fmt.Errorf("huntflow: decode list board %s: %w", board, err)
	}
	return lp.Data.Vacancies.Items, nil
}

// detail fetches one vacancy's detail payload and maps it to a Job, returning ok=false
// when the fetch or decode fails so the caller skips just that vacancy.
func (h huntflow) detail(ctx context.Context, e CompanyEntry, it hfItem) (Job, bool) {
	node, err := h.payload(ctx, fmt.Sprintf(huntflowDetailURL, e.Board, it.Slug))
	if err != nil {
		return Job{}, false
	}
	vac := findVacancy(node)
	if vac == nil {
		return Job{}, false
	}
	var d struct {
		City         string `json:"city"`
		Money        string `json:"money"`
		Division     string `json:"division"`
		Intro        string `json:"intro"`
		Body         string `json:"body"`
		Requirements string `json:"requirements"`
		Conditions   string `json:"conditions"`
	}
	if err := remarshal(vac, &d); err != nil {
		return Job{}, false
	}

	// The feed has no salary field on Job, so fold money into the description (when set)
	// rather than drop it — enrichment reads salary from the description text.
	body := d.Intro + d.Body + d.Requirements + d.Conditions
	if d.Money != "" {
		body = "<p>" + d.Money + "</p>" + body
	}

	// On a community hub the real employer lives in the vacancy's division breadcrumb, not in
	// the configured (hub) company; an ordinary board keeps its configured company.
	company := e.Company
	if e.Hub {
		company = companyFromDivision(d.Division, e.Company)
	}

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(huntflowVacancyURL, e.Board, it.Slug),
		Title:       it.Position,
		Company:     company,
		Location:    d.City,
		Description: sanitizeHTML(body),
		Remote:      isRemote(d.City),
		PostedAt:    nil, // the public career feed carries no publish date
	}, true
}

// companyFromDivision resolves a hub board's per-vacancy employer from Huntflow's division
// breadcrumb. Huntflow encodes division leaf-first as
// "<sub-team> · … · <Company> · Partners · Vacancies", where the huntflowHubFolder holds every
// partner company, so the employer is the segment immediately before that folder; deeper
// segments are the company's own sub-teams. The separator is "·" (U+00B7) in the list payload
// and "•" (U+2022) in the detail payload. Falls back to the given company when the division has
// no such structure (e.g. the hub's own internal roles).
func companyFromDivision(division, fallback string) string {
	segments := strings.Split(strings.ReplaceAll(division, "•", "·"), "·")
	for i, seg := range segments {
		if i > 0 && strings.TrimSpace(seg) == huntflowHubFolder {
			if company := strings.TrimSpace(segments[i-1]); company != "" {
				return company
			}
			break
		}
	}
	return fallback
}

// payload fetches a Nuxt _payload.json and resolves its devalue references into a plain
// nested value (maps/slices/scalars).
func (h huntflow) payload(ctx context.Context, url string) (any, error) {
	var raw []json.RawMessage
	if err := h.http.GetJSON(ctx, url, &raw); err != nil {
		return nil, err
	}
	return unflattenPayload(raw)
}

// unflattenPayload resolves a devalue-encoded array (Nuxt's _payload.json format). Every
// value is a node in raw; an object's field values and an array's elements are integer
// indices into raw, and scalars are stored once and shared by index. A string-headed
// array is a type wrapper (e.g. ShallowReactive) and resolves to its wrapped value.
func unflattenPayload(raw []json.RawMessage) (any, error) {
	memo := make([]any, len(raw))
	done := make([]bool, len(raw))

	var hydrate func(i int) (any, error)
	hydrate = func(i int) (any, error) {
		if i < 0 || i >= len(raw) {
			return nil, nil
		}
		if done[i] {
			return memo[i], nil
		}
		var tok any
		if err := json.Unmarshal(raw[i], &tok); err != nil {
			return nil, err
		}

		var out any
		switch t := tok.(type) {
		case map[string]any:
			obj := make(map[string]any, len(t))
			for k, ref := range t {
				v, err := hydrateRef(ref, hydrate)
				if err != nil {
					return nil, err
				}
				obj[k] = v
			}
			out = obj
		case []any:
			// A string-headed array is a devalue type token (e.g. ShallowReactive) that
			// wraps one referenced value rather than listing elements; resolve to it.
			if len(t) > 0 {
				if _, isWrapper := t[0].(string); isWrapper {
					if len(t) > 1 {
						wrapped, err := hydrateRef(t[1], hydrate)
						if err != nil {
							return nil, err
						}
						out = wrapped
					}
					break
				}
			}
			arr := make([]any, 0, len(t))
			for _, ref := range t {
				v, err := hydrateRef(ref, hydrate)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			out = arr
		default:
			out = tok
		}

		memo[i], done[i] = out, true
		return out, nil
	}
	return hydrate(0)
}

// hydrateRef resolves a single field value or array element: a JSON number is an index
// into the payload, anything else is taken literally.
func hydrateRef(ref any, hydrate func(int) (any, error)) (any, error) {
	if idx, ok := ref.(float64); ok {
		return hydrate(int(idx))
	}
	return ref, nil
}

// findVacancy walks a decoded detail payload for the vacancy object — the map carrying
// both a body and a position. Walking rather than assuming a fixed key path keeps the
// adapter robust to Nuxt's surrounding route-state nesting.
func findVacancy(node any) map[string]any {
	switch n := node.(type) {
	case map[string]any:
		_, hasBody := n["body"]
		_, hasPosition := n["position"]
		if hasBody && hasPosition {
			return n
		}
		for _, v := range n {
			if r := findVacancy(v); r != nil {
				return r
			}
		}
	case []any:
		for _, v := range n {
			if r := findVacancy(v); r != nil {
				return r
			}
		}
	}
	return nil
}

// remarshal re-encodes a decoded payload node and decodes it into v, so a typed struct
// can read the dynamic devalue result.
func remarshal(node, v any) error {
	b, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
