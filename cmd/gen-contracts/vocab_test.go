package main

import (
	"strings"
	"testing"
)

func TestGenVocabEmitsRoleLabels(t *testing.T) {
	got := genVocab()
	if !strings.Contains(got, "export const ROLE_LABELS = {") {
		t.Errorf("genVocab() missing ROLE_LABELS map:\n%s", got)
	}
	// The catalog is the source of truth for picker labels — a named role must
	// carry its human label.
	if !strings.Contains(got, "'founding_engineer': 'Founding Engineer'") {
		t.Errorf("genVocab() ROLE_LABELS missing founding_engineer label")
	}
}

func TestEmitVocab(t *testing.T) {
	got := emitVocab("Seniority", "SENIORITY_VALUES", []string{"junior", "senior"})
	want := "export const SENIORITY_VALUES = ['junior', 'senior'] as const;\n" +
		"export type Seniority = (typeof SENIORITY_VALUES)[number];\n"
	if got != want {
		t.Errorf("emitVocab mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEmitVocabEmpty(t *testing.T) {
	got := emitVocab("X", "X_VALUES", nil)
	want := "export const X_VALUES = [] as const;\n" +
		"export type X = (typeof X_VALUES)[number];\n"
	if got != want {
		t.Errorf("emitVocab(empty) = %q, want %q", got, want)
	}
}

func TestEmitMap(t *testing.T) {
	// Keys must be emitted in sorted order — the output is committed, so it has to be
	// deterministic regardless of Go's random map iteration.
	got := emitMap("CityCountry", "CITY_COUNTRY_MAP", map[string]string{"Berlin": "de", "Amsterdam": "nl"})
	want := "export const CITY_COUNTRY_MAP = {\n" +
		"  'Amsterdam': 'nl',\n" +
		"  'Berlin': 'de',\n" +
		"} as const;\n" +
		"export type CityCountry = typeof CITY_COUNTRY_MAP;\n"
	if got != want {
		t.Errorf("emitMap mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEmitMapEmpty(t *testing.T) {
	got := emitMap("X", "X_MAP", nil)
	want := "export const X_MAP = {} as const;\n" +
		"export type X = typeof X_MAP;\n"
	if got != want {
		t.Errorf("emitMap(empty) = %q, want %q", got, want)
	}
}
