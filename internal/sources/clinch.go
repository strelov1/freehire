package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/location"
)

// clinch adapts ClinchTalent / career-pages.com career sites (e.g. careers.withwaymo.com).
// The board is the career-site host. Unlike the other sitemap adapters (successfactors,
// meta) it CANNOT fetch the job detail pages: ClinchTalent fronts them with an AWS WAF
// "challenge" action (an interactive JS proof-of-work), so a plain or fingerprint-spoofed
// GET is served a 202 challenge, never the posting. The publicly served sitemap is the only
// reachable surface, so a posting is reconstructed from its URL slug alone:
//
//	/jobs/{title-words}-{city}-{state}-{country}[-{uuid}]
//
// Title and location are split out of the slug (clinchSplitSlug); the description is left
// empty (the detail page is unreachable), so enrichment and the description-derived facets
// stay blank for these jobs while the title-derived facets and the geography facet — the
// location dictionary resolves the slug's place tail — still populate. This mirrors the
// sitemap-only approach the ClinchTalent platform forces on every crawler.
type clinch struct {
	http XMLGetter
}

// NewClinch builds the ClinchTalent adapter over the given XML client.
func NewClinch(c XMLGetter) Source { return clinch{http: c} }

func (clinch) Provider() string { return "clinch" }

func (c clinch) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("https://%s/sitemap.xml", e.Board)
	sitemap, err := getSitemap(ctx, c.http, url)
	if err != nil {
		return nil, fmt.Errorf("clinch: sitemap %s: %w", e.Board, err)
	}

	var jobs []Job
	for _, entry := range sitemap.URLs {
		slug := clinchJobSlug(entry.Loc)
		if slug == "" {
			continue // not a /jobs/ URL (marketing page) or empty slug
		}
		title, loc := clinchSplitSlug(slug)
		jobs = append(jobs, Job{
			ExternalID:  clinchExternalID(slug),
			URL:         entry.Loc,
			Title:       title,
			Company:     e.Company,
			Location:    loc,
			Description: "", // detail page is AWS-WAF-blocked; nothing to fetch
			Remote:      isRemote(loc),
			PostedAt:    parseDate(entry.LastMod),
		})
	}
	return jobs, nil
}

// clinchJobSlug returns the posting slug from a career-site URL — the path segment after
// "/jobs/" — or "" for a non-job URL (the sitemap also lists marketing pages).
func clinchJobSlug(loc string) string {
	const marker = "/jobs/"
	i := strings.Index(loc, marker)
	if i < 0 {
		return ""
	}
	return strings.Trim(loc[i+len(marker):], "/")
}

// clinchUUIDSuffix matches a trailing "-{uuid}" that ClinchTalent appends to some slugs.
var clinchUUIDSuffix = regexp.MustCompile(`-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// clinchExternalID is the stable dedup id for a slug: the trailing UUID when present (it
// survives a title edit that reshuffles the slug words), else the whole slug.
func clinchExternalID(slug string) string {
	if m := clinchUUIDSuffix.FindStringSubmatch(slug); m != nil {
		return m[1]
	}
	return slug
}

// clinchSplitSlug splits a posting slug into a display title and a location string. The
// location is the slug's place tail ("{city}-{state}-{country}"); the cut point is the
// leftmost token that starts a place the location dictionary recognizes (a single- or
// multi-word city such as "warsaw" or "mountain-view") with a country somewhere after it.
// Bare two-letter title tokens (e.g. "be" for Backend, which is also Belgium's ISO code)
// never start the location because the dictionary is consulted by place NAME, length > 2.
// When nothing resolves, the whole slug becomes the title and the location is empty.
func clinchSplitSlug(slug string) (title, locationText string) {
	clean := clinchUUIDSuffix.ReplaceAllString(slug, "")
	tokens := strings.Split(clean, "-")

	cut := clinchLocationStart(tokens)
	if cut < 0 {
		return titleCase(strings.Join(tokens, " ")), ""
	}
	return titleCase(strings.Join(tokens[:cut], " ")), clinchFormatLocation(tokens[cut:])
}

// clinchFormatLocation joins the location tail into a comma-separated, title-cased string,
// keeping a recognized multi-word place ("mountain view", "united states") together as one
// segment while a token the dictionary does not resolve ("masovian", "voivodeship") stands
// alone. The result reads cleanly AND re-resolves through location.Parse for the geo facet.
func clinchFormatLocation(tokens []string) string {
	var segs []string
	for i := 0; i < len(tokens); {
		// Take the SHORTEST window that resolves, so a multi-word place name ("mountain
		// view") groups but a city+state phrase ("mountain view california", which
		// resolves only via its trailing subdivision) does not swallow the next segment.
		n := 1
		for k := 2; k <= 3 && i+k <= len(tokens); k++ {
			if clinchResolves(strings.Join(tokens[i:i+1], " ")) {
				break // single token already resolves — don't extend
			}
			if clinchResolves(strings.Join(tokens[i:i+k], " ")) {
				n = k
				break
			}
		}
		segs = append(segs, titleCase(strings.Join(tokens[i:i+n], " ")))
		i += n
	}
	return strings.Join(segs, ", ")
}

// clinchResolves reports whether a candidate place phrase resolves to geography. The
// length > 2 guard keeps a bare ISO code from matching as a country.
func clinchResolves(cand string) bool {
	if len(cand) <= 2 {
		return false
	}
	geo := location.Parse(cand)
	return len(geo.Countries) > 0 || len(geo.Regions) > 0
}

// clinchLocationStart returns the index of the first token that begins the location tail,
// or -1 when no location resolves. A valid start is a token (or 2–3 token window, for
// multi-word cities) the dictionary resolves to geography, with a country anywhere in the
// remaining tail. Index 0 is never a start, so the title is never empty.
func clinchLocationStart(tokens []string) int {
	for i := 1; i < len(tokens); i++ {
		if !clinchPlaceStartsAt(tokens, i) {
			continue
		}
		if len(location.Parse(strings.Join(tokens[i:], ", ")).Countries) == 0 {
			continue
		}
		return i
	}
	return -1
}

// clinchPlaceStartsAt reports whether a recognized place begins at token i, trying a 1-,
// 2-, then 3-word window so multi-word cities ("mountain view", "new york city") anchor on
// their first word. Candidates of length ≤ 2 are skipped so a bare ISO code embedded in the
// title cannot masquerade as a country.
func clinchPlaceStartsAt(tokens []string, i int) bool {
	for k := 1; k <= 3 && i+k <= len(tokens); k++ {
		if clinchResolves(strings.Join(tokens[i:i+k], " ")) {
			return true
		}
	}
	return false
}

// titleCase upper-cases the first letter of each space-separated word, leaving the rest as
// is. The slug tokens are lowercase ASCII, so this is enough for a readable display title
// without pulling in a Unicode caser.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
