package sources

import (
	"context"
	"strings"
	"testing"
)

// The fixtures are trimmed from live dsml.kz responses fetched 2026-11-18: the sitemap cut to the
// home page, a city page, two postings and one posting whose page is gone, and each posting page
// cut to its ld+json blocks verbatim (the site Organization graph and the JobPosting).

const (
	dsmlOfficeURL = "https://dsml.kz/jobs/2ff294cd-5100-4288-b599-3bcc2141d2b8"
	dsmlRemoteURL = "https://dsml.kz/jobs/84a0c5f4-7174-441f-9720-82b6d16d019b"
	dsmlGoneURL   = "https://dsml.kz/jobs/00000000-0000-4000-8000-000000000000"
)

func fastDSML(f *sitemapBoardFake) dsml {
	s := NewDSML(f).(dsml)
	s.limiter = noWait{}
	return s
}

func TestDSMLID(t *testing.T) {
	for loc, want := range map[string]string{
		dsmlOfficeURL:                       "2ff294cd-5100-4288-b599-3bcc2141d2b8",
		"https://dsml.kz/jobs/almaty":       "",
		"https://dsml.kz/jobs/data-science": "",
		"https://dsml.kz":                   "",
		"https://elsewhere.example/jobs/2ff294cd-5100-4288-b599-3bcc2141d2b8": "",
	} {
		if got := dsmlID(loc); got != want {
			t.Errorf("dsmlID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestDSMLHydratesPostingsWithSalaryAndDropsAGonePage(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "dsml", files: map[string]string{
		dsmlSitemapURL: "sitemap.xml",
		dsmlOfficeURL:  "posting.html",
		dsmlRemoteURL:  "remote.html",
	}}
	jobs, err := fastDSML(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}

	office := byID["2ff294cd-5100-4288-b599-3bcc2141d2b8"]
	if office.Title != "Senior LLMOps Engineer" || office.Company != "Social Health Insurance Fund" || office.URL != dsmlOfficeURL {
		t.Errorf("office identity = %q / %q / %q", office.Title, office.Company, office.URL)
	}
	if office.Location != "Astana" || len(office.Countries) != 1 || office.Countries[0] != "kz" || office.Remote {
		t.Errorf("office place = %q %v remote=%v", office.Location, office.Countries, office.Remote)
	}
	if office.SalaryCurrency != "KZT" || office.SalaryPeriod != "month" || office.SalaryMin == nil || *office.SalaryMin != 1500000 || office.SalaryMax == nil || *office.SalaryMax != 2000000 {
		t.Errorf("office salary = %v-%v %q %q", office.SalaryMin, office.SalaryMax, office.SalaryCurrency, office.SalaryPeriod)
	}
	if office.EmploymentType != "full_time" || !strings.Contains(office.Description, "vLLM") {
		t.Errorf("office type/body = %q / %.60q", office.EmploymentType, office.Description)
	}

	remote := byID["84a0c5f4-7174-441f-9720-82b6d16d019b"]
	if !remote.Remote || remote.Countries != nil || len(remote.FallbackCountries) != 1 || remote.FallbackCountries[0] != "kz" {
		t.Errorf("remote place = remote=%v countries=%v fallback=%v", remote.Remote, remote.Countries, remote.FallbackCountries)
	}
	if remote.SalaryCurrency != "USD" || remote.SalaryMin == nil || *remote.SalaryMin != 1600 {
		t.Errorf("remote salary = %v %q", remote.SalaryMin, remote.SalaryCurrency)
	}

	if !f.fetched(dsmlGoneURL) {
		t.Error("the posting the sitemap names was never tried")
	}
	if f.fetched("https://dsml.kz/jobs/almaty") || f.fetched("https://dsml.kz") {
		t.Error("a sitemap entry that is not a posting was fetched")
	}
}

func TestDSMLFailsWhenTheSitemapIsUnreadable(t *testing.T) {
	f := &sitemapBoardFake{t: t, dir: "dsml", files: map[string]string{}}
	if _, err := fastDSML(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an unreadable sitemap must fail the run, or the sweep closes every posting")
	}
}
