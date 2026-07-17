package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// topco adapts top.co, the careers board of the TOP (TON/Open Platform) ecosystem. The
// board lists many portfolio companies (Mira, Wallet, Tribute, …), each posting carrying its
// own employer, so it is a boardless aggregator. top.co is a Next.js App Router app: the
// careers page inlines every posting into its RSC flight as a "vacancies":[…] array (id,
// position, company), but the posting body is lazy — the array's body/requirements/conditions
// are "$undefined" placeholders. The full HTML body lives on the per-posting RSC data endpoint
// (/careers/{id} reached with the "RSC: 1" header), so each posting takes one detail fetch.
type topco struct {
	http topcoClient
}

// topcoClient is the transport surface topco needs: the careers page (HTML) plus the
// per-posting RSC data endpoint (raw text gated behind the RSC header).
type topcoClient interface {
	HTMLGetter
	HeaderTextGetter
}

const (
	topcoCareersURL = "https://top.co/careers"
	// topcoJobURL is both the public posting page and, with the RSC header, its data endpoint.
	topcoJobURL = "https://top.co/careers/%s"
)

// NewTopco builds the top.co adapter over the given HTTP client.
func NewTopco(c topcoClient) Source { return topco{http: c} }

func (topco) Provider() string { return "topco" }

// topco is one careers page with no per-tenant board id.
func (topco) boardless() {}

// topco aggregates postings from many portfolio companies, so it stays in the source facet.
func (topco) aggregator() {}

// topcoVacancy is the subset of a careers-flight vacancies entry the adapter maps. Location is
// always null in the flight (the ecosystem's roles are global/remote); the body fields are the
// "$undefined" placeholder here and fetched from the detail endpoint instead.
type topcoVacancy struct {
	ID       string `json:"id"`
	Position string `json:"position"`
	Company  struct {
		Name string `json:"name"`
	} `json:"company"`
}

func (s topco) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	flight, err := fetchFlight(ctx, s.http, topcoCareersURL)
	if err != nil {
		return nil, fmt.Errorf("topco: %w", err)
	}
	// The postings are the flight's "vacancies":[…] array.
	vacancies, err := flightArray[topcoVacancy](flight, `"vacancies":`)
	if err != nil {
		return nil, fmt.Errorf("topco: %w", err)
	}

	// Each posting takes one RSC detail fetch for its body; a failed fetch keeps the posting
	// with an empty description rather than dropping it (fetch never returns ok=false for a
	// detail error — only an empty id is dropped, as it would collide on the dedup key).
	return fetchDetails(vacancies, defaultDetailWorkers, func(v topcoVacancy) (Job, bool) {
		if v.ID == "" {
			return Job{}, false
		}
		return Job{
			ExternalID:  v.ID,
			URL:         fmt.Sprintf(topcoJobURL, v.ID),
			Title:       v.Position,
			Company:     firstNonEmpty(v.Company.Name, e.Company),
			Description: sanitizeHTML(s.detailBody(ctx, v.ID)),
		}, true
	}), nil
}

// detailBody fetches a posting's RSC data endpoint (with the "RSC: 1" header that makes top.co
// return the flight rather than HTML) and composes the body from its body/requirements/
// conditions HTML sections. A failed request or a still-lazy "$undefined" placeholder yields an
// empty body, so a posting is never dropped over a missing description.
func (s topco) detailBody(ctx context.Context, id string) string {
	flight, err := s.http.GetTextWithHeaders(ctx, fmt.Sprintf(topcoJobURL, id), map[string]string{"RSC": "1"})
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, marker := range []string{`"body":`, `"requirements":`, `"conditions":`} {
		v, ok := jsonStringField(flight, marker)
		// A "$…" value is an unresolved RSC reference/placeholder ("$undefined"), not content.
		if ok && !strings.HasPrefix(v, "$") {
			b.WriteString(v)
		}
	}
	return b.String()
}

// jsonStringField returns the JSON string value that immediately follows the first occurrence
// of marker (a `"key":` prefix) in s, JSON-decoding its escapes. ok is false when the marker is
// absent or is not followed by a JSON string literal. It scans the literal honoring backslash
// escapes, so an embedded escaped quote does not end it early. Used to pull individual fields
// out of a raw RSC flight stream, which is not wrapped as a decodable document.
func jsonStringField(s, marker string) (string, bool) {
	at := strings.Index(s, marker)
	if at < 0 {
		return "", false
	}
	i := at + len(marker)
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	if i >= len(s) || s[i] != '"' {
		return "", false
	}
	start := i
	for i++; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++ // skip the escaped byte
		case '"':
			var out string
			if err := json.Unmarshal([]byte(s[start:i+1]), &out); err != nil {
				return "", false
			}
			return out, true
		}
	}
	return "", false
}
