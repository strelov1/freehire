package handler

import (
	"slices"
	"testing"
)

// resumeProfile composes the deterministic dictionaries (skilltag/classify); these cases
// pin the fields it resolves from sample résumé text, that unresolved fields come back
// empty, and that it surfaces every category the résumé spans (not just one).
func TestResumeProfile(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		wantSeniority  string
		wantCategories []string // exact set the headline resolves (order-independent)
		wantSkills     []string // subset that must be present (order-independent)
		wantNoSkills   bool     // skills must be empty
	}{
		{
			name:           "full résumé resolves seniority, category and skills",
			text:           "Senior Backend Engineer\nBuilt payment services in Go and PostgreSQL, deployed on Kubernetes.",
			wantSeniority:  "senior",
			wantCategories: []string{"backend"},
			wantSkills:     []string{"postgresql", "kubernetes"},
		},
		{
			name:           "category without a grade leaves seniority empty (never guessed)",
			text:           "Frontend Developer with React and TypeScript.",
			wantCategories: []string{"frontend"},
			wantSkills:     []string{"react", "typescript"},
		},
		{
			// A résumé that spans several functions surfaces every category, primary first.
			name:           "multiple categories in the headline are all surfaced",
			text:           "Senior Backend Engineer & Data Engineer\nGo, PostgreSQL, Airflow.",
			wantSeniority:  "senior",
			wantCategories: []string{"data_engineering", "backend"}, // categoryOrder precedence
			wantSkills:     []string{"postgresql"},
		},
		{
			// Real résumés lead with name + contact lines and put the title below them;
			// the headline must skip that noise so the title still resolves.
			name:           "title below the contact block still resolves (contact lines skipped)",
			text:           "John Doe\njohn.doe@gmail.com | +1 555 123 4567\nSan Francisco, CA · linkedin.com/in/johndoe\n\nSenior Backend Engineer\n\nBuilds payment systems in Go, PostgreSQL, Kubernetes and AWS.",
			wantSeniority:  "senior",
			wantCategories: []string{"backend"},
			wantSkills:     []string{"aws", "kubernetes", "postgresql"},
		},
		{
			// The headline (title + summary top) is what classify sees; grade words buried
			// in the career history below must not over-promote the current grade.
			name:           "grade words deep in the CV history don't override the headline",
			text:           "Senior Backend Engineer\nBuilt payment platforms in Go.\nSkills: Kubernetes, PostgreSQL.\n\nExperience:\n2014 — reported to the Head of Product and the CTO.",
			wantSeniority:  "senior",
			wantCategories: []string{"backend"},
			wantSkills:     []string{"kubernetes", "postgresql"},
		},
		{
			name:       "skills-only text resolves no category/seniority",
			text:       "Proficient in Python.",
			wantSkills: []string{"python"},
		},
		{
			name:         "empty text resolves nothing",
			text:         "",
			wantNoSkills: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := resumeProfile(tc.text)
			if p.Seniority != tc.wantSeniority {
				t.Errorf("seniority = %q, want %q", p.Seniority, tc.wantSeniority)
			}
			if !slices.Equal(p.Categories, tc.wantCategories) {
				t.Errorf("categories = %v, want %v", p.Categories, tc.wantCategories)
			}
			if tc.wantNoSkills && len(p.Skills) != 0 {
				t.Errorf("skills = %v, want empty", p.Skills)
			}
			for _, s := range tc.wantSkills {
				if !slices.Contains(p.Skills, s) {
					t.Errorf("skills = %v, missing %q", p.Skills, s)
				}
			}
		})
	}
}

// Skills and Categories are always non-nil so they serialize as [] not null (the response
// contract the frontend relies on), even when nothing resolves.
func TestResumeProfileSlicesNeverNil(t *testing.T) {
	p := resumeProfile("")
	if p.Skills == nil {
		t.Error("Skills is nil; want non-nil empty slice")
	}
	if p.Categories == nil {
		t.Error("Categories is nil; want non-nil empty slice")
	}
}
