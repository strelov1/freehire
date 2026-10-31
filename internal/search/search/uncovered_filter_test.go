package search

import (
	"reflect"
	"testing"
)

func TestAndNotSkills_AppendsNeqGroupsToRole(t *testing.T) {
	role := Filter([]string{Eq("enrichment.category", "backend")})
	got := AndNotSkills(role, []string{"go", "docker"})
	want := [][]string{
		{`enrichment.category = "backend"`},
		{`skills != "go"`},
		{`skills != "docker"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestAndNotSkills_NilRole(t *testing.T) {
	got := AndNotSkills(nil, []string{"go"})
	want := [][]string{{`skills != "go"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestAndNotSkills_EmptySkillsLeavesRoleUnchanged(t *testing.T) {
	role := Filter([]string{Eq("enrichment.category", "backend")})
	got := AndNotSkills(role, nil)
	if !reflect.DeepEqual(got, role) {
		t.Errorf("got %#v, want role unchanged %#v", got, role)
	}
}

func TestAndNotSkills_NilRoleEmptySkillsIsNil(t *testing.T) {
	if got := AndNotSkills(nil, nil); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

func TestAndNotSkills_SkipsEmptySkill(t *testing.T) {
	got := AndNotSkills(nil, []string{"", "go", ""})
	want := [][]string{{`skills != "go"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestAndInStrings_AppendsOneInGroup(t *testing.T) {
	base := [][]string{{`category = "backend"`}}
	got := AndInStrings(base, "company_slug", []string{"acme", `a"b`})
	want := [][]string{{`category = "backend"`}, {`company_slug IN ["acme", "a\"b"]`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AndInStrings = %#v, want %#v", got, want)
	}
	if got := AndInStrings(nil, "company_slug", []string{"acme"}); !reflect.DeepEqual(got, [][]string{{`company_slug IN ["acme"]`}}) {
		t.Errorf("AndInStrings(nil) = %#v", got)
	}
}
