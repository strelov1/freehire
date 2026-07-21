package cv

import (
	"context"
	"os"
	"path/filepath"
)

// sampleDocument is the fixed résumé rendered into every template's preview thumbnail. It is
// deliberately generic (a fictional engineer) and sized to fit one A4 page so each preview is
// a single .svg. Kept small on purpose — previews are illustrative, not a full CV.
func sampleDocument() Document {
	return Document{
		Header: Header{
			FullName: "Jordan Rivera",
			Email:    "jordan.rivera@example.com",
			Phone:    "+1 555 0142",
			Location: "Remote",
			Links:    []string{"linkedin.com/in/jrivera", "github.com/jrivera"},
		},
		Summary: "Senior software engineer with 8+ years building reliable backends and developer tooling.",
		Experience: []ExperienceItem{
			{Role: "Senior Software Engineer", Company: "Northwind", Location: "Remote", Start: "2021", End: "Present",
				Summary: "SaaS platform serving 5M+ users.",
				Bullets: []string{"Cut API latency 40% by reworking hot query paths.", "Led migration to event-driven services with zero downtime."},
				Stack:   []string{"Go", "PostgreSQL", "Kafka"}},
			{Role: "Software Engineer", Company: "Acme Corp", Location: "Berlin", Start: "2017", End: "2021",
				Bullets: []string{"Built the billing service from scratch, now core revenue infra."},
				Stack:   []string{"Python", "React"}},
		},
		Education: []EducationItem{{Degree: "BSc", Field: "Computer Science", Institution: "State University", Start: "2013", End: "2017"}},
		Skills: []SkillGroup{
			{Group: "Languages", Items: []string{"Go", "Python", "TypeScript", "SQL"}},
			{Group: "Infra", Items: []string{"Kubernetes", "Docker", "AWS"}},
		},
		Languages: []Language{{Name: "English", Level: "Native"}, {Name: "Spanish", Level: "B2"}},
		Projects:  []Project{{Name: "opensched", Link: "opensched.dev", Bullets: []string{"A tiny cron scheduler with a web UI."}}},
	}
}

// GeneratePreviews renders the sample document through every registered template to SVG and
// writes <outDir>/<id>.svg, returning the template ids written. It iterates the registry so the
// preview set can never drift from it. Dev-only (invoked by cmd/cv-previews); the SVGs are
// committed so the frontend serves them statically.
func GeneratePreviews(ctx context.Context, r *TypstRenderer, outDir string) ([]string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	doc := sampleDocument()
	var written []string
	for _, ti := range Templates() {
		tmpl, err := ResolveTemplate(ti.ID)
		if err != nil {
			return written, err
		}
		svg, err := r.compile(ctx, doc, tmpl, "svg")
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(filepath.Join(outDir, ti.ID+".svg"), svg, 0o644); err != nil {
			return written, err
		}
		written = append(written, ti.ID)
	}
	return written, nil
}
