package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// velorahrSiteJSON and velorahrOffersJSON are api.velorahr.com's answers as captured
// 2026-10-27, trimmed to the fields the prober reads.
func velorahrSiteJSON(id int, company, publicURL string) string {
	return fmt.Sprintf(`{"data":{"id":%d,"company_name":%q,"public_url":%q,"master":false},"success":true}`, id, company, publicURL)
}

func velorahrOffersJSON(owner string, ids ...int) string {
	body := `{"success":true,"data":[`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"id":%d,"career_site_id":%s}`, id, owner)
	}
	return body + `]}`
}

// velorahrFakeAPI serves the sites given and answers every other site id with the platform's 404.
func velorahrFakeAPI(sites map[int][2]string, offers map[int]string, upTo int) statusGetter {
	g := statusGetter{fakeGetter: fakeGetter{}, status: map[string]int{}}
	for id := 1; id <= upTo; id++ {
		site, listing := velorahrSiteURL(fmt.Sprint(id)), velorahrOffersURL(fmt.Sprint(id))
		s, ok := sites[id]
		if !ok {
			g.status[site] = http.StatusNotFound
			g.fakeGetter[listing] = `{"data":[],"success":true}`
			continue
		}
		g.fakeGetter[site] = velorahrSiteJSON(id, s[0], s[1])
		g.fakeGetter[listing] = offers[id]
	}
	return g
}

func TestVelorahrProbe(t *testing.T) {
	g := velorahrFakeAPI(map[int][2]string{
		158: {"Wetaca", "https://wetaca.career.velorahr.com/wetaca-158"},
		223: {"Inditex", "https://demo-velora-antonio.career.velorahr.com/fashion-223"},
		279: {"Serunion", "https://serunion.career.velorahr.com/serunion-279"},
	}, map[int]string{
		158: velorahrOffersJSON("null", 14979, 13825, 8337, 7705, 7468),
		223: velorahrOffersJSON("null", 15001),
		// The master site re-lists offers its child sites own.
		279: velorahrOffersJSON("194", 15221, 15209),
	}, 300)
	p := velorahrProber{}
	if name, n, err := p.probe(context.Background(), g, "158"); err != nil || name != "Wetaca" || n != 5 {
		t.Errorf("live: got (%q,%d,%v), want (Wetaca,5,nil)", name, n, err)
	}
	for _, board := range []string{
		"223",    // the vendor's demo site, dressed as a prospect
		"279",    // lists only offers other sites own
		"99",     // no such site
		"wetaca", // not a site id
	} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}

func TestVelorahrDiscoverWalksTheIDsAndKeepsOneSiteOfATwin(t *testing.T) {
	g := velorahrFakeAPI(map[int][2]string{
		5:   {"Empty", "https://empty.career.velorahr.com/empty-5"},
		80:  {"Magic Costa Blanca", "https://costa-blanca.career.velorahr.com/magic-costa-blanca-80"},
		147: {"Magic Costa Blanca", "https://costa-blanca.career.velorahr.com/magic-costa-blanca-147"},
		194: {"Serunion", "https://serunion.career.velorahr.com/serunion-194"},
		204: {"Singularis", "https://serunion.career.velorahr.com/singularis-204"},
		212: {"Kids&Us", "https://demo-mango.career.velorahr.com/viena-212"},
		279: {"Serunion", "https://serunion.career.velorahr.com/serunion-279"},
		// Past a gap shorter than the walk's patience: still found.
		340: {"Robert Arrigo & Sons", "https://mts-globe.career.velorahr.com/robert-arrigo-sons-340"},
		// Past a gap longer than it: never reached.
		600: {"Too far", "https://far.career.velorahr.com/far-600"},
	}, map[int]string{
		5: velorahrOffersJSON("null"),
		// Two sites of one tenant listing the same offers: one board is enough.
		80:  velorahrOffersJSON("null", 1, 2, 3),
		147: velorahrOffersJSON("null", 1, 2, 3),
		194: velorahrOffersJSON("null", 10, 11),
		204: velorahrOffersJSON("null", 11, 12),
		212: velorahrOffersJSON("null", 20),
		279: velorahrOffersJSON("194", 10, 11),
		340: velorahrOffersJSON("null", 30),
		600: velorahrOffersJSON("null", 40),
	}, 700)
	got, err := velorahrProber{}.discover(context.Background(), g)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	// 147 repeats 80; 194 and 204 overlap but each lists something the other does not; 5 and
	// 279 own nothing; 212 is a demo; 600 lies beyond the walk.
	if want := []string{"80", "194", "204", "340"}; !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}
