package pii

import (
	"context"
	"errors"
	"testing"
)

// errDetector always fails, standing in for an unavailable detection endpoint.
type errDetector struct{}

func (errDetector) Detect(context.Context, string) ([]Span, error) {
	return nil, errors.New("boom")
}

func TestBuildFailsClosedWhenDetectorNil(t *testing.T) {
	if _, err := Build(context.Background(), "Ada Lovelace", Contacts{}, nil); err == nil {
		t.Fatal("expected error for nil detector (fail-closed), got nil")
	}
}

func TestBuildFailsClosedWhenDetectorErrors(t *testing.T) {
	_, err := Build(context.Background(), "Ada Lovelace", Contacts{}, errDetector{})
	if err == nil {
		t.Fatal("expected error when detector fails (fail-closed), got nil")
	}
}
