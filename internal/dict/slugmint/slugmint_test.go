package slugmint_test

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/slugmint"
)

func TestNew_BuildsHyphenatedBasePlusSuffix(t *testing.T) {
	got, err := slugmint.New("Remote Go Engineer", "fallback")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	const wantBase = "remote-go-engineer-"
	if !strings.HasPrefix(got, wantBase) {
		t.Fatalf("New(%q) = %q, want prefix %q", "Remote Go Engineer", got, wantBase)
	}

	suffix := strings.TrimPrefix(got, wantBase)
	if len(suffix) != slugmint.SuffixLen {
		t.Fatalf("suffix %q has length %d, want %d", suffix, len(suffix), slugmint.SuffixLen)
	}
}

func TestNew_FallsBackWhenNameHasNoSlugCharacters(t *testing.T) {
	got, err := slugmint.New("!!!", "fallback")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if !strings.HasPrefix(got, "fallback-") {
		t.Fatalf("New(%q) = %q, want prefix %q", "!!!", got, "fallback-")
	}
}

func TestNew_TruncatesLongBase(t *testing.T) {
	longName := strings.Repeat("a ", 100) // transliterates to "a-a-a-..." far past BaseMaxLen
	got, err := slugmint.New(longName, "fallback")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	base := got[:len(got)-slugmint.SuffixLen-1] // trim "-XXXX" suffix
	if len(base) > slugmint.BaseMaxLen {
		t.Fatalf("base %q has length %d, want at most %d", base, len(base), slugmint.BaseMaxLen)
	}
	if strings.HasSuffix(base, "-") {
		t.Fatalf("base %q should not end with a trailing hyphen after truncation", base)
	}
}

func TestNew_ProducesDifferentSuffixesAcrossCalls(t *testing.T) {
	first, err := slugmint.New("Same Name", "fallback")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	second, err := slugmint.New("Same Name", "fallback")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if first == second {
		t.Fatalf("New called twice with the same name produced identical slugs %q — suffix is not random", first)
	}
}
