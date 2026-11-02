package search

import (
	"reflect"
	"testing"
)

func TestSourceFamilyFilter(t *testing.T) {
	in := facetEq("source", "source", "whatjobs")
	if want := `source IN ["whatjobs", "whatjobs-ae"`; len(in) < len(want) || in[:len(want)] != want {
		t.Errorf("include = %.60s…, want an IN over the family", in)
	}
	if got := facetEq("source", "source", "greenhouse"); got != `source = "greenhouse"` {
		t.Errorf("plain source = %q", got)
	}
	if ex := facetNeq("source", "source", "whatjobs"); ex[:len("source NOT IN")] != "source NOT IN" {
		t.Errorf("exclude = %.40s…", ex)
	}
}

func TestFoldSourceFamilies(t *testing.T) {
	dist := map[string]int64{"whatjobs": 5, "whatjobs-br": 3, "whatjobs-de": 2, "lever": 7}
	foldSourceFamilies(dist)
	want := map[string]int64{"whatjobs": 10, "lever": 7}
	if !reflect.DeepEqual(dist, want) {
		t.Errorf("got %v, want %v", dist, want)
	}
	foldSourceFamilies(nil)
}
