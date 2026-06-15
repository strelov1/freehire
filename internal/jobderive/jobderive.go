// Package jobderive computes the deterministic, dictionary-derived facets of a job
// from its raw content fields: the company and public slugs, the geography
// (countries/regions/work-mode), and the skill tags. It is the single home for the
// derivation shared by the ingest pipeline and the moderator write path, so both
// produce identical facets from the same inputs.
package jobderive

import (
	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/skilltag"
)

// Input is the raw job content the derivation reads. Source and ExternalID are the
// already-resolved storage identity (the caller decides how to namespace them); they
// feed the public slug so it is stable with the dedup key. WorkMode is the caller's
// structured signal (an ATS workplace-type enum), if any — it wins over the parsed hint.
type Input struct {
	Title       string
	Company     string
	Source      string
	ExternalID  string
	Location    string
	Description string
	WorkMode    string
}

// Derived is the set of facets computed from an Input.
type Derived struct {
	CompanySlug string
	PublicSlug  string
	Countries   []string
	Regions     []string
	WorkMode    string
	Skills      []string
	Seniority   string
	Category    string
}

// Derive computes the slugs and dictionary facets for a job. Geography, skills, and the
// title classification (seniority/category) come from the curated dictionaries (which
// emit nothing for what they cannot resolve); the work-mode precedence is structured
// signal first, then the location parser's hint, then a conservative phrase match in
// the description.
func Derive(in Input) Derived {
	geo := location.Parse(in.Location)
	// Work-mode precedence: structured (ATS) → location marker → description phrase.
	// Each lower source only fills a value the higher ones left empty.
	workMode := in.WorkMode
	if workMode == "" {
		workMode = geo.WorkMode
	}
	if workMode == "" {
		workMode = location.WorkModeFromDescription(in.Description)
	}
	class := classify.Parse(in.Title)
	return Derived{
		CompanySlug: normalize.Slug(in.Company),
		PublicSlug:  normalize.JobSlug(in.Title, in.Company, in.Source, in.ExternalID),
		Countries:   geo.Countries,
		Regions:     geo.Regions,
		WorkMode:    workMode,
		Skills:      skilltag.Parse(in.Description),
		Seniority:   class.Seniority,
		Category:    class.Category,
	}
}
