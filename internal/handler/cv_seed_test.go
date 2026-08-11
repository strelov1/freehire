package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

type stubHistory struct {
	hist experience.SeedHistory
	err  error
}

func (s stubHistory) SeedHistory(context.Context, int64) (experience.SeedHistory, error) {
	return s.hist, s.err
}

// bankedSeeder must satisfy cv.Seeder — that is what keeps internal/cv ignorant of the bank.
var _ cv.Seeder = bankedSeeder{}

// The loop closes here: evidence confirmed while tailoring for one vacancy appears in the
// base CV the candidate creates next, even though it is in no uploaded file.
func TestBankedSeederTakesExperienceFromTheBank(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			FullName:  "Ada Lovelace",
			Email:     "ada@example.com",
			Phone:     "+351 900 000 000",
			Location:  "Lisbon, PT",
			Links:     []string{"github.com/ada"},
			Headline:  "Staff Backend Engineer",
			Skills:    []string{"Go", "Kafka"},
			Education: []resumeextract.Education{{Degree: "BSc"}},
			// The structure's own copy is stale and must be replaced, not merged.
			Experience: []resumeextract.Experience{{Company: "STALE"}},
		}},
		bank: stubHistory{hist: experience.SeedHistory{
			HasJobEmployments: true,
			Experience: []resumeextract.Experience{
				{Company: "RingCentral", Title: "SWE", Highlights: []string{"Confirmed in chat"}},
			},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want a seedable structure", ok, err)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked history replacing the structure's", st.Experience)
	}
	if st.FullName != "Ada Lovelace" || len(st.Education) != 1 {
		t.Errorf("the file-owned sections were lost: %+v", st)
	}

	// Contacts and skills come from the structure and must survive the bank swap — the same
	// way skills always seed into the Document. Losing them here is the blank-header bug.
	doc := cv.Seed(st)
	if len(doc.Experience) != 1 || doc.Experience[0].Company != "RingCentral" {
		t.Errorf("seeded document experience = %+v", doc.Experience)
	}
	if len(doc.Experience[0].Bullets) != 1 {
		t.Errorf("the banked claim did not become a bullet: %+v", doc.Experience[0])
	}
	if doc.Header.FullName != "Ada Lovelace" || doc.Header.Email != "ada@example.com" ||
		doc.Header.Phone != "+351 900 000 000" || doc.Header.Location != "Lisbon, PT" ||
		len(doc.Header.Links) != 1 || doc.Header.Links[0] != "github.com/ada" {
		t.Errorf("seeded header = %+v, want full contacts from the structure", doc.Header)
	}
	if len(doc.Skills) != 1 || len(doc.Skills[0].Items) != 2 ||
		doc.Skills[0].Items[0] != "Go" || doc.Skills[0].Items[1] != "Kafka" {
		t.Errorf("seeded skills = %+v, want Go/Kafka from the structure", doc.Skills)
	}
}

