package employer

import (
	"context"
	"testing"
)

func TestListPendingClaims_ExplainsCurrentDomainCheck(t *testing.T) {
	for _, tt := range []struct {
		name, website, email, reason string
	}{
		{"unknown company", "", "hr@acme.test", "unknown_website"},
		{"unusable website", "not a website", "hr@acme.test", "unknown_website"},
		{"different domain", "https://acme.test", "hr@other.test", "domain_mismatch"},
		{"academic domain", "https://example.edu", "student@example.edu", "academic_domain"},
		{"website now matches", "https://www.acme.test/jobs", "hr@ACME.test", "manual_review"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			codes := newFakeCodeIssuer()
			s := New(repo, codes, &fakeClaimMailer{}, nil, nil)
			if _, err := s.Claim(context.Background(), 1, "Acme", tt.email); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
				t.Fatal(err)
			}
			// A catalogue update after confirmation must be visible without re-confirming.
			repo.companies["acme"] = struct{ name, website string }{"Acme", tt.website}
			claims, err := s.ListPendingClaims(context.Background())
			if err != nil || len(claims) != 1 {
				t.Fatalf("claims = %+v, err = %v", claims, err)
			}
			if claims[0].Website != tt.website || string(claims[0].ReviewReason) != tt.reason {
				t.Fatalf("claim = %+v, want website %q, reason %q", claims[0], tt.website, tt.reason)
			}
			if claims[0].Status != StatusPending {
				t.Fatal("reading the queue must not activate a claim")
			}
		})
	}
}
