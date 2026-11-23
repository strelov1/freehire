package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

type nativeEmployerCatalog struct{ company string }

func (c nativeEmployerCatalog) Entry(_ context.Context, source, board string) (sources.CompanyEntry, bool, error) {
	return sources.CompanyEntry{Provider: source, Board: board, Company: c.company}, true, nil
}

type nativeEmployerProof struct{}

func (nativeEmployerProof) Resolve(context.Context, string) (string, string, string, bool) {
	return "", "", "", false
}
func (nativeEmployerProof) ResolveCompany(_ context.Context, raw string) (string, string, string, bool) {
	return "hrsmart", "gistworld.mua.hrdepartment.com", "", raw == "https://www.gistworld.com"
}

type nativeTestSource struct{ jobs []sources.Job }

func (nativeTestSource) Provider() string { return "hrsmart" }
func (s nativeTestSource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	jobs := append([]sources.Job(nil), s.jobs...)
	for i := range jobs {
		jobs[i].Company = e.Company
	}
	return jobs, nil
}

func TestResolveNativeUsesTheCrawlIdentity(t *testing.T) {
	const raw = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn"
	row := db.Job{Source: "linkedin", Title: "Java Engineer", Company: "Gist Limited"}
	reg := map[string]sources.Source{"hrsmart": nativeTestSource{jobs: []sources.Job{{ExternalID: "3735", URL: raw, Title: row.Title, Description: "Build Java services."}}}}
	got, err := (&Importer{ingest: reg, catalog: nativeEmployerCatalog{company: row.Company}}).ResolveNative(context.Background(), row, raw, "")
	if err != nil || got.Entry.Provider != "hrsmart" || got.Entry.Board != "gistworld.mua.hrdepartment.com" || got.Job.ExternalID != "gistworld.mua.hrdepartment.com:3735" || got.Job.Company != row.Company {
		t.Fatalf("native resolution = %+v, %v", got, err)
	}
	reg["hrsmart"] = nativeTestSource{jobs: []sources.Job{{ExternalID: "3736", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3736", Title: row.Title, Description: "A readable different posting."}}}
	if _, err := (&Importer{ingest: reg, catalog: nativeEmployerCatalog{company: row.Company}}).ResolveNative(context.Background(), row, raw, ""); err == nil {
		t.Fatal("must not transfer to a different posting on the same ATS board")
	}
}

func TestResolveNativeRefusesAnUnverifiedEmployerBoard(t *testing.T) {
	row := db.Job{Source: "linkedin", Title: "Java Engineer", Company: "Gist Limited"}
	raw := "https://unrelated.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"
	reg := map[string]sources.Source{"hrsmart": nativeTestSource{jobs: []sources.Job{{ExternalID: "3735", URL: raw, Title: row.Title, Description: "A different employer hiring for the same title."}}}}
	if _, err := (&Importer{ingest: reg}).ResolveNative(context.Background(), row, raw, ""); err == nil {
		t.Fatal("a fallback employer copied from discovery is not independent evidence")
	}
}

func TestResolveNativeChecksTheEmployerWebsiteForANewBoard(t *testing.T) {
	const raw = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"
	row := db.Job{Source: "linkedin", Title: "Java Engineer", Company: "Gist Limited"}
	reg := map[string]sources.Source{"hrsmart": nativeTestSource{jobs: []sources.Job{{ExternalID: "3735", URL: raw, Title: row.Title, Description: "Build Java services."}}}}
	im := &Importer{ingest: reg, boards: nativeEmployerProof{}}
	if _, err := im.ResolveNative(context.Background(), row, raw, "https://www.gistworld.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := im.ResolveNative(context.Background(), row, "https://unrelated.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", "https://www.gistworld.com"); err == nil {
		t.Fatal("same title on an unrelated board passed the employer website proof")
	}
}

func TestResolveNativeUsesStoredUSLeverBoard(t *testing.T) {
	const raw = "https://jobs.lever.co/acme/22222222-2222-4222-8222-222222222222"
	row := db.Job{Title: "Engineer", Company: "Acme US"}
	catalog := linkedInRegionCatalog{
		"us": {Provider: "lever", Board: "acme", Region: "us", Company: row.Company},
		"eu": {Provider: "lever", Board: "acme", Region: "eu", Company: "Acme EU"},
	}
	im := &Importer{ingest: map[string]sources.Source{"lever": linkedInLeverSource{}}, catalog: catalog}
	got, err := im.ResolveNative(context.Background(), row, raw, "")
	if err != nil || got.Entry.Region != "us" || got.Job.Company != row.Company {
		t.Fatalf("native resolution = %+v, %v", got, err)
	}
}
