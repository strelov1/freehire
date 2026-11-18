package atscheck

import (
	"reflect"
	"testing"
)

func TestTopRoleSkills_RanksByDemandThenSlug(t *testing.T) {
	facet := map[string]int64{"go": 50, "rust": 50, "sql": 80, "perl": 1}
	got := TopRoleSkills(facet, 3)
	want := []string{"sql", "go", "rust"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TopRoleSkills = %v, want %v", got, want)
	}
	if all := TopRoleSkills(facet, 10); len(all) != 4 {
		t.Errorf("a cap above the facet size returned %d, want all 4", len(all))
	}
	if none := TopRoleSkills(nil, RoleTopN); len(none) != 0 {
		t.Errorf("an empty facet returned %v", none)
	}
}

func TestTopGaps_BiggestRecoverableFirst(t *testing.T) {
	r := Report{Categories: []ScoreCategory{
		{Items: []LineItem{
			{Points: 4, Text: "small", Status: StatusFail},
			{Points: 20, Text: "done", Status: StatusPass},
			{Points: 9, Text: "big", Status: StatusWarn},
		}},
		{Items: []LineItem{
			{Points: 9, Text: "tie, later", Status: StatusFail},
			{Points: 0, Text: "worth nothing", Status: StatusFail},
		}},
	}}
	got := r.TopGaps(3)
	want := []string{"big", "tie, later", "small"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TopGaps = %v, want %v", got, want)
	}
	if two := r.TopGaps(2); len(two) != 2 {
		t.Errorf("TopGaps(2) returned %d", len(two))
	}
	if none := (Report{}).TopGaps(3); len(none) != 0 {
		t.Errorf("an empty report returned %v", none)
	}
}
