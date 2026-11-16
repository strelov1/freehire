package boardresolve

import (
	"context"
	"fmt"
	"testing"
)

type companyPages map[string]string

func (p companyPages) GetText(_ context.Context, raw string) (string, error) {
	if body, ok := p[raw]; ok {
		return body, nil
	}
	return "", fmt.Errorf("unexpected page %s", raw)
}

func TestCompanyDiscoveryPreservesExplicitATSDataRegion(t *testing.T) {
	r := &Resolver{http: companyPages{"https://example.com/": `<a href="https://jobs.eu.lever.co/acme">Careers</a>`}}
	source, board, canonical, ok := r.ResolveCompany(context.Background(), "https://example.com/")
	if !ok || source != "lever" || board != "acme" || canonical != "https://jobs.eu.lever.co/acme" {
		t.Fatalf("native EU board = %s/%s/%s/%v", source, board, canonical, ok)
	}
}

func TestCompanyDiscoveryFollowsObservedCareersLinksToNativeATS(t *testing.T) {
	r := &Resolver{http: companyPages{
		"https://www.gistworld.com/":                    `<a href="/work-for-us/careers">Careers</a>`,
		"https://www.gistworld.com/work-for-us/careers": `<a href="https://gistworld.mua.hrdepartment.com/hrsmart/ats/JobSearch/viewAll">Current vacancies</a>`,
	}}
	source, board, _, ok := r.ResolveCompany(context.Background(), "https://www.gistworld.com/")
	if !ok || source != "hrsmart" || board != "gistworld.mua.hrdepartment.com" {
		t.Fatalf("native board = %s/%s/%v", source, board, ok)
	}
}

type redirectedCompanyPages struct{ companyPages }

func (p redirectedCompanyPages) GetTextResolved(ctx context.Context, raw string) (string, string, error) {
	if raw == "https://old.example/ats" {
		return "", "https://jobs.eu.lever.co/acme", nil
	}
	if raw == "https://old.example/start" {
		return `<a href="careers">Careers</a>`, "https://new.example/team/", nil
	}
	body, err := p.GetText(ctx, raw)
	return body, raw, err
}
func TestCompanyDiscoveryUsesTheFinalWebsiteForRelativeCareerLinks(t *testing.T) {
	r := &Resolver{http: redirectedCompanyPages{companyPages{"https://new.example/team/careers": `<a href="//gistworld.mua.hrdepartment.com/hrsmart/ats/JobSearch/viewAll">Jobs</a>`}}}
	source, board, _, ok := r.ResolveCompany(context.Background(), "https://old.example/start")
	if !ok || source != "hrsmart" || board != "gistworld.mua.hrdepartment.com" {
		t.Fatalf("redirected employer discovery: %s/%s/%v", source, board, ok)
	}
}
func TestCompanyDiscoveryRecognizesProtocolRelativeATSWithoutFetchingIt(t *testing.T) {
	r := &Resolver{http: companyPages{"https://www.gistworld.com/": `<a href="//gistworld.mua.hrdepartment.com/hrsmart/ats/JobSearch/viewAll">Jobs</a>`}}
	source, board, _, ok := r.ResolveCompany(context.Background(), "https://www.gistworld.com/")
	if !ok || source != "hrsmart" || board != "gistworld.mua.hrdepartment.com" {
		t.Fatalf("protocol-relative ATS discovery: %s/%s/%v", source, board, ok)
	}
}

func TestCompanyDiscoveryRecognizesFinalATSWithoutBodyLinks(t *testing.T) {
	r := &Resolver{http: redirectedCompanyPages{}}
	source, board, canonical, ok := r.ResolveCompany(context.Background(), "https://old.example/ats")
	if !ok || source != "lever" || board != "acme" || canonical != "https://jobs.eu.lever.co/acme" {
		t.Fatalf("redirected ATS = %s/%s/%s/%v", source, board, canonical, ok)
	}
}
