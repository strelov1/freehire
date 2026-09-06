package jobfacts

import (
	"strings"
	"testing"
)

var realish = `<div><h2>About the role</h2><p>We are a fast growing fintech.</p>` +
	strings.Repeat(`<p>You will build and operate services in Go and Postgres, working closely with product.</p>`, 12) +
	`<h3>Requirements</h3><ul>` + strings.Repeat(`<li>5+ years of backend engineering, English B2</li>`, 8) + `</ul>` +
	`<h3>Nice to have</h3><ul>` + strings.Repeat(`<li>Kubernetes, Terraform, a PMP</li>`, 6) + `</ul>` +
	`<h3>What we offer</h3><ul>` + strings.Repeat(`<li>Competitive salary and equity</li>`, 6) + `</ul></div>`

var noPreferred = strings.ReplaceAll(realish, "Nice to have", "Also relevant")

// The derive path runs these four matchers over every posting in the catalogue, and
// cmd/backfill-derive runs them over all ~11M rows in one pass, so what masking costs
// here is what it costs there. Both shapes are measured because the optional-clause
// mask is the expensive half and a posting with no preferred section still pays for the
// scan that finds none.
func BenchmarkDerivePathWithPreferredSection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		EmploymentType("Backend Engineer", realish)
		ExperienceYearsMin(realish)
		EducationLevel(realish)
		EnglishLevel(realish)
	}
}

func BenchmarkDerivePathWithoutPreferredSection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		EmploymentType("Backend Engineer", noPreferred)
		ExperienceYearsMin(noPreferred)
		EducationLevel(noPreferred)
		EnglishLevel(noPreferred)
	}
}
