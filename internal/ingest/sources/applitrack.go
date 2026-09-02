package sources

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// applitrack adapts AppliTrack, Frontline Education's applicant tracking system for school
// districts. The board is the TENANT path segment of the career site, so a posting linked as
//
//	https://www.applitrack.com/saskatoon/onlineapp/jobpostings/view.asp?AppliTrackJobId=739
//
// belongs to board "saskatoon". The segment is case-insensitive (Saskatoon, SASKATOON and
// saskatoon all answer the same board), which the board-file dedup already folds on.
//
// # This adapter crawls the district's technology categories, never the whole board
//
// That is the load-bearing decision here, and it is not a preference. A school district's ATS is
// almost entirely teaching and school-support work, and freehire's non-technical gate does not
// hold against that vocabulary: it was written against tech-company boards and has no term for
// K-12 classified staff. Measured with the repo's own classify.ConfirmedNonTech over all 86,033
// postings the 2,387 candidate boards were live-listing on 2026-09-02, it turned away 45.5% and
// KEPT 46,855 — crossing guards, lunchroom supervisors, bus monitors, paraeducators, lifeguards,
// assistant football coaches — while classify.IsTech recognised 45 titles in the whole fleet.
// A full crawl would therefore have added tens of thousands of school-support postings to an IT
// catalogue to gain a few dozen technical ones.
//
// AppliTrack has a server-side facet that answers this: every district publishes its own category
// menu and "?category=<name>" filters the listing to it. Restricted to the categories a district
// files IT work under (applitrackTechnology), the same fleet listed 360 postings — computer
// technicians, network and systems administrators, help desk, a cybersecurity analyst — of which
// the non-technical gate turned away 12 (3.3%), and every one of those correctly (a technology
// TEACHER, a custodian, a support paraprofessional). So the crawl is one cheap menu request per
// board plus one listing per technology category, and a district's non-technical postings never
// travel and never cost a detail fetch.
//
// The categories are the DISTRICT'S OWN words, not a platform vocabulary, so the allowlist is
// curated and its exclusions are argued where it is defined.
//
// Traps, all verified live against the platform:
//
//   - Nothing is served as HTML. view.asp is a shell that document.writes a script from
//     Output.asp in the same directory, and Output.asp answers "text/javascript" whose
//     document.write('…') arguments carry the whole page — listing rows, posting bodies and
//     all. So both halves of a crawl are read as raw text, unescaped, and only then parsed
//     (applitrackPage). Output.asp is what this adapter asks for: view.asp merely re-emits it
//     inside a <script>, so reading the shell would mean digging the same payload back out.
//   - The unfiltered listing renders the CATEGORY MENU and no postings, which is why it is what
//     the crawl opens with. "?all=1" is the site's own "All Vacancies" link and is what would
//     list a whole board; nothing here asks for it. No listing paginates in any form.
//     "AppliTrackLayoutMode=condensed" is what keeps a response to its rows: the detailed layout
//     inlines every posting's BODY (737 KB for 14 postings on one board), and an unrecognised
//     layout value is not rejected — it silently serves the detailed one, so a typo there buys
//     every body rather than failing.
//   - There is no feed and no second listing endpoint. rss.asp, rss.xml, feed.asp and
//     JobPostings.asp all 404 under and beside /onlineapp/jobpostings/, and
//     /onlineapp/default.aspx — the other path a posting URL suggests — is the applicant
//     sign-in portal, carrying no posting link at all. Output.asp is the whole API.
//   - The listing STATES a count above its rows ("Viewing All Types (116 openings)") and it
//     counts ROWS, not postings, so it cannot be read as a posting total. A statewide
//     consortium lists one requisition once per participating district and all of those rows
//     carry the SAME AppliTrackJobId — Alaska's pool states 1,229 and links 870 distinct ids.
//     Both the walk and this list collapse them, because they are one requisition with one
//     posting page behind them; it is the number that is not what it looks like.
//   - The pages declare no character set anywhere and are served as windows-1252, so the body
//     goes through charset.NewReader before it is parsed (the hrmdirect case again). Most are
//     pure ASCII; the ones that are not are text pasted in from a word processor, so it is
//     exactly the smart quotes and dashes that would otherwise arrive as U+FFFD.
//   - An id the board no longer carries is answered 200 with an EMPTY posting list rather than
//     a 404, so the posting block is selected by the id that was ASKED for ("p<id>_"): a
//     response about anything else simply has no such node.
//   - The listing carries a posting's title and identity but no body, which only the posting
//     page holds — hence HydratingSource.
//   - The platform states no geography. "Location" is the SITE inside the district a posting
//     belongs to ("Central Maintenance and Operations", "Various Locations", "Petersburg High
//     School"), so it is passed through as what the posting says about itself and nothing more.
//     Measured over 1,004 ingested postings, the location dictionary resolved a city for 19 of
//     them and a country for 4, and the ones it did resolve were as often an artifact as a
//     place: a Pennsylvania programme's acronym "(PATTAN)" reads as Pattan, India. There is
//     nothing better to feed it — no page anywhere on the platform names the district's town —
//     and dropping the field would lose the one thing a candidate needs to place the job.
//   - A steady-state crawl is small and stays small: one menu request per board, one listing per
//     technology category, and a detail request only for a posting the catalogue has not seen.
//     The 12 postings a run the non-technical gate turns away are re-hydrated every crawl —
//     a rejected posting is never stored, so it is never `seen` — and at that size the waste
//     does not need bounding.
type applitrack struct {
	http applitrackHTTP
}

