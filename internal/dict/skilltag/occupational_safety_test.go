package skilltag

import (
	"slices"
	"testing"
)

// The occupational-safety vocabulary. It matters more than the size of the population
// suggests, because nothing else can tag it: an HSE posting derives `is_tech = false`,
// the enrichment enqueue gate reads `is_tech IS TRUE`, so the LLM never sees these
// postings and this dictionary is the only thing that will ever put a skill on them.
// Measured on prod 2026-09-24, only 2,535 of 6,254 live HSE postings carried any skill
// at all, and the profession had no representation here whatsoever — no osha, no
// nebosh, no iso-45001, no hazop.
//
// The terms below were mined rather than written down: 2,500 live descriptions from
// this exact population, HTML-stripped, counted as 1–3-grams by document frequency with
// the existing vocabulary subtracted. The percentages are that document frequency.
func TestParse_OccupationalSafetyCore(t *testing.T) {
	cases := []struct {
		text string
		want string
		pct  string
	}{
		{"Ensure OSHA compliance across all three sites.", "osha", "27%"},
		{"Maintain our ISO 14001 environmental management system.", "iso-14001", "20%"},
		{"Own the ISO 45001 certification programme.", "iso-45001", "19%"},
		{"Lead emergency response drills and preparedness planning.", "emergency-response", "20%"},
		{"Specify and audit personal protective equipment for the site.", "personal-protective-equipment", "14%"},
		{"Conduct root cause analysis on every recordable.", "root-cause-analysis", "14%"},
		{"Own environmental compliance reporting to the state agency.", "environmental-compliance", "14%"},
		{"Carry out risk assessments before each lift.", "risk-assessment", "13%"},
		{"Lead incident investigations and close out the actions.", "incident-investigation", "13%"},
		{"Maintain the safety management system documentation.", "safety-management-system", "13%"},
		{"Support industrial hygiene monitoring across the plant.", "industrial-hygiene", "12%"},
		{"Report to the EPA under our air permit.", "epa", "11%"},
		{"Track corrective actions to closure.", "corrective-action", "29%"},
	}
	for _, tc := range cases {
		if got := Parse(tc.text); !slices.Contains(got, tc.want) {
			t.Errorf("Parse(%q) = %v, want it to contain %q (%s of mined corpus)", tc.text, got, tc.want, tc.pct)
		}
	}
}

func TestParse_OccupationalSafetySecondary(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Oversee hazardous waste and waste management programmes.", "waste-management"},
		{"Hold a current first aid certificate.", "first-aid"},
		{"Manage hazardous waste manifests and disposal.", "hazardous-waste"},
		{"Deliver toolbox talks to the crews each morning.", "toolbox-talks"},
		{"Plan and run the annual safety audit.", "safety-audit"},
		{"Drive hazard identification across the operation.", "hazard-identification"},
		{"Own contractor safety and the permit process.", "contractor-safety"},
		{"Enforce lockout tagout on all energised equipment.", "lockout-tagout"},
		{"Supervise confined space entries.", "confined-space"},
		{"Apply NFPA standards to the fire systems.", "nfpa"},
		{"Inspect fall protection anchors quarterly.", "fall-protection"},
		{"Review machine guarding on the production line.", "machine-guarding"},
		{"Issue hot work permits.", "hot-work"},
		{"Grow near miss reporting from the shop floor.", "near-miss-reporting"},
		{"Support the process safety management programme.", "process-safety-management"},
		{"Facilitate HAZOP studies for the new unit.", "hazop"},
		{"Administer the permit to work system.", "permit-to-work"},
		{"Write job safety analysis for non-routine tasks.", "job-safety-analysis"},
		{"Manage RCRA generator status and reporting.", "rcra"},
		{"Run our behavior based safety observation programme.", "behavior-based-safety"},
		{"Certify crews for working at height.", "working-at-height"},
	}
	for _, tc := range cases {
		if got := Parse(tc.text); !slices.Contains(got, tc.want) {
			t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, tc.want)
		}
	}
}

// The tool half of the craft. Each is a named EHS platform, the equivalent of naming
// Jira in a project-management posting.
func TestParse_OccupationalSafetyPlatforms(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Experience administering Enablon is a plus.", "enablon"},
		{"We run Intelex for incident tracking.", "intelex"},
		{"Migrating our EHS records into VelocityEHS.", "velocityehs"},
		{"Sphera is our environmental reporting platform.", "sphera"},
		{"Cority administration experience preferred.", "cority"},
	}
	for _, tc := range cases {
		if got := Parse(tc.text); !slices.Contains(got, tc.want) {
			t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, tc.want)
		}
	}
}

