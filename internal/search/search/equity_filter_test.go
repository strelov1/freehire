package search

import (
	"net/url"
	"reflect"
	"testing"
)

// equity is a closed string vocabulary whose "not stated" is the attribute's absence:
// not_stated must reach the postings that carry no equity value, never an equality
// against a value no document is written with.
func TestFilterFromValues_Equity(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{"a stated value is an equality", "equity=equity_only", [][]string{{`equity = "equity_only"`}}},
		{"not_stated selects the absent attribute", "equity=not_stated", [][]string{{"equity NOT EXISTS"}}},
		{"Not_Stated is not_stated, whatever its case", "equity=Not_Stated", [][]string{{"equity NOT EXISTS"}}},
		{"not_stated ORs with a stated value", "equity=not_stated&equity=salary_and_equity", [][]string{{`equity = "salary_and_equity"`, "equity NOT EXISTS"}}},
		{"excluding not_stated keeps the stated", "equity_exclude=not_stated", [][]string{{"equity EXISTS"}}},
		{"excluding a value is an inequality", "equity_exclude=equity_only", [][]string{{`equity != "equity_only"`}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			if got := normalizeGroups(t, FilterFromValues(v)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filter = %v, want %v", got, tt.want)
			}
		})
	}
}

// A value outside the vocabulary narrows to nothing, so it is named in
// meta.ignored_values; the reserved not_stated is known.
func TestIgnoredValues_Equity(t *testing.T) {
	v, _ := url.ParseQuery("equity=not_stated&equity=salary_and_equity&equity=stock&equity_exclude=equity_only")
	got := IgnoredValues(v)
	want := map[string][]string{"equity": {"stock"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("IgnoredValues = %v, want %v", got, want)
	}
}
