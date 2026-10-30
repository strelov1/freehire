package sources

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
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
// # This adapter crawls the whole board
//
// A school district's ATS is almost entirely teaching and school-support work. It used to be
// crawled through an allowlist of the district's technology categories only, back when the
// catalogue was IT-only; the catalogue is broader now, and the pipeline's own dictionaries decide
// what a posting is. So the crawl reads the site's own "All Vacancies" listing ("?all=1"): one
// request per board, measured 2026-10-20 at 8-130 postings and 0.9-1.6 s on three live boards.
//
// Traps, all verified live against the platform:
//
//   - Nothing is served as HTML. view.asp is a shell that document.writes a script from
//     Output.asp in the same directory, and Output.asp answers "text/javascript" whose
//     document.write('…') arguments carry the whole page — listing rows, posting bodies and
//     all. So both halves of a crawl are read as raw text, unescaped, and only then parsed
//     (applitrackPage). Output.asp is what this adapter asks for: view.asp merely re-emits it
//     inside a <script>, so reading the shell would mean digging the same payload back out.
//   - The unfiltered listing renders the CATEGORY MENU and no postings. "?all=1" is the site's own
//     "All Vacancies" link and is what lists a whole board. No listing paginates in any form.
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
//     goes through charset.NewReader before it is parsed. Most are
//     pure ASCII; the ones that are not are text pasted in from a word processor, so it is
//     exactly the smart quotes and dashes that would otherwise arrive as U+FFFD.
//   - An id the board no longer carries is answered 200 with an EMPTY posting list rather than
//     a 404, so the posting block is selected by the id that was ASKED for ("p<id>_"): a
//     response about anything else simply has no such node.
//   - The listing carries a posting's title and identity but no body, which only the posting
//     page holds — hence HydratingSource.
//   - A posting's "Location" is the SITE inside the district it belongs to ("Central Maintenance
//     and Operations", "Various Locations", "Petersburg High School"), never a town. What does
//     place a board is its footer: every page links the district's state on the platform's
//     sister board, K12JobSpot ("You can also view <a href='…/illinois-teaching-jobs'>Illinois
//     teaching jobs</a>"), rendered from the tenant's own state setting; a Canadian tenant's
//     reads "Canad- Saskatchewan". So the state is appended to the site and its country set as
//     the posting's structured country (applitrackBoardState) — "Georgia" there is the state,
//     which a location line alone could never say. A state no dictionary knows (one tenant's
//     renders as "Californicredential or") places nothing. Before this the site went out alone
//     and ~96% of the open postings had no country (2026-10-29); the few the dictionary did
//     place were as often an artifact as a place ("(PATTAN)" read as Pattan, India).
//   - A steady-state crawl is small and stays small: one listing request per board and a detail
//     request only for a posting the catalogue has not seen.
type applitrack struct {
	http applitrackHTTP
}

// applitrackHTTP is the transport this adapter needs. Every page of a crawl — the board's listing
// and a posting — is read as RAW TEXT rather than through the DOM path: what the platform serves
// is JavaScript, and the markup only exists once its document.write arguments have been unescaped.
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

// applitrackState is the state or province a board's district is in, as its footer names it:
// the display name and its country code. Zero when the footer names none the dictionary knows.
type applitrackState struct {
	name, country string
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: it hydrates
// every listed posting. FetchNew is the hydrating path ingest prefers.
func (s applitrack) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, state, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p applitrackPosting) (Job, bool) {
		return s.detail(ctx, e, p, state)
	}), nil
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have — seen
// reports whether an id is already ingested. A seen posting yields the listing row alone, flagged
// SeenRefresh, so the pipeline refreshes liveness without spending a request or overwriting the
// body hydrated when the posting was new. The title travels with that refresh on purpose: the
// refresh path re-applies the catalogue filter to it, which is how a stored posting the
// dictionary now turns away ages out instead of being kept alive by its own re-listing.
func (s applitrack) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, state, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p applitrackPosting) (Job, bool) {
		if seen(p.id) {
			j := p.job(e, state)
			j.SeenRefresh = true
			return j, true
		}
		return s.detail(ctx, e, p, state)
	}), nil
}

// list reads the board's whole listing (see the header) and the state its footer names.
func (s applitrack) list(ctx context.Context, e CompanyEntry) ([]applitrackPosting, applitrackState, error) {
	root, err := s.page(ctx, applitrackAllURL(e.Board))
	if err != nil {
		return nil, applitrackState{}, fmt.Errorf("applitrack: listing %s: %w", e.Board, err)
	}
	return applitrackListing(root), applitrackBoardState(root), nil
}

