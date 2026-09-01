package main

import (
	"slices"
	"testing"
)

func TestWaveTakesTheMostDemandedUndescribedSkills(t *testing.T) {
	canonicals := []string{"go", "kubernetes", "cobol", "dbt"}
	described := map[string]string{"go": "A compiled language."}
	demand := map[string]int{"go": 9000, "kubernetes": 400, "dbt": 120, "cobol": 3}

	got := wave(canonicals, described, demand, 2)

	want := []string{"kubernetes", "dbt"}
	if !slices.Equal(got, want) {
		t.Errorf("wave = %v, want %v", got, want)
	}
}

// A skill the catalogue does not currently name still needs a description — the
// glossary covers the vocabulary, not the week's postings. It sorts last, which is
// exactly the wave ordering the programme wants, but it must not vanish.
func TestWaveKeepsSkillsWithNoPostings(t *testing.T) {
	canonicals := []string{"go", "as400"}
	demand := map[string]int{"go": 9000}

	got := wave(canonicals, nil, demand, 0)

	want := []string{"go", "as400"}
	if !slices.Equal(got, want) {
		t.Errorf("wave = %v, want %v", got, want)
	}
}

// Two skills at the same count must not swap places between runs: the operator reviews
// a wave, edits it, and re-runs to check what is left.
func TestWaveBreaksTiesBySlug(t *testing.T) {
	canonicals := []string{"zsh", "awk", "sed"}
	demand := map[string]int{"zsh": 5, "awk": 5, "sed": 5}

	got := wave(canonicals, nil, demand, 0)

	want := []string{"awk", "sed", "zsh"}
	if !slices.Equal(got, want) {
		t.Errorf("wave = %v, want %v", got, want)
	}
}

func TestWaveWithNothingLeftToDescribe(t *testing.T) {
	described := map[string]string{"go": "A compiled language."}
	if got := wave([]string{"go"}, described, nil, 10); len(got) != 0 {
		t.Errorf("wave = %v, want empty", got)
	}
}

// A limit past the end is not an error: the last wave asks for more than remains.
func TestWaveLimitBeyondTheRemainder(t *testing.T) {
	got := wave([]string{"go"}, nil, map[string]int{"go": 1}, 500)
	if !slices.Equal(got, []string{"go"}) {
		t.Errorf("wave = %v, want [go]", got)
	}
}

func TestParseSkillDemandReadsTheFacetsEnvelope(t *testing.T) {
	body := []byte(`{"data":{"total":812345,"facets":{"skills":{"go":9000,"dbt":120}}}}`)

	got, err := parseSkillDemand(body)
	if err != nil {
		t.Fatalf("parseSkillDemand: %v", err)
	}
	if got["go"] != 9000 || got["dbt"] != 120 || len(got) != 2 {
		t.Errorf("parseSkillDemand = %v, want {go:9000 dbt:120}", got)
	}
}

// An envelope with no skills distribution would silently order the whole vocabulary by
// zero, which reads as "the wave is alphabetical" rather than as a failed read.
func TestParseSkillDemandRejectsAnEnvelopeWithoutSkills(t *testing.T) {
	for _, body := range []string{
		`{"data":{"total":1,"facets":{"seniority":{"senior":5}}}}`,
		`{"data":{"total":1,"facets":{}}}`,
		`{"error":"nope"}`,
		`not json`,
	} {
		if _, err := parseSkillDemand([]byte(body)); err == nil {
			t.Errorf("parseSkillDemand(%q) = nil error, want one", body)
		}
	}
}
