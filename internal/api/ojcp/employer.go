package ojcp

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
)

// EmployerContextFrom projects a company into OJCP's employer context — what an agent can
// learn about an employer beyond the one posting it found.
//
// The key is the company slug, which is what a posting publishes as `ojcp_employer_id`, so
// an agent moves from a search result to this tool without a second lookup.
func EmployerContextFrom(c db.Company) EmployerContextResponse {
	return EmployerContextResponse{
		EmployerID:  c.Slug,
		Name:        c.Name,
		Description: c.Tagline.String,
		Industries:  c.Industries,
		HQLocation:  headquarters(c),
		// Floor at zero: the column is rollup-materialised and the schema's `integer` would
		// accept a negative.
		OpenRolesCount: max(int(c.JobCount), 0),
	}.Finalize()
}

// headquarters is the company's home, or nil where we know of none. An empty block would
// say "we know where this company is" and then say nothing.
//
// The country is uppercased: stored lowercase like every geography facet, while the
// standard's fields are ISO 3166-1 alpha-2.
func headquarters(c db.Company) *EmployerLocation {
	country := strings.ToUpper(strings.TrimSpace(c.HqCountry.String))
	if country == "" {
		return nil
	}
	return &EmployerLocation{Country: country}
}
