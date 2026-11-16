package sources

import (
	"context"
	"os"
	"testing"
)

func TestClinchRealDetailMetadata(t *testing.T) {
	for _, tc := range []struct{ name, host, slug, company, title, location string }{
		{
			name:     "armaguard",
			host:     "armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com",
			slug:     "cash-processor-bendigo-vic-australia",
			company:  "Armaguard Group",
			title:    "Cash Processor",
			location: "Bendigo, VIC",
		},
		{
			name:     "mader",
			host:     "careers.madergroup.com.au",
			slug:     "automotive-electricians-southwest-midwest-and-metro-western-australia-wa-australia",
			company:  "Mader Group",
			title:    "Automotive Electricians - Southwest, Midwest and Metro",
			location: "Western Australia, WA",
		},
		{
			name:     "743",
			host:     "jobs.citadel.edu",
			slug:     "campus-services-officer-charleston-south-carolina-united-states",
			company:  "The Citadel",
			title:    "Campus Services Officer",
			location: "Charleston, South Carolina, United States",
		},
		{
			name:     "586",
			host:     "careers.moretonbay.qld.gov.au",
			slug:     "parks-labourer-expression-of-interest-eoi-grounds-maintenance-natural-areas-trees-city-of-moreton-bay-qld-australia",
			company:  "City of Moreton Bay",
			title:    "Parks Labourer Expression Of Interest (EOI) | Grounds Maintenance, Natural Areas & Trees",
			location: "City of Moreton Bay, QLD",
		},
		{
			name:     "1138",
			host:     "careers.fire.nsw.gov.au",
			slug:     "on-call-firefighter-rn1-279-dorrigo-port-macquarie-nsw-australia-8d9050f0-73bd-4ba0-b320-6dafe43aee2c",
			company:  "Fire and Rescue NSW",
			title:    "On-Call Firefighter - RN1 - 279 Dorrigo",
			location: "Regional North - Port Macquarie & Mid North Coast NSW",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := os.ReadFile("testdata/clinch/" + tc.name + ".html")
			if err != nil {
				t.Fatal(err)
			}
			url := "https://" + tc.host + "/jobs/" + tc.slug
			sitemap := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(url))
			detail := &routedDetail{routes: map[string]string{url: string(body)}}
			jobs, err := NewClinch(sitemap, detail).Fetch(context.Background(), CompanyEntry{Board: tc.host, Company: tc.company})
			if err != nil || len(jobs) != 1 {
				t.Fatalf("Fetch = %v, %v", jobs, err)
			}
			j := jobs[0]
			if j.Title != tc.title || j.Location != tc.location {
				t.Errorf("metadata = %q / %q; want %q / %q", j.Title, j.Location, tc.title, tc.location)
			}
			if j.URL != url || j.ExternalID != clinchExternalID(tc.slug) || j.Company != tc.company {
				t.Errorf("identity changed: %+v", j)
			}
			if len(j.Description) < 1000 || j.PostedAt == nil || j.Unreadable {
				t.Errorf("lost body or sitemap completeness: %+v", j)
			}
		})
	}
}
