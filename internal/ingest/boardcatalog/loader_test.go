package boardcatalog

import (
	"context"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// fakeRepo is a minimal in-memory Repository for loader tests that need no persistence
// behavior, only ListActiveForProvider's return value.
type fakeRepo struct {
	byProvider map[string][]Board
}

func (f *fakeRepo) InsertRow(context.Context, InsertInput, Status, string) (Board, error) {
	panic("not used by loader tests")
}
func (f *fakeRepo) Activate(context.Context, string, string, string) (bool, error) {
	panic("not used by loader tests")
}
func (f *fakeRepo) Retire(context.Context, string, string, string) (bool, error) {
	panic("not used by loader tests")
}
func (f *fakeRepo) Rename(context.Context, string, string, string, string) (bool, error) {
	panic("not used by loader tests")
}
func (f *fakeRepo) ListActiveForProvider(_ context.Context, provider string) ([]Board, error) {
	return f.byProvider[provider], nil
}
func (f *fakeRepo) ListBySubmitter(context.Context, int64) ([]Board, error) {
	panic("not used by loader tests")
}

func TestLoadForProviderMapsBoardsToCompanyEntries(t *testing.T) {
	repo := &fakeRepo{byProvider: map[string][]Board{
		"greenhouse": {
			{Provider: "greenhouse", Board: "cohere", Region: "eu", Company: "Cohere", Hub: false, Tenants: nil, Status: StatusActive},
			{Provider: "greenhouse", Board: "acme", Company: "Acme", Hub: true, Tenants: map[string]string{"k": "v"}, Status: StatusPending},
		},
	}}

	entries, err := LoadForProvider(context.Background(), repo, "greenhouse")
	if err != nil {
		t.Fatalf("LoadForProvider: %v", err)
	}
	want := []sources.CompanyEntry{
		{Company: "Cohere", Provider: "greenhouse", Board: "cohere", Region: "eu"},
		{Company: "Acme", Provider: "greenhouse", Board: "acme", Hub: true, Tenants: map[string]string{"k": "v"}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v, want %+v", entries, want)
	}
}

func TestLoadForProviderReturnsEmptyForUnknownProvider(t *testing.T) {
	repo := &fakeRepo{byProvider: map[string][]Board{}}

	entries, err := LoadForProvider(context.Background(), repo, "nobody-here")
	if err != nil {
		t.Fatalf("LoadForProvider: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want empty", entries)
	}
}
