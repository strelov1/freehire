package jobderive

import (
	"testing"
)

func TestDerive_StructuredWorkModeWins(t *testing.T) {
	got := Derive(Input{
		Title:      "Dev",
		Company:    "Acme",
		Source:     "greenhouse",
		ExternalID: "board:1",
		Location:   "Remote - Germany",
		WorkMode:   "hybrid",
	})
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (structured wins over parsed remote)", got.WorkMode)
	}
}

// Work-mode source precedence: structured → location → description. The
// description is the lowest-priority source and only fills a value the structured
// signal and the location marker both left empty.
func TestDerive_DescriptionFillsWorkModeWhenLocationSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin", // no work-mode marker
		Description: "This is a fully remote position open to the EU.",
	})
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (description fills when location silent)", got.WorkMode)
	}
	// The beacon city is forwarded from location.Parse to the cities facet.
	if len(got.Cities) != 1 || got.Cities[0] != "Berlin" {
		t.Errorf("Cities = %v, want [Berlin]", got.Cities)
	}
}

func TestDerive_DescriptionContradictsLocationRemoteWithHybrid(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany",       // location marker → remote
		Description: "This is a hybrid role.", // explicit arrangement contradicts the location bucket
	})
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (explicit arrangement contradicts remote)", got.WorkMode)
	}
}

func TestDerive_StructuredBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin", // silent
		WorkMode:    "onsite", // structured
		Description: "This is a fully remote position.",
	})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (structured beats description)", got.WorkMode)
	}
}

// The contradiction tier sits ABOVE the precedence chain: a description that explicitly
// denies remote work overrules a remote work mode whichever source produced it. See
// location.RemoteContradicted.
func TestDerive_DescriptionContradictsLocationRemote(t *testing.T) {
	got := Derive(Input{
		Title:      "Senior Structural Test Engineer",
		Company:    "NVIDIA",
		Source:     "workday",
		ExternalID: "board:1",
		// The employer's own Workday location bucket, with remoteType unset (freehire#2555).
		Location: "US, TX, Remote",
		Description: "This position is 100% on-site based at either our Dallas or Houston " +
			"Contract Manufacturing (CM) facility.",
	})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (description contradicts the location's remote)", got.WorkMode)
	}
}

func TestDerive_DescriptionContradictsStructuredRemote(t *testing.T) {
	got := Derive(Input{
		Title:       "Executive Assistant",
		Company:     "Acme",
		Source:      "paycor",
		ExternalID:  "board:1",
		Location:    "Mentor, OH",
		WorkMode:    "remote", // the ATS's own field, and wrong
		Description: "Onsite - not a remote position. Join our team supporting leadership.",
	})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (description contradicts the ATS remote flag)", got.WorkMode)
	}
}

// Only a remote result is ever overruled. "Not a remote position" is a true statement about a
// hybrid job, so reading it as onsite would replace a right answer with a wrong one.
func TestDerive_DenialLeavesHybridAlone(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin",
		WorkMode:    "hybrid",
		Description: "This is not a remote position; you will be in the office three days a week.",
	})
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (a denial of remote says nothing against hybrid)", got.WorkMode)
	}
}

func TestDerive_GenuineRemoteSurvives(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany",
		Description: "Collaborate with a remote team; we are fully remote across the EU.",
	})
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (no denial in the description)", got.WorkMode)
	}
}

func TestDerive_NoisyDescriptionYieldsNoWorkMode(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin", // silent
		Description: "Experience with distributed systems and hybrid cloud.",
	})
	if got.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (noisy description, no real phrase)", got.WorkMode)
	}
}

// Seniority source precedence: title dictionary → description. The description
// only fills a grade the title left empty; category is unaffected.
func TestDerive_DescriptionFillsSeniorityWhenTitleSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Developer", // no grade word
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "We are looking for a senior engineer to own the platform.",
	})
	if got.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior (description fills when title silent)", got.Seniority)
	}
	if got.Category != "backend" {
		t.Errorf("Category = %q, want backend (unaffected)", got.Category)
	}
}

func TestDerive_TitleSeniorityBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Lead Backend Engineer", // title → lead
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "You will work with a senior team.", // description → senior, but loses
	})
	if got.Seniority != "lead" {
		t.Errorf("Seniority = %q, want lead (title beats description)", got.Seniority)
	}
}

func TestDerive_NoisyDescriptionYieldsNoSeniority(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Developer", // no grade
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "Collaborate with senior management and lead the team to success.",
	})
	if got.Seniority != "" {
		t.Errorf("Seniority = %q, want empty (noisy description, no anchored grade)", got.Seniority)
	}
}

// The description fills a non-technical category when the source and title are both
// silent — the lowest tier of the category precedence, feeding the enrichment gate.
func TestDerive_DescriptionFillsNonTechCategoryWhenTitleSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Join Our Team", // no category word
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "We are hiring a sales representative for our growing team.",
	})
	if got.Category != "sales" {
		t.Errorf("Category = %q, want sales (description fills when title silent)", got.Category)
	}
}

// The title dictionary beats the description tier: a resolvable title is never
// overwritten by a non-tech phrase in the prose.
func TestDerive_TitleCategoryBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Developer", // title → backend
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "You will support our sales representative onboarding.",
	})
	if got.Category != "backend" {
		t.Errorf("Category = %q, want backend (title beats description)", got.Category)
	}
}

// The description tier resolves only non-technical categories: a title-silent job
// whose prose describes a technical role stays empty (never enriched-then-gated).
func TestDerive_TechDescriptionYieldsNoCategory(t *testing.T) {
	got := Derive(Input{
		Title:       "Join Our Team", // no category word
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "Build resilient backend systems in Go and mentor the team.",
	})
	if got.Category != "" {
		t.Errorf("Category = %q, want empty (description resolves no technical category)", got.Category)
	}
}

// A structured category from the source wins over the description tier too.
func TestDerive_StructuredCategoryBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Join Our Team", // no category word
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "1",
		Category:    "frontend", // structured source signal
		Description: "We are hiring a sales representative.",
	})
	if got.Category != "frontend" {
		t.Errorf("Category = %q, want frontend (structured source wins over description)", got.Category)
	}
}
