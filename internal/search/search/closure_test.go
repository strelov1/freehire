package search

import (
	"reflect"
	"testing"
)

func TestWithOpenJobsPreservesFilterGroups(t *testing.T) {
	cases := []struct {
		name        string
		input, want any
	}{
		{"unfiltered", nil, "NOT is_closed = true"},
		{"expression", `source = "a" OR source = "b"`, `(source = "a" OR source = "b") AND NOT is_closed = true`},
		{"groups", [][]string{{`source = "a"`, `source = "b"`}, {`country = "DE"`}}, [][]string{{`source = "a"`, `source = "b"`}, {`country = "DE"`}, {"NOT is_closed = true"}}},
		{"flat", []string{`country = "DE"`}, []string{`country = "DE"`, "NOT is_closed = true"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withOpenJobs(c.input); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("filter=%v, want %v", got, c.want)
			}
		})
	}
}
