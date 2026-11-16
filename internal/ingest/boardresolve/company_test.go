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
