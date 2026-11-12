package certification

import (
	"reflect"
	"testing"
)

func TestMaintenanceReliabilityCredential(t *testing.T) {
	got := Canonicalize([]string{
		"CMRP", "cmrp", "Certified Maintenance & Reliability Professional",
		"Certified Maintenance and Reliability Professional", "NEBOSH", "IOSH",
		"API", "ASME", // Organizations alone do not identify a certification.
	})
	want := []string{"cmrp", "iosh", "nebosh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Canonicalize() = %v, want %v", got, want)
	}
}
