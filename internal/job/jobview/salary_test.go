package jobview

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
)

func salaryRow(desc string) db.Job {
	return db.Job{
		ID: 7, Source: "whatjobs", ExternalID: "wj:1", Title: "Software Engineer",
		Company: "Obscurity Labs", CompanySlug: "obscurity-labs",
		PublicSlug:  "software-engineer-obscurity-labs-7",
		Countries:   []string{"us"},
		Description: desc,
		CreatedAt:   pgtype.Timestamptz{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}
}

func TestFromRow_LabeledSalaryWhenEnrichmentStatesNone(t *testing.T) {
	view, err := FromRow(salaryRow("<p>Salary: $120,000 - 185,000 per year</p>"))
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.Enrichment.SalaryMin == nil || *view.Enrichment.SalaryMin != 120000 ||
		view.Enrichment.SalaryMax == nil || *view.Enrichment.SalaryMax != 185000 ||
		view.Enrichment.SalaryCurrency != "USD" || view.Enrichment.SalaryPeriod != "year" {
		t.Fatalf("salary = %+v", view.Enrichment)
	}
}

func TestFromRow_EnrichmentSalaryWinsOverLabel(t *testing.T) {
	row := salaryRow("<p>Salary: $120,000 - 185,000 per year</p>")
	raw, err := json.Marshal(enrich.Enrichment{SalaryMin: ptr(1)})
	if err != nil {
		t.Fatal(err)
	}
	row.Enrichment = raw
	view, err := FromRow(row)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.Enrichment.SalaryMin == nil || *view.Enrichment.SalaryMin != 1 || view.Enrichment.SalaryMax != nil {
		t.Fatalf("salary = %+v, want the enrichment figure", view.Enrichment)
	}
}

func TestFromRow_SourceSalaryWinsOverLabel(t *testing.T) {
	row := salaryRow("<p>Salary: $120,000 - 185,000 per year</p>")
	row.SalaryMinSource = pgtype.Int4{Int32: 90000, Valid: true}
	row.SalaryCurrencySource = "EUR"
	row.SalaryPeriodSource = "year"
	view, err := FromRow(row)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.Enrichment.SalaryMin == nil || *view.Enrichment.SalaryMin != 90000 ||
		view.Enrichment.SalaryCurrency != "EUR" || view.Enrichment.SalaryPeriod != "year" {
		t.Fatalf("salary = %+v, want the source figure", view.Enrichment)
	}
}
