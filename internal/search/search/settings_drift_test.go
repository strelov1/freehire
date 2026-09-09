package search

import (
	"reflect"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

func TestDiffSettingsReportsNothingWhenLiveMatchesWant(t *testing.T) {
	want := &meilisearch.Settings{
		SortableAttributes:   []string{"posted_at", "view_count"},
		FilterableAttributes: []string{"is_tech", "source"},
		Embedders:            map[string]meilisearch.Embedder{"skills": {}},
	}
	live := &meilisearch.Settings{
		SortableAttributes:   []string{"posted_at", "view_count"},
		FilterableAttributes: []string{"is_tech", "source"},
		Embedders:            map[string]meilisearch.Embedder{"skills": {}},
	}
	if got := diffSettings("jobs", live, want); got != nil {
		t.Errorf("diffSettings() = %v, want nil (live matches want exactly)", got)
	}
}

func TestDiffSettingsReportsAMissingSortableAttribute(t *testing.T) {
	want := &meilisearch.Settings{SortableAttributes: []string{"posted_at", "view_count"}}
	live := &meilisearch.Settings{SortableAttributes: []string{"posted_at"}}
	got := diffSettings("jobs", live, want)
	want2 := []string{`jobs: sortable attribute "view_count" not yet live`}
	if !reflect.DeepEqual(got, want2) {
		t.Errorf("diffSettings() = %v, want %v", got, want2)
	}
}

func TestDiffSettingsReportsAMissingFilterableAttribute(t *testing.T) {
	want := &meilisearch.Settings{FilterableAttributes: []string{"is_tech", "role_type"}}
	live := &meilisearch.Settings{FilterableAttributes: []string{"is_tech"}}
	got := diffSettings("jobs", live, want)
	want2 := []string{`jobs: filterable attribute "role_type" not yet live`}
	if !reflect.DeepEqual(got, want2) {
		t.Errorf("diffSettings() = %v, want %v", got, want2)
	}
}

func TestDiffSettingsReportsAMissingEmbedder(t *testing.T) {
	want := &meilisearch.Settings{Embedders: map[string]meilisearch.Embedder{SkillEmbedder: {}}}
	live := &meilisearch.Settings{Embedders: map[string]meilisearch.Embedder{}}
	got := diffSettings("jobs", live, want)
	want2 := []string{`jobs: embedder "skills" not yet live`}
	if !reflect.DeepEqual(got, want2) {
		t.Errorf("diffSettings() = %v, want %v", got, want2)
	}
}

// The hazard is one-directional (see AGENTS.md's "Adding a filterable attribute"): a
// live index still declaring something this binary no longer asks for never breaks a
// request, so it must never be reported as drift.
func TestDiffSettingsIgnoresALiveAttributeTheBinaryNoLongerWants(t *testing.T) {
	want := &meilisearch.Settings{SortableAttributes: []string{"posted_at"}}
	live := &meilisearch.Settings{SortableAttributes: []string{"posted_at", "retired_attribute"}}
	if got := diffSettings("jobs", live, want); got != nil {
		t.Errorf("diffSettings() = %v, want nil (a retired-but-still-live attribute is not drift)", got)
	}
}

func TestDiffSettingsNamesTheIndexInEveryLine(t *testing.T) {
	want := &meilisearch.Settings{SortableAttributes: []string{"job_count"}}
	live := &meilisearch.Settings{}
	got := diffSettings("companies", live, want)
	want2 := []string{`companies: sortable attribute "job_count" not yet live`}
	if !reflect.DeepEqual(got, want2) {
		t.Errorf("diffSettings() = %v, want %v", got, want2)
	}
}
