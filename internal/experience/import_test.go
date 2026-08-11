package experience

import (
	"context"
	"strings"
	"testing"
)

// ringCentral is one role as a CV states it, used across the import tests.
func ringCentral() ImportEntry {
	return ImportEntry{
		Employment: Employment{
			Kind: KindJob, Company: "RingCentral", Role: "Senior Software Engineer",
			Location: "USA, Remote", Start: "2023-09", End: "Present", Current: true,
			Summary: "Global SaaS leader in business communications",
			Stack:   []string{"typescript", "golang", "mongodb"},
		},
		Claims: []string{
			"Re-architected a hot composite MongoDB index into three targeted indexes",
			"Sustained a 99.999% uptime SLA through incident response and on-call ownership",
		},
	}
}

func TestImportSeedsAnEmptyBank(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	got, err := s.Import(ctx, owner, []ImportEntry{ringCentral()}, "2026-07-28T10:00:00Z")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if got.EmploymentsCreated != 1 || got.AtomsCreated != 2 {
		t.Errorf("result = %+v, want 1 employment and 2 atoms created", got)
	}

	employments, err := s.ListEmployments(ctx, owner)
	if err != nil {
		t.Fatalf("ListEmployments: %v", err)
	}
	if len(employments) != 1 || employments[0].Company != "RingCentral" {
		t.Fatalf("employments = %+v, want one RingCentral", employments)
	}

	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	if len(atoms) != 2 {
		t.Fatalf("atoms = %d, want 2", len(atoms))
	}
	for _, a := range atoms {
		if a.Provenance != ProvenanceCVImport {
			t.Errorf("atom provenance = %q, want cv_import", a.Provenance)
		}
		if a.SourceRef != "2026-07-28T10:00:00Z" {
			t.Errorf("atom source_ref = %q, want the upload stamp", a.SourceRef)
		}
		if a.EmploymentID == nil || *a.EmploymentID != employments[0].ID {
			t.Errorf("atom is not attached to its role")
		}
	}
}

// Skills come from the bullet's own text, mined with Parse — that text IS prose, so the
// corroboration rule belongs here. The role's stack stays on the employment.
func TestImportMinesSkillsFromTheBulletNotTheRoleStack(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.Import(ctx, owner, []ImportEntry{ringCentral()}, "stamp"); err != nil {
		t.Fatalf("Import: %v", err)
	}
	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}

	var mongo, sla Atom
	for _, a := range atoms {
		if strings.HasPrefix(a.Claim, "Re-architected") {
			mongo = a
		} else {
			sla = a
		}
	}
	if !contains(mongo.Skills, "mongodb") {
		t.Errorf("the MongoDB bullet has skills %q, want mongodb mined from its own text", mongo.Skills)
	}
	if len(sla.Skills) != 0 {
		t.Errorf("the uptime bullet has skills %q, want none — the role's stack must not be copied onto every bullet", sla.Skills)
	}
}

// The bank's central promise. A CV re-uploaded — or trimmed to one page and re-uploaded —
// adds what is new and takes nothing away.
func TestImportIsAdditiveAndIdempotent(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.Import(ctx, owner, []ImportEntry{ringCentral()}, "first"); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	second, err := s.Import(ctx, owner, []ImportEntry{ringCentral()}, "second")
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if second.EmploymentsCreated != 0 || second.AtomsCreated != 0 || second.AtomsSkipped != 2 {
		t.Errorf("re-import = %+v, want nothing created and 2 atoms recognised as already banked", second)
	}

	// A trimmed CV: same role, one of the two bullets, plus a new one.
	trimmed := ringCentral()
	trimmed.Claims = []string{
		"Sustained a 99.999% uptime SLA through incident response and on-call ownership",
		"Scaled Kafka-based services to 84K RPS across 21 distributed services",
	}
	third, err := s.Import(ctx, owner, []ImportEntry{trimmed}, "third")
	if err != nil {
		t.Fatalf("third Import: %v", err)
	}
	if third.AtomsCreated != 1 || third.AtomsSkipped != 1 {
		t.Errorf("trimmed re-import = %+v, want the new bullet added and the known one skipped", third)
	}

	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	if len(atoms) != 3 {
		t.Errorf("atoms = %d, want 3 — a trimmed CV must not shrink the bank", len(atoms))
	}
}