// Bank-only is not a usable whole-document seed: contacts live on the structure, and a
// structure-absent composition would blank the header on apply.
func TestBankedSeederBankAloneIsNotUsable(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: false},
		bank: stubHistory{hist: experience.SeedHistory{
			HasJobEmployments: true,
			Experience:        []resumeextract.Experience{{Company: "RingCentral"}},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil {
		t.Fatalf("Structured: %v", err)
	}
	if ok {
		t.Fatal("bank-only composition must not be usable for whole-document seed")
	}
	if len(st.Experience) != 1 {
		t.Errorf("experience = %+v, want the banked role still composed (usable=false)", st.Experience)
	}
}

func TestBankedSeederProvisionalContactsPlusBankIsUsable(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{
			ok:            false,
			provisionalOK: true,
			provisional: resumeextract.Structured{
				FullName: "Ada Lovelace",
				Email:    "ada@example.com",
				Phone:    "+44",
				Summary:  "MUST NOT LEAK", // provisionalContacts strips this at the store; fake still shouldn't seed it
			},
		},
		bank: stubHistory{hist: experience.SeedHistory{
			HasJobEmployments: true,
			Experience:        []resumeextract.Experience{{Company: "RingCentral", Title: "SWE"}},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want usable provisional+bank seed", ok, err)
	}
	if st.FullName != "Ada Lovelace" || st.Email != "ada@example.com" {
		t.Errorf("contacts = %+v, want provisional identity", st)
	}
	if st.Summary != "" {
		t.Errorf("summary = %q, want empty (no superseded semantics)", st.Summary)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want bank", st.Experience)
	}
	doc := cv.Seed(st)
	if doc.Header.FullName != "Ada Lovelace" || doc.Summary != "" {
		t.Errorf("seeded = header:%+v summary:%q", doc.Header, doc.Summary)
	}
}

// bankedSeeder.Structured must follow StructureForSeed for identity: owned overlay,
// current body, stale contacts-only. Empty bank — no experience overlay.
func TestBankedSeederFollowsIdentityTable(t *testing.T) {
	cases := []struct {
		name          string
		ok            bool
		ret           resumeextract.Structured
		provisionalOK bool
		provisional   resumeextract.Structured
		wantName      string
		wantSummary   string
		wantOK        bool
	}{
		{
			name:        "current extract, no owned overlay in fake ret",
			ok:          true,
			ret:         resumeextract.Structured{FullName: "From Blob", Summary: "Staff"},
			wantName:    "From Blob",
			wantSummary: "Staff",
			wantOK:      true,
		},
		{
			name:          "pending contacts-only",
			provisionalOK: true,
			provisional:   resumeextract.Structured{FullName: "Ada", Summary: "MUST NOT LEAK"},
			wantName:      "Ada",
			wantOK:        true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seeder := bankedSeeder{resume: fakeStructuredResume{
				ok: tc.ok, ret: tc.ret,
				provisionalOK: tc.provisionalOK, provisional: tc.provisional,
			}}
			st, ok, err := seeder.Structured(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tc.wantOK)
			}
			if st.FullName != tc.wantName || st.Summary != tc.wantSummary {
				t.Fatalf("name=%q summary=%q, want %q / %q", st.FullName, st.Summary, tc.wantName, tc.wantSummary)
			}
		})
	}
}

func TestBankedSeederPendingBlobDropsSupersededSemantics(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{
			ok: false,
			ret: resumeextract.Structured{
				Summary: "Staff engineer summary",
				Skills:  []string{"Go", "Kafka"},
				Projects: []resumeextract.Project{
					{Name: "Sandrock", Link: "https://example.com"},
				},
			},
			provisionalOK: true,
			provisional: resumeextract.Structured{
				FullName: "Ada Lovelace",
				Email:    "ada@example.com",
			},
		},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v", ok, err)
	}
	if st.FullName != "Ada Lovelace" || st.Email != "ada@example.com" {
		t.Fatalf("contacts = %+v", st)
	}
	if st.Summary != "" || len(st.Skills) != 0 || len(st.Projects) != 0 {
		t.Fatalf("pending seed leaked superseded semantics: summary:%q skills:%v projects:%v", st.Summary, st.Skills, st.Projects)
	}
}

func TestBankedSeederProvisionalWithoutBankStillUsableFromName(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{
			ok: false, provisionalOK: true,
			provisional: resumeextract.Structured{FullName: "Ada Lovelace"},
		},
		bank: stubHistory{},
	}
	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want name alone to seed", ok, err)
	}
	if st.FullName != "Ada Lovelace" {
		t.Errorf("FullName = %q", st.FullName)
	}
}

// A current structure that is empty in every seeded field is not usable even with a bank
// that also contributes nothing — seedable still gates after the structure check.
func TestBankedSeederEmptyStructureIsNotUsable(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{}},
		bank:   stubHistory{},
	}

	_, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil {
		t.Fatalf("Structured: %v", err)
	}
	if ok {
		t.Fatal("empty structure with empty bank must not be usable")
	}
}

// Nothing known anywhere is what the tailoring bootstrap turns into "add a résumé first".
func TestBankedSeederReportsNothingToSeedFrom(t *testing.T) {
	seeder := bankedSeeder{resume: fakeStructuredResume{ok: false}, bank: stubHistory{}}

	_, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil {
		t.Fatalf("Structured: %v", err)
	}
	if ok {
		t.Error("reported something to seed from with an empty bank and no structure")
	}
}

