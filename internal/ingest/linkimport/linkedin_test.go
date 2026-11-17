package linkimport

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

type linkedInTestSource struct {
	posting sources.Job
	err     error
}

func (linkedInTestSource) Provider() string { return "linkedin" }
func (linkedInTestSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return nil, nil
}
func (s linkedInTestSource) Posting(context.Context, string) (sources.Job, error) {
	return s.posting, s.err
}
func (linkedInTestSource) CompanyWebsite(context.Context, string) (string, error) {
	return "https://www.gistworld.com", nil
}

func TestPrepareLinkedInPreservesTheNativeApplicationGate(t *testing.T) {
	for _, method := range []sources.ApplicationMethod{sources.ApplyOnSite, sources.ApplyExternal, ""} {
		t.Run(string(method), func(t *testing.T) {
			j := sources.Job{ExternalID: "123", URL: "https://www.linkedin.com/jobs/view/123/", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services", ApplyMethod: method}
			im := &Importer{ingest: map[string]sources.Source{"linkedin": linkedInTestSource{posting: j}}}
			got, err := im.PrepareLinkedIn(context.Background(), "https://uk.linkedin.com/jobs/view/java-engineer-123/?tracking=1")
			if err != nil {
				t.Fatal(err)
			}
			if got.Posting.ApplyMethod != method || got.Posting.ExternalID != "123" {
				t.Fatalf("prepared = %+v", got)
			}
			resolved, ok, err := im.Resolve(context.Background(), j.URL, Board{})
			if err != nil {
				t.Fatal(err)
			}
			if ok != (method == sources.ApplyOnSite) {
				t.Fatalf("method=%s resolved=%+v ok=%v", method, resolved, ok)
			}
			if ok && (resolved.Source != "linkedin" || resolved.Job.ExternalID != "123") {
				t.Fatalf("wrong ownership: %+v", resolved)
			}
		})
	}
}

func TestPrepareLinkedInResolvesExternalPostingUnderItsATS(t *testing.T) {
	const raw = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"
	j := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services", ApplyMethod: sources.ApplyExternal, ExternalApplyURL: raw}
	im := &Importer{ingest: map[string]sources.Source{
		"linkedin": linkedInTestSource{posting: j},
		"hrsmart":  nativeTestSource{jobs: []sources.Job{{ExternalID: "3735", URL: raw, Title: j.Title, Description: j.Description}}},
	}, catalog: nativeEmployerCatalog{company: j.Company}}
	got, err := im.PrepareLinkedIn(context.Background(), "https://www.linkedin.com/jobs/view/123/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Native == nil || got.Native.Job.ExternalID != "gistworld.mua.hrdepartment.com:3735" || got.Board.Source != "hrsmart" {
		t.Fatalf("native destination = %+v", got)
	}
}

func TestPrepareLinkedInQueuesHiddenDestinationBoardWithoutInventingPosting(t *testing.T) {
	j := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services", ApplyMethod: sources.ApplyExternal, CompanyProfileURL: "https://www.linkedin.com/company/gist/"}
	im := &Importer{ingest: map[string]sources.Source{"linkedin": linkedInTestSource{posting: j}, "hrsmart": nativeTestSource{}}, boards: nativeEmployerProof{}}
	got, err := im.PrepareLinkedIn(context.Background(), "https://www.linkedin.com/jobs/view/123/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Native != nil || got.Board.Source != "hrsmart" || got.Board.Board != "gistworld.mua.hrdepartment.com" {
		t.Fatalf("board-only discovery = %+v", got)
	}
}

func TestPrepareLinkedInRefusesUnreadableRemovedAndMismatchedPostings(t *testing.T) {
	valid := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Acme", Description: "Build Java services.", ApplyMethod: sources.ApplyOnSite}
	removed, mismatched, unreadable := valid, valid, valid
	removed.Removed = true
	mismatched.ExternalID = "456"
	unreadable.Unreadable = true
	for _, tc := range []struct {
		name   string
		source linkedInTestSource
	}{
		{"fetch refused", linkedInTestSource{err: errors.New("refused")}},
		{"removed", linkedInTestSource{posting: removed}},
		{"identity mismatch", linkedInTestSource{posting: mismatched}},
		{"unreadable", linkedInTestSource{posting: unreadable}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im := &Importer{ingest: map[string]sources.Source{"linkedin": tc.source}}
			if _, ok, err := im.Resolve(context.Background(), "https://www.linkedin.com/jobs/view/123/", Board{}); err == nil || ok {
				t.Fatalf("unsafe posting imported: ok=%v err=%v", ok, err)
			}
		})
	}
}