// detail fetches one posting page and returns the listing row's job with everything that page
// states filled in. It reports ok=false — so the caller skips just this posting — when the page
// cannot be fetched, when it carries no block for the id that was asked for, or when it yields no
// body at all. Deferring a posting by one crawl is recoverable; storing it body-less is not,
// because a stored row is `seen` and so is never hydrated again once it ages past the pipeline's
// hydration-retry window, leaving a posting no search can reach.
func (s applitrack) detail(ctx context.Context, e CompanyEntry, p applitrackPosting, state applitrackState) (Job, bool) {
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

	site := applitrackField(block, "Location")
	j := p.job(e, state)
	j.Location = joinNonEmpty(site, state.name)
	j.Description = description
	// The platform states no work arrangement of its own, so the site text is the only remote
	// signal (never the title, which false-positives on "Remote …" role names). WorkMode stays
	// empty: that field carries structured signal only, and the pipeline reads the location
	// string for itself.
	j.Remote = isRemote(site)
	j.PostedAt = applitrackDate(applitrackField(block, "Date Posted"))
	return j, true
}

// job maps a listing row onto the identity every path shares — the same shape a SeenRefresh
// carries and the base a hydrated posting fills in. The title comes from the LISTING on both
// paths, though the posting page restates it, so a refreshed row and a hydrated one describe
// the posting identically. The employer is the configured entry's: a board is one district,
// and so is its state.
func (p applitrackPosting) job(e CompanyEntry, state applitrackState) Job {
	j := Job{
		ExternalID: p.id,
		URL:        applitrackPostingURL(e.Board, p.id),
		Title:      p.title,
		Company:    e.Company,
	}
	if state.country != "" {
		j.Countries = []string{state.country}
	}
	return j
}

// applitrackDir is a board's job-postings directory, which every URL below hangs off.
func applitrackDir(board string) string {
	return fmt.Sprintf("%s/%s/onlineapp/jobpostings/", applitrackBaseURL, url.PathEscape(board))
}

// applitrackAllURL is the board's "All Vacancies" listing — every posting the district carries.
func applitrackAllURL(board string) string {
	return applitrackDir(board) + "Output.asp?AppliTrackLayoutMode=condensed&all=1"
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
// would read the rows that did arrive and the crawl would take an incomplete listing for a
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
// store one requisition once per district that links it.
var applitrackJobIDPattern = regexp.MustCompile(`(?i)AppliTrackJobId(?:=|%3D)(\d+)`)

// applitrackJobID extracts the native posting id from a posting link, or "" when it carries none.
func applitrackJobID(href string) string {
	return firstSubmatch(applitrackJobIDPattern, href)
}

// applitrackListing reads the listing's posting rows in first-seen order. A row renders its title
// and a "view" control that links the posting; the id lives in that control's href, so a row
// without one is not a posting row.
func applitrackListing(root *html.Node) []applitrackPosting {
	var out []applitrackPosting
	listed := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "span" || !HasClass(n, "title") {
			return true
		}
		view := firstByTag(n, "a")
		if view == nil {
			return true
		}
		id := applitrackJobID(Attr(view, "href"))
		if id == "" || listed[id] {
			return true
		}
		listed[id] = true
		out = append(out, applitrackPosting{id: id, title: applitrackRowTitle(n, view)})
		return false // a row never nests another
	})
	return out
}

// applitrackStateLink captures the anchor text of the footer's K12JobSpot link — "Illinois" out
// of "<a href='http://www.k12jobspot.com/illinois-teaching-jobs'>Illinois teaching jobs</a>".
var applitrackStateLink = regexp.MustCompile(`(?i)k12jobspot\.com/[^'">]*['"]?\s*>\s*([^<]+?)\s+teaching jobs\s*</a>`)

// applitrackBoardState reads the state the page footer names (see the header). The link sits in
// a <noscript>, which the parser keeps as raw text rather than elements, so it is matched in that
// text. A Canadian tenant's name carries a "Canad-" prefix, and the province after it must then
// resolve to Canada. Anything the dictionary does not know yields the zero state.
func applitrackBoardState(root *html.Node) applitrackState {
	var raw string
	walk(root, func(n *html.Node) bool {
		if raw != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "noscript" {
			raw = firstSubmatch(applitrackStateLink, textContent(n))
			return false
		}
		return true
	})
	name, canadian := raw, false
	if prefix, rest, ok := strings.Cut(raw, "-"); ok && strings.HasPrefix(strings.ToLower(prefix), "canad") {
		name, canadian = strings.TrimSpace(rest), true
	}
	country := location.StateCountry(name)
	if country == "" || (canadian && country != "ca") {
		return applitrackState{}
	}
	return applitrackState{name: name, country: country}
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
		if n.Type != html.ElementNode || n.Data != "span" || !HasClass(n, "label") {
			return true
		}
		if strings.TrimSpace(strings.TrimSuffix(textContent(n), ":")) != label {
			return true
		}
		// The value is the next "normal" span in the same row; NextSibling cannot leave the
		// row, so a row whose markup has moved yields no value rather than the next row's.
		for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
			if sib.Type == html.ElementNode && sib.Data == "span" && HasClass(sib, "normal") {
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
		if n.Type != html.ElementNode || n.Data != "span" || !HasClass(n, "normal") {
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