// A failing bank costs the work history from the bank, not the whole bootstrap: fall back
// to the structure's experience so a pending/unavailable bank does not blank roles.
func TestBankedSeederSurvivesAFailingBank(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			Headline:   "Staff Backend Engineer",
			Experience: []resumeextract.Experience{{Company: "Analytical Engines"}},
		}},
		bank: stubHistory{err: errors.New("database down")},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want the structure still seedable", ok, err)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "Analytical Engines" {
		t.Errorf("experience = %+v, want structure fallback when the bank fails", st.Experience)
	}
}

func TestBankedSeederEmptyBankFallsBackToStructureExperience(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			FullName:   "Ada Lovelace",
			Experience: []resumeextract.Experience{{Company: "Analytical Engines", Title: "Engineer"}},
			Projects:   []resumeextract.Project{{Name: "opensched", Link: "https://opensched.dev"}},
		}},
		bank: stubHistory{}, // HasJobEmployments/HasProjectEmployments both false
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v", ok, err)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "Analytical Engines" {
		t.Errorf("experience = %+v, want structure when the bank is empty", st.Experience)
	}
	if len(st.Projects) != 1 || st.Projects[0].Link != "https://opensched.dev" {
		t.Errorf("projects = %+v, want structure projects when the bank has none", st.Projects)
	}
}

func TestBankedSeederUsesBankProjectsWhenPresent(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			FullName: "Ada Lovelace",
			Projects: []resumeextract.Project{{Name: "STALE", Link: "https://stale.example"}},
		}},
		bank: stubHistory{hist: experience.SeedHistory{
			HasProjectEmployments: true,
			Projects: []resumeextract.Project{
				{Name: "telagon.io", Link: "https://telagon.io", Highlights: []string{"1.4M+ channels"}},
			},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v", ok, err)
	}
	if len(st.Experience) != 0 {
		t.Errorf("experience = %+v, want none when the structure has none and the bank has only projects", st.Experience)
	}
	if len(st.Projects) != 1 || st.Projects[0].Name != "telagon.io" || st.Projects[0].Link != "https://telagon.io" {
		t.Errorf("projects = %+v, want banked project with link", st.Projects)
	}
}

// The regression this PR shipped with: a candidate with a résumé that HAS real job history,
// plus a bank holding only a project-kind row, must keep the structure's Experience — not
// have it blanked because the bank was "touched" at all. See professional.go HasJobEmployments.
func TestBankedSeederPreservesStructureExperienceWhenBankHasOnlyProjects(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			FullName:   "Ada Lovelace",
			Experience: []resumeextract.Experience{{Company: "Acme", Title: "SWE"}},
		}},
		bank: stubHistory{hist: experience.SeedHistory{
			HasProjectEmployments: true,
			Projects: []resumeextract.Project{
				{Name: "telagon.io", Link: "https://telagon.io"},
			},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v", ok, err)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "Acme" {
		t.Errorf("experience = %+v, want the structure's real job history preserved", st.Experience)
	}
	if len(st.Projects) != 1 || st.Projects[0].Name != "telagon.io" {
		t.Errorf("projects = %+v, want the banked project", st.Projects)
	}
}

func TestBankedSeederKeepsStructureProjectsWhenBankHasJobsOnly(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			FullName: "Ada Lovelace",
			Projects: []resumeextract.Project{{Name: "opensched", Link: "https://opensched.dev"}},
		}},
		bank: stubHistory{hist: experience.SeedHistory{
			HasJobEmployments: true,
			Experience:        []resumeextract.Experience{{Company: "RingCentral", Title: "SWE"}},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v", ok, err)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want banked job", st.Experience)
	}
	if len(st.Projects) != 1 || st.Projects[0].Name != "opensched" {
		t.Errorf("projects = %+v, want structure projects when bank has no project-kind rows", st.Projects)
	}
}

func TestBankedSeederSeedableFromCertifications(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			Certifications: []string{"AWS Certified Solutions Architect"},
		}},
		bank: stubHistory{},
	}
	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want certs alone to be seedable", ok, err)
	}
	doc := cv.Seed(st)
	if len(doc.Certifications) != 1 || doc.Certifications[0].Name != "AWS Certified Solutions Architect" {
		t.Errorf("certifications = %+v", doc.Certifications)
	}
}
