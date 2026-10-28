package search

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestFromJobWritesTheSourceTypeItIsGiven(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "greenhouse"}, nil, "ats")
	if err != nil {
		t.Fatal(err)
	}
	if doc.SourceType != "ats" {
		t.Errorf("SourceType = %q, want ats", doc.SourceType)
	}
}

func TestSourceTypeFilter(t *testing.T) {
	cases := map[string]string{
		"source_type=ats&source_type=company": `[["source_type = \"ats\"" "source_type = \"company\""]]`,
		"source_type_exclude=aggregator":      `[["source_type != \"aggregator\""]]`,
	}
	for query, want := range cases {
		v, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%q", FilterFromValues(v)); got != want {
			t.Errorf("FilterFromValues(%q) = %s, want %s", query, got, want)
		}
	}
}

func TestSourceTypeIsAClosedVocabulary(t *testing.T) {
	v := url.Values{"source_type": {"ats", "aggregators"}}
	want := map[string][]string{"source_type": {"aggregators"}}
	if got := IgnoredValues(v); !reflect.DeepEqual(got, want) {
		t.Errorf("IgnoredValues = %v, want %v", got, want)
	}
}

func TestAnUnclassifiedDocumentStillCarriesTheSourceTypeKey(t *testing.T) {
	// Meilisearch's `!=` does not match a document that lacks the attribute, so with the key
	// omitted `source_type_exclude=aggregator` would also drop every manual import and pasted
	// link. An empty value keeps them, the same way role_type's empty value does.
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "weblink"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatal(err)
	}
	if _, ok := full["source_type"]; !ok {
		t.Errorf("document omits source_type, so an exclusion filter drops it: %s", raw)
	}
}
