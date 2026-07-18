package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	data := []byte(`
- company: Cohere
  board: cohere
- company: Stripe
  board: stripe
`)

	cfg, err := ParseConfig("greenhouse", data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Provider != "greenhouse" {
		t.Errorf("Provider = %q, want greenhouse", cfg.Provider)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2", len(cfg.Sources))
	}
	want := CompanyEntry{Company: "Cohere", Provider: "greenhouse", Board: "cohere"}
	if cfg.Sources[0] != want {
		t.Errorf("Sources[0] = %+v, want %+v", cfg.Sources[0], want)
	}
}

// Two entries whose board ids differ only by case address the same case-insensitive ATS
// tenant, so they crawl identical postings but namespace external_id differently — one
// posting becomes two rows under the same company_slug, and the post-run unseen sweep
// (scoped by company_slug) closes whichever side a run failed to refresh. ParseConfig
// collapses such entries to the first occurrence so only one row-set is ever created.
func TestParseConfigDropsCaseInsensitiveBoardDuplicates(t *testing.T) {
	data := []byte(`
- company: SopraSteria1
  board: SopraSteria1
- company: Stripe
  board: stripe
- company: soprasteria1
  board: soprasteria1
`)
	cfg, err := ParseConfig("smartrecruiters", data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	want := []CompanyEntry{
		{Company: "SopraSteria1", Provider: "smartrecruiters", Board: "SopraSteria1"}, // first wins
		{Company: "Stripe", Provider: "smartrecruiters", Board: "stripe"},
	}
	if len(cfg.Sources) != len(want) {
		t.Fatalf("len(Sources) = %d, want %d: %+v", len(cfg.Sources), len(want), cfg.Sources)
	}
	for i, w := range want {
		if cfg.Sources[i] != w {
			t.Errorf("Sources[%d] = %+v, want %+v", i, cfg.Sources[i], w)
		}
	}
}

// A same-name board on two different regional hosts (distinct Region) is a genuinely
// different crawl target, not a case duplicate, so both are kept.
func TestParseConfigKeepsSameBoardOnDifferentRegions(t *testing.T) {
	data := []byte(`
- company: Acme
  board: acme
  region: us
- company: Acme
  board: acme
  region: eu
`)
	cfg, err := ParseConfig("lever", data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2 (region variants kept): %+v", len(cfg.Sources), cfg.Sources)
	}
}

// Boardless entries carry no board id, so they must never collapse together on an empty
// board key — each is its own company.
func TestParseConfigKeepsBoardlessEntries(t *testing.T) {
	data := []byte(`
- company: VK
  provider: vk
- company: Ozon
  provider: ozon
`)
	cfg, err := ParseConfig("custom", data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2 (boardless entries kept): %+v", len(cfg.Sources), cfg.Sources)
	}
}

// LoadConfig takes the provider from the file name, so the board file never repeats
// it per entry.
func TestLoadConfigInfersProviderFromFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ashby.yml")
	if err := os.WriteFile(path, []byte("- company: Vercel\n  board: vercel\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Provider != "ashby" {
		t.Errorf("Provider = %q, want ashby (from file name)", cfg.Provider)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Provider != "ashby" {
		t.Errorf("Sources = %+v, want one ashby entry", cfg.Sources)
	}
}

func TestConfigValidateRejectsUnknownProvider(t *testing.T) {
	cfg := Config{Provider: "myspace", Sources: []CompanyEntry{{Company: "Acme", Board: "acme"}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}))
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "myspace") {
		t.Errorf("error %q should name the unknown provider", err.Error())
	}
}

func TestConfigValidateRejectsEmptyBoard(t *testing.T) {
	cfg := Config{Provider: "greenhouse", Sources: []CompanyEntry{{Company: "Cohere", Board: ""}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}))
	if err == nil {
		t.Fatal("expected error for empty board, got nil")
	}
	if !strings.Contains(err.Error(), "Cohere") {
		t.Errorf("error %q should name the offending company", err.Error())
	}
}

func TestConfigValidateRejectsEmptyCompany(t *testing.T) {
	cfg := Config{Provider: "greenhouse", Sources: []CompanyEntry{{Company: "", Board: "cohere"}}}

	if err := cfg.Validate(reg(fakeSource{"greenhouse"})); err == nil {
		t.Fatal("expected error for empty company, got nil")
	}
}