// A region lookup must retain the stored crawl entry rather than borrowing a
// same-slug board from another API residency or replacing its metadata.
type linkedInRegionCatalog map[string]sources.CompanyEntry

func (c linkedInRegionCatalog) Entry(_ context.Context, _, _ string) (sources.CompanyEntry, bool, error) {
	e, found := c[""]
	return e, found, nil
}
func (c linkedInRegionCatalog) EntryForRegion(_ context.Context, _, _, region string) (sources.CompanyEntry, bool, error) {
	e, found := c[region]
	return e, found, nil
}

type linkedInLeverSource struct{}

func (linkedInLeverSource) Provider() string { return "lever" }
func (linkedInLeverSource) Fetch(_ context.Context, entry sources.CompanyEntry) ([]sources.Job, error) {
	id, host := "22222222-2222-4222-8222-222222222222", "jobs.lever.co"
	if entry.Region == "eu" {
		id, host = "11111111-1111-4111-8111-111111111111", "jobs.eu.lever.co"
	}
	return []sources.Job{{ExternalID: id, URL: "https://" + host + "/acme/" + id, Title: "Engineer", Company: entry.Company, Description: "Build services."}}, nil
}

type linkedInLeverProof struct{ region string }

func (linkedInLeverProof) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}
func (p linkedInLeverProof) ResolveCompany(context.Context, string) (string, string, string, bool) {
	host := "jobs.lever.co"
	if p.region == "eu" {
		host = "jobs.eu.lever.co"
	}
	return "lever", "acme", "https://" + host + "/acme", true
}

func TestLinkedInNativeResolutionKeepsCatalogRegion(t *testing.T) {
	const raw = "https://jobs.eu.lever.co/acme/11111111-1111-4111-8111-111111111111"
	eu := sources.CompanyEntry{Provider: "lever", Board: "acme", Region: "eu", Company: "Acme Europe", Hub: true, Tenants: map[string]string{"keep": "stored"}}
	catalog := linkedInRegionCatalog{"": {Provider: "lever", Board: "acme", Company: "Acme US"}, "eu": eu}
	im := &Importer{ingest: map[string]sources.Source{"lever": linkedInLeverSource{}}, catalog: catalog}
	got, err := im.ResolveNative(context.Background(), db.Job{Source: "linkedin", Company: eu.Company, Title: "Engineer"}, raw, "")
	if err != nil || got.Entry.Region != "eu" || got.Entry.Company != eu.Company || !got.Entry.Hub || got.Entry.Tenants["keep"] != "stored" {
		t.Fatalf("regional catalogue metadata=%+v error=%v", got, err)
	}
}

func TestLinkedInNativeResolutionRejectsAnotherWebsiteRegion(t *testing.T) {
	const raw = "https://jobs.eu.lever.co/acme/11111111-1111-4111-8111-111111111111"
	for _, region := range []string{"", "eu"} {
		t.Run(region, func(t *testing.T) {
			im := &Importer{ingest: map[string]sources.Source{"lever": linkedInLeverSource{}}, boards: linkedInLeverProof{region: region}}
			got, err := im.ResolveNative(context.Background(), db.Job{Source: "linkedin", Company: "Acme Europe", Title: "Engineer"}, raw, "https://acme.example")
			if region != "eu" {
				if err == nil || got.Entry.Provider != "" || got.Job.ExternalID != "" {
					t.Fatalf("another residency proved the employer: %+v error=%v", got, err)
				}
			} else if err != nil || got.Entry.Region != "eu" || got.Job.ExternalID == "" {
				t.Fatalf("matching residency refused: %+v error=%v", got, err)
			}
		})
	}
}

func TestLinkedInUnverifiedNativeReturnsNoTrustedMetadata(t *testing.T) {
	raw := "https://unrelated.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"
	im := &Importer{ingest: map[string]sources.Source{"hrsmart": nativeTestSource{}}}
	got, err := im.ResolveNative(context.Background(), db.Job{Source: "linkedin", Company: "Gist Limited", Title: "Engineer"}, raw, "")
	if err == nil || got.Entry.Provider != "" || got.Job.ExternalID != "" {
		t.Fatalf("unverified employer returned trusted metadata: %+v error=%v", got, err)
	}
}
