package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
)

// jettycloud adapts JettyCloud's own careers site (jettycloud.com/vacancies), a single-company
// source with no per-tenant board id (boardless). It is here rather than under an ATS because
// the employer runs no ATS a crawl can reach: probed 2026-09-23, its name resolves to nothing
// on greenhouse, lever, huntflow, hurma and teamtailor, and the four vendor hosts that DID
// answer 200 (ashby, recruitee, bamboohr, peopleforce) each served the vendor's own marketing
// page rather than a tenant. The site is the only place these postings exist.
//
// The site is a Next.js app whose GraphQL endpoint is not reachable from outside — every path
// serves the SPA shell — so a posting's fields are read from the Apollo cache Next.js embeds in
// the page's __NEXT_DATA__ script, the same seam alignerr and epam already read.
type jettycloud struct {
	http HTMLGetter
}

const (
	jettyBaseURL    = "https://www.jettycloud.com"
	jettyListingURL = jettyBaseURL + "/vacancies?"
	jettyVacancyPre = "/vacancies/"
)

// NewJettyCloud builds the JettyCloud adapter over the given HTTP client.
func NewJettyCloud(c HTMLGetter) Source { return jettycloud{http: c} }

func (jettycloud) Provider() string { return "jettycloud" }

// jettycloud is single-company, so its config entries carry no board.
func (jettycloud) boardless() {}

func (j jettycloud) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	urls, err := j.vacancyURLs(ctx)
	if err != nil {
		return nil, err
	}

	// A closed vacancy still answers 200 (see detail), so the count here is the listing's,
	// not the crawl's — what this guards is an empty LISTING, which is never an employer with
	// no roles.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return j.detail(ctx, e, u)
	}), nil
}

// vacancyURLs reads the listing and returns every vacancy page it links.
//
// The links are taken from anchors only, never from the page text: the listing styles its
// grid/list view switcher with background-image: url('/vacancies/grid_icon.svg'), so a scan
// matching the path anywhere would collect three icons as vacancies.
//
// A listing that links nothing is a hard error rather than an empty result. The site publishes
// a handful of roles at a time, so "no vacancies" and "the markup moved" look identical in the
// output — and read as the former, one re-templated page retires the whole employer from the
// catalogue on the next unseen sweep.
func (j jettycloud) vacancyURLs(ctx context.Context) ([]string, error) {
	root, err := j.http.GetHTML(ctx, jettyListingURL)
	if err != nil {
		return nil, fmt.Errorf("jettycloud: listing: %w", err)
	}
	base, err := url.Parse(jettyBaseURL + "/")
	if err != nil {
		return nil, fmt.Errorf("jettycloud: base url: %w", err)
	}
	urls := jobLinks(base, root, func(href string) bool {
		path := href
		if u, err := url.Parse(href); err == nil {
			path = u.Path
		}
		return strings.HasPrefix(path, jettyVacancyPre) && len(path) > len(jettyVacancyPre)
	})
	if len(urls) == 0 {
		return nil, fmt.Errorf("jettycloud: listing links no vacancies — the markup changed, or the crawl is being refused")
	}
	return urls, nil
}

// jettyNextData is the slice of the vacancy page's __NEXT_DATA__ payload we read: Apollo's
// normalised cache, keyed "Vacancy:<id>" / "Category:<id>" / … . Only the Vacancy entries are
// decoded, so a cache that gains another entity type costs nothing.
type jettyNextData struct {
	Props struct {
		PageProps struct {
			InitialApolloState map[string]json.RawMessage `json:"initialApolloState"`
		} `json:"pageProps"`
	} `json:"props"`
}

// jettyVacancy is the posting itself. content is Editor.js blocks held as a JSON STRING, not as
// nested JSON — the site stores the editor's own serialisation verbatim.
type jettyVacancy struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

// jettyVacancyKey is the Apollo cache key prefix a posting is stored under.
const jettyVacancyKey = "Vacancy:"

