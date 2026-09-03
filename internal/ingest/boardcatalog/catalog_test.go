package boardcatalog

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestValidateRejectsUnknownProvider(t *testing.T) {
	err := Validate(InsertInput{Provider: "myspace", Company: "Acme", Board: "acme"}, sources.Taxonomy())
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "myspace") {
		t.Errorf("error %q should name the unknown provider", err.Error())
	}
}

func TestValidateRejectsEmptyBoardForBoardBasedProvider(t *testing.T) {
	err := Validate(InsertInput{Provider: "greenhouse", Company: "Cohere"}, sources.Taxonomy())
	if err == nil {
		t.Fatal("expected error for empty board on a board-based provider, got nil")
	}
}

func TestValidateAcceptsEmptyBoardForBoardlessProvider(t *testing.T) {
	err := Validate(InsertInput{Provider: "ozon", Company: "Ozon"}, sources.Taxonomy())
	if err != nil {
		t.Errorf("Validate: boardless provider with empty board should be accepted, got %v", err)
	}
}

func TestValidateAcceptsValidBoardBasedEntry(t *testing.T) {
	err := Validate(InsertInput{Provider: "greenhouse", Company: "Cohere", Board: "cohere"}, sources.Taxonomy())
	if err != nil {
		t.Errorf("Validate: unexpected error %v", err)
	}
}
