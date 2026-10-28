package search

import (
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
