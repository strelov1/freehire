package privatejob_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/privatejob"
)

// fakeQueries captures every InsertPrivateJob call it is handed and returns a canned
// row, so the writer's derivation and param-mapping can be tested without a database.
type fakeQueries struct {
	calls []db.InsertPrivateJobParams
	next  int64
}

func (f *fakeQueries) InsertPrivateJob(_ context.Context, arg db.InsertPrivateJobParams) (db.Job, error) {
	f.calls = append(f.calls, arg)
	f.next++
	return db.Job{
		ID:          f.next,
		Source:      arg.Source,
		ExternalID:  arg.ExternalID,
		URL:         arg.URL,
		Title:       arg.Title,
		Company:     arg.Company,
		CompanySlug: arg.CompanySlug,
		PublicSlug:  arg.PublicSlug,
		Description: arg.Description,
		Countries:   arg.Countries,
		Regions:     arg.Regions,
		Cities:      arg.Cities,
		Skills:      arg.Skills,
		CreatedBy:   pgtype.Int8{Int64: arg.CreatedBy, Valid: true},
		IsPrivate:   true,
	}, nil
}

func TestCreate_DerivesFacetsAndPersists(t *testing.T) {
	f := &fakeQueries{}
	w := privatejob.NewWriter(f)

	_, err := w.Create(context.Background(), 42, privatejob.SourcePasted, privatejob.Input{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang and PostgreSQL.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("InsertPrivateJob calls = %d, want 1", len(f.calls))
	}
	got := f.calls[0]
	if !reflect.DeepEqual(got.Skills, []string{"go", "postgresql"}) {
		t.Errorf("Skills = %v, want [go postgresql]", got.Skills)
	}
	if got.CreatedBy != 42 {
		t.Errorf("CreatedBy = %d, want 42", got.CreatedBy)
	}
	if got.Source != privatejob.SourcePasted {
		t.Errorf("Source = %q, want %q", got.Source, privatejob.SourcePasted)
	}
	if got.Title != "Senior Go Developer" {
		t.Errorf("Title = %q, want the submitted title", got.Title)
	}
	if got.Company != "Acme" {
		t.Errorf("Company = %q, want Acme", got.Company)
	}
	if got.CompanySlug != "acme" {
		t.Errorf("CompanySlug = %q, want acme", got.CompanySlug)
	}
	if got.PublicSlug == "" {
		t.Error("PublicSlug is empty, want a minted slug")
	}
	if len(got.Countries) == 0 || got.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de ...] (derived from Location)", got.Countries)
	}
}

// A private submission can carry a scraped third-party page's markup (the
// unrecognized-URL branch), and jobs.description is rendered as trusted HTML on the
// job-detail page — so it must go through the same sanitizer every other write path
// uses, not be persisted verbatim.
func TestCreate_SanitizesDescription(t *testing.T) {
	f := &fakeQueries{}
	w := privatejob.NewWriter(f)

	_, err := w.Create(context.Background(), 1, privatejob.SourceWeblink, privatejob.Input{
		Title:       "Engineer",
		Description: `<script>alert(1)</script><p>Real content.</p>`,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := f.calls[0].Description
	if strings.Contains(got, "<script") {
		t.Errorf("Description = %q, want no <script> tag", got)
	}
	if !strings.Contains(got, "Real content.") {
		t.Errorf("Description = %q, want the sanitized content preserved", got)
	}
}

func TestCreate_ExternalIDsAreUniquePerSubmission(t *testing.T) {
	f := &fakeQueries{}
	w := privatejob.NewWriter(f)
	in := privatejob.Input{Title: "Backend Engineer", Company: "Acme", Description: "Go and Kubernetes."}

	if _, err := w.Create(context.Background(), 1, privatejob.SourcePasted, in); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	if _, err := w.Create(context.Background(), 1, privatejob.SourcePasted, in); err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("InsertPrivateJob calls = %d, want 2", len(f.calls))
	}
	if f.calls[0].ExternalID == f.calls[1].ExternalID {
		t.Errorf("both submissions got ExternalID %q, want distinct values", f.calls[0].ExternalID)
	}
}

func TestCreate_TitleFallsBackToFirstLineOfDescription(t *testing.T) {
	f := &fakeQueries{}
	w := privatejob.NewWriter(f)

	_, err := w.Create(context.Background(), 1, privatejob.SourcePasted, privatejob.Input{
		Description: "Backend Engineer at Acme\nWe build things.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := f.calls[0].Title; got != "Backend Engineer at Acme" {
		t.Errorf("Title = %q, want first line of description", got)
	}
}

func TestCreate_TitleFallsBackToPlaceholderWhenDescriptionBlank(t *testing.T) {
	f := &fakeQueries{}
	w := privatejob.NewWriter(f)

	_, err := w.Create(context.Background(), 1, privatejob.SourcePasted, privatejob.Input{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := f.calls[0].Title; got == "" {
		t.Errorf("Title = %q, want a non-empty placeholder", got)
	}
}

func TestCreate_URLSourceIsRecorded(t *testing.T) {
	f := &fakeQueries{}
	w := privatejob.NewWriter(f)

	_, err := w.Create(context.Background(), 1, privatejob.SourceWeblink, privatejob.Input{
		Title:       "Engineer",
		Description: "Some scraped JD text.",
		URL:         "https://example.com/careers/123",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := f.calls[0]
	if got.Source != privatejob.SourceWeblink {
		t.Errorf("Source = %q, want %q", got.Source, privatejob.SourceWeblink)
	}
	if got.URL != "https://example.com/careers/123" {
		t.Errorf("URL = %q, want the submitted URL", got.URL)
	}
}
