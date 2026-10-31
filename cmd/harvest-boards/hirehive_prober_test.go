package main

import (
	"context"
	"net/http"
	"testing"
)

func TestHireHiveProbe(t *testing.T) {
	const api = "https://amcsgroup.hirehive.com/api/v2/jobs?page=1&page_size=1"
	g := statusGetter{
		fakeGetter: fakeGetter{
			api:                               `{"meta":{"total_items":59},"items":[{"id":"job_iZEZFh"}]}`,
			"https://amcsgroup.hirehive.com/": `<html><head><title>Careers | Jobs at AMCS Group</title></head></html>`,
			"https://emptyco.hirehive.com/api/v2/jobs?page=1&page_size=1":  `{"meta":{"total_items":0},"items":[]}`,
			"https://reshaped.hirehive.com/api/v2/jobs?page=1&page_size=1": `{"items":[]}`,
			"https://nameless.hirehive.com/api/v2/jobs?page=1&page_size=1": `{"meta":{"total_items":3},"items":[{"id":"job_x"}]}`,
			"https://reversed.hirehive.com/api/v2/jobs?page=1&page_size=1": `{"meta":{"total_items":1},"items":[{"id":"job_y"}]}`,
			"https://reversed.hirehive.com/":                               `<html><head><title>Jobs at Acme | HireHive</title></head></html>`,
		},
		status: map[string]int{
			"https://missing.hirehive.com/api/v2/jobs?page=1&page_size=1": http.StatusNotFound,
		},
	}
	p := hirehiveProber{}
	if name, n, err := p.probe(context.Background(), g, "AMCSGroup"); err != nil || name != "AMCS Group" || n != 59 {
		t.Errorf("live: got (%q,%d,%v), want (AMCS Group,59,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "reversed"); err != nil || name != "Acme" || n != 1 {
		t.Errorf("reversed title: got (%q,%d,%v), want (Acme,1,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "nameless"); err != nil || name != "" || n != 3 {
		t.Errorf("nameless: got (%q,%d,%v), want a live unnamed board", name, n, err)
	}
	for _, board := range []string{"emptyco", "missing", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	if _, _, err := p.probe(context.Background(), g, "reshaped"); err == nil {
		t.Error("a listing without a count must be surfaced")
	}
	if _, _, err := p.probe(context.Background(), fakeGetter{}, "refused"); err == nil {
		t.Error("a transport failure must be surfaced")
	}
}
