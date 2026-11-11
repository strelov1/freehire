package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// betterteam adapts Betterteam career sites. The board is the tenant subdomain (e.g.
// "110grill"), so the career site is "<board>.betterteam.com". The root page lists each
// posting at /<slug>-<id> (e.g. /bartender-21); each job page carries a schema.org
// JobPosting ld+json block, so the description comes from a per-job detail fetch
// (bounded-concurrency), like the other schema.org detail adapters (freshteam/softgarden).
//
// The listing is a single page: Betterteam renders the whole board on the site root, so a
// tenant with more postings than the root holds is truncated. In practice Betterteam
// tenants (small businesses) are small.
type betterteam struct {
	http HTMLGetter
}

// NewBetterteam builds the Betterteam adapter over the given HTTP client.
func NewBetterteam(c HTMLGetter) Source { return betterteam{http: c} }

func (betterteam) Provider() string { return "betterteam" }

func (b betterteam) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries the scheme+host; the listing's relative "/<slug>-<id>" hrefs resolve
	// against it into fetchable absolute URLs.
	base, err := url.Parse(fmt.Sprintf("https://%s.betterteam.com/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("betterteam: board %q: %w", e.Board, err)
	}

	root, err := b.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("betterteam: listing %s: %w", e.Board, err)
	}
	slugs, native, err := btNativeSlugs(root)
	if err != nil {
		return nil, fmt.Errorf("betterteam: listing %s: %w", e.Board, err)
	}
	type link struct{ id, url string }
	var links []link
	if native {
		for _, slug := range slugs {
			u := base.ResolveReference(&url.URL{Path: "/" + slug, RawPath: "/" + url.PathEscape(slug)})
			links = append(links, link{slug, u.String()})
		}
	} else {
		for _, u := range jobLinks(base, root, func(href string) bool { return btJobID(href) != "" }) {
			links = append(links, link{btJobID(u), u})
		}
	}
	// The live CareersPage loader stops at 300 entries (Turing, 2026-11-11)
	// without a total or a continuation cursor. Even the company-scoped sweep
	// must not mistake that prefix for everything the board published.
	if len(links) >= 300 {
		return nil, fmt.Errorf("betterteam: listing %s reached the 300-job ceiling without an end proof", e.Board)
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(links, defaultDetailWorkers, func(l link) (Job, bool) {
		return b.detail(ctx, e, l.url, l.id)
	}), nil
}

var btStreamPayload = regexp.MustCompile(`streamController\.enqueue\s*\(\s*("(?:\\.|[^"\\])*")\s*\)`)

// Modern Betterteam pages publish the native slugs in the CareersPage loader.
// These are identities, not a guessed URL shape: some have parentheses or no
// numeric suffix. Read only the required path in React Router's reference table.
func btNativeSlugs(root *html.Node) ([]string, bool, error) {
	var encoded string
	found := false
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "script" && encoded == "" {
			text := textContent(n)
			found = found || strings.Contains(text, "streamController.enqueue")
			encoded = firstSubmatch(btStreamPayload, text)
		}
		return true
	})
	if encoded == "" {
		if found {
			return nil, true, fmt.Errorf("invalid native listing stream")
		}
		return nil, false, nil
	}
	var payload string
	var nodes []json.RawMessage
	if json.Unmarshal([]byte(encoded), &payload) != nil || json.Unmarshal([]byte(payload), &nodes) != nil {
		return nil, true, fmt.Errorf("invalid native listing payload")
	}
	index := 0
	for _, key := range []string{"loaderData", "routes/CareersPage", "jobs"} {
		var err error
		index, err = btPayloadField(nodes, index, key)
		if err != nil {
			return nil, true, err
		}
	}
	var refs []int
	if len(nodes[index]) == 0 || nodes[index][0] != '[' || json.Unmarshal(nodes[index], &refs) != nil {
		return nil, true, fmt.Errorf("native listing has no jobs array")
	}
	slugs := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		index, err := btPayloadField(nodes, ref, "slug")
		if err != nil {
			return nil, true, err
		}
		var slug string
		if json.Unmarshal(nodes[index], &slug) != nil || strings.TrimSpace(slug) == "" || seen[slug] {
			return nil, true, fmt.Errorf("native listing has an invalid or repeated slug")
		}
		seen[slug] = true
		slugs = append(slugs, slug)
	}
	return slugs, true, nil
}

func btPayloadField(nodes []json.RawMessage, index int, name string) (int, error) {
	if index < 0 || index >= len(nodes) {
		return 0, fmt.Errorf("native listing reference is out of bounds")
	}
	var object map[string]int
	if err := json.Unmarshal(nodes[index], &object); err != nil {
		return 0, fmt.Errorf("native listing field %s is not an object", name)
	}
	for key, ref := range object {
		keyIndex, err := strconv.Atoi(strings.TrimPrefix(key, "_"))
		if !strings.HasPrefix(key, "_") || err != nil || keyIndex < 0 || keyIndex >= len(nodes) {
			return 0, fmt.Errorf("native listing key is invalid")
		}
		var field string
		if json.Unmarshal(nodes[keyIndex], &field) != nil {
			return 0, fmt.Errorf("native listing key is not a string")
		}
		if field == name {
			if ref < 0 || ref >= len(nodes) {
				return 0, fmt.Errorf("native listing field %s is absent", name)
			}
			return ref, nil
		}
	}
	return 0, fmt.Errorf("native listing has no %s", name)
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (b betterteam) detail(ctx context.Context, e CompanyEntry, jobURL, id string) (Job, bool) {
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := b.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p btPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()
	// Betterteam's JobPosting carries no explicit remote flag, so remote is inferred from the
	// location only (never the title, which false-positives on "Remote …" role names).
	remote := isRemote(location)

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// btJobIDPattern captures the native posting id from a job URL's path. A posting permalink
// is a single "/<slug>-<id>" segment ending in the numeric id (e.g. /bartender-21); the id
// stored is the whole slug-id segment, which Betterteam echoes as the ld+json identifier.
var btJobIDPattern = regexp.MustCompile(`^/([^/]+-\d+)$`)

// btJobID extracts the native posting id (the slug-id path segment) from a job URL, or ""
// when the URL is not a job permalink. Only the path is matched, so absolute and relative
// hrefs on the tenant host both resolve.
func btJobID(u string) string {
	p := u
	if parsed, err := url.Parse(u); err == nil {
		p = parsed.EscapedPath()
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	id, err := url.PathUnescape(firstSubmatch(btJobIDPattern, p))
	if err != nil {
		return ""
	}
	return id
}

// btPosting is the schema.org JobPosting decoded from a Betterteam job page's
// application/ld+json block. jobLocation is a single Place object.
type btPosting struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	DatePosted  string      `json:"datePosted"`
	JobLocation schemaPlace `json:"jobLocation"`
}
