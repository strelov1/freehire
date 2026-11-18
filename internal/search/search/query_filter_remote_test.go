package search

import (
	"reflect"
	"testing"
)

// WorkModeRemoteAnywhereParam is the modifier behind the profile's most common
// shape: remote from anywhere, plus hybrid/on-site within the chosen geography.
// One group of the filter is swapped for a disjunction; every test here asserts
// the exact Meili expression so the swap cannot silently change shape.
//
// Only `remote` stands bare in the group: a bare physical mode would admit a
// hybrid posting from any country. And only the SELECTED physical modes join the
// geography arm — remote+hybrid does not quietly add on-site at home.
func TestFilterFromValues_RemoteAnywhereDisjunction(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&work_mode_remote_anywhere=true")))
	want := [][]string{{`(work_mode IN ["hybrid"] AND (countries = "de"))`, `work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The whole location group joins the physical arm — regions/countries/cities/
// metros OR together into it, exactly as they OR in the plain location group.
func TestFilterFromValues_RemoteAnywhereSpansTheLocationGroup(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid,onsite&regions=eu&cities=Berlin&work_mode_remote_anywhere=true")))
	want := [][]string{{`(work_mode IN ["hybrid", "onsite"] AND (cities = "Berlin" OR regions IN ["eu", "uk"]))`, `work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// No location include → nothing to bound the physical arm by, so the modifier
// would only widen the answer past the caller's mode list. Inert.
// The value check accepts work modes case-insensitively, so the modifier must too.
func TestFilterFromValues_RemoteAnywhereNormalizesModes(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=Remote,%20Hybrid&countries=de&work_mode_remote_anywhere=true")))
	want := [][]string{{`(work_mode IN ["hybrid"] AND (countries = "de"))`, `work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// An unknown closed-facet value is not a location: on the plain reading it matches
// nothing, so it must not unlock the unbounded remote arm.
func TestFilterFromValues_RemoteAnywhereIgnoresUnknownRegion(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&regions=not-a-region&work_mode_remote_anywhere=true")))
	want := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&regions=not-a-region")))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("modifier activated by an unknown region: got %v, want %v", got, want)
	}
}

// The remote reach bounds the remote arm only; the location group stays the
// physical arm's. Regions expand exactly as the `regions` facet does.
func TestFilterFromValues_RemoteAnywhereWithReach(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&work_mode_remote_anywhere=true&work_mode_remote_regions=eu&work_mode_remote_countries=us")))
	want := [][]string{{
		`(work_mode = "remote" AND (countries = "us" OR regions IN ["eu", "uk"]))`,
		`(work_mode IN ["hybrid"] AND (countries = "de"))`,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The reach is part of the modifier: with the modifier inert it reads nothing.
func TestFilterFromValues_RemoteReachAloneReadsNothing(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&work_mode_remote_regions=eu")))
	want := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de")))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_RemoteAnywhereNeedsALocation(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&work_mode_remote_anywhere=true")))
	want := [][]string{{`work_mode = "hybrid"`, `work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Without remote among the modes the physical arm would RELAX the filter the
// caller wrote (they named no remote arm to trade against). Inert.
func TestFilterFromValues_RemoteAnywhereNeedsRemote(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=hybrid,onsite&countries=de&work_mode_remote_anywhere=true")))
	want := [][]string{{`countries = "de"`}, {`work_mode = "hybrid"`, `work_mode = "onsite"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// AND mode is a different filter (a job must be every named mode) and cannot
// compose with the disjunction. The explicit `_mode` wins; the modifier is inert.
func TestFilterFromValues_RemoteAnywhereYieldsToAndMode(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&work_mode_mode=and&countries=de&work_mode_remote_anywhere=true")))
	want := [][]string{{`countries = "de"`}, {`work_mode = "hybrid"`}, {`work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Excludes are not folded into the disjunction: "not the US" holds in every arm.
// A location include AND a location exclude is contradictory on the plain reading
// too (the include group and the exclude group AND together), and this reading is
// no different — the exclude group sits beside the swapped work-mode group.
func TestFilterFromValues_RemoteAnywhereKeepsGlobalExcludes(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&countries_exclude=us&work_mode_remote_anywhere=true")))
	want := [][]string{
		{`(work_mode IN ["hybrid"] AND (countries = "de"))`, `work_mode = "remote"`},
		{`countries != "us"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Other facets keep their ordinary AND beside the disjunction.
func TestFilterFromValues_RemoteAnywhereComposesWithOtherFacets(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&seniority=senior&work_mode_remote_anywhere=true")))
	want := [][]string{
		{`(work_mode IN ["hybrid"] AND (countries = "de"))`, `work_mode = "remote"`},
		{`enrichment.seniority = "senior"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The value is read as a boolean: anything but "true" (case-insensitively, like
// every other bool scalar) leaves the modifier unset.
func TestFilterFromValues_RemoteAnywhereReadsTrueOnly(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&work_mode_remote_anywhere=yes")))
	want := [][]string{{`countries = "de"`}, {`work_mode = "hybrid"`, `work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("yes: got %v, want %v", got, want)
	}
	got = normalizeGroups(t, FilterFromValues(vals("work_mode=remote,hybrid&countries=de&work_mode_remote_anywhere=TRUE")))
	want = [][]string{{`(work_mode IN ["hybrid"] AND (countries = "de"))`, `work_mode = "remote"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TRUE: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_RemoteAnywhereNeedsAPhysicalMode(t *testing.T) {
	query := "work_mode=remote&countries=de"
	got := normalizeGroups(t, FilterFromValues(vals(query+"&work_mode_remote_anywhere=true")))
	want := normalizeGroups(t, FilterFromValues(vals(query)))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