// detail reads one vacancy page. ok is false when the page carries no vacancy, which is not a
// failure: a CLOSED posting keeps its URL, answers 200 with the ordinary layout, and simply
// drops out of the Apollo cache — measured 2026-09-23 on /vacancies/go-developer and
// /vacancies/frontend-developer-billing-team-1, both still linked from search results. Treating
// that page as a posting would write a job with no id and no title; treating it as an error
// would fail a crawl that merely raced a closure. The listing is the source of truth.
func (j jettycloud) detail(ctx context.Context, e CompanyEntry, pageURL string) (Job, bool) {
	root, err := j.http.GetHTML(ctx, pageURL)
	if err != nil {
		return Job{}, false
	}
	v, ok := jettyVacancyOf(root, jettySlugOf(pageURL))
	if !ok || v.ID == 0 || v.Name == "" {
		return Job{}, false
	}
	return Job{
		ExternalID: strconv.Itoa(v.ID),
		URL:        jettyVacancyURL(v.Slug, pageURL),
		Title:      v.Name,
		Company:    e.Company,
		// The site's own `locations` array is empty on every posting sampled, so there is no
		// structured location to carry; the pipeline's dictionaries read the description.
		Description: sanitizeHTML(editorJSHTML(v.Content)),
	}, true
}

// jettyVacancyURL prefers the canonical slug the payload names, falling back to the URL that
// was actually fetched when the payload carries no slug.
func jettyVacancyURL(slug, fetched string) string {
	if slug == "" {
		return fetched
	}
	return jettyBaseURL + jettyVacancyPre + slug
}

// jettyVacancyOf pulls the page's OWN vacancy out of its Apollo cache, identified by the slug
// the page was fetched under.
//
// The cache holds more than one: a posting carries its `related` vacancies, and those are in
// the same normalised store under the same "Vacancy:<id>" key shape. Taking whichever comes
// first is not merely imprecise — Go randomises map iteration, so the adapter would return a
// different posting on each run, and a related entry carries no body, so the description would
// sometimes be empty. Measured against the live site before this was keyed on the slug: two of
// three pages resolved to the same id and one of them had no description.
//
// A page whose cache names no vacancy matching its slug yields nothing, which is how a closed
// posting is recognised (see detail).
func jettyVacancyOf(root *xhtml.Node, slug string) (jettyVacancy, bool) {
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return jettyVacancy{}, false
	}
	var data jettyNextData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return jettyVacancy{}, false
	}
	for key, entry := range data.Props.PageProps.InitialApolloState {
		if !strings.HasPrefix(key, jettyVacancyKey) {
			continue
		}
		var v jettyVacancy
		if json.Unmarshal(entry, &v) != nil {
			continue
		}
		if v.Slug == slug {
			return v, true
		}
	}
	return jettyVacancy{}, false
}

// jettySlugOf returns the vacancy slug a page URL addresses, or "" when the URL names none.
func jettySlugOf(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(u.Path, jettyVacancyPre), "/")
}

// editorJSBlock is one block of the editor's document. data is left raw because its shape is
// per-type, and only the handled types are decoded any further.
type editorJSBlock struct {
	Type string `json:"type"`
	Data struct {
		Text  string   `json:"text"`
		Level int      `json:"level"`
		Items []string `json:"items"`
	} `json:"data"`
}

// editorJSHTML renders Editor.js blocks as the HTML the rest of the pipeline expects.
//
// An unrecognised block yields nothing and stops nothing: these documents are authored by hand
// in the site's own editor, so a new block type is a content decision rather than a crawl
// failure, and dropping the whole description over one image carousel would lose the posting.
// A block's `text` already carries inline HTML (<b>, <a>, &nbsp;), so it is passed through for
// the caller to sanitize rather than escaped here; a list ITEM is escaped, since it is plain
// text the editor stores unmarked.
func editorJSHTML(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	var blocks []editorJSBlock
	if json.Unmarshal([]byte(content), &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, blk := range blocks {
		switch blk.Type {
		case "paragraph":
			if blk.Data.Text != "" {
				b.WriteString("<p>" + blk.Data.Text + "</p>")
			}
		case "header":
			if blk.Data.Text != "" {
				tag := editorJSHeaderTag(blk.Data.Level)
				b.WriteString("<" + tag + ">" + blk.Data.Text + "</" + tag + ">")
			}
		case "list":
			if len(blk.Data.Items) > 0 {
				b.WriteString("<ul>")
				for _, item := range blk.Data.Items {
					b.WriteString("<li>" + html.EscapeString(item) + "</li>")
				}
				b.WriteString("</ul>")
			}
		}
	}
	return b.String()
}

// editorJSHeaderTag maps the editor's heading level onto a tag, clamping anything outside
// h2..h4 — the document is a fragment inside our own page, so its headings never outrank it.
func editorJSHeaderTag(level int) string {
	if level < 2 || level > 4 {
		return "h3"
	}
	return "h" + strconv.Itoa(level)
}
