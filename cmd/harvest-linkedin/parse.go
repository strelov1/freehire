package main

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
	xhtml "golang.org/x/net/html"
)

// card is one posting as the public search listing presents it. The listing names the employer and
// links its profile, so a company the catalogue covers is dropped before any further request.
type card struct {
	company    string
	profileURL string
	postingURL string
}

var (
	// cardSplitRe is the per-posting anchor the listing repeats. Parsing each fragment on its own means
	// a markup change costs one posting, not the page.
	cardSplitRe = regexp.MustCompile(`data-entity-urn="urn:li:jobPosting:`)
	fullLinkRe  = regexp.MustCompile(`class="base-card__full-link[^"]*"[^>]*href="([^"]+)"`)
	titleRe     = regexp.MustCompile(`class="base-search-card__title"[^>]*>([\s\S]*?)</h3>`)
	subtitleRe  = regexp.MustCompile(`class="base-search-card__subtitle"[^>]*>([\s\S]*?)</h4>`)
	hrefRe      = regexp.MustCompile(`href="([^"]+)"`)
	tagRe       = regexp.MustCompile(`<[^>]+>`)
)

// parseCards extracts the postings from a search listing. A fragment without a title or employer
// profile is dropped, not guessed at: both are needed downstream.
func parseCards(html string) []card {
	fragments := cardSplitRe.Split(html, -1)
	if len(fragments) < 2 {
		return nil
	}
	var out []card
	for _, fragment := range fragments[1:] {
		title := textOf(titleRe, fragment)
		subtitle := matchOf(subtitleRe, fragment)
		if title == "" || subtitle == "" {
			continue
		}
		profile := stripQuery(decodeEntities(matchOf(hrefRe, subtitle)))
		if profile == "" {
			continue
		}
		out = append(out, card{
			company:    clean(subtitle),
			profileURL: profile,
			postingURL: stripQuery(decodeEntities(matchOf(fullLinkRe, fragment))),
		})
	}
	return out
}

// ldJSONRe captures each JSON-LD block on a page. A posting publishes one; a company profile
// publishes one holding an Organization node, sometimes wrapped in an @graph array.
var ldJSONRe = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)

// postingExternalID returns the id the posting carries in the employer's OWN applicant tracking
// system, from its structured metadata, or "" when it publishes none (common, not an error).
//
// This id is what lets a board slug be guessed and then proven: harvest-boards keeps a guessed
// board only when the platform reports a live posting carrying exactly this id.
func postingExternalID(html string) string {
	for _, block := range ldJSONRe.FindAllStringSubmatch(html, -1) {
		var posting struct {
			Identifier struct {
				Value json.RawMessage `json:"value"`
			} `json:"identifier"`
		}
		if err := json.Unmarshal([]byte(block[1]), &posting); err != nil {
			continue
		}
		if v := scalarString(posting.Identifier.Value); v != "" {
			return v
		}
	}
	return ""
}

// profileWebsite returns the employer's website (the Organization node's sameAs), or "" when the
// profile publishes none and resolve has nothing to follow.
func profileWebsite(body string) string {
	root, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	return sources.LinkedInCompanyWebsite(root)
}

// scalarString reads a JSON string, taking the first entry when the field is an array (sameAs may be
// one URL or several). Any other shape yields "".
func scalarString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return strings.TrimSpace(many[0])
	}
	return ""
}

func matchOf(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func textOf(re *regexp.Regexp, s string) string { return clean(matchOf(re, s)) }

func clean(html string) string {
	return decodeEntities(strings.Join(strings.Fields(tagRe.ReplaceAllString(html, " ")), " "))
}

var entities = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&apos;", "'", "&nbsp;", " ",
)

func decodeEntities(s string) string { return entities.Replace(s) }

// stripQuery drops a URL's tracking query and fragment, leaving the identity-bearing path.
func stripQuery(rawURL string) string {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}