// applitrackHTTP is the transport this adapter needs. Every page of a crawl — the category menu,
// a category's listing, a posting — is read as RAW TEXT rather than through the DOM path: what
// the platform serves is JavaScript, and the markup only exists once its document.write
// arguments have been unescaped.
type applitrackHTTP interface{ TextGetter }

// NewAppliTrack builds the AppliTrack adapter over the given HTTP client.
func NewAppliTrack(c applitrackHTTP) Source { return applitrack{http: c} }

func (applitrack) Provider() string { return "applitrack" }

const applitrackBaseURL = "https://www.applitrack.com"

// applitrackPosting is one row of the listing: the identity of a posting and the title the row
// states. Nothing else is read here — the row's own date and location columns are exactly what
// the posting page restates, so they are read there, where a posting that is being hydrated has
// them anyway.
type applitrackPosting struct {
	id    string
	title string
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (s applitrack) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p applitrackPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have — seen
// reports whether an id is already ingested. A seen posting yields the listing row alone, flagged
// SeenRefresh, so the pipeline refreshes liveness without spending a request or overwriting the
// body hydrated when the posting was new. The title travels with that refresh on purpose: the
// refresh path re-applies the catalogue filter to it, which is how a stored posting the
// dictionary now turns away ages out instead of being kept alive by its own re-listing.
func (s applitrack) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p applitrackPosting) (Job, bool) {
		if seen(p.id) {
			j := p.job(e)
			j.SeenRefresh = true
			return j, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// list reads the board's own category vocabulary and then the postings of the categories that
// district files its technical work under — never the whole board (see the header).
//
// A category that cannot be fetched fails the WHOLE board rather than yielding what the others
// held. The categories are independent slices, not pages of one walk, and this provider is swept
// per company: returning the survivors would read as a shrunken board and close the failed
// slice's postings. Almost every board has exactly one technology category anyway, so the
// partial this refuses is rarely even reachable.
func (s applitrack) list(ctx context.Context, e CompanyEntry) ([]applitrackPosting, error) {
	menu, err := s.page(ctx, applitrackMenuURL(e.Board))
	if err != nil {
		return nil, fmt.Errorf("applitrack: categories %s: %w", e.Board, err)
	}
	var out []applitrackPosting
	listed := map[string]bool{}
	for _, category := range applitrackTechnologyCategories(menu) {
		root, err := s.page(ctx, applitrackCategoryURL(e.Board, category))
		if err != nil {
			return nil, fmt.Errorf("applitrack: listing %s category %q: %w", e.Board, category, err)
		}
		for _, p := range applitrackListing(root) {
			if listed[p.id] {
				continue // a posting filed under two technology categories is still one posting
			}
			listed[p.id] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// detail fetches one posting page and returns the listing row's job with everything that page
// states filled in. It reports ok=false — so the caller skips just this posting — when the page
// cannot be fetched, when it carries no block for the id that was asked for, or when it yields no
// body at all. Deferring a posting by one crawl is recoverable; storing it body-less is not,
// because a stored row is `seen` and so is never hydrated again once it ages past the pipeline's
// hydration-retry window, leaving a posting no search can reach.
func (s applitrack) detail(ctx context.Context, e CompanyEntry, p applitrackPosting) (Job, bool) {
	root, err := s.page(ctx, applitrackDetailURL(e.Board, p.id))
	if err != nil {
		return Job{}, false
	}
	// "p<id>_" is what the platform names a posting's block, and asking for the id this crawl
	// wanted is what makes a response about anything else answer nothing.
	block := firstByID(root, "p"+p.id+"_")
	if block == nil {
		return Job{}, false
	}
	description := sanitizeHTML(applitrackDescription(block))
	if description == "" {
		return Job{}, false
	}

	j := p.job(e)
	j.Location = applitrackField(block, "Location")
	j.Description = description
	// The platform states no work arrangement of its own, so the location text is the only
	// remote signal (never the title, which false-positives on "Remote …" role names). WorkMode
	// stays empty: that field carries structured signal only, and the pipeline reads the
	// location string for itself.
	j.Remote = isRemote(j.Location)
	j.PostedAt = applitrackDate(applitrackField(block, "Date Posted"))
	return j, true
}

// job maps a listing row onto the identity every path shares — the same shape a SeenRefresh
// carries and the base a hydrated posting fills in. The title comes from the LISTING on both
// paths, though the posting page restates it, so a refreshed row and a hydrated one describe
// the posting identically. The employer is the configured entry's: a board is one district.
func (p applitrackPosting) job(e CompanyEntry) Job {
	return Job{
		ExternalID: p.id,
		URL:        applitrackPostingURL(e.Board, p.id),
		Title:      p.title,
		Company:    e.Company,
	}
}

// applitrackDir is a board's job-postings directory, which every URL below hangs off.
func applitrackDir(board string) string {
	return fmt.Sprintf("%s/%s/onlineapp/jobpostings/", applitrackBaseURL, url.PathEscape(board))
}

// applitrackMenuURL is the unfiltered listing, which renders the board's category menu and no
// postings at all — the cheapest response that states the district's own vocabulary.
func applitrackMenuURL(board string) string {
	return applitrackDir(board) + "Output.asp?AppliTrackLayoutMode=condensed"
}

// applitrackCategoryURL is one category's slice of a board. The filter is applied by the site,
// so a district's non-technical postings never travel and never cost a detail fetch.
func applitrackCategoryURL(board, category string) string {
	return applitrackDir(board) + "Output.asp?AppliTrackLayoutMode=condensed&category=" +
		url.QueryEscape(category)
}

// applitrackDetailURL is the payload endpoint one posting's page is built from.
func applitrackDetailURL(board, id string) string {
	return applitrackDir(board) + "Output.asp?AppliTrackJobId=" + url.QueryEscape(id)
}

// applitrackPostingURL is the human-facing permalink stored on the job — the shell page, not the
// payload endpoint the adapter reads, which serves JavaScript to a browser.
func applitrackPostingURL(board, id string) string {
	return applitrackDir(board) + "view.asp?AppliTrackJobId=" + url.QueryEscape(id)
}

// page fetches one Output.asp response and parses the markup its document.write calls emit.
// The character set is settled before parsing: the responses declare none, in the header or
// anywhere in the body, and are served as windows-1252 — exactly the case charset.NewReader
// resolves the way the HTML standard does.
func (s applitrack) page(ctx context.Context, pageURL string) (*html.Node, error) {
	body, err := s.http.GetText(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	decoded, err := charset.NewReader(strings.NewReader(body), "")
	if err != nil {
		return nil, err
	}
	text, err := io.ReadAll(decoded)
	if err != nil {
		return nil, err
	}
	markup, complete := applitrackWritten(string(text))
	if !complete {
		return nil, fmt.Errorf("truncated response: a document.write ran off the end of the body")
	}
	return html.Parse(strings.NewReader(markup))
}

// applitrackWriteCall opens a document.write of a single-quoted string — the one form the
// platform emits its markup through. The response also document.writes an unescape() call for
// its analytics tag, which does not start this way and is therefore not markup this reads.
const applitrackWriteCall = "document.write('"

// applitrackWritten concatenates the markup a response's document.write calls emit, undoing the
// JavaScript string escaping as it goes. A backslash escapes the character after it — in
// practice always the apostrophe the platform's own markup is full of ('class=\'label\”).
//
// complete reports whether every call it read was terminated. A call that runs off the end of
// the body is a response that was cut short, and the markup before the cut PARSES: the walk
// would read the rows that did arrive and the crawl would take an incomplete category for a
// complete one, which on a company-swept provider closes every posting past the cut. There is no
// other symptom to read — GetText caps a body at maxTextBody without saying so, and an origin
// that stops mid-response looks the same — so the caller refuses the page.
func applitrackWritten(body string) (markup string, complete bool) {
	var out strings.Builder
	for {
		_, rest, ok := strings.Cut(body, applitrackWriteCall)
		if !ok {
			return out.String(), true
		}
		body = rest
		for len(body) > 0 && body[0] != '\'' {
			if body[0] == '\\' && len(body) > 1 {
				out.WriteByte(body[1])
				body = body[2:]
				continue
			}
			out.WriteByte(body[0])
			body = body[1:]
		}
		if body == "" {
			return out.String(), false // the call was never closed
		}
	}
}

// applitrackJobIDPattern captures a posting id from the "view" control's href. The listing links
// a posting through a javascript: call whose argument is percent-encoded, so the parameter
// separator arrives there as "%3D" and in an ordinary permalink as "="; accepting both lets one
// pattern read either.
//
// The id is digits, and taking only the digits is deliberate rather than lax. A consortium's
// posting is linked from OUTSIDE the platform as "AppliTrackJobId=6536_37241", where the suffix
// selects which member district's page to land on — and the endpoint ignores it: both spellings
// answer the identical posting, "JobID: 6536" (confirmed live). Keying on the whole token would
// store one requisition once per district that links it. No board's own listing emits the
// composite form: across the 263 committed boards' technology slices, all 367 linked ids are
// plain digits.
var applitrackJobIDPattern = regexp.MustCompile(`(?i)AppliTrackJobId(?:=|%3D)(\d+)`)

// applitrackJobID extracts the native posting id from a posting link, or "" when it carries none.
func applitrackJobID(href string) string {
	return firstSubmatch(applitrackJobIDPattern, href)
}

// applitrackTechnology is the set of category names a district files its IT work under, matched
// case-insensitively against the district's OWN vocabulary — AppliTrack has no shared one, so
// this is a curated list and not a rule. It was drawn from the 4,260 distinct category names the
// 2,387 candidate boards published, and every entry was read live before it went in.
//
// What is NOT here matters as much as what is, because school vocabulary is full of look-alikes
// and each of these was checked against its live postings: "Ed Tech" and "Educational
// Technician" are Maine's words for a classroom paraprofessional; "Career and Technical
// Education", "Technology Education" and "Digital Learning" are teaching subjects and
// instructional coaching; "Media Services" and "Library/Media" are the school librarian;
// "Technical" and "Technical Assistant" were dental assistants, voter registration and
// library aides; and "Professional/Technical" is a bargaining unit that holds a network
// engineer and a diesel mechanic side by side. A mixed category is excluded rather than
// admitted, because nothing downstream can separate it — see the header.
var applitrackTechnology = map[string]bool{
	"technology":                          true,
	"information technology":              true,
	"information technology (it)":         true,
	"informationtechnology":               true,
	"classified - information technology": true,
	"it department":                       true,
	"information services":                true,
	"technology & information services":   true,
	"technology and information systems":  true,
	"technology services":                 true,
	"technology support - district":       true,
	"technology postions":                 true, // the district's own spelling
	"technology/data services":            true,
	"technology - non licensed":           true,
	"technology-certificated":             true,
	"support staff - technology":          true,
	"support and tech":                    true,
	"3 - central office technology":       true,
	"computer & network":                  true,
	"data processing":                     true,
}

// AppliTrackTechnologyCategory reports whether an AppliTrack category name is one a district
// files its IT work under. It is exported for cmd/harvest-boards, which must validate a
// candidate board on the SAME slice the crawl reads — a district with a full board and no
// technology category is not a board worth committing — and one copy of a curated list is the
// only way the two answers cannot drift apart.
func AppliTrackTechnologyCategory(name string) bool {
	return applitrackTechnology[strings.ToLower(strings.TrimSpace(name))]
}

// applitrackCategoryID captures a category's name out of a menu option. The option's value is a
// JavaScript object literal the site's own search script reads — {id:"…",vals:[…]} — and the id
// is the string the listing's "category=" filter takes.
var applitrackCategoryID = regexp.MustCompile(`^\{id:"([^"]*)"`)

// applitrackTechnologyCategories returns the technology categories a board publishes, in the
// order its own menu lists them. A board that names none yields nothing to crawl, which is the
// intended answer and not a failure: the district has no IT opening filed as such.
func applitrackTechnologyCategories(root *html.Node) []string {
	menu := firstByID(root, "AppliTrackSearchCategory")
	if menu == nil {
		return nil
	}
	var out []string
	walk(menu, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "option" {
			return true
		}
		name := firstSubmatch(applitrackCategoryID, attr(n, "value"))
		if name != "" && AppliTrackTechnologyCategory(name) {
			out = append(out, name)
		}
		return true
	})
	return out
}

// applitrackListing reads the listing's posting rows in first-seen order. A row renders its title
// and a "view" control that links the posting; the id lives in that control's href, so a row
// without one is not a posting row.
func applitrackListing(root *html.Node) []applitrackPosting {
	var out []applitrackPosting
	listed := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "span" || !hasClass(n, "title") {
			return true
		}
		view := firstByTag(n, "a")
		if view == nil {
			return true
		}
		id := applitrackJobID(attr(view, "href"))
		if id == "" || listed[id] {
			return true
		}
		listed[id] = true
		out = append(out, applitrackPosting{id: id, title: applitrackRowTitle(n, view)})
		return false // a row never nests another
	})
	return out
}

// applitrackRowTitle reads the posting title out of a listing row: the row's text with the
// trailing "view" control's own label removed.
func applitrackRowTitle(row, view *html.Node) string {
	return strings.TrimSpace(strings.TrimSuffix(textContent(row), textContent(view)))
}

// applitrackField returns the value of the posting block's row carrying the given label (the
// labels are rendered with a trailing colon, and some with stray spaces around it), or "" when
// the block has no such row. It is keyed by label rather than by position because which rows a
// posting states varies: Location, Date Available and Closing Date are each optional, and a
// tenant's own picklist adds more.
func applitrackField(block *html.Node, label string) string {
	var value string
	walk(block, func(n *html.Node) bool {
		if value != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "span" || !hasClass(n, "label") {
			return true
		}
		if strings.TrimSpace(strings.TrimSuffix(textContent(n), ":")) != label {
			return true
		}
		// The value is the next "normal" span in the same row; NextSibling cannot leave the
		// row, so a row whose markup has moved yields no value rather than the next row's.
		for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
			if sib.Type == html.ElementNode && sib.Data == "span" && hasClass(sib, "normal") {
				value = textContent(sib)
				break
			}
		}
		return true
	})
	return value
}

// applitrackDescription returns the rendered body of a posting block. Nothing in the markup names
// it: the block states its fields as list rows — each a label span followed by its value in a
// "normal" span — and then the body in a "normal" span of its own, outside every row. So the body
// is the first such span with no row above it, which is the only thing that tells it from a field
// value.
func applitrackDescription(block *html.Node) string {
	var out string
	walk(block, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "span" || !hasClass(n, "normal") {
			return true
		}
		if applitrackInRow(n, block) {
			return true
		}
		out = innerHTML(n)
		return false
	})
	return out
}

// applitrackInRow reports whether n sits inside one of the posting block's field rows. The walk
// stops at the block, so a list in the body — which the body is full of — is never mistaken for
// one of them.
func applitrackInRow(n, block *html.Node) bool {
	for p := n.Parent; p != nil && p != block; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "li" {
			return true
		}
	}
	return false
}

// applitrackDate parses a posting's publish date, which the platform renders in the US
// month/day/year order with neither part zero-padded ("9/1/2026"). Anything else — an empty row,
// or one of the free-text values the neighbouring date rows carry ("ASAP", "Open until Filled") —
// yields nil, since posted_at is nullable and a wrong date sorts a posting to the top of the
// freshest-first browse.
func applitrackDate(s string) *time.Time {
	return parseLayout("1/2/2006", strings.TrimSpace(s))
}
