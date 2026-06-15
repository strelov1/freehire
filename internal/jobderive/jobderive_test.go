package jobderive

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/normalize"
)

func TestDerive_SlugsAndFacets(t *testing.T) {
	got := Derive(Input{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Location:    "Remote - Germany",
		Description: "We use Golang and PostgreSQL.",
	})

	wantSlug := normalize.JobSlug("Senior Go Developer", "Acme", "manual", "https://acme.example/jobs/1")
	if got.PublicSlug != wantSlug {
		t.Errorf("PublicSlug = %q, want %q", got.PublicSlug, wantSlug)
	}
	if got.CompanySlug != normalize.Slug("Acme") {
		t.Errorf("CompanySlug = %q", got.CompanySlug)
	}
	if len(got.Countries) == 0 || got.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de ...]", got.Countries)
	}
	if !reflect.DeepEqual(got.Skills, []string{"go", "postgresql"}) {
		t.Errorf("Skills = %v, want [go postgresql]", got.Skills)
	}
	// No structured work mode supplied → the parser's hint (remote) is used.
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (parsed)", got.WorkMode)
	}
}

// A structured work-mode signal from the caller (e.g. an ATS workplace-type enum)
// beats the free-text parser hint.
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
}

func TestDerive_LocationWorkModeBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany",       // location marker → remote
		Description: "This is a hybrid role.", // description → hybrid, but loses
	})
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (location beats description)", got.WorkMode)
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