// The same achievement, differently punctuated, is one atom. Import must recognise that
// through the claim key rather than banking it twice.
func TestImportMatchesAClaimAcrossSpellings(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.Import(ctx, owner, []ImportEntry{{
		Employment: Employment{Kind: KindJob, Company: "Sber", Role: "Team Lead"},
		Claims:     []string{"Cut report load by 95%"},
	}}, "first"); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	got, err := s.Import(ctx, owner, []ImportEntry{{
		Employment: Employment{Kind: KindJob, Company: "sber", Role: "team lead"}, // a differently-cased CV
		Claims:     []string{"cut  report load by 95%."},
	}}, "second")
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if got.EmploymentsCreated != 0 {
		t.Errorf("employments created = %d, want 0 — the match is case-insensitive", got.EmploymentsCreated)
	}
	if got.AtomsSkipped != 1 {
		t.Errorf("atoms skipped = %d, want 1 — the claim key ignores case and punctuation", got.AtomsSkipped)
	}
}

// A role the user corrected by hand must survive the re-upload of the CV it came from.
func TestImportNeverOverwritesAUserEdit(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	created, err := s.CreateEmployment(ctx, owner, Employment{
		Kind: KindJob, Company: "RingCentral", Role: "Senior Software Engineer",
		Location: "Florianópolis", // the user's own correction
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}

	if _, err := s.Import(ctx, owner, []ImportEntry{ringCentral()}, "stamp"); err != nil {
		t.Fatalf("Import: %v", err)
	}

	got, err := s.GetEmployment(ctx, created.ID, owner)
	if err != nil {
		t.Fatalf("GetEmployment: %v", err)
	}
	if got.Location != "Florianópolis" {
		t.Errorf("location = %q, want the user's correction preserved", got.Location)
	}
	if got.Summary == "" {
		t.Error("summary is still blank, want it filled from the CV")
	}
}

func TestImportFillsEmptyProjectLinkAndPreservesExisting(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	created, err := s.CreateEmployment(ctx, owner, Employment{
		Kind: KindProject, Company: "telagon.io",
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}

	if _, err := s.Import(ctx, owner, []ImportEntry{{
		Employment: Employment{Kind: KindProject, Company: "telagon.io", Link: "https://telagon.io"},
		Claims:     []string{"1.4M+ channels indexed"},
	}}, "stamp"); err != nil {
		t.Fatalf("Import fill: %v", err)
	}
	got, err := s.GetEmployment(ctx, created.ID, owner)
	if err != nil {
		t.Fatalf("GetEmployment: %v", err)
	}
	if got.Link != "https://telagon.io" {
		t.Errorf("link = %q, want blank filled from the CV", got.Link)
	}

	if _, err := s.Import(ctx, owner, []ImportEntry{{
		Employment: Employment{Kind: KindProject, Company: "telagon.io", Link: "https://other.example"},
	}}, "stamp2"); err != nil {
		t.Fatalf("Import preserve: %v", err)
	}
	got, err = s.GetEmployment(ctx, created.ID, owner)
	if err != nil {
		t.Fatalf("GetEmployment after re-import: %v", err)
	}
	if got.Link != "https://telagon.io" {
		t.Errorf("link = %q, want the earlier value preserved", got.Link)
	}
}

// A CV row that names no company and no role names no PLACE — but its bullets are still
// achievements. They are banked standing alone rather than discarded: the bias of this
// domain is that evidence is never lost because its context was unparseable. Nor may such
// a row abort the import of the rows around it.
func TestImportBanksClaimsWhoseRoleIsUnnamed(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	got, err := s.Import(ctx, owner, []ImportEntry{
		{Employment: Employment{Kind: KindJob}, Claims: []string{"AWS Certified Solutions Architect"}},
		ringCentral(),
	}, "stamp")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if got.EmploymentsCreated != 1 {
		t.Errorf("employments created = %d, want 1 — the unnamed place is not a place", got.EmploymentsCreated)
	}
	if got.AtomsCreated != 3 {
		t.Errorf("atoms created = %d, want 3 — the unnamed row still contributed its claim", got.AtomsCreated)
	}

	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	var standalone int
	for _, a := range atoms {
		if a.EmploymentID == nil {
			standalone++
		}
	}
	if standalone != 1 {
		t.Errorf("standalone atoms = %d, want 1", standalone)
	}
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
