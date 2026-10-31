package jobderive

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Equity is the matcher's reading of the title and description, and "" — never a "no
// equity" value — when they state nothing it reads.
func TestDerive_Equity(t *testing.T) {
	derive := func(title, desc string) string {
		return Derive(Input{Title: title, Company: "Acme", Source: "greenhouse", ExternalID: "1", Description: desc}).Equity
	}
	if got := derive("Backend Engineer", "<p>Competitive salary + equity.</p>"); got != vocab.EquityWithSalary {
		t.Fatalf("salary and equity: Equity = %q, want %q", got, vocab.EquityWithSalary)
	}
	if got := derive("Founding Engineer (Equity Only)", "<p>Join us early.</p>"); got != vocab.EquityOnly {
		t.Fatalf("equity-only title: Equity = %q, want %q", got, vocab.EquityOnly)
	}
	if got := derive("Backend Engineer", "<p>We are committed to diversity, equity and inclusion.</p>"); got != "" {
		t.Fatalf("DEI statement: Equity = %q, want empty", got)
	}
}
