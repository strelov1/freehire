package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

// card is one posting as the public search listing presents it. The listing already names the
// employer and links its profile, which is what lets the run drop a company the catalogue
// already covers before spending a single further request on it.
type card struct {
	company    string
	profileURL string
	postingURL string
}

var (
	// cardSplitRe is the per-posting anchor the listing repeats. Splitting on it and parsing
	// each fragment on its own means a markup change to one card costs that posting, not the
	// page — the failure mode a single whole-page regex has.
	cardSplitRe = regexp.MustCompile(`data-entity-urn="urn:li:jobPosting:`)
	fullLinkRe  = regexp.MustCompile(`class="base-card__full-link[^"]*"[^>]*href="([^"]+)"`)
	titleRe     = regexp.MustCompile(`class="base-search-card__title"[^>]*>([\s\S]*?)</h3>`)
	subtitleRe  = regexp.MustCompile(`class="base-search-card__subtitle"[^>]*>([\s\S]*?)</h4>`)
	hrefRe      = regexp.MustCompile(`href="([^"]+)"`)
	tagRe       = regexp.MustCompile(`<[^>]+>`)
)

// parseCards extracts the postings from a search listing. A fragment without a title or
// without an employer profile is dropped rather than guessed at: both are needed downstream,
// and a card missing them is a card whose markup we no longer understand.
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
// system, published in the posting's structured metadata. It is "" when the posting publishes
// none, which is common and not an error — roughly half of sampled postings carry no id.
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

// profileWebsite returns the employer's own website, published as the Organization node's
// sameAs in the profile's structured metadata. It is "" when the profile publishes none, in
// which case there is nothing for the resolve step to follow.
func profileWebsite(html string) string {
	for _, block := range ldJSONRe.FindAllStringSubmatch(html, -1) {
		var doc struct {
			Type   string          `json:"@type"`
			SameAs json.RawMessage `json:"sameAs"`
			Graph  []struct {
				Type   string          `json:"@type"`
				SameAs json.RawMessage `json:"sameAs"`
			} `json:"@graph"`
		}
		if err := json.Unmarshal([]byte(block[1]), &doc); err != nil {
			continue
		}
		if doc.Type == "Organization" {
			if s := scalarString(doc.SameAs); s != "" {
				return s
			}
		}
		for _, node := range doc.Graph {
			if node.Type != "Organization" {
				continue
			}
			if s := scalarString(node.SameAs); s != "" {
				return s
			}
		}
	}
	return ""
}

// scalarString reads a JSON value that is a string, tolerating the array form the same field
// sometimes takes (sameAs may be one URL or several) by taking the first entry. A number or an
// object yields "" — those are shapes we do not understand and must not guess at.
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
