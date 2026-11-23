package linksource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestMokaHRBoardCoverageHydratesTheFragmentPosting(t *testing.T) {
	const root = "https://app.mokahr.com/social-recruitment/hengrui/145996"
	const earlier = "d5cfe41a-9a9b-41a0-a064-7627c45025f9"
	const target = "9ef6ed89-30bc-4ac9-a55d-76eaa5d2475b"
	adapter := &fakeHydratingBoard{provider: "mokahr", jobs: []sources.Job{
		{ExternalID: earlier, URL: root + "#/job/" + earlier, Title: "Earlier vacancy"},
		{ExternalID: target, URL: root + "#/job/" + target, Title: "Requested vacancy", Description: "Details"},
	}}
	catalog := fakeCatalog{entry: sources.CompanyEntry{Provider: "mokahr", Board: "hengrui/145996", Company: "Hengrui"}}
	coverage := NewBoardCoverage(map[string]sources.Source{"mokahr": adapter}, catalog)
	job, found, err := coverage.Resolve(context.Background(), root+"?utm_source=test#/job/"+target)
	if err != nil || !found || job.Title != "Requested vacancy" || job.SeenRefresh || !slices.Equal(adapter.hydrated, []string{target}) {
		t.Fatalf("resolved %+v, %v, %v, hydrated %v", job, found, err, adapter.hydrated)
	}
	if _, found, err := coverage.Resolve(context.Background(), root); err != nil || found {
		t.Fatalf("board root returned a posting: %v, %v", found, err)
	}
}

type mokaRegionalCatalog struct{}

func (mokaRegionalCatalog) Entry(context.Context, string, string) (sources.CompanyEntry, bool, error) {
	return sources.CompanyEntry{}, false, errors.New("unexpected cross-region lookup")
}

func (mokaRegionalCatalog) EntryForRegion(_ context.Context, provider, board, region string) (sources.CompanyEntry, bool, error) {
	if region != "r1" {
		return sources.CompanyEntry{}, false, fmt.Errorf("wrong region %q", region)
	}
	return sources.CompanyEntry{Provider: provider, Board: board, Region: region, Company: "Acme R1"}, true, nil
}

func TestMokaHRBoardCoverageUsesTheR1Entry(t *testing.T) {
	const id = "749997fb-fd4b-4384-9ea5-2b6fb7f10afc"
	const raw = "https://hire-r1.mokahr.com/social-recruitment/acme/2933#/job/" + id
	adapter := &fakeBoard{provider: "mokahr", jobs: []sources.Job{{ExternalID: id, URL: raw, Title: "Engineer"}}}
	coverage := NewBoardCoverage(map[string]sources.Source{"mokahr": adapter}, mokaRegionalCatalog{})
	_, found, err := coverage.Resolve(context.Background(), raw)
	if err != nil || !found || len(adapter.asked) != 1 || adapter.asked[0].Region != "r1" || adapter.asked[0].Company != "Acme R1" {
		t.Fatalf("resolved %v, %v, entry %+v", found, err, adapter.asked)
	}
}

func TestMokaHRBoardCoverageRequiresEmployerInTheRequestedRegion(t *testing.T) {
	const raw = "https://hire-r1.mokahr.com/social-recruitment/acme/2933#/job/749997fb-fd4b-4384-9ea5-2b6fb7f10afc"
	wrongRegion := fakeCatalog{entry: sources.CompanyEntry{Provider: "mokahr", Board: "acme/2933", Company: "Acme"}}
	for _, catalog := range []BoardCatalog{nil, wrongRegion} {
		adapter := &fakeBoard{provider: "mokahr"}
		coverage := NewBoardCoverage(map[string]sources.Source{"mokahr": adapter}, catalog)
		_, found, err := coverage.Resolve(context.Background(), raw)
		if err == nil || found || len(adapter.asked) != 0 {
			t.Fatalf("resolved %v, %v, attempted entry %+v", found, err, adapter.asked)
		}
	}
}

type compleoDetailHTTP struct {
	t         *testing.T
	detailErr error
}

func (f compleoDetailHTTP) GetXML(context.Context, string, any) error {
	f.t.Fatal("single posting lookup must not fetch the sitemap")
	return nil
}

func (f compleoDetailHTTP) GetHTML(_ context.Context, raw string) (*html.Node, error) {
	if raw != "https://jobs.compleo.app/wafx/jobdetail/HJ06276A" {
		f.t.Fatalf("unexpected detail URL %q", raw)
	}
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return html.Parse(strings.NewReader(`<script type="application/ld+json">{"@type":"JobPosting","title":"Engineer","hiringOrganization":{"name":"WA FENIX"},"description":"Build software."}</script>`))
}

func TestCompleoPostingFetchReturnsNamespacedVacancy(t *testing.T) {
	const raw = "http://tester@jobs.compleo.app:80/wafx/jobdetail/HJ06276A?utm_source=test"
	adapter := sources.NewCompleo(compleoDetailHTTP{t: t})
	coverage := NewBoardCoverage(map[string]sources.Source{"compleo": adapter}, nil)
	job, found, err := coverage.Resolve(context.Background(), raw)
	if err != nil || !found || job.ExternalID != "wafx:HJ06276A" || job.Company != "WA FENIX" {
		t.Fatalf("resolved %+v, %v, %v", job, found, err)
	}
}

func TestCompleoPostingFetchDistinguishesFailureFromRemoval(t *testing.T) {
	const raw = "https://jobs.compleo.app/wafx/jobdetail/HJ06276A?utm_source=test"
	for _, code := range []int{http.StatusServiceUnavailable, http.StatusGone} {
		fetchErr := &sources.StatusError{Code: code}
		adapter := sources.NewCompleo(compleoDetailHTTP{t: t, detailErr: fetchErr})
		coverage := NewBoardCoverage(map[string]sources.Source{"compleo": adapter}, nil)
		_, found, err := coverage.Resolve(context.Background(), raw)
		if found || (code == http.StatusServiceUnavailable && !errors.Is(err, fetchErr)) || (code == http.StatusGone && err != nil) {
			t.Fatalf("status %d: resolved %v, %v", code, found, err)
		}
	}
}
