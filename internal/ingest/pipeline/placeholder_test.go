package pipeline

import (
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestPlaceholderTitle(t *testing.T) {
	for title, want := range map[string]bool{
		"Test":                                 true,
		"test job":                             true,
		"Test Job 2":                           true,
		"Testing":                              true,
		"Sample Job":                           true,
		"Test Opening":                         true,
		"TEST - DO NOT APPLY":                  true,
		"DO NOT APPLY - TEST SC - 10.26.2026":  true,
		"AC 1.7 test - please ignore this job": true,
		"Test job- do not apply, eero":         true,
		"Test Engineer":                        false,
		"Software Test Lead":                   false,
		"Testing Specialist":                   false,
		"Sample Preparation Technician":        false,
		"Allstate Agency Sales Team Member - DO NOT APPLY UNLESS YOU HAVE INSURANCE LICENSE": false,
		"Admin Assistant - DO NOT APPLY": false,
		"":                               false,
	} {
		if got := placeholderTitle(title); got != want {
			t.Errorf("placeholderTitle(%q) = %v, want %v", title, got, want)
		}
	}
}

// The placeholder is refused before construction, with its own error, so the caller skips it.
func TestNormalizeJobRefusesAPlaceholder(t *testing.T) {
	e := sources.CompanyEntry{Company: "Nozomi Networks", Provider: "bamboohr", Board: "nozominetworks"}
	_, err := normalizeJob(e, sources.Job{ExternalID: "22", Title: "Test Opening", Company: "Nozomi Networks"}, nil, false)
	if !errors.Is(err, errPlaceholderPosting) {
		t.Fatalf("err = %v, want errPlaceholderPosting", err)
	}
	if _, err := normalizeJob(e, sources.Job{ExternalID: "23", Title: "Test Engineer", Company: "Nozomi Networks"}, nil, false); err != nil {
		t.Fatalf("a real title was refused: %v", err)
	}
}
