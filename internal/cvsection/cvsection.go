// Package cvsection segments a CV's plain text into a Skills section and the body
// (everything else), then skill-tags each segment independently. It is pure and
// I/O-free (mirrors internal/atscheck): the caller supplies the extracted CV text.
//
// The split is the basis for the "strong vs hidden" skill status and the coherence
// score on the verdict page — a skill declared in the Skills section reads
// differently from one that only surfaces in the experience prose. Heading
// detection is line-based and dictionary-driven (never guesses): when no Skills
// heading is found, `declared` is empty and every tagged skill falls into `body`.
package cvsection

import (
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// skillsHeaders and otherHeaders are the section headings we recognize (EN + RU),
// normalized (lowercased, trimmed, trailing colon stripped). A line matching a
// Skills heading opens the Skills section; a line matching any heading closes it.
var (
	skillsHeaders = set(
		"skills", "technical skills", "core skills", "key skills",
		"core competencies", "technologies", "tech stack", "toolset",
		"навыки", "ключевые навыки", "технологии", "стек",
	)
	otherHeaders = set(
		"experience", "work experience", "professional experience",
		"employment", "employment history", "work history", "education",
		"projects", "summary", "profile", "objective", "certifications",
		"languages", "publications", "awards", "interests",
		"опыт", "опыт работы", "образование", "проекты", "о себе",
	)
)

// Parse splits cvText into the Skills section vs the body and skill-tags each,
// returning the `declared` (Skills-section), `body` (everything else), and `all`
// (their union) skill-slug sets. Pure and deterministic.
func Parse(cvText string) (declared, body, all []string) {
	var skillsLines, bodyLines []string
	inSkills := false
	for _, line := range strings.Split(cvText, "\n") {
		if skills, ok := heading(line); ok {
			inSkills = skills
			bodyLines = append(bodyLines, line) // the heading itself carries no skills
			continue
		}
		if inSkills {
			skillsLines = append(skillsLines, line)
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	declared = skilltag.Parse(strings.Join(skillsLines, "\n"), skilltag.WithResumeAcronyms())
	body = skilltag.Parse(strings.Join(bodyLines, "\n"), skilltag.WithResumeAcronyms())
	return declared, body, union(declared, body)
}

// heading reports whether the line is a section heading and, if so, whether it is
// a Skills heading.
func heading(line string) (skills, ok bool) {
	norm := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(strings.ToLower(line)), ":"))
	if norm == "" {
		return false, false
	}
	if skillsHeaders[norm] {
		return true, true
	}
	if otherHeaders[norm] {
		return false, true
	}
	return false, false
}

func union(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}
