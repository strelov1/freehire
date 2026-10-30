package jobderive

import "testing"

// ViaRecruiter is true when the description says a recruiter is speaking, and nil —
// never false — otherwise.
func TestDerive_ViaRecruiter(t *testing.T) {
	derive := func(desc string) *bool {
		return Derive(Input{Title: "Backend Engineer", Company: "Acme", Source: "greenhouse", ExternalID: "1", Description: desc}).ViaRecruiter
	}
	if got := derive("Our client is a fast-growing fintech looking for a Go engineer."); got == nil || !*got {
		t.Fatalf("recruiter statement: ViaRecruiter = %v, want true", got)
	}
	if got := derive("We are hiring a backend engineer to work on our Go services."); got != nil {
		t.Fatalf("silent description: ViaRecruiter = %v, want nil", *got)
	}
}
