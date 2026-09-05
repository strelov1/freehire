package experience

import (
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// This file is the ONE adapter that names a concrete source. Import itself takes
// []ImportEntry and knows nothing about résumés, so the bank stays source-agnostic; a
// LinkedIn export or a bulk paste would add a sibling file here rather than change it.

// EntriesFromResume maps a parsed résumé onto the bank's import shape. This is
// the seam that keeps internal/candidate/experience free of any one importer's vocabulary: roles
// and portfolio projects are both places where evidence was produced, and their
// highlights are the evidence.
//
// Nothing from the contact block crosses over. The bank holds what the candidate did; who
// they are stays on the résumé record, where the contact whitelist already governs it.
func EntriesFromResume(st resumeextract.Structured) []ImportEntry {
	entries := make([]ImportEntry, 0, len(st.Experience)+len(st.Projects))

	for _, role := range st.Experience {
		entries = append(entries, ImportEntry{
			Employment: Employment{
				Kind:     KindJob,
				Company:  role.Company,
				Role:     role.Title,
				Location: role.Location,
				Start:    role.Start,
				End:      role.End,
				Current:  role.Current,
				Summary:  role.Summary,
				Stack:    role.Stack,
			},
			Claims: role.Highlights,
		})
	}

	// A portfolio project is a place too: it is named, it produced achievements, and a
	// tailored CV draws on it the same way it draws on a job.
	for _, project := range st.Projects {
		entries = append(entries, ImportEntry{
			Employment: Employment{
				Kind:    KindProject,
				Company: project.Name,
				Link:    project.Link,
			},
			Claims: project.Highlights,
		})
	}

	return entries
}
