package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// meincheckin adapts CHECK-IN (mein-check-in.de), a German applicant portal used mostly by the
// public sector and the savings-bank and cooperative-bank groups: Sparkassen, Volksbanken,
// insurers, district and city councils. A tenant is a path segment on one of three branded hosts
// (sparkasse., vr. and www.mein-check-in.de). www serves its own tenants and redirects the
// sparkasse. ones to their host; the vr. tenants (cooperative banks, often keyed by their bank
// code, 85095004) answer only on vr. and are a 404 on www. No slug was measured on both, so the
// board is the bare tenant slug (ksk-ratzeburg, sv-informatik, aalen, 85095004) and the start
// page is asked of www first, then of vr.
//
// The tenant's start page is server-rendered and its menu links every open position
// (/<tenant>/position-<id>), grouped under the tenant's sections (Stellenangebote, Ausbildung,
// Praktika). A position page carries a schema.org JobPosting as MICRODATA, not ld+json; the
// listing carries only titles, so this is a HydratingSource. See the "CHECK-IN traps" section of
// AGENTS.md.
type meincheckin struct {
	http meincheckinHTTP
}

type meincheckinHTTP interface {
	HTMLGetter
	HTMLResolvedGetter
}

// NewMeinCheckIn builds the adapter over the given HTTP client.
func NewMeinCheckIn(c meincheckinHTTP) Source { return meincheckin{http: c} }

func (meincheckin) Provider() string { return "meincheckin" }

// meincheckinBoard is the shape of a tenant slug: lower-case words, or a bank's routing number.
var meincheckinBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// meincheckinOpenApplication matches the open-application entry most tenants list among their
// positions (~7% of measured entries). It is not a vacancy.
var meincheckinOpenApplication = regexp.MustCompile(`(?i)initiativbewerbung`)

type meincheckinPosition struct {
	id    string
	title string
	url   string
}

// externalID qualifies the position id with its tenant: ids are numbered per HOST, and the
// sparkasse. and vr. hosts number in the same range (120473 and 120315 both exist), so a bare id
// could name two postings.
func meincheckinExternalID(board, id string) string { return board + "/" + id }

// Fetch reads the listing and every position's page.
func (s meincheckin) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew spends a page request only on positions the catalogue does not hold; a seen one is
// re-listed as a SeenRefresh, the menu proving it is still open.
func (s meincheckin) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, positions, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(positions, defaultDetailWorkers, func(p meincheckinPosition) (Job, bool) {
		id := meincheckinExternalID(board, p.id)
		if seen(id) {
			return Job{ExternalID: id, URL: p.url, Title: p.title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, board, p)
	}), nil
}

// meincheckinStartHosts are the hosts a tenant's start page is asked of, in order; see the type.
var meincheckinStartHosts = []string{"https://www.mein-check-in.de/", "https://vr.mein-check-in.de/"}

// list reads the tenant's start page. A tenant unknown to every start host is gone; a start page
// that landed anywhere but the tenant is not its menu.
func (s meincheckin) list(ctx context.Context, rawBoard string) (string, []meincheckinPosition, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !meincheckinBoard.MatchString(board) {
		return "", nil, fmt.Errorf("meincheckin: board %q is not a tenant slug", rawBoard)
	}
	var root *xhtml.Node
	var final string
	var err error
	for _, host := range meincheckinStartHosts {
		root, final, err = s.http.GetHTMLResolved(ctx, host+board)
		var se *StatusError
		if !errors.As(err, &se) || se.Code != http.StatusNotFound {
			break
		}
	}
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", nil, fmt.Errorf("meincheckin: board %q: %w", board, ErrBoardGone)
		}
		return "", nil, fmt.Errorf("meincheckin: board %s: %w", board, err)
	}
	base, err := url.Parse(final)
	if err != nil || !strings.HasSuffix(base.Hostname(), "mein-check-in.de") ||
		strings.Trim(base.Path, "/") != board {
		return "", nil, fmt.Errorf("meincheckin: board %s: start page landed on %q", board, final)
	}
	link := regexp.MustCompile(`^/` + regexp.QuoteMeta(board) + `/position-(\d+)$`)
	seenIDs := map[string]bool{}
	var out []meincheckinPosition
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "a" || !HasClass(n, "mainmenu_position_link") {
			return true
		}
		id := firstSubmatch(link, Attr(n, "href"))
		title := strings.Join(strings.Fields(textContent(n)), " ")
		if id == "" || seenIDs[id] || title == "" || meincheckinOpenApplication.MatchString(title) {
			return false
		}
		seenIDs[id] = true
		out = append(out, meincheckinPosition{
			id: id, title: title,
			url: base.Scheme + "://" + base.Host + "/" + board + "/position-" + id,
		})
		return false
	})
	return board, out, nil
}

