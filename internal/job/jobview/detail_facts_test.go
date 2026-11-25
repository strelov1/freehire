package jobview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/enrich"
)

func TestDetailFacts_LiftsEmploymentTypeSalaryAndSite(t *testing.T) {
	lo, hi := 1500, 2500
	j := Job{Company: "Hire Hangar", Enrichment: enrich.Enrichment{
		EmploymentType: "contract", SalaryMin: &lo, SalaryMax: &hi, SalaryCurrency: "USD", SalaryPeriod: "month",
	}}
	j.DetailFacts("https://hirehangar.com", "hirehangar.com")

	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The fixture's enrichment carries employment_type too, so read it off the root object.
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if root["employment_type"] != "contract" {
		t.Errorf("top-level employment_type = %v, want contract", root["employment_type"])
	}
	for _, want := range []string{
		`"salary":{"min":1500,"max":2500,"currency":"USD","period":"month"}`,
		`"company_website":"https://hirehangar.com"`,
		`"company_logo":"https://logo.freehire.me/Hire%20Hangar?domain=hirehangar.com"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("detail JSON lacks %s:\n%s", want, raw)
		}
	}
}

// Without a site on record the employer gets no logo: a name alone resolves to a namesake's
// mark often enough. Without a stated range there is no salary object at all.
func TestDetailFacts_OmitsWhatIsNotKnown(t *testing.T) {
	j := Job{Company: "Motive"}
	j.DetailFacts("", "")

	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, absent := range []string{`"employment_type"`, `"salary"`, `"company_website"`, `"company_logo"`} {
		if strings.Contains(string(raw), absent) {
			t.Errorf("detail JSON carries %s though nothing is known:\n%s", absent, raw)
		}
	}
}