// The acronym collisions. Three letters that already mean something else here resolve
// only when the caller supplies the category, and two are excluded outright.
func TestParse_OccupationalSafetyAcronymsAreScoped(t *testing.T) {
	const text = "BBS programme ownership is part of this role."

	scoped := Parse(text, WithAcronymCategory("occupational_safety"))
	if !slices.Contains(scoped, "behavior-based-safety") {
		t.Errorf("Parse(%q, occupational_safety) = %v, want it to contain behavior-based-safety", text, scoped)
	}

	// The same three letters in a software posting are a Bulletin Board System, so
	// nothing resolves.
	unscoped := Parse(text, WithAcronymCategory("software_engineering"))
	if slices.Contains(unscoped, "behavior-based-safety") {
		t.Errorf("Parse(%q, software_engineering) = %v, want no behavior-based-safety", text, unscoped)
	}

	// CSP, CIH and CHMM are NOT in this table. They name credentials rather than skills,
	// and the dictionary invariant is explicit that a scoped acronym must resolve to a
	// canonical that already exists here — an acronym is another alias, never a new facet
	// value. They live in internal/dict/certification; see its test.
	const creds = "CSP, CIH or CHMM certification required."
	for _, unwanted := range []string{"csp", "cih", "chmm"} {
		if got := Parse(creds, WithAcronymCategory("occupational_safety")); slices.Contains(got, unwanted) {
			t.Errorf("Parse(%q, occupational_safety) = %v, want no %q: credentials are not skills", creds, got, unwanted)
		}
	}
}

// PSM keeps the meaning it already has. The scoped-acronym table holds one canonical per
// key, and Process Safety Management cannot join without widening its shape — measured,
// the acronym and the spelled-out phrase each appear in 2% of the corpus, so admitting
// only the phrase loses almost nothing and leaves every existing entry alone.
func TestParse_ProcessSafetyManagementIsPhraseOnly(t *testing.T) {
	const acronym = "PSM certification required for this role."
	if got := Parse(acronym, WithAcronymCategory("project_management")); !slices.Contains(got, "professional-scrum-master") {
		t.Errorf("Parse(%q, project_management) = %v, want it to contain professional-scrum-master", acronym, got)
	}
	if got := Parse(acronym, WithAcronymCategory("occupational_safety")); slices.Contains(got, "process-safety-management") {
		t.Errorf("Parse(%q, occupational_safety) = %v, want no process-safety-management: the acronym is not admitted", acronym, got)
	}

	const phrase = "Own the process safety management programme for the refinery."
	if got := Parse(phrase); !slices.Contains(got, "process-safety-management") {
		t.Errorf("Parse(%q) = %v, want it to contain process-safety-management", phrase, got)
	}
}

// ASP and DOT are excluded outright rather than scoped. The corpus probe that measured
// this vocabulary reported ASP in 17% of postings, and that was the prefix of "aspects"
// and "aspiring" — which is the reminder that a bare three-letter alias must be measured
// against live text before it is admitted, not reasoned about.
func TestParse_RejectedSafetyAcronyms(t *testing.T) {
	for _, text := range []string{
		"Consider all aspects of the operation.",
		"We are looking for an aspiring safety professional.",
		"Connect the dot between the finding and the fix.",
	} {
		got := Parse(text, WithAcronymCategory("occupational_safety"))
		for _, unwanted := range []string{"asp", "associate-safety-professional", "dot"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(%q, occupational_safety) = %v, want no %q", text, got, unwanted)
			}
		}
	}
}

// What these phrases do to a posting that is NOT about occupational safety — the half
// the first draft of this file was missing entirely, and the half a review found broken.
//
// A phrase match is a STRONG match unless the canonical is listed in
// nonCorroboratingPhrases, and one strong match releases every gated `ambiguousWords`
// token in the same text. Safety vocabulary is exactly the shape that doctrine was
// written for: "first aid", "corrective action" and "risk assessment" appear in
// childcare, warehouse, nursing and retail postings at scale, and naming a safety
// practice is evidence that the posting is SUBJECT to it, never that the person filling
// it is technical. Before the fix, a warehouse posting came back carrying `react` and
// `sketch`.
func TestParse_SafetyPhrasesDoNotVouchForGatedWords(t *testing.T) {
	cases := []struct {
		label string
		text  string
	}{
		{
			"warehouse associate",
			"Complete first aid training and follow lockout/tagout on the assembly line. " +
				"Report near misses. You must react to changing priorities and sketch out " +
				"improvements in an agile way.",
		},
		{
			"registered nurse",
			"Maintain first aid and CPR certification. Participate in emergency response " +
				"and root cause analysis. Must react calmly and work in an agile team.",
		},
		{
			"retail store manager",
			"Issue corrective action where needed and support safety audits. Review " +
				"analytics in our CRM and react to store performance.",
		},
	}
	// The gated technical words that have no business on any of these postings.
	gated := []string{"react", "sketch", "assembly", "crm", "analytics", "agile", "rest", "scrum"}
	for _, tc := range cases {
		got := Parse(tc.text)
		for _, unwanted := range gated {
			if slices.Contains(got, unwanted) {
				t.Errorf("%s: Parse(...) = %v, want no %q — a safety phrase must not vouch for a gated word", tc.label, got, unwanted)
			}
		}
	}

	// …while still tagging the safety terms themselves. A non-corroborating phrase is
	// still a match; it just does not rescue its neighbours.
	got := Parse(cases[0].text)
	for _, want := range []string{"first-aid", "lockout-tagout", "near-miss-reporting"} {
		if !slices.Contains(got, want) {
			t.Errorf("Parse(warehouse) = %v, want it to still contain %q", got, want)
		}
	}
}