// detail fetches one position page. A 404 is the platform's answer that the position closed and
// drops it; any other failure, or a page without its JobPosting, is an unreadableDetail marker —
// the page is the only source of the body.
func (s meincheckin) detail(ctx context.Context, e CompanyEntry, board string, p meincheckinPosition) (Job, bool) {
	id := meincheckinExternalID(board, p.id)
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, p.url, e.Company), true
		}
		return Job{}, false
	}
	j, ok := meincheckinJob(root, e, p)
	if !ok {
		return unreadableDetail(id, p.url, e.Company), true
	}
	j.ExternalID = id
	return j, true
}

// meincheckinJob maps a position page's microdata. Every property is read inside its own scope:
// "name" occurs in the identifier, the organisation and the place alike.
func meincheckinJob(root *xhtml.Node, e CompanyEntry, p meincheckinPosition) (Job, bool) {
	posting := itemscopeOf(root, "http://schema.org/JobPosting")
	if posting == nil {
		return Job{}, false
	}
	var company, locality, country string
	if org := firstItemprop(posting, "hiringOrganization"); org != nil {
		company = strings.TrimSpace(itempropText(org, "name"))
	}
	if addr := firstItemprop(posting, "address"); addr != nil {
		// Some tenants hyphenate place names for line breaking (a soft hyphen in "Eislingen/Fils").
		locality = strings.ReplaceAll(strings.TrimSpace(itempropText(addr, "addressLocality")), "\u00ad", "")
		country = strings.TrimSpace(itempropText(addr, "addressCountry"))
	}
	j := Job{
		URL:            p.url,
		Title:          firstNonEmpty(meincheckinTitle(root), p.title),
		Company:        firstNonEmpty(company, e.Company),
		Location:       locality,
		Description:    sanitizeHTML(itempropHTML(posting, "description")),
		PostedAt:       parseRFC3339(strings.TrimSpace(itempropText(posting, "datePosted"))),
		EmploymentType: meincheckinEmploymentType(itempropText(posting, "employmentType")),
		Countries:      countryFromCode(country),
	}
	return j, j.Description != ""
}

// meincheckinTitle reads the title from og:title ("<title> | <tenant> Onlinebewerbung"). The
// microdata title cannot serve: its <h2> also holds the "Jetzt hier bewerben" button.
func meincheckinTitle(root *xhtml.Node) string {
	t := strings.TrimSpace(metaProperty(root, "og:title"))
	if i := strings.LastIndex(t, " | "); i > 0 {
		t = t[:i]
	}
	return strings.Join(strings.Fields(t), " ")
}

// meincheckinEmploymentType maps the JobPosting's employmentType, which is NOT an employment type
// here: it names the tenant's menu section the position sits in ("stellenangebote", "ausbildung",
// "praktika"). Only "praktika" has a counterpart; a vacancy says nothing about its hours.
func meincheckinEmploymentType(section string) string {
	if strings.EqualFold(strings.TrimSpace(section), "praktika") {
		return "internship"
	}
	return ""
}

// itemscopeOf returns the first element declaring itemscope of the given itemtype.
func itemscopeOf(root *xhtml.Node, itemtype string) *xhtml.Node {
	var found *xhtml.Node
	walk(root, func(n *xhtml.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == xhtml.ElementNode && Attr(n, "itemtype") == itemtype {
			found = n
			return false
		}
		return true
	})
	return found
}

// firstItemprop returns the first element under root carrying the itemprop, or nil.
func firstItemprop(root *xhtml.Node, prop string) *xhtml.Node {
	if ns := findItemprops(root, prop); len(ns) > 0 {
		return ns[0]
	}
	return nil
}
