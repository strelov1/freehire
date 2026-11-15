package coverletter

import (
	"slices"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// SelectedProfile keeps chronology and education; achievement prose comes only from selected atoms.
func SelectedProfile(p resumeextract.Professional) resumeextract.Professional {
	p.Headline = ""
	p.Summary = ""
	p.Experience = slices.Clone(p.Experience)
	for i := range p.Experience {
		p.Experience[i].Summary = ""
		p.Experience[i].Highlights = nil
	}
	p.Projects = slices.Clone(p.Projects)
	for i := range p.Projects {
		p.Projects[i].Highlights = nil
	}
	return p
}