func TestConfigValidateAcceptsKnownProviders(t *testing.T) {
	cfg := Config{Provider: "greenhouse", Sources: []CompanyEntry{{Company: "Cohere", Board: "cohere"}}}

	if err := cfg.Validate(reg(fakeSource{"greenhouse"})); err != nil {
		t.Errorf("Validate: unexpected error %v", err)
	}
}

// A single-company adapter that declares itself boardless may omit board.
func TestConfigValidateAcceptsEmptyBoardForBoardlessProvider(t *testing.T) {
	cfg := Config{Provider: "ozon", Sources: []CompanyEntry{{Company: "Ozon", Board: ""}}}

	if err := cfg.Validate(reg(fakeBoardlessSource{"ozon"})); err != nil {
		t.Errorf("Validate: boardless provider with empty board should be accepted, got %v", err)
	}
}

// A boardless provider still needs a company.
func TestConfigValidateRejectsEmptyCompanyEvenForBoardlessProvider(t *testing.T) {
	cfg := Config{Provider: "ozon", Sources: []CompanyEntry{{Company: "", Board: ""}}}

	if err := cfg.Validate(reg(fakeBoardlessSource{"ozon"})); err == nil {
		t.Fatal("expected error for empty company, got nil")
	}
}

// An entry may name its own provider; it wins over the file-name default. An entry that
// omits provider falls back to the file name, so existing per-provider files are unchanged.
// One file can thus carry several providers (e.g. a shared custom.yml).
func TestParseConfigKeepsPerEntryProvider(t *testing.T) {
	data := []byte(`
- company: VK
  provider: vk
- company: Yandex
  provider: yandex
  board: ru
- company: NoProv
  board: x
`)
	cfg, err := ParseConfig("custom", data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	want := []CompanyEntry{
		{Company: "VK", Provider: "vk"},
		{Company: "Yandex", Provider: "yandex", Board: "ru"},
		{Company: "NoProv", Provider: "custom", Board: "x"}, // fell back to the file name
	}
	for i, w := range want {
		if cfg.Sources[i] != w {
			t.Errorf("Sources[%d] = %+v, want %+v", i, cfg.Sources[i], w)
		}
	}
}

// Validation resolves each entry's provider independently, so a single file with mixed
// providers validates each against its own adapter.
func TestConfigValidateAcceptsMixedPerEntryProviders(t *testing.T) {
	cfg := Config{Provider: "custom", Sources: []CompanyEntry{
		{Company: "VK", Provider: "vk"},                          // boardless: empty board ok
		{Company: "Acme", Provider: "greenhouse", Board: "acme"}, // board-based: has board
	}}

	if err := cfg.Validate(reg(fakeBoardlessSource{"vk"}, fakeSource{"greenhouse"})); err != nil {
		t.Errorf("Validate: mixed per-entry providers should pass, got %v", err)
	}
}

// An entry whose resolved provider has no adapter fails fast — including the custom.yml
// case where the file name "custom" is not a provider and the entry omitted one.
func TestConfigValidateRejectsUnknownPerEntryProvider(t *testing.T) {
	cfg := Config{Provider: "custom", Sources: []CompanyEntry{{Company: "Orphan", Board: "x"}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}))
	if err == nil {
		t.Fatal("expected error for an entry resolving to an unregistered provider, got nil")
	}
	if !strings.Contains(err.Error(), "custom") {
		t.Errorf("error %q should name the unknown resolved provider", err.Error())
	}
}

// A board-based provider named per entry still requires a board.
func TestConfigValidateRejectsEmptyBoardForPerEntryBoardProvider(t *testing.T) {
	cfg := Config{Provider: "custom", Sources: []CompanyEntry{{Company: "Acme", Provider: "greenhouse"}}}

	err := cfg.Validate(reg(fakeSource{"greenhouse"}, fakeBoardlessSource{"vk"}))
	if err == nil || !strings.Contains(err.Error(), "Acme") {
		t.Fatalf("expected empty-board error naming Acme, got %v", err)
	}
}
