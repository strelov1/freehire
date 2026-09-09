// Package wikicompany resolves a company's name against Wikidata/Wikipedia's free
// public APIs, accepting a match only when the resolved entity is confidently typed
// as a business or organization (see buildOrganizationCheckQuery), never by
// keyword-scanning its description. It is the client behind the
// company-info-wikipedia-backfill worker (see openspec/changes/company-info-wikipedia-backfill).
package wikicompany

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Match is a company confidently resolved on Wikidata/Wikipedia. Tagline and
// Summary are independently optional: either may be empty if the entity carries
// only one of the two, but Lookup never returns a Match where both are empty.
type Match struct {
	QID     string
	Tagline string // Wikidata's own short description, e.g. "Uranium company based in Western Australia".
	Summary string // The Wikipedia article's lead paragraph, when the entity has an enwiki sitelink.
}

// Client resolves company names against Wikidata/Wikipedia. The three base URLs
// are fields (not constants) so a test can point them at an httptest server.
type Client struct {
	HTTP *http.Client

	WikidataAPIURL string
	SPARQLURL      string
	WikipediaURL   string

	MaxAttempts int
	Backoff     time.Duration
}

// New returns a Client configured against the real Wikidata/Wikipedia hosts.
func New(httpClient *http.Client) *Client {
	return &Client{
		HTTP:           httpClient,
		WikidataAPIURL: "https://www.wikidata.org/w/api.php",
		SPARQLURL:      "https://query.wikidata.org/sparql",
		WikipediaURL:   "https://en.wikipedia.org",
		MaxAttempts:    3,
		Backoff:        2 * time.Second,
	}
}

// Lookup resolves name against Wikidata and returns a Match only when the top
// search candidate is confidently typed as a business/organization. A nil Match
// with a nil error means "no confident match" — that is the expected outcome for
// most names, not an error condition.
func (c *Client) Lookup(ctx context.Context, name string) (*Match, error) {
	qid, description, err := c.searchCandidate(ctx, name)
	if err != nil {
		return nil, err
	}
	if qid == "" {
		return nil, nil
	}
	if !isValidQID(qid) {
		return nil, fmt.Errorf("wikicompany: unexpected entity id shape %q for %q", qid, name)
	}

	isOrg, err := c.isOrganization(ctx, qid)
	if err != nil {
		return nil, err
	}
	if !isOrg {
		return nil, nil
	}

	summary, err := c.wikipediaSummary(ctx, qid)
	if err != nil {
		return nil, err
	}

	if description == "" && summary == "" {
		return nil, nil
	}
	return &Match{QID: qid, Tagline: description, Summary: summary}, nil
}

func (c *Client) searchCandidate(ctx context.Context, name string) (qid, description string, err error) {
	q := url.Values{
		"action":   {"wbsearchentities"},
		"search":   {name},
		"language": {"en"},
		"type":     {"item"},
		"limit":    {"1"},
		"format":   {"json"},
	}
	var body struct {
		Search []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"search"`
	}
	if err := c.getJSON(ctx, c.WikidataAPIURL+"?"+q.Encode(), &body); err != nil {
		return "", "", err
	}
	if len(body.Search) == 0 {
		return "", "", nil
	}
	return body.Search[0].ID, body.Search[0].Description, nil
}

func (c *Client) isOrganization(ctx context.Context, qid string) (bool, error) {
	q := url.Values{
		"query":  {buildOrganizationCheckQuery(qid)},
		"format": {"json"},
	}
	var body struct {
		Boolean bool `json:"boolean"`
	}
	if err := c.getJSON(ctx, c.SPARQLURL+"?"+q.Encode(), &body); err != nil {
		return false, err
	}
	return body.Boolean, nil
}

func (c *Client) wikipediaSummary(ctx context.Context, qid string) (string, error) {
	title, err := c.enwikiTitle(ctx, qid)
	if err != nil || title == "" {
		return "", err
	}
	var body struct {
		Extract string `json:"extract"`
	}
	summaryURL := c.WikipediaURL + "/api/rest_v1/page/summary/" + url.PathEscape(title)
	if err := c.getJSON(ctx, summaryURL, &body); err != nil {
		return "", err
	}
	return body.Extract, nil
}

func (c *Client) enwikiTitle(ctx context.Context, qid string) (string, error) {
	q := url.Values{
		"action":     {"wbgetentities"},
		"ids":        {qid},
		"props":      {"sitelinks"},
		"sitefilter": {"enwiki"},
		"format":     {"json"},
	}
	var body struct {
		Entities map[string]struct {
			Sitelinks struct {
				Enwiki struct {
					Title string `json:"title"`
				} `json:"enwiki"`
			} `json:"sitelinks"`
		} `json:"entities"`
	}
	if err := c.getJSON(ctx, c.WikidataAPIURL+"?"+q.Encode(), &body); err != nil {
		return "", err
	}
	return body.Entities[qid].Sitelinks.Enwiki.Title, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("wikicompany: build request: %w", err)
	}
	req.Header.Set("User-Agent", "freehire-company-info-backfill/1.0 (https://freehire.me)")

	resp, err := doWithRetry(c.HTTP, req, c.MaxAttempts, c.Backoff)
	if err != nil {
		return fmt.Errorf("wikicompany: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("wikicompany: read response body: %w", err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("wikicompany: decode response from %s: %w", rawURL, err)
	}
	return nil
}
