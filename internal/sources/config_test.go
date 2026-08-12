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
// case where the file name "custom" is not a provider: ParseConfig resolves the entry's
// omitted provider to the file name, which then has no adapter.
func TestConfigValidateRejectsUnknownPerEntryProvider(t *testing.T) {
	cfg, err := ParseConfig("custom", []byte("- company: Orphan\n  board: x\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	err = cfg.Validate(reg(fakeSource{"greenhouse"}))
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

// ParseConfig decodes strictly: a typo'd key has no required-field error to catch it (it
// just leaves the intended field at its zero value), so an unrecognized key must fail the
// parse outright instead of silently doing nothing.
func TestParseConfigRejectsUnknownField(t *testing.T) {
	_, err := ParseConfig("greenhouse", []byte("- company: Cohere\n  regoin: eu\n  board: cohere\n"))
	if err == nil {
		t.Fatal("expected an error for the unrecognized field, got nil")
	}
}

// A board file that is comments only (e.g. a deprecated provider kept registered but
// superseded, see sources/careerspage.yml) has no YAML document at all, which the
// underlying decoder reports as io.EOF — that must read as zero entries, not a failure.
func TestParseConfigAcceptsCommentOnlyFile(t *testing.T) {
	cfg, err := ParseConfig("careerspage", []byte("# superseded by sources/manatal.yml\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Sources) != 0 {
		t.Errorf("Sources = %+v, want none", cfg.Sources)
	}
}

// DuplicateBoards is what cmd/validate-sources uses to fail loudly on the exact
// collisions ParseConfig's dedupeBoards otherwise fixes quietly.
func TestDuplicateBoardsReportsCaseVariantCollisions(t *testing.T) {
	entries := []CompanyEntry{
		{Company: "SopraSteria1", Provider: "smartrecruiters", Board: "SopraSteria1"},
		{Company: "Stripe", Provider: "smartrecruiters", Board: "stripe"},
		{Company: "soprasteria1", Provider: "smartrecruiters", Board: "soprasteria1"},
	}
	dups := DuplicateBoards(entries)
	if len(dups) != 1 {
		t.Fatalf("DuplicateBoards = %v, want exactly one collision", dups)
	}
	if !strings.Contains(dups[0], "SopraSteria1") || !strings.Contains(dups[0], "soprasteria1") {
		t.Errorf("message %q should name both colliding companies", dups[0])
	}
}

// A same-name board on two different regions is a real, distinct crawl target — not a
// duplicate — matching TestParseConfigKeepsSameBoardOnDifferentRegions.
func TestDuplicateBoardsKeepsSameBoardOnDifferentRegions(t *testing.T) {
	entries := []CompanyEntry{
		{Company: "Acme", Provider: "lever", Board: "acme", Region: "us"},
		{Company: "Acme", Provider: "lever", Board: "acme", Region: "eu"},
	}
	if dups := DuplicateBoards(entries); len(dups) != 0 {
		t.Errorf("DuplicateBoards = %v, want none (region variants)", dups)
	}
}

// Boardless entries have no tenant id, so two of them must never read as duplicates of
// each other just because both have an empty board.
func TestDuplicateBoardsKeepsBoardlessEntries(t *testing.T) {
	entries := []CompanyEntry{
		{Company: "VK", Provider: "vk"},
		{Company: "Ozon", Provider: "ozon"},
	}
	if dups := DuplicateBoards(entries); len(dups) != 0 {
		t.Errorf("DuplicateBoards = %v, want none (boardless entries)", dups)
	}
}

// boardDedupeKey folds case on Provider and Region too, not just Board — a region typed
// "US" on one entry and "us" on another is the same host, and a case-variant Provider
// would otherwise dodge the collision boardDedupeKey exists to catch.
func TestDuplicateBoardsCatchesRegionCaseVariant(t *testing.T) {
	entries := []CompanyEntry{
		{Company: "Acme", Provider: "lever", Board: "acme", Region: "US"},
		{Company: "Acme Inc", Provider: "lever", Board: "acme", Region: "us"},
	}
	if dups := DuplicateBoards(entries); len(dups) != 1 {
		t.Errorf("DuplicateBoards = %v, want exactly one collision (region case variant)", dups)
	}
}

func TestDuplicateBoardsCatchesProviderCaseVariant(t *testing.T) {
	entries := []CompanyEntry{
		{Company: "Acme", Provider: "Lever", Board: "acme"},
		{Company: "Acme Inc", Provider: "lever", Board: "acme"},
	}
	if dups := DuplicateBoards(entries); len(dups) != 1 {
		t.Errorf("DuplicateBoards = %v, want exactly one collision (provider case variant)", dups)
	}
}

// A `---`-separated second YAML document would bypass this decoder's KnownFields check
// entirely — it only inspects the document it decodes into — so ParseConfig must reject
// it outright rather than silently parse just the first and ignore the rest.
func TestParseConfigRejectsSecondYAMLDocument(t *testing.T) {
	data := []byte("- company: Cohere\n  board: cohere\n---\n- company: Stripe\n  board: stripe\n")
	if _, err := ParseConfig("greenhouse", data); err == nil {
		t.Fatal("expected an error for a second YAML document, got nil")
	}
}
