package mentorship

import (
	"context"
	"testing"
)

// Directory's three new filters (seniority, free-text query, no-reviews-yet) each
// narrow independently, mirroring the fake's existing company/topic/language coverage —
// the SQL predicates themselves are proven against a real Postgres in
// internal/platform/db, this only proves the fake models the same contract.
func TestDirectoryAppliesTheNewFilters(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)

	submitApproved := func(mutate func(*ProfileInput)) Profile {
		in := validInput()
		in.Slug = ""
		mutate(&in)
		p, err := svc.SubmitProfile(context.Background(), in)
		if err != nil {
			t.Fatalf("SubmitProfile: %v", err)
		}
		p, err = svc.Decide(context.Background(), p.ID, 99, StatusApproved)
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		return p
	}

	senior := submitApproved(func(in *ProfileInput) {
		in.DisplayName = "Jane Vance"
		in.Headline = "Senior Backend Engineer"
		in.UserID = 101
		in.Seniority = "senior"
	})
	junior := submitApproved(func(in *ProfileInput) {
		in.DisplayName = "Jamie Torres"
		in.Headline = "Junior Frontend Engineer"
		in.UserID = 102
		in.Seniority = "junior"
	})
	// A rating: fakeRepo carries RatingCount as a plain field, set directly the way
	// ListPublishedProfiles' real SQL join would populate it — the fake models the
	// result of that join, not the join itself.
	reviewed := repo.profiles[senior.ID]
	reviewed.RatingCount = 3
	repo.profiles[senior.ID] = reviewed

	t.Run("by seniority", func(t *testing.T) {
		out, err := svc.Directory(context.Background(), DirectoryFilter{Seniority: "junior"})
		if err != nil {
			t.Fatalf("Directory: %v", err)
		}
		if len(out) != 1 || out[0].ID != junior.ID {
			t.Errorf("Directory(seniority=junior) = %+v, want just %q", out, junior.Slug)
		}
	})

	t.Run("by free-text query matching the headline", func(t *testing.T) {
		out, err := svc.Directory(context.Background(), DirectoryFilter{Query: "senior"})
		if err != nil {
			t.Fatalf("Directory: %v", err)
		}
		if len(out) != 1 || out[0].ID != senior.ID {
			t.Errorf("Directory(query=senior) = %+v, want just %q", out, senior.Slug)
		}
	})

	t.Run("no reviews yet excludes a reviewed mentor", func(t *testing.T) {
		out, err := svc.Directory(context.Background(), DirectoryFilter{NoReviewsOnly: true})
		if err != nil {
			t.Fatalf("Directory: %v", err)
		}
		if len(out) != 1 || out[0].ID != junior.ID {
			t.Errorf("Directory(no_reviews=true) = %+v, want just the unreviewed mentor %q", out, junior.Slug)
		}
	})

	t.Run("unfiltered returns both", func(t *testing.T) {
		out, err := svc.Directory(context.Background(), DirectoryFilter{})
		if err != nil {
			t.Fatalf("Directory: %v", err)
		}
		if len(out) != 2 {
			t.Errorf("Directory() = %d profiles, want 2", len(out))
		}
	})
}

// A company-less mentor is a full directory member — unfiltered, they appear like any
// other — but "unset never matches a filter" holds for company exactly as it already
// does for seniority: no company named means no company filter can ever select them.
func TestDirectoryIncludesACompanyLessMentorButNeverMatchesACompanyFilter(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)

	submitApproved := func(mutate func(*ProfileInput)) Profile {
		in := validInput()
		in.Slug = ""
		mutate(&in)
		p, err := svc.SubmitProfile(context.Background(), in)
		if err != nil {
			t.Fatalf("SubmitProfile: %v", err)
		}
		p, err = svc.Decide(context.Background(), p.ID, 99, StatusApproved)
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		return p
	}

	independent := submitApproved(func(in *ProfileInput) {
		in.DisplayName = "Robin Freelance"
		in.UserID = 103
		in.CompanySlug = ""
	})
	atCompany := submitApproved(func(in *ProfileInput) {
		in.DisplayName = "Casey Employed"
		in.UserID = 104
		in.CompanySlug = "acme"
	})

	t.Run("unfiltered includes the company-less mentor", func(t *testing.T) {
		out, err := svc.Directory(context.Background(), DirectoryFilter{})
		if err != nil {
			t.Fatalf("Directory: %v", err)
		}
		var sawIndependent bool
		for _, p := range out {
			if p.ID == independent.ID {
				sawIndependent = true
				if p.CompanySlug != "" {
					t.Errorf("independent mentor's CompanySlug = %q, want empty", p.CompanySlug)
				}
			}
		}
		if !sawIndependent || len(out) != 2 {
			t.Errorf("Directory() = %+v, want both mentors including the company-less one", out)
		}
	})

	t.Run("filtering by company excludes the company-less mentor", func(t *testing.T) {
		out, err := svc.Directory(context.Background(), DirectoryFilter{CompanySlug: "acme"})
		if err != nil {
			t.Fatalf("Directory: %v", err)
		}
		if len(out) != 1 || out[0].ID != atCompany.ID {
			t.Errorf("Directory(company=acme) = %+v, want just %q", out, atCompany.Slug)
		}
	})
}
