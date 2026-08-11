package resume

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// TestIdentityTableReaders locks the three-layer identity table in AGENTS.md.
// Structured is stamp-gated (no owned overlay). ProfileRead is current-or-provisional
// (handler overlays owned). StructureForSeed overlays owned as a block and strips
// superseded semantics. bankedSeeder is asserted in handler/cv_seed_test.go.
func TestIdentityTableReaders(t *testing.T) {
	tOld := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	tNew := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	extract := resumeextract.Structured{
		FullName: "From Blob",
		Email:    "blob@example.com",
		Summary:  "Ten years of systems work.",
		Skills:   []string{"Go"},
	}
	blob, err := json.Marshal(extract)
	if err != nil {
		t.Fatal(err)
	}
	owned := Contacts{FullName: "Ada Lovelace", Email: "ada@example.com"}

	type want struct {
		structuredOK      bool
		structuredName    string
		structuredSummary string
		profileCurrent    bool
		profilePending    bool
		profileName       string
		profileSummary    string
		provisionalOK     bool
		provisionalName   string
		seedOK            bool
		seedName          string
		seedSummary       string
	}

	cases := []struct {
		name   string
		owned  bool // whether candidate-owned contacts are set for this case
		setup  func(*fakeRepo)
		expect want
	}{
		{
			name: "current stamp, no owned",
			setup: func(r *fakeRepo) {
				r.uploadedAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
				r.structured[7] = blob
				r.structAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
			},
			expect: want{
				structuredOK: true, structuredName: "From Blob", structuredSummary: "Ten years of systems work.",
				profileCurrent: true, profileName: "From Blob", profileSummary: "Ten years of systems work.",
				seedOK: true, seedName: "From Blob", seedSummary: "Ten years of systems work.",
			},
		},
		{
			name: "pending superseded blob, no owned",
			setup: func(r *fakeRepo) {
				r.uploadedAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
				r.structured[7] = blob
				r.structAt[7] = pgtype.Timestamptz{Time: tOld, Valid: true}
			},
			expect: want{
				profilePending: true, profileName: "From Blob",
				provisionalOK: true, provisionalName: "From Blob",
				seedOK: true, seedName: "From Blob",
			},
		},
		{
			name:  "current stamp, owned overlay",
			owned: true,
			setup: func(r *fakeRepo) {
				r.uploadedAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
				r.structured[7] = blob
				r.structAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
			},
			expect: want{
				structuredOK: true, structuredName: "From Blob", structuredSummary: "Ten years of systems work.",
				profileCurrent: true, profileName: "From Blob", profileSummary: "Ten years of systems work.",
				seedOK: true, seedName: "Ada Lovelace", seedSummary: "Ten years of systems work.",
			},
		},
		{
			name:  "pending superseded blob, owned overlay",
			owned: true,
			setup: func(r *fakeRepo) {
				r.uploadedAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
				r.structured[7] = blob
				r.structAt[7] = pgtype.Timestamptz{Time: tOld, Valid: true}
			},
			expect: want{
				profilePending: true, profileName: "From Blob",
				provisionalOK: true, provisionalName: "From Blob",
				seedOK: true, seedName: "Ada Lovelace",
			},
		},
		{
			name:  "résumé deleted, owned contacts remain",
			owned: true,
			setup: func(r *fakeRepo) {},
			expect: want{
				seedOK: true, seedName: "Ada Lovelace",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			s := New(nil, repo)
			ctx := context.Background()
			if tc.owned {
				if _, err := s.SetCandidateContacts(ctx, 7, owned); err != nil {
					t.Fatal(err)
				}
			}
			tc.setup(repo)

			st, ok, err := s.Structured(ctx, 7)
			if err != nil {
				t.Fatalf("Structured: %v", err)
			}
			if ok != tc.expect.structuredOK {
				t.Errorf("Structured ok=%v, want %v", ok, tc.expect.structuredOK)
			}
			if ok && (st.FullName != tc.expect.structuredName || st.Summary != tc.expect.structuredSummary) {
				t.Errorf("Structured = name:%q summary:%q, want %q / %q", st.FullName, st.Summary, tc.expect.structuredName, tc.expect.structuredSummary)
			}

			pr, err := s.ProfileReadForUser(ctx, 7)
			if err != nil {
				t.Fatalf("ProfileRead: %v", err)
			}
			if pr.Current != tc.expect.profileCurrent || pr.Pending != tc.expect.profilePending {
				t.Errorf("ProfileRead current=%v pending=%v, want %v / %v", pr.Current, pr.Pending, tc.expect.profileCurrent, tc.expect.profilePending)
			}
			if pr.Structure.FullName != tc.expect.profileName || pr.Structure.Summary != tc.expect.profileSummary {
				t.Errorf("ProfileRead = name:%q summary:%q, want %q / %q", pr.Structure.FullName, pr.Structure.Summary, tc.expect.profileName, tc.expect.profileSummary)
			}

			pc, pok, err := s.ProvisionalContacts(ctx, 7)
			if err != nil {
				t.Fatalf("ProvisionalContacts: %v", err)
			}
			if pok != tc.expect.provisionalOK {
				t.Errorf("ProvisionalContacts ok=%v, want %v", pok, tc.expect.provisionalOK)
			}
			if pok && pc.FullName != tc.expect.provisionalName {
				t.Errorf("ProvisionalContacts name=%q, want %q", pc.FullName, tc.expect.provisionalName)
			}
			if pok && pc.Summary != "" {
				t.Errorf("ProvisionalContacts leaked summary %q", pc.Summary)
			}

			seed, sok, err := s.StructureForSeed(ctx, 7)
			if err != nil {
				t.Fatalf("StructureForSeed: %v", err)
			}
			if sok != tc.expect.seedOK {
				t.Errorf("StructureForSeed ok=%v, want %v", sok, tc.expect.seedOK)
			}
			if seed.FullName != tc.expect.seedName || seed.Summary != tc.expect.seedSummary {
				t.Errorf("StructureForSeed = name:%q summary:%q, want %q / %q", seed.FullName, seed.Summary, tc.expect.seedName, tc.expect.seedSummary)
			}
			if !tc.expect.profileCurrent && seed.Summary != "" {
				t.Errorf("stale StructureForSeed leaked summary %q", seed.Summary)
			}
		})
	}
}
