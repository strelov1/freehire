package main

import (
	"context"
	"net/http"
	"testing"
)

func TestDigitalrecruitersProbe(t *testing.T) {
	const (
		api     = "https://api.digitalrecruiters.com"
		site    = api + "/careers/v1/careers-sites/"
		listing = api + "/public/v1/careers-site/job-ads?domainName="
	)
	g := statusGetter{
		fakeGetter: fakeGetter{
			// An aggregating site: two of its four postings belong to another site.
			site + "recrute.ouicare.com":                                                           `{"is_online":true,"is_internal":false,"default_locale":{"iso_code":"fr"}}`,
			listing + "recrute.ouicare.com&limit=1000&locale=fr_FR&page=1":                         `{"count":4,"items":[{"job_ad_id":11,"career_domain":"emploi.o2recrute.fr"},{"job_ad_id":12,"career_domain":"recrute.ouicare.com"},{"job_ad_id":13,"career_domain":"recrute.ouicare.com"},{"job_ad_id":14,"career_domain":"emploi.o2recrute.fr"}]}`,
			api + "/public/v1/careers-site/job-ads/12?domainName=recrute.ouicare.com&locale=fr_FR": `{"brand_name":"Oui Care"}`,
			site + "mobilite.avril.ca":                                                             `{"is_online":true,"is_internal":true,"default_locale":{"iso_code":"fr_CA"}}`,
			site + "offline.example.com":                                                           `{"is_online":false,"is_internal":false,"default_locale":{"iso_code":"fr"}}`,
			site + "aggregator.example.com":                                                        `{"is_online":true,"is_internal":false,"default_locale":{"iso_code":"en"}}`,
			listing + "aggregator.example.com&limit=1000&locale=en_GB&page=1":                      `{"count":1,"items":[{"job_ad_id":21,"career_domain":"other.example.com"}]}`,
		},
		status: map[string]int{site + "gone.example.com": http.StatusNotFound},
	}
	p := digitalrecruitersProber{}
	if name, n, err := p.probe(context.Background(), g, "Recrute.Ouicare.com/"); err != nil || name != "Oui Care" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (Oui Care,2,nil)", name, n, err)
	}
	for _, board := range []string{"gone.example.com", "mobilite.avril.ca", "offline.example.com", "aggregator.example.com", "https://x.example.com", "nodot"} {
		if name, n, err := p.probe(context.Background(), g, board); err != nil || n != 0 || name != "" {
			t.Errorf("%q: got (%q,%d,%v), want nothing", board, name, n, err)
		}
	}
}
