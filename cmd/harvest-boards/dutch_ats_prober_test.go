package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// xmlStatusGetter serves canned bodies like fakeGetter and answers the listed status for a URL
// read as XML.
type xmlStatusGetter struct {
	fakeGetter
	status map[string]int
}

func (g xmlStatusGetter) GetXML(ctx context.Context, url string, v any) error {
	if code, ok := g.status[url]; ok {
		return &sources.StatusError{Method: http.MethodGet, Code: code, URL: url}
	}
	return g.fakeGetter.GetXML(ctx, url, v)
}

func TestHomerunProbe(t *testing.T) {
	g := xmlStatusGetter{
		fakeGetter: fakeGetter{
			// Feeds as captured 2026-10-25, entries trimmed to their ids.
			"https://feed.homerun.co/douglas": `<?xml version="1.0" encoding="UTF-8" ?>
<feed xmlns="http://www.w3.org/2005/Atom"><title type="text">Vacatures - Werken bij Douglas NL</title>
<entry><id>job_vteObe02LBfH2m7YjA8B</id></entry><entry><id>job_W4O6B45ovsKe11fvXCMV</id></entry></feed>`,
			"https://feed.homerun.co/autokan": `<?xml version="1.0" encoding="UTF-8" ?>
<feed xmlns="http://www.w3.org/2005/Atom"><title type="text">Autokan</title><entry><id>job_rqQlBEIMndKkNPX3gyPm</id></entry></feed>`,
			"https://feed.homerun.co/quiet": `<?xml version="1.0" encoding="UTF-8" ?>
<feed xmlns="http://www.w3.org/2005/Atom"><title type="text">Werken bij Quiet</title></feed>`,
		},
		status: map[string]int{"https://feed.homerun.co/innovactory-gone": http.StatusNotFound},
	}
	p := homerunProber{}
	for board, want := range map[string]struct {
		name string
		n    int
	}{
		"douglas": {"Douglas NL", 2},
		"autokan": {"Autokan", 1},
		"quiet":   {"Quiet", 0},
	} {
		if name, n, err := p.probe(context.Background(), g, board); err != nil || name != want.name || n != want.n {
			t.Errorf("%s: got (%q,%d,%v), want (%q,%d,nil)", board, name, n, err, want.name, want.n)
		}
	}
	for _, board := range []string{"innovactory-gone", "Not A Label"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
}

func TestEasyCruitProbe(t *testing.T) {
	const export = ".easycruit.com/export/xml/vacancy/list.xml"
	g := xmlStatusGetter{
		fakeGetter: fakeGetter{
			// jeugdzorg's list as captured 2026-10-25, trimmed to what the probe reads.
			"https://jeugdzorg" + export: `<?xml version="1.0" encoding="UTF-8"?>
<VacancyList xmlns="urn:EasyCruit"><Vacancy id="3643611"><Versions><Version language="nl"><Title>Medewerker Crisisinterventieteam</Title>
<AlternativeCompanyName> Jeugdbescherming Rotterdam Rijnmond</AlternativeCompanyName></Version></Versions>
<Departments><Department id="49012"><Name>Jeugdbescherming Rotterdam Rijnmond</Name></Department></Departments></Vacancy>
<Vacancy id="3667721"><Versions><Version language="nl"><Title>Stage</Title></Version></Versions></Vacancy></VacancyList>`,
			// No alternative company name: the department names the employer.
			"https://kongsberg" + export: `<VacancyList xmlns="urn:EasyCruit"><Vacancy id="1"><Versions><Version language="us"><Title>x</Title></Version></Versions>
<Departments><Department id="2"><Name>Kongsberg Discovery</Name></Department></Departments></Vacancy></VacancyList>`,
			"https://vre" + export: `<?xml version="1.0" encoding="UTF-8"?><VacancyList xmlns="urn:EasyCruit"/>`,
		},
		status: map[string]int{"https://nope" + export: http.StatusNotFound},
	}
	p := easycruitProber{}
	for board, want := range map[string]struct {
		name string
		n    int
	}{
		"jeugdzorg": {"Jeugdbescherming Rotterdam Rijnmond", 2},
		"kongsberg": {"Kongsberg Discovery", 1},
		"vre":       {"", 0},
	} {
		if name, n, err := p.probe(context.Background(), g, board); err != nil || name != want.name || n != want.n {
			t.Errorf("%s: got (%q,%d,%v), want (%q,%d,nil)", board, name, n, err, want.name, want.n)
		}
	}
	if _, n, err := p.probe(context.Background(), g, "nope"); err != nil || n != 0 {
		t.Errorf("nope: got n=%d err=%v, want a skip", n, err)
	}
}
