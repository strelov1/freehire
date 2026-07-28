package main

import (
	"context"
	"slices"
	"testing"
)

// opencatsPortalHTML is a portal listing carrying n postings, titled as an install titles it.
func opencatsPortalHTML(title string, ids ...string) string {
	body := `<html><head><title>` + title + `</title></head><body><table>`
	for _, id := range ids {
		body += `<tr><td><a href="index.php?m=careers&amp;p=showJob&amp;ID=` + id + `">Engineer ` + id + `</a></td></tr>`
	}
	return body + `</table></body></html>`
}

func TestOpencatsProberKeepsPortalWithPostings(t *testing.T) {
	f := fakeGetter{
		"https://careers.crewlogix.com/careers/index.php?m=careers&p=showAll": opencatsPortalHTML(
			"Crewlogix Technologies - Careers", "114", "113"),
	}

	name, jobs, err := (opencatsProber{}).probe(context.Background(), f, "careers.crewlogix.com/careers")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if jobs != 2 {
		t.Errorf("openJobs = %d, want 2", jobs)
	}
	if name != "Crewlogix Technologies" {
		t.Errorf("company = %q, want the portal title without its Careers suffix", name)
	}
}

// TestOpencatsProberNamesFromHostWhenTitleIsUseless covers the common install that never
// changed its title, so the title is just the hostname again.
func TestOpencatsProberNamesFromHostWhenTitleIsUseless(t *testing.T) {
	f := fakeGetter{
		"https://careers.boomit.pt/careers/index.php?m=careers&p=showAll": opencatsPortalHTML(
			"careers.boomit.pt - Careers", "51"),
	}

	name, jobs, err := (opencatsProber{}).probe(context.Background(), f, "careers.boomit.pt/careers")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if jobs != 1 {
		t.Errorf("openJobs = %d, want 1", jobs)
	}
	if name != "careers.boomit.pt" {
		t.Errorf("company = %q, want the host as the fallback name", name)
	}
}

func TestOpencatsProberSkipsUnreachableOrEmpty(t *testing.T) {
	f := fakeGetter{
		"https://empty.example.com/index.php?m=careers&p=showAll": opencatsPortalHTML("Empty - Careers"),
	}

	for _, board := range []string{"empty.example.com", "missing.example.com"} {
		name, jobs, err := (opencatsProber{}).probe(context.Background(), f, board)
		if err != nil {
			t.Errorf("board %q: probe returned an error, want a silent skip: %v", board, err)
		}
		if name != "" || jobs != 0 {
			t.Errorf("board %q: got (%q, %d), want a skip", board, name, jobs)
		}
	}
}

// TestOpencatsDiscoveryRejectsIneligibleHosts guards the two ways discovery poisons the board
// file: commercial CATS hosts, already crawled under their own provider (a cross-provider
// duplicate the (source, external_id) key cannot catch), and hosts that are not a company
// portal at all.
func TestOpencatsDiscoveryRejectsIneligibleHosts(t *testing.T) {
	rejected := []string{
		"gulfstream.catsone.com",
		"kommlink.catsone.com",
		"email.catsone.com",
		"44.224.147.179",
		"opencats.org",
		"www.opencats.org",
		"documentation.opencats.org",
	}
	for _, h := range rejected {
		if opencatsEligibleHost(h) {
			t.Errorf("host %q was accepted, want it rejected", h)
		}
	}

	accepted := []string{
		"careers.crewlogix.com",
		"atscareers.g4s.com",
		"itsource.indovisionglobal.com",
		"opencats.gorgany.com",
	}
	for _, h := range accepted {
		if !opencatsEligibleHost(h) {
			t.Errorf("host %q was rejected, want it accepted", h)
		}
	}
}

// TestOpencatsResolveMountPicksOneBoardPerHost is what keeps a host from entering the board
// file twice. Installs mount the portal differently, and a board is the mount point, so the
// same portal reachable at two mounts would namespace one posting under two external ids.
func TestOpencatsResolveMountPicksOneBoardPerHost(t *testing.T) {
	f := fakeGetter{
		"https://careers.boomit.pt/careers/index.php?m=careers&p=showAll": opencatsPortalHTML("Boomit", "51"),
		"https://atscareers.g4s.com/index.php?m=careers&p=showAll":        opencatsPortalHTML("G4S", "30156"),
		// A host answering at both mounts must still yield exactly one board.
		"https://both.example.com/careers/index.php?m=careers&p=showAll": opencatsPortalHTML("Both", "1"),
		"https://both.example.com/index.php?m=careers&p=showAll":         opencatsPortalHTML("Both", "1"),
	}

	cases := map[string]string{
		"careers.boomit.pt":     "careers.boomit.pt/careers",
		"atscareers.g4s.com":    "atscareers.g4s.com",
		"both.example.com":      "both.example.com/careers",
		"nonportal.example.com": "",
	}
	for host, want := range cases {
		if got := opencatsResolveMount(context.Background(), f, host); got != want {
			t.Errorf("resolveMount(%q) = %q, want %q", host, got, want)
		}
	}
}

// TestOpencatsDiscoverUnionsSignatures checks the discovery contract: several signature
// queries, one de-duplicated candidate list, each entry already resolved to its mount.
func TestOpencatsDiscoverUnionsSignatures(t *testing.T) {
	f := fakeGetter{}
	for _, q := range opencatsDiscoveryQueries {
		f[opencatsSearchURL(q)] = `{"results":[
			{"task":{"domain":"careers.boomit.pt"}},
			{"task":{"domain":"gulfstream.catsone.com"}},
			{"task":{"domain":"atscareers.g4s.com"}}
		]}`
	}
	f["https://careers.boomit.pt/careers/index.php?m=careers&p=showAll"] = opencatsPortalHTML("Boomit", "51")
	f["https://atscareers.g4s.com/index.php?m=careers&p=showAll"] = opencatsPortalHTML("G4S", "30156")

	got, err := (opencatsProber{}).discover(context.Background(), f)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	slices.Sort(got)
	want := []string{"atscareers.g4s.com", "careers.boomit.pt/careers"}
	if !slices.Equal(got, want) {
		t.Errorf("discover() = %v, want %v (de-duplicated across queries, CATS excluded)", got, want)
	}
}

func TestOpencatsProberRegistered(t *testing.T) {
	p, ok := probers["opencats"]
	if !ok {
		t.Fatal("probers missing opencats")
	}
	if _, ok := p.(discoverer); !ok {
		t.Error("opencats prober must support discovery: it has no seed list to fall back on")
	}
}
