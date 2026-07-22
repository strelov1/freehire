package credentials

import (
	"sort"
	"testing"
)

func TestScanFindsRecognizedCredentials(t *testing.T) {
	text := "Must hold CISSP and an AWS Certified Solutions Architect. PMP is a plus."
	got := Scan(text)
	sort.Strings(got)
	want := []string{"aws-solutions-architect", "cissp", "pmp"}
	if len(got) != len(want) {
		t.Fatalf("Scan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Scan = %v, want %v", got, want)
		}
	}
}

func TestScanDedupesAndIgnoresUnknown(t *testing.T) {
	got := Scan("CISSP cissp certified information systems security professional; underwater basket weaving")
	if len(got) != 1 || got[0] != "cissp" {
		t.Errorf("Scan dedup = %v, want [cissp]", got)
	}
}

func TestScanNoCredential(t *testing.T) {
	if got := Scan("We build backend services in Go and Postgres."); len(got) != 0 {
		t.Errorf("Scan = %v, want empty", got)
	}
	if got := Scan(""); len(got) != 0 {
		t.Errorf("Scan(empty) = %v, want empty", got)
	}
}
