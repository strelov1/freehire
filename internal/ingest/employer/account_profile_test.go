package employer

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateCompanyProfile_AppliesThePatchForAnActiveAccount(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	tagline := "We build things"
	if _, err := s.UpdateCompanyProfile(context.Background(), 1, CompanyProfilePatch{Tagline: &tagline}); err != nil {
		t.Fatalf("UpdateCompanyProfile: %v", err)
	}
	if len(repo.profilePatches) != 1 || repo.profilePatches[0].Tagline == nil || *repo.profilePatches[0].Tagline != tagline {
		t.Errorf("profilePatches = %+v, want one patch carrying the tagline", repo.profilePatches)
	}
}

func TestUpdateCompanyProfile_RefusesAPendingAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	tagline := "We build things"
	if _, err := s.UpdateCompanyProfile(context.Background(), 1, CompanyProfilePatch{Tagline: &tagline}); !errors.Is(err, ErrNotActive) {
		t.Fatalf("err = %v, want ErrNotActive", err)
	}
	if len(repo.profilePatches) != 0 {
		t.Error("a refused edit must not reach the repository")
	}
}

func TestMyAccount_ReturnsAPendingAccountWithoutRefusing(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	acc, err := s.MyAccount(context.Background(), 1)
	if err != nil {
		t.Fatalf("MyAccount: %v, want no error for a pending account", err)
	}
	if acc.Status != StatusPending {
		t.Errorf("status = %q, want pending", acc.Status)
	}
}

func TestMyAccount_ReportsErrNotFoundForNoAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.MyAccount(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestActiveAccount_RefusesAPendingAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ActiveAccount(context.Background(), 1); !errors.Is(err, ErrNotActive) {
		t.Errorf("err = %v, want ErrNotActive for a pending account", err)
	}
}

func TestActiveAccount_RefusesARevokedAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.RevokeAccount(context.Background(), 1); err != nil {
		t.Fatalf("RevokeAccount: %v", err)
	}
	if _, err := s.ActiveAccount(context.Background(), 1); !errors.Is(err, ErrNotActive) {
		t.Errorf("err = %v, want ErrNotActive for a revoked account", err)
	}
}

func TestActiveAccount_ReturnsAnActiveAccount(t *testing.T) {
	repo := newFakeRepo()
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)

	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	acc, err := s.ActiveAccount(context.Background(), 1)
	if err != nil {
		t.Fatalf("ActiveAccount: %v", err)
	}
	if acc.CompanySlug != "acme" {
		t.Errorf("company_slug = %q, want acme", acc.CompanySlug)
	}
}

// The facts card draws a flag from hq_country, so free text typed into the field is stored
// as the alpha-2 code it names (a capital resolves to its country) or refused outright.
func TestUpdateCompanyProfile_NormalizesOrRefusesHqCountry(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		invalid  bool
	}{
		{in: "SE", want: "se"},
		{in: " Sweden ", want: "se"},
		{in: "USA", want: "us"},
		{in: "Stockholm", want: "se"},
		{in: "Mars", invalid: true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			repo := newFakeRepo()
			repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
			s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, nil, nil)
			if _, err := s.Claim(context.Background(), 1, "Acme", "hr@acme.test"); err != nil {
				t.Fatalf("Claim: %v", err)
			}
			if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
				t.Fatalf("ConfirmClaim: %v", err)
			}

			in := tc.in
			_, err := s.UpdateCompanyProfile(context.Background(), 1, CompanyProfilePatch{HqCountry: &in})
			if tc.invalid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				if len(repo.profilePatches) != 0 {
					t.Errorf("a refused patch reached the repository: %+v", repo.profilePatches)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateCompanyProfile: %v", err)
			}
			if got := repo.profilePatches[0].HqCountry; got == nil || *got != tc.want {
				t.Errorf("stored hq_country = %v, want %q", got, tc.want)
			}
		})
	}
}
