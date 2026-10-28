package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

// talentsoftProber validates a Talentsoft career site (the board is its host) by reading page
// one of the listing the adapter walks, /offre-de-emploi/liste-toutes-offres.aspx?all=1&mode=list,
// which states the site's posting count in #…_Pagination_TotalOffers. It mirrors
// internal/ingest/sources/talentsoft.go, which is unexported.
//
// The employer is the page title's lead ("GEODIS - Résultat de votre recherche (520 offres…)").
// A site with no open posting renders the platform's own empty result and is reported with a
// zero count; a page carrying neither is not a Talentsoft listing.
type talentsoftProber struct{}

var (
	talentsoftProbeBoard  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	talentsoftProbeDigits = regexp.MustCompile(`\d+`)
)

func (talentsoftProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(board), "/"))
	if !talentsoftProbeBoard.MatchString(board) {
		return "", 0, nil
	}
	root, err := c.GetHTML(ctx, "https://"+board+"/offre-de-emploi/liste-toutes-offres.aspx?all=1&mode=list")
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	total, ok := talentsoftProbeTotal(root)
	if !ok {
		return "", 0, nil
	}
	return talentsoftProbeCompany(root), total, nil
}

// talentsoftProbeTotal reads the stated count, or reports false when the page is not a
// Talentsoft listing. The platform's empty result counts as zero.
func talentsoftProbeTotal(root *html.Node) (int, bool) {
	var total, empty *html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			id := sources.Attr(n, "id")
			switch {
			case total == nil && strings.HasSuffix(id, "_Pagination_TotalOffers"):
				total = n
			case empty == nil && strings.HasSuffix(id, "_NoResult_NoResult"):
				empty = n
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
	}
	visit(root)
	if total == nil {
		return 0, empty != nil
	}
	text := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '.' || r == ',' {
			return -1
		}
		return r
	}, bizneoNodeText(total))
	n, err := strconv.Atoi(talentsoftProbeDigits.FindString(text))
	return n, err == nil
}

// talentsoftProbeCompany is the title's lead before " - ", or "" when the title has none.
func talentsoftProbeCompany(root *html.Node) string {
	lead, _, ok := strings.Cut(strings.Join(strings.Fields(pageTitle(root)), " "), " - ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(lead)
}
