package linkimport

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
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
	for _, source := range []linkedInTestSource{
		{err: errors.New("refused")},
		{posting: sources.Job{ExternalID: "123", Removed: true}},
		{posting: sources.Job{ExternalID: "456", ApplyMethod: sources.ApplyOnSite}},
	} {
		im := &Importer{ingest: map[string]sources.Source{"linkedin": source}}
		if _, ok, err := im.Resolve(context.Background(), "https://www.linkedin.com/jobs/view/123/", Board{}); err == nil || ok {
			t.Fatalf("unsafe posting imported: ok=%v err=%v", ok, err)
		}
	}
}
