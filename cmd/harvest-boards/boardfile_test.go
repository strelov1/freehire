package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewBoards(t *testing.T) {
	existing := map[string]bool{"acme": true, "Globex": true}
	got := newBoards([]string{"acme", "Globex", "initech", "globex", "initech"}, existing)
	// case-sensitive ("globex" != "Globex" survives) and de-duplicated within the seed.
	want := []string{"initech", "globex"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestAppendEntries(t *testing.T) {
	existing := "- company: Acme\n  board: acme\n"
	out, err := appendEntries(existing, []entry{
		{Company: "Initech", Board: "initech"},
		{Company: "Findhelp, A PBC", Board: "findhelp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Existing preserved verbatim; additions sorted by board (findhelp < initech). A comma
	// in a block scalar is not a YAML flow indicator, so yaml.v3 leaves the name unquoted.
	want := "- company: Acme\n  board: acme\n" +
		"- company: Findhelp, A PBC\n  board: findhelp\n" +
		"- company: Initech\n  board: initech\n"
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}

	// Round-trip guard: the merged file parses back into the union of all entries.
	var entries []entry
	if err := yaml.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("merged output does not parse: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("round-trip: got %d entries, want 3", len(entries))
	}
}
