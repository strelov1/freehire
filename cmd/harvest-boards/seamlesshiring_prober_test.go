package main

import (
	"context"
	"net/http"
	"testing"
)

func TestSeamlessHiringProbe(t *testing.T) {
	const api = "https://letshego.seamlesshiring.com/v2/jobs/job-list?limit=1&page=1"
	g := statusGetter{
		fakeGetter: fakeGetter{
			api: `{
				"status": "success",
				"status_code": 200,
				"data": {"jobs": {"total": 2, "data": [{"company": {"name": "Letshego Financial Services Namibia"}}]}}
			}`,
			"https://coronationgroup.seamlesshiring.com/v2/jobs/job-list?limit=1&page=1": `{
				"status": "success",
				"status_code": 200,
				"data": {"jobs": {"total": 0, "data": []}}
			}`,
			"https://reshaped.seamlesshiring.com/v2/jobs/job-list?limit=1&page=1": `{"status":"success","status_code":200,"data":{}}`,
		},
		status: map[string]int{
			"https://missing.seamlesshiring.com/v2/jobs/job-list?limit=1&page=1": http.StatusNotFound,
		},
	}
	p := seamlesshiringProber{}
	if name, n, err := p.probe(context.Background(), g, "Letshego"); err != nil || name != "Letshego Financial Services Namibia" || n != 2 {
		t.Errorf("live: got (%q,%d,%v)", name, n, err)
	}
	for _, board := range []string{"coronationgroup", "missing", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	if _, _, err := p.probe(context.Background(), g, "reshaped"); err == nil {
		t.Error("a listing without jobs must be surfaced")
	}
	if _, _, err := p.probe(context.Background(), fakeGetter{}, "refused"); err == nil {
		t.Error("a transport failure must be surfaced")
	}
}
