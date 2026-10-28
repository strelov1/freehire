package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// velorahrProber validates a Velora HR career site (the board is its numeric id) the way
// internal/ingest/sources/velorahr.go crawls it, which is unexported: the site's configuration
// names the employer and answers 404 when there is no such site, and the site's listing is
// counted for the offers it OWNS — a master site re-lists its child sites' offers, each naming
// its owner, and the adapter leaves those to the owner's board.
//
// Two kinds of site are declined. The vendor's demo sites live on tenant hosts named "demo"
// (demo-mango, demo-velora-antonio, velora-demo-tech, demo-clientes-ant, measured 2026-10-27)
// and are dressed as prospects — one calls itself Inditex. And discovery keeps one site of a
// twin: a tenant often publishes the same offers on two sites (Circet, Magic Costa Blanca, OCN),
// and since the pipeline namespaces a posting by its board, two boards would store each offer
// twice.
type velorahrProber struct{}

const (
	velorahrProbeAPI = "https://api.velorahr.com/api/public/career-site/"
	// velorahrDiscoveryGap is how many consecutive ids without a site end the walk. Site ids are
	// sparse — 2026-10-27, 1 and 5 exist, 2 and 37 do not, the highest was 345 — and the widest
	// measured gap between two live sites is under 60.
	velorahrDiscoveryGap = 100
)

var (
	velorahrProbeBoard = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	// velorahrProbeListing mirrors the adapter's listing request.
	velorahrProbeListing = url.Values{
		"filter": {"{}"},
		"range":  {"[0,999]"},
		"sort":   {`["id","DESC"]`},
	}.Encode()
)

func velorahrSiteURL(id string) string { return velorahrProbeAPI + id }
func velorahrOffersURL(id string) string {
	return velorahrProbeAPI + id + "/offers?" + velorahrProbeListing
}

// maxRequestsPerSecond keeps a harvest polite toward the one API host every customer shares.
func (velorahrProber) maxRequestsPerSecond() float64 { return 2 }

func (velorahrProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !velorahrProbeBoard.MatchString(board) {
		return "", 0, nil
	}
	company, found, demo, err := velorahrSite(ctx, c, board)
	if err != nil || !found || demo {
		return "", 0, err
	}
	owned, err := velorahrOwnedOffers(ctx, c, board)
	if err != nil {
		return "", 0, err
	}
	return company, len(owned), nil
}

// discover walks the site ids from 1 until velorahrDiscoveryGap ids in a row have no site, and
// proposes every live, non-demo site that owns an offer no other site owns — of two sites owning
// the same offers, the lower id.
func (velorahrProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	var ids []int
	owned := map[int]map[int64]bool{}
	for id, last := 1, 0; id <= last+velorahrDiscoveryGap; id++ {
		board := strconv.Itoa(id)
		_, found, demo, err := velorahrSite(ctx, c, board)
		if err != nil {
			return nil, fmt.Errorf("velorahr: discover: site %d: %w", id, err)
		}
		if !found {
			continue
		}
		last = id
		if demo {
			continue
		}
		offers, err := velorahrOwnedOffers(ctx, c, board)
		if err != nil {
			return nil, fmt.Errorf("velorahr: discover: site %d: %w", id, err)
		}
		if len(offers) > 0 {
			ids = append(ids, id)
			owned[id] = offers
		}
	}
	var out []string
	for _, id := range ids {
		if !velorahrRepeated(id, ids, owned) {
			out = append(out, strconv.Itoa(id))
		}
	}
	return out, nil
}

// velorahrRepeated reports whether every offer site id owns is owned by another site too — a
// strict superset, or an equal set on a lower id.
func velorahrRepeated(id int, ids []int, owned map[int]map[int64]bool) bool {
	mine := owned[id]
	for _, other := range ids {
		theirs := owned[other]
		if other == id || len(theirs) < len(mine) || (len(theirs) == len(mine) && other > id) {
			continue
		}
		covered := true
		for offer := range mine {
			if !theirs[offer] {
				covered = false
				break
			}
		}
		if covered {
			return true
		}
	}
	return false
}

// velorahrSite reads a site's configuration: the employer it names, whether the site exists, and
// whether it is one of the vendor's demos rather than a customer's.
func velorahrSite(ctx context.Context, c httpClient, board string) (company string, found, demo bool, err error) {
	var site struct {
		Data struct {
			CompanyName string `json:"company_name"`
			PublicURL   string `json:"public_url"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, velorahrSiteURL(board), &site); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	u, err := url.Parse(site.Data.PublicURL)
	if err != nil {
		return "", true, true, nil // a site without a readable address is no one's career site
	}
	tenant, _, _ := strings.Cut(strings.ToLower(u.Hostname()), ".")
	return strings.TrimSpace(site.Data.CompanyName), true, strings.Contains(tenant, "demo"), nil
}

// velorahrOwnedOffers is the set of offer ids the site's listing carries as its own.
func velorahrOwnedOffers(ctx context.Context, c httpClient, board string) (map[int64]bool, error) {
	var listing struct {
		Data []struct {
			ID           int64  `json:"id"`
			CareerSiteID *int64 `json:"career_site_id"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, velorahrOffersURL(board), &listing); err != nil {
		return nil, err
	}
	id, _ := strconv.ParseInt(board, 10, 64)
	out := map[int64]bool{}
	for _, o := range listing.Data {
		if o.ID > 0 && (o.CareerSiteID == nil || *o.CareerSiteID == id) {
			out[o.ID] = true
		}
	}
	return out, nil
}
