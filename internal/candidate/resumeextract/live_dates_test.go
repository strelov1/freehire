//go:build llmlive

// A measurement against the configured model, not a unit test: it costs tokens and is
// behind the llmlive tag, so CI never runs it.
//
//	LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=<alias> go test -tags=llmlive ./internal/candidate/resumeextract/ -run ExperienceDates -v
//
// It checks one thing: given the dates the CV prints, does THIS model copy them to the right
// role. This complements the full Extract path's date-validation tests.
package resumeextract

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// datesLayoutCV is synthetic — no real candidate's text. Five consecutive roles, the
// shape that exposed the shift: each role's period sits on its own line under the title,
// and the periods chain (one role's start year is the next role's end year).
const datesLayoutCV = `EXPERIENCE
Price Analyst - Northwind Supply
08/2023 - 07/2024 | Stockholm, Sweden | 3D printing / manufacturing
   Built pricing collectors in Python and a supplier comparison tool for management.

Data and Market Analyst - Contoso Holding
05/2016 - 06/2024 | Moscow, Russia | cement plants holding
   Led the first centralized reporting system on MS SQL Server and VBA.

Data Analyst - Fabrikam Betting
12/2015 - 04/2016 | Moscow, Russia | betting / gaming
   Built automated reporting on betting databases with SQL, VBA and Excel.

Marketing Analyst - Adventure Finance
07/2013 - 02/2015 | Moscow, Russia | financial services / microfinance
   Ran customer research across 200+ branches and built segmentation and scoring.

Data Analyst - Tailspin Cement group
06/2010 - 04/2013 | Moscow, Russia | cement production
   Automated daily sales and production reporting for 14 cement plants.
`

func TestLive_ExperienceDatesFollowTheCV(t *testing.T) {
	c := liveClient(t)

	schema, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	// The options Extract passes, so this measures production and not a friendlier call.
	got := extractOnce(t, c, datesLayoutCV,
		llm.WithSchema(schemaName, schema), llm.WithReasoning(llm.ReasoningNone))

	want := map[string][2]perioddate.PeriodDate{
		"Northwind Supply":      {{Year: 2023, Month: 8}, {Year: 2024, Month: 7}},
		"Contoso Holding":       {{Year: 2016, Month: 5}, {Year: 2024, Month: 6}},
		"Fabrikam Betting":      {{Year: 2015, Month: 12}, {Year: 2016, Month: 4}},
		"Adventure Finance":     {{Year: 2013, Month: 7}, {Year: 2015, Month: 2}},
		"Tailspin Cement group": {{Year: 2010, Month: 6}, {Year: 2013, Month: 4}},
	}

	if len(got.Experience) != len(want) {
		t.Fatalf("got %d roles, want %d", len(got.Experience), len(want))
	}

	for _, e := range got.Experience {
		period, ok := want[e.Company]
		if !ok {
			t.Errorf("unexpected company %q", e.Company)
			continue
		}
		if e.Start == nil || *e.Start != period[0] {
			t.Errorf("%s: start = %s, want %s", e.Company, e.Start.Format(), period[0].Format())
		}
		if e.End == nil || *e.End != period[1] {
			t.Errorf("%s: end = %s, want %s", e.Company, e.End.Format(), period[1].Format())
		}
	}
}
