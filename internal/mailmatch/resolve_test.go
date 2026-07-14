package mailmatch

import "testing"

func TestResolve(t *testing.T) {
	candidates := []Candidate{
		{JobID: 1, Company: "Block Labs", ThreadIDs: []string{"t-block-1"}},
		{JobID: 2, Company: "Hyperproof"},
		{JobID: 3, Company: "Nametag"},
	}

	t.Run("thread continuity wins over name", func(t *testing.T) {
		// Subject names Hyperproof, but the thread already belongs to Block Labs.
		email := Email{ThreadID: "t-block-1", FromName: "", Subject: "Thank you for applying to Hyperproof"}
		m := Resolve(email, candidates)
		if m.Tier != TierThread || m.JobID != 1 {
			t.Fatalf("got tier=%v jobID=%d, want TierThread jobID=1", m.Tier, m.JobID)
		}
		if m.Confidence < 0.99 {
			t.Fatalf("thread match confidence = %v, want ~1.0", m.Confidence)
		}
	})

	t.Run("unique company-name match", func(t *testing.T) {
		email := Email{FromName: "Hyperproof Hiring Team", Subject: "Hyperproof Application Update"}
		m := Resolve(email, candidates)
		if m.Tier != TierName || m.JobID != 2 {
			t.Fatalf("got tier=%v jobID=%d, want TierName jobID=2", m.Tier, m.JobID)
		}
		if m.Confidence <= 0.5 {
			t.Fatalf("name match confidence = %v, want high", m.Confidence)
		}
	})

	t.Run("ambiguous name yields no auto match", func(t *testing.T) {
		dupes := []Candidate{
			{JobID: 10, Company: "Acme"},
			{JobID: 11, Company: "Acme"},
		}
		email := Email{Subject: "Thank you for applying to Acme"}
		m := Resolve(email, dupes)
		if m.Tier != TierAmbiguous || m.JobID != 0 {
			t.Fatalf("got tier=%v jobID=%d, want TierAmbiguous jobID=0", m.Tier, m.JobID)
		}
	})

	t.Run("no candidate matches", func(t *testing.T) {
		email := Email{Subject: "Thank you for applying to Speechify"}
		m := Resolve(email, candidates)
		if m.Tier != TierNone || m.JobID != 0 {
			t.Fatalf("got tier=%v jobID=%d, want TierNone jobID=0", m.Tier, m.JobID)
		}
	})

	t.Run("unresolvable email is TierNone", func(t *testing.T) {
		email := Email{Subject: "Ilya, we've received your resume"}
		m := Resolve(email, candidates)
		if m.Tier != TierNone {
			t.Fatalf("got tier=%v, want TierNone", m.Tier)
		}
	})
}
