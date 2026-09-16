package ojcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyPathPointsAtTheFormNotTheDescription(t *testing.T) {
	// Lever serves the description and the form on two different pages. This repo already
	// paid for that once: a live attempt parked as `unrecognized_form_layout` on a page
	// that loaded fine and had no form on it. The schema calls this field "the application
	// entry point", so handing an agent the description page repeats the same failure.
	row := openPostingRow()
	row.Source = "lever"
	row.URL = "https://jobs.lever.co/acme/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037"

	path := projector().JobPosting(viewOf(row), capturedForm("lever")).ApplyPaths[0]

	if !strings.Contains(path.URL, "/apply") {
		t.Errorf("url = %q, want the Lever apply page", path.URL)
	}
}

func TestApplyPathLeavesOtherPlatformsAlone(t *testing.T) {
	// Greenhouse serves the form on the posting page itself. Appending /apply there would
	// invent a 404.
	path := projector().JobPosting(openPosting(), capturedForm("greenhouse")).ApplyPaths[0]

	if strings.Contains(path.URL, "/apply") {
		t.Errorf("url = %q, want the posting page unchanged for greenhouse", path.URL)
	}
}

func TestDescriptionReachesTheAgentAsText(t *testing.T) {
	// OJCP's description field is "Full job description text" and the schema offers no
	// format parameter, unlike our own agent search. Handing over stored markup makes an
	// agent pay several times the tokens to read it, and puts tags into anything it quotes
	// to a candidate.
	row := openPostingRow()
	row.Description = `<div class="intro"><p><strong>About us</strong></p><p>We build things.</p></div>`

	posting := projector().JobPosting(viewOf(row), nil)

	if strings.Contains(posting.Description, "<") {
		t.Errorf("description carries markup: %q", posting.Description)
	}
	if !strings.Contains(posting.Description, "We build things.") {
		t.Errorf("description lost its text: %q", posting.Description)
	}
}

func TestSalaryIsOmittedWhereThePeriodCannotBeStated(t *testing.T) {
	// Our vocabulary has "day"; OJCP's unitText enum does not. Publishing the figures
	// without a period lets an agent read a EUR 400/day contract as EUR 400 a year and
	// bury a six-figure role, or drop it against a salary_min filter.
	row := openPostingRow()
	row.Enrichment = json.RawMessage(`{"salary_min":400,"salary_currency":"EUR","salary_period":"day"}`)

	posting := projector().JobPosting(viewOf(row), nil)

	if posting.BaseSalary != nil {
		t.Errorf("baseSalary = %+v, want nil when the period cannot be expressed", posting.BaseSalary)
	}
}

func TestSalaryIsOmittedWithoutACurrency(t *testing.T) {
	// A bare number in a field the schema documents as carrying an ISO 4217 code is worse
	// than no figure: an agent comparing it against another posting's dollars is comparing
	// nothing.
	row := openPostingRow()
	row.Enrichment = json.RawMessage(`{"salary_min":120000,"salary_period":"year"}`)

	posting := projector().JobPosting(viewOf(row), nil)

	if posting.BaseSalary != nil {
		t.Errorf("baseSalary = %+v, want nil without a currency", posting.BaseSalary)
	}
}

func TestEmployerCarriesTheKeyTheThirdToolNeeds(t *testing.T) {
	// get_employer_context takes an employer_id, and a posting's employer block is the only
	// place an agent can learn one. Without it the tool is unreachable from a search result.
	posting := projector().JobPosting(openPosting(), nil)

	if posting.Employer.OJCPEmployerID != "acme" {
		t.Errorf("employer.ojcp_employer_id = %q, want the company slug", posting.Employer.OJCPEmployerID)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with an employer id rejected: %v", err)
	}
}

func TestAnUnsetOriginNeverPublishesARelativeURL(t *testing.T) {
	// `url` is format: uri, so a deployment that forgot to configure the origin would serve
	// "/jobs/<slug>" to every agent — a path nothing can dereference.
	posting := NewProjector("", greenhouseOnly).JobPosting(openPosting(), nil)

	if strings.HasPrefix(posting.URL, "/") {
		t.Errorf("url = %q, want no relative URL published", posting.URL)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting from an origin-less projector rejected: %v", err)
	}
}
