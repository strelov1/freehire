package main

import (
	"context"
	"testing"
)

func TestKekaProbe(t *testing.T) {
	const base = "https://coditude.keka.com/careers"
	g := fakeGetter{
		// coditude's listing as captured 2026-10-25, postings trimmed to their ids and titles.
		base + "/api/jobs/default/active":                   `[{"id":167673,"title":"INTERNSHIP: AI Engineering Intern"},{"id":167001,"title":"QA Engineer"}]`,
		base + "/api/organization/default/careerportalinfo": `{"name":"Coditude","shortName":"Coditude","careersPortalDomain":"coditude.keka.com"}`,
		// The page Keka redirects an unknown label to, served under HTTP 200.
		"https://zzqqxx.keka.com/careers/api/jobs/default/active": "\ufeff<!DOCTYPE html>\n<html style=\"height:100%\">\n<head>\n    <meta charset=\"utf-8\" />\n    <title>Invalid Tenant</title>",
		// The page an organisation running Keka's HRMS without its hiring module redirects to.
		"https://hrmsonly.keka.com/careers/api/jobs/default/active": "\ufeff<!DOCTYPE html>\n<html style=\"height:100%\">\n<head>\n    <title>Forbidden Access</title>",
		"https://emptyco.keka.com/careers/api/jobs/default/active":  `[]`,
		"https://reshaped.keka.com/careers/api/jobs/default/active": `{"jobs":[]}`,
		"https://nameless.keka.com/careers/api/jobs/default/active": `[{"id":1}]`,
	}
	p := kekaProber{}
	if name, n, err := p.probe(context.Background(), g, "coditude"); err != nil || name != "Coditude" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (Coditude,2,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), g, "nameless"); err != nil || name != "" || n != 1 {
		t.Errorf("nameless: got (%q,%d,%v), want a live board with no name", name, n, err)
	}
	for _, board := range []string{"zzqqxx", "hrmsonly", "emptyco", "../x"} {
		if _, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want a skip", board, n, err)
		}
	}
	for _, board := range []string{"reshaped", "refused"} {
		if _, _, err := p.probe(context.Background(), g, board); err == nil {
			t.Errorf("%s: must be surfaced, not read as an absent board", board)
		}
	}
}
