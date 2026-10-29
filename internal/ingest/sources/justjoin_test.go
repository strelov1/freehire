package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// justjoinFake serves the real fixtures (fetched 2026-10-28): the sitemap index and its one part
// trimmed to four offers plus a non-offer URL. Each offer page is the real offer object and its
// text rows, re-wrapped in two flight chunks: Sii (remote, fourteen cities, employment and B2B
// pay), DCG (expired, isOfferActive=false), KAMSOFT (no amount stated) and a RocketJobs offer
// (hybrid, B2B before employment). The fourth sitemap offer answers 404, as an unknown slug does.
type justjoinFake struct {
	partErr   error
	noOffer   bool
	pageErr   error
	mu        sync.Mutex
	pages     []string
	sitemaps  []string
	indexBody string
}

func (f *justjoinFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.sitemaps = append(f.sitemaps, u)
	f.mu.Unlock()
	var path string
	switch u {
	case "https://justjoin.it/sitemaps/active-jobs.xml":
		if f.indexBody != "" {
			return xml.Unmarshal([]byte(f.indexBody), v)
		}
		path = "testdata/justjoin/sitemap_index.xml"
	case "https://justjoin.it/sitemaps/active-jobs/part0.xml":
		if f.partErr != nil {
			return f.partErr
		}
		path = "testdata/justjoin/sitemap_part0.xml"
	default:
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *justjoinFake) GetText(_ context.Context, u string) (string, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return "", f.pageErr
	}
	if f.noOffer {
		return "<html><body>Just a moment...</body></html>", nil
	}
	var path string
	switch {
	case strings.HasSuffix(u, "/sii-senior-data-engineer-with-gcp-and-powerbi-f-m-x--gdansk-data"):
		path = "testdata/justjoin/offer_sii.html"
	case strings.HasSuffix(u, "/dcg-senior-fullstack-developer-warszawa-java-e5ce713f"):
		path = "testdata/justjoin/offer_inactive.html"
	case strings.HasSuffix(u, "/kamsoft-s-a--programista-programistka-net-katowice-net"):
		path = "testdata/justjoin/offer_kamsoft.html"
	case strings.HasSuffix(u, "/fresh-office-specjalista-ds-rozwoju-sprzedazy--krakow-sales-uslugi-profesjonalne"):
		path = "testdata/justjoin/offer_rocketjobs.html"
	default:
		return "", &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	return string(raw), err
}

func newTestJustJoin(f *justjoinFake) justjoinSite {
	s := NewJustJoin(f).(justjoinSite)
	s.limiter = noWait{}
	return s
}

func justjoinByID(jobs []Job) map[string]Job {
	m := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestJustJoinHydratesNewOffersAndRefreshesSeenOnes(t *testing.T) {
	f := &justjoinFake{}
	seenSlug := "link-group-senior-security-developer-krakow-security"
	jobs, err := newTestJustJoin(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == seenSlug })
	if err != nil {
		t.Fatal(err)
	}
	byID := justjoinByID(jobs)
	if len(byID) != 3 {
		t.Fatalf("got %d jobs, want 3 (seen refresh, Sii, KAMSOFT; DCG expired): %v", len(byID), byID)
	}
	if r := byID[seenSlug]; !r.SeenRefresh || r.URL != "https://justjoin.it/job-offer/"+seenSlug {
		t.Errorf("seen offer = %+v, want a SeenRefresh by identity", r)
	}
	for _, u := range f.pages {
		if strings.HasSuffix(u, seenSlug) {
			t.Errorf("bought the page of a seen offer: %s", u)
		}
	}
	if _, ok := byID["dcg-senior-fullstack-developer-warszawa-java-e5ce713f"]; ok {
		t.Error("an offer whose page says isOfferActive=false must not be yielded")
	}

	sii := byID["sii-senior-data-engineer-with-gcp-and-powerbi-f-m-x--gdansk-data"]
	if sii.Title != "Senior Data Engineer with GCP and PowerBI (f/m/x)" || sii.Company != "Sii" {
		t.Errorf("Sii title/company = %q / %q", sii.Title, sii.Company)
	}
	if sii.Location != "Gdańsk" || strings.Join(sii.Countries, ",") != "pl" || strings.Join(sii.FallbackCountries, ",") != "pl" {
		t.Errorf("Sii geography = %q %v %v", sii.Location, sii.Countries, sii.FallbackCountries)
	}
	if sii.WorkMode != "remote" || !sii.Remote || sii.Seniority != "senior" || sii.EmploymentType != "full_time" || sii.EnglishLevel != "c1" {
		t.Errorf("Sii facets = mode %q remote %v seniority %q type %q english %q", sii.WorkMode, sii.Remote, sii.Seniority, sii.EmploymentType, sii.EnglishLevel)
	}
	if !sii.IsTechHint {
		t.Error("a justjoin.it offer must carry IsTechHint")
	}
	if sii.SalaryMin == nil || *sii.SalaryMin != 16000 || *sii.SalaryMax != 18000 || sii.SalaryCurrency != "PLN" || sii.SalaryPeriod != "month" {
		t.Errorf("Sii salary = %v-%v %q %q, want the employment contract's 16000-18000 PLN/month", sii.SalaryMin, sii.SalaryMax, sii.SalaryCurrency, sii.SalaryPeriod)
	}
	for _, want := range []string{"umowa o pracę: 16000 – 18000 PLN / mies. brutto", "B2B: 20000 – 24000 PLN / mies. netto"} {
		if !strings.Contains(sii.Description, want) {
			t.Errorf("Sii description lacks the pay line %q", want)
		}
	}
	if !strings.Contains(strings.Join(sii.Skills, ","), "python") {
		t.Errorf("Sii skills = %v, want python among them", sii.Skills)
	}
	if sii.PostedAt == nil || sii.PostedAt.Format("2006-01-02") != "2026-10-26" {
		t.Errorf("Sii posted = %v", sii.PostedAt)
	}

	kam := byID["kamsoft-s-a--programista-programistka-net-katowice-net"]
	if kam.SalaryMin != nil || strings.Contains(kam.Description, "Wynagrodzenie") {
		t.Errorf("an offer stating no amount must carry no salary: %v %q", kam.SalaryMin, kam.Description[:80])
	}
	if !strings.Contains(kam.Description, "O nas:") {
		t.Errorf("KAMSOFT body not resolved from its text row: %q", kam.Description[:80])
	}
}

func TestRocketJobsReadsTheSameOfferShape(t *testing.T) {
	s := NewRocketJobs(&justjoinFake{}).(justjoinSite)
	p, ok := s.parsePosting(sitemapLoc{Loc: "https://rocketjobs.pl/oferta-pracy/fresh-office-specjalista-ds-rozwoju-sprzedazy--krakow-sales-uslugi-profesjonalne"})
	if !ok {
		t.Fatal("rocketjobs offer URL not recognised")
	}
	s.limiter = noWait{}
	j, ok, err := s.detail(context.Background(), p)
	if err != nil || !ok {
		t.Fatalf("detail = %v %v", ok, err)
	}
	if j.Company != "Fresh Office" || j.WorkMode != "hybrid" || j.IsTechHint || j.Location != "Kraków" {
		t.Errorf("rocketjobs job = %q mode %q tech %v loc %q", j.Company, j.WorkMode, j.IsTechHint, j.Location)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 8000 || *j.SalaryMax != 15000 {
		t.Errorf("salary = %v-%v, want the employment contract's 8000-15000 even though B2B is listed first", j.SalaryMin, j.SalaryMax)
	}
	if _, ok := s.parsePosting(sitemapLoc{Loc: "https://justjoin.it/job-offer/fresh-office-x"}); ok {
		t.Error("another site's offer URL must not be read as a rocketjobs offer")
	}
}

func TestJustJoinFailsOnAPartialListing(t *testing.T) {
	f := &justjoinFake{partErr: errors.New("boom")}
	if _, err := newTestJustJoin(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a sitemap part that failed must fail the crawl")
	}
	f = &justjoinFake{indexBody: `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"></sitemapindex>`}
	if _, err := newTestJustJoin(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("an index naming no part must fail the crawl")
	}
}

func TestJustJoinFailsWhenNoPageAnswers(t *testing.T) {
	f := &justjoinFake{noOffer: true}
	if _, err := newTestJustJoin(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a run whose every page lacked an offer is broken, not quiet")
	}
	f = &justjoinFake{pageErr: &StatusError{Method: "GET", Code: 503}}
	if _, err := newTestJustJoin(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a run whose every page failed is broken, not quiet")
	}
}

func TestJustJoinStopsHydratingNearTheDeadline(t *testing.T) {
	f := &justjoinFake{}
	ctx, cancel := context.WithTimeout(context.Background(), justjoinDeadlineMargin-time.Second)
	defer cancel()
	jobs, err := newTestJustJoin(f).FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 0 || len(jobs) != 0 {
		t.Errorf("inside the margin no page may be bought: pages %v jobs %d", f.pages, len(jobs))
	}
}

func TestJustJoinPreferredPay(t *testing.T) {
	f, to := 100.0, 140.0
	pays := justjoinOriginalPays([]justjoinPay{
		{Type: "b2b", Currency: "PLN", CurrencySource: "conversion", From: &f, To: &to, Unit: "hour"},
		{Type: "b2b", Currency: "PLN", CurrencySource: "original", From: &f, To: nil, Unit: "hour"},
		{Type: "b2b", Currency: "PLN", CurrencySource: "original", From: &f, To: &to, Unit: "week"},
		{Type: "b2b", Currency: "PLN", CurrencySource: "original", From: &f, To: &to, Unit: "hour"},
	})
	if len(pays) != 1 || pays[0].Unit != "hour" {
		t.Fatalf("kept %v, want only the original, full-range entry with a known period", pays)
	}
	if p, ok := justjoinPreferredPay(pays); !ok || p.Type != "b2b" {
		t.Errorf("preferred = %v %v, want the B2B entry when no employment contract is stated", p, ok)
	}
}
