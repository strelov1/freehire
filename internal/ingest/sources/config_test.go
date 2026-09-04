package sources

import (
	"strings"
	"testing"
)

func TestConfigValidateRejectsUnknownProvider(t *testing.T) {
	cfg := Config{Provider: "myspace", Sources: []CompanyEntry{{Company: "Acme", Provider: "myspace", Board: "acme"}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}))
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "myspace") {
		t.Errorf("error %q should name the unknown provider", err.Error())
	}
}

func TestConfigValidateRejectsEmptyBoard(t *testing.T) {
	cfg := Config{Provider: "greenhouse", Sources: []CompanyEntry{{Company: "Cohere", Provider: "greenhouse"}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}))
	if err == nil {
		t.Fatal("expected error for empty board, got nil")
	}
	if !strings.Contains(err.Error(), "Cohere") {
		t.Errorf("error %q should name the offending company", err.Error())
	}
}

func TestConfigValidateRejectsEmptyCompany(t *testing.T) {
	cfg := Config{Provider: "greenhouse", Sources: []CompanyEntry{{Provider: "greenhouse", Board: "cohere"}}}

	if err := cfg.Validate(reg(fakeSource{"greenhouse"})); err == nil {
		t.Fatal("expected error for empty company, got nil")
	}
}

func TestConfigValidateAcceptsKnownProviders(t *testing.T) {
	cfg := Config{Provider: "greenhouse", Sources: []CompanyEntry{{Company: "Cohere", Provider: "greenhouse", Board: "cohere"}}}

	if err := cfg.Validate(reg(fakeSource{"greenhouse"})); err != nil {
		t.Errorf("Validate: unexpected error %v", err)
	}
}

// A single-company adapter that declares itself boardless may omit board.
func TestConfigValidateAcceptsEmptyBoardForBoardlessProvider(t *testing.T) {
	cfg := Config{Provider: "ozon", Sources: []CompanyEntry{{Company: "Ozon", Provider: "ozon"}}}

	if err := cfg.Validate(reg(fakeBoardlessSource{"ozon"})); err != nil {
		t.Errorf("Validate: boardless provider with empty board should be accepted, got %v", err)
	}
}

// A boardless provider still needs a company.
func TestConfigValidateRejectsEmptyCompanyEvenForBoardlessProvider(t *testing.T) {
	cfg := Config{Provider: "ozon", Sources: []CompanyEntry{{Provider: "ozon"}}}

	if err := cfg.Validate(reg(fakeBoardlessSource{"ozon"})); err == nil {
		t.Fatal("expected error for empty company, got nil")
	}
}

// Validation resolves each entry's provider independently, so one Config carrying boards
// for several providers validates each against its own adapter.
func TestConfigValidateAcceptsMixedPerEntryProviders(t *testing.T) {
	cfg := Config{Provider: "custom", Sources: []CompanyEntry{
		{Company: "VK", Provider: "vk"},
		{Company: "Yandex", Provider: "yandex", Board: "ru"},
	}}

	if err := cfg.Validate(reg(fakeBoardlessSource{"vk"}, fakeSource{"yandex"})); err != nil {
		t.Errorf("Validate: unexpected error %v", err)
	}
}

// A board-based provider still requires a board even when the Config's own provider is
// something else.
func TestConfigValidateRejectsEmptyBoardForPerEntryBoardProvider(t *testing.T) {
	cfg := Config{Provider: "custom", Sources: []CompanyEntry{{Company: "Acme", Provider: "greenhouse"}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}, fakeBoardlessSource{"vk"}))
	if err == nil || !strings.Contains(err.Error(), "Acme") {
		t.Fatalf("expected empty-board error naming Acme, got %v", err)
	}
}

// Two entries whose board ids differ only by case address the same case-insensitive ATS
// tenant, so they crawl identical postings but namespace external_id differently — one
// posting becomes two rows under the same company_slug, and the post-run unseen sweep
// (scoped by company_slug) closes whichever side a run failed to refresh.
func TestBoardDedupeKeyFoldsCase(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "SopraSteria1", Provider: "smartrecruiters", Board: "SopraSteria1"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "soprasteria1", Provider: "smartrecruiters", Board: "soprasteria1"})
	if a != b {
		t.Errorf("case-variant boards should share a key, got %q and %q", a, b)
	}
}

// Two spellings of one iCIMS board address the same site: the adapter builds
// "careers-<slug>.icims.com" from a bare slug and takes a dotted board as the host
// verbatim, so "vet" and "careers-vet.icims.com" are one crawl target under two names.
// Case folding alone does not see that, and 37 such pairs were once crawled twice.
func TestBoardDedupeKeyFoldsBoardSpellingsAddressingTheSameSite(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "Vet", Provider: "icims", Board: "vet"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "Vet", Provider: "icims", Board: "careers-vet.icims.com"})
	if a != b {
		t.Errorf("two spellings of one iCIMS board should share a key, got %q and %q", a, b)
	}
}

// The fold is per-provider: a bare "vet" and a "careers-vet.icims.com" on a provider that
// does NOT resolve both to one host are two different boards, and collapsing them would
// silently drop a live crawl target.
func TestBoardDedupeKeyFoldsSpellingsOnlyForTheOwningProvider(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "Vet", Provider: "greenhouse", Board: "vet"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "Vet", Provider: "greenhouse", Board: "careers-vet.icims.com"})
	if a == b {
		t.Error("greenhouse does not fold iCIMS host spellings; the two boards must differ")
	}
}

// A same-name board on two different regional hosts is a genuinely different crawl
// target, not a duplicate, so the keys must differ.
func TestBoardDedupeKeySeparatesRegions(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "Acme", Provider: "lever", Board: "acme", Region: "us"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "Acme", Provider: "lever", Board: "acme", Region: "eu"})
	if a == b {
		t.Error("boards on different regions must not share a key")
	}
}

// The fold covers Provider and Region too, not just Board — a region typed "US" on one
// row and "us" on another is the same host, and a case-variant provider would otherwise
// dodge the collision this key exists to catch.
func TestBoardDedupeKeyFoldsProviderAndRegionCase(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "Acme", Provider: "Lever", Board: "acme", Region: "US"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "Acme Inc", Provider: "lever", Board: "acme", Region: "us"})
	if a != b {
		t.Errorf("provider/region case variants should share a key, got %q and %q", a, b)
	}
}

// Boardless entries have no tenant id, so two of them must never read as duplicates of
// each other just because both have an empty board.
func TestBoardDedupeKeyRefusesBoardlessEntries(t *testing.T) {
	if _, ok := BoardDedupeKey(CompanyEntry{Company: "VK", Provider: "vk"}); ok {
		t.Error("a boardless entry has no dedupe identity")
	}
}
