package main

import (
	"reflect"
	"testing"
)

func TestSplitList(t *testing.T) {
	if got := splitList("himalayas, remoteok ,"); !reflect.DeepEqual(got, []string{"himalayas", "remoteok"}) {
		t.Errorf("splitList = %v, want [himalayas remoteok]", got)
	}
	if got := splitList("  "); got != nil {
		t.Errorf("splitList of blanks = %v, want nil", got)
	}
}

// A source misspelt on the command line selects no rows, and an empty worklist is
// indistinguishable from "every company already has an ATS" — so the run must reject the
// name instead of reporting success.
func TestNotInCatchesUnknownSources(t *testing.T) {
	all := []string{"himalayas", "remoteok"}
	if got := notIn([]string{"himalayas", "himalaya"}, all); !reflect.DeepEqual(got, []string{"himalaya"}) {
		t.Errorf("notIn = %v, want [himalaya]", got)
	}
	if got := notIn([]string{"himalayas", "remoteok"}, all); got != nil {
		t.Errorf("notIn of known sources = %v, want nil", got)
	}
}

// Every default aggregator must be a real aggregator source, or the tool's own default
// selects nothing.
func TestRemoteAggregatorsAreKnownSources(t *testing.T) {
	if len(remoteAggregators) == 0 {
		t.Fatal("no default aggregators")
	}
	if missing := notIn(remoteAggregators, allAggregatorProviders()); len(missing) > 0 {
		t.Errorf("defaults are not aggregator sources: %v", missing)
	}
}
