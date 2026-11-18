package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadQueriesExpandsAMatrixAfterTheExplicitQueries(t *testing.T) {
	queries, err := loadQueries(writeTemp(t, `
queries:
  - keywords: golang
    location: Germany
    pages: 3
matrix:
  - keywords: [golang, data engineer]
    locations: [Germany, France]
    jobage: 14
`))
	if err != nil {
		t.Fatal(err)
	}
	// golang/Germany keeps its explicit spelling and budget: the first occurrence wins.
	want := []query{
		{Keywords: "golang", Location: "Germany", Pages: 3, JobAge: defaultJobAge},
		{Keywords: "golang", Location: "France", Pages: defaultPages, JobAge: 14},
		{Keywords: "data engineer", Location: "Germany", Pages: defaultPages, JobAge: 14},
		{Keywords: "data engineer", Location: "France", Pages: defaultPages, JobAge: 14},
	}
	if !reflect.DeepEqual(queries, want) {
		t.Fatalf("matrix expansion:\n got %+v\nwant %+v", queries, want)
	}
}

func TestLoadQueriesRejectsAnIncompleteMatrix(t *testing.T) {
	for _, doc := range []string{
		"matrix:\n  - keywords: [golang]\n",
		"matrix:\n  - locations: [Germany]\n",
		"matrix:\n  - keywords: [golang]\n    locations: [Germany]\n    pages: 1.5\n",
	} {
		if _, err := loadQueries(writeTemp(t, doc)); err == nil || !strings.Contains(err.Error(), "matrix") && !strings.Contains(err.Error(), "pages") {
			t.Errorf("accepted %q: %v", doc, err)
		}
	}
}
