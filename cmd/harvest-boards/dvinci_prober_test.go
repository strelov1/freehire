package main

import (
	"context"
	"net/http"
	"testing"
)

func TestDvinciProbe(t *testing.T) {
	g := statusGetter{
		fakeGetter: fakeGetter{
			// cadenas as captured 2026-10-25, trimmed to two publications.
			"https://cadenas.dvinci-hr.com/jobPublication/list.json": `[{"id":20291,"jobOpening":{"id":10254,"company":{"name":"CADENAS GmbH"}}},{"id":20294,"jobOpening":{"id":10257,"company":{"name":"CADENAS GmbH"}}}]`,
			"https://empty.dvinci-easy.com/jobPublication/list.json": `[]`,
			"https://null.dvinci-easy.com/jobPublication/list.json":  `null`,
		},
		status: map[string]int{
			"https://gone.dvinci-hr.com/jobPublication/list.json":    http.StatusNotFound,
			"https://refused.dvinci-hr.com/jobPublication/list.json": http.StatusForbidden,
		},
	}
	p := dvinciProber{}
	if name, n, err := p.probe(context.Background(), g, "Cadenas.dvinci-hr.com"); err != nil || name != "CADENAS GmbH" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (CADENAS GmbH,2,nil)", name, n, err)
	}
	for _, board := range []string{"empty.dvinci-easy.com", "gone.dvinci-hr.com", "cadenas", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	for _, board := range []string{"refused.dvinci-hr.com", "null.dvinci-easy.com"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: must be surfaced, not read as an absent board", board)
		}
	}
}
