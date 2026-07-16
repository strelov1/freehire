package cv

import "github.com/strelov1/freehire/internal/resumeextract"

// Seed builds a Document from the user's extracted résumé structure so a new CV starts
// pre-filled instead of blank. It is a pure field mapping: the caller is responsible for
// Sanitize before persisting (the source Structured is already sanitized on extraction).
//
// Extracted skills seed a single "Skills" group; per-language levels are not extracted,
// so language levels are left for the user to fill in.
func Seed(s resumeextract.Structured) Document {
	// The tagline under the name is the CV's summary. Prefer the extracted summary; fall
	// back to the headline line when the résumé stated no separate summary.
	summary := s.Summary
	if summary == "" {
		summary = s.Headline
	}
	doc := Document{
		Header: Header{
			FullName: s.FullName,
			Email:    s.Email,
			Phone:    s.Phone,
			Location: s.Location,
			Links:    s.Links,
		},
		Summary: summary,
	}

	for _, e := range s.Experience {
		exp := ExperienceItem{
			Role:     e.Title,
			Company:  e.Company,
			Location: e.Location,
			Start:    e.Start,
			End:      e.End,
			Summary:  e.Summary,
			Bullets:  e.Highlights,
			Stack:    e.Stack,
		}
		doc.Experience = append(doc.Experience, exp)
	}

	for _, ed := range s.Education {
		doc.Education = append(doc.Education, EducationItem{
			Institution: ed.Institution,
			Degree:      ed.Degree,
			End:         ed.Year,
		})
	}

	for _, lang := range s.Languages {
		doc.Languages = append(doc.Languages, Language{Name: lang})
	}

	// The extracted skills seed a single unnamed group (the "SKILLS" section heading is
	// enough — a "Skills:" group label under it would be redundant); the user can split
	// them into named groups in the editor. Empty when the CV stated none.
	if len(s.Skills) > 0 {
		doc.Skills = []SkillGroup{{Items: s.Skills}}
	}

	for _, p := range s.Projects {
		doc.Projects = append(doc.Projects, Project{
			Name:    p.Name,
			Link:    p.Link,
			Bullets: p.Highlights,
		})
	}

	return doc
}
