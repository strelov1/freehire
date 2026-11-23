package atsboard

import "testing"

func TestRecognizeCataloguePostingURLs(t *testing.T) {
	for _, tc := range []struct{ url, provider, board, canonical, region string }{
		{"https://vouch.careers/companies/cmghojtua00p5et0dvusuxx5o/cmq57wqsj02160ls6ku4nm9zw", "vouch", "cmghojtua00p5et0dvusuxx5o", "https://vouch.careers/companies/cmghojtua00p5et0dvusuxx5o", ""},
		{"https://vouch.careers/companies/contact", "", "", "", ""},
		{"https://public-rest41.bullhornstaffing.com/rest-services/DJVM1/entity/JobOrder/45535", "bullhorn", "41:DJVM1", "https://public-rest41.bullhornstaffing.com/rest-services/DJVM1/search/JobOrder?fields=id&query=%28isOpen%3A1%29", ""},
		{"https://public-rest41.bullhornstaffing.com/rest-services/DJVM1/entity/ClientContact/45535", "", "", "", ""},
		{"https://jobs.bytedance.com/experienced/position/7706738016731482421/detail", "feishujobs", "jobs.bytedance.com", "https://jobs.bytedance.com", ""},
		{"https://jobs.bytedance.com/login", "", "", "", ""},
		{"https://cdn.talentlyft.com/file", "", "", "", ""},
		{"https://cp.static.catsone.com/file", "", "", "", ""},
		{"https://static.dvinci-easy.com/file", "", "", "", ""},
		{"https://sjobs.brassring.com/TGnewUI/Search/home/HomeWithPreLoad?PartnerId=25416&siteid=5998&jobid=3397967", "brassring", "sjobs.brassring.com?partnerid=25416&siteid=5998", "https://sjobs.brassring.com/TGnewUI/Search/home/HomeWithPreLoad?partnerid=25416&siteid=5998", ""},
		{"https://sjobs.brassring.com/TGnewUI/Search/home/HomeWithPreLoad?PartnerId=25416&partnerid=3&siteid=5998", "", "", "", ""},
		{"https://sjobs.brassring.com/search?partnerid=25416&siteid=5998", "", "", "", ""},
		{"https://jobs.compleo.app/wafx/jobdetail/RR06234C", "compleo", "wafx", "https://jobs.compleo.app/wafx/jobdetail/RR06234C", ""},
		{"https://jobs.compleo.app/sitemap.xml", "", "", "", ""},
		{"https://app.mokahr.com/social-recruitment/hengrui/145996#/job/d5cfe41a-9a9b-41a0-a064-7627c45025f9", "mokahr", "hengrui/145996", "https://app.mokahr.com/social-recruitment/hengrui/145996", ""},
		{"https://hire-r1.mokahr.com/social-recruitment/acme/2933#/job/abc", "mokahr", "acme/2933", "https://hire-r1.mokahr.com/social-recruitment/acme/2933", "r1"},
		{"https://app.mokahr.com/social-recruitment/acme", "", "", "", ""},
		{"https://tas-hyatt.taleo.net/careersection/mobile+-+hyatt.com+-+all+external+positions+posted/jobdetail.ftl?job=PAL001411&lang=en", "taleo", "tas-hyatt.taleo.net/mobile+-+hyatt.com+-+all+external+positions+posted", "https://tas-hyatt.taleo.net/careersection/mobile+-+hyatt.com+-+all+external+positions+posted/jobsearch.ftl", ""},
	} {
		t.Run(tc.url, func(t *testing.T) {
			source, board, canonical, ok := Recognize(tc.url)
			if source != tc.provider || board != tc.board || canonical != tc.canonical || ok != (tc.provider != "") || Region(canonical) != tc.region {
				t.Fatalf("got %q, %q, %q, %v, region %q", source, board, canonical, ok, Region(canonical))
			}
		})
	}
}
