package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/cv"
)

func TestContactHeaderEmpty(t *testing.T) {
	if !contactHeaderEmpty(cv.Header{}) {
		t.Fatal("empty header should report empty")
	}
	if contactHeaderEmpty(cv.Header{FullName: "Ada"}) {
		t.Fatal("name alone is not empty")
	}
}

func TestMergeSeedHeaderFillsGapsOnly(t *testing.T) {
	keep := cv.Header{FullName: "Keep", Email: ""}
	seeded := cv.Header{FullName: "Seed", Email: "new@example.com"}
	got := mergeSeedHeader(keep, seeded)
	if got.FullName != "Seed" {
		t.Errorf("FullName = %q, want Seed (non-empty seed replaces)", got.FullName)
	}
	if got.Email != "new@example.com" {
		t.Errorf("Email = %q, want filled", got.Email)
	}
	got2 := mergeSeedHeader(cv.Header{FullName: "Keep"}, cv.Header{FullName: ""})
	if got2.FullName != "Keep" {
		t.Errorf("empty seed must keep existing name, got %q", got2.FullName)
	}
}

func TestFillEmptyHeaderFieldsKeepsExistingName(t *testing.T) {
	keep := cv.Header{FullName: "Keep Me"}
	seed := cv.Header{FullName: "From Blob", Email: "blob@example.com"}
	got := fillEmptyHeaderFields(keep, seed)
	if got.FullName != "Keep Me" {
		t.Errorf("FullName = %q, want Keep Me", got.FullName)
	}
	if got.Email != "blob@example.com" {
		t.Errorf("Email = %q, want filled", got.Email)
	}
}
