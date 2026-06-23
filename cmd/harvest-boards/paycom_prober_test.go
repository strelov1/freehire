package main

import (
	"context"
	"testing"
)

func TestPaycomProbe(t *testing.T) {
	p := paycomProber{}
	page := `<script>var configsFromHost = {"sessionJWT":"JWT9","atsPortalMantleServiceUrl":"https:\/\/portal-applicant-tracking.us-cent.paycomonline.net\/"};</script>`
	getter := fakeGetter{
		"https://www.paycomonline.net/v4/ats/web.php/portal/LIVEKEY/jobs/1":                              page,
		"https://www.paycomonline.net/v4/ats/web.php/portal/EMPTYKEY/jobs/1":                             page,
		"https://portal-applicant-tracking.us-cent.paycomonline.net/api/ats/job-posting-previews/search": `{"jobPostingPreviewsCount":328}`,
		"https://portal-applicant-tracking.us-cent.paycomonline.net/api/ats/company-name":                `{"companyName":"City Electric Supply"}`,
	}
	// live board: name from company-name, count from search
	if name, n, err := p.probe(context.Background(), getter, "LIVEKEY"); err != nil || name != "City Electric Supply" || n != 328 {
		t.Errorf("live: got (%q,%d,%v), want (City Electric Supply,328,nil)", name, n, err)
	}
	// a board whose portal page is absent => skip
	if _, n, err := p.probe(context.Background(), getter, "GONEKEY"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

func TestPaycomProbeEmpty(t *testing.T) {
	p := paycomProber{}
	page := `<script>var configsFromHost = {"sessionJWT":"JWT9","x":"https:\/\/portal-applicant-tracking.us-cent.paycomonline.net\/"};</script>`
	getter := fakeGetter{
		"https://www.paycomonline.net/v4/ats/web.php/portal/EMPTYKEY/jobs/1":                             page,
		"https://portal-applicant-tracking.us-cent.paycomonline.net/api/ats/job-posting-previews/search": `{"jobPostingPreviewsCount":0}`,
	}
	// 0 open jobs => not kept
	if _, n, err := p.probe(context.Background(), getter, "EMPTYKEY"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
}
