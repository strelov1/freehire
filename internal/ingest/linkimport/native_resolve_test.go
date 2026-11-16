package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

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
	got, err := (&Importer{ingest: reg}).ResolveNative(context.Background(), row, raw)
	if err != nil || got.Entry.Provider != "hrsmart" || got.Entry.Board != "gistworld.mua.hrdepartment.com" || got.Job.ExternalID != "gistworld.mua.hrdepartment.com:3735" || got.Job.Company != row.Company {
		t.Fatalf("native resolution = %+v, %v", got, err)
	}
	reg["hrsmart"] = nativeTestSource{jobs: []sources.Job{{ExternalID: "3736", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3736", Title: row.Title}}}
	if _, err := (&Importer{ingest: reg}).ResolveNative(context.Background(), row, raw); err == nil {
		t.Fatal("must not transfer to a different posting on the same ATS board")
	}
}
