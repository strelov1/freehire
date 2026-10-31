package main

import (
	"context"
	"net/http"
	"testing"
)

func TestInploiProbe(t *testing.T) {
	const search = "https://www.inploi.com/company/"
	const api = "https://api.inploi.com/search/results?filters[segment_ids][0]="
	g := textStatusGetter{
		fakeGetter: fakeGetter{
			search + "compass/search":            `<html><title>Jobs and Careers at Compass Group UK | Apply for Vacancies</title><script>pk_live_example_key_1 "segment_ids","segment","248"</script></html>`,
			search + "haven/search":              `<html><title>Jobs</title><script>pk_live_example_key_1 "segment_ids","Segment","497"</script></html>`,
			search + "smith/search":              `<html><title>Jobs at Smith &amp; Co | Careers</title><script>pk_live_example_key_1 "segment_ids","segment","3"</script></html>`,
			search + "empty/search":              `<html><script>pk_live_example_key_1 "segment_ids","segment","1"</script></html>`,
			search + "bare/search":               `<html><script>pk_live_example_key_1</script></html>`,
			api + "248&query=&page=1&per_page=1": `{"data":[{"id":1}],"pagination":{"total":2197}}`,
			api + "497&query=&page=1&per_page=1": `{"data":[{"id":1}],"pagination":{"total":122}}`,
			api + "1&query=&page=1&per_page=1":   `{"data":[],"pagination":{"total":0}}`,
			api + "3&query=&page=1&per_page=1":   `{"data":[{"id":1}],"pagination":{"total":2}}`,
		},
		status: map[string]int{
			search + "missing/search": http.StatusNotFound,
		},
	}
	p := inploiProber{}
	if name, n, err := p.probe(context.Background(), g, "Compass"); err != nil || name != "Compass Group UK" || n != 2197 {
		t.Errorf("compass: got (%q,%d,%v)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "smith"); err != nil || name != "Smith & Co" || n != 2 {
		t.Errorf("smith: got (%q,%d,%v)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "haven"); err != nil || name != "" || n != 122 {
		t.Errorf("haven: got (%q,%d,%v), want a count without a parsed name", name, n, err)
	}
	for _, board := range []string{"empty", "missing", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	if _, _, err := p.probe(context.Background(), g, "bare"); err == nil {
		t.Error("a search page without a segment must be surfaced")
	}
}

func TestInploiCompanyNameDecodesHTMLEntities(t *testing.T) {
	for _, title := range []string{
		`Jobs and Careers at Smith &amp; Co | Apply for Vacancies`,
		`Jobs and Careers &#97;t Smith &amp; Co | Apply for Vacancies`,
	} {
		if got := inploiCompanyName("<title>" + title + "</title>"); got != "Smith & Co" {
			t.Errorf("inploiCompanyName(%q) = %q, want Smith & Co", title, got)
		}
	}
	if got := inploiCompanyName("<title>İş İlanları at Acme | Careers</title>"); got != "Acme" {
		t.Errorf("inploiCompanyName(dotted I) = %q, want Acme", got)
	}
}
