package pii

import (
	"reflect"
	"sort"
	"testing"
)

// collect returns the matched substrings for a given kind, sorted, so a test can
// assert on values regardless of detection order.
func collect(text string, spans []Span, kind string) []string {
	var out []string
	for _, s := range spans {
		if s.Kind == kind {
			out = append(out, text[s.Start:s.End])
		}
	}
	sort.Strings(out)
	return out
}

func TestRegexSpans(t *testing.T) {
	tests := []struct {
		name string
		text string
		kind string
		want []string
	}{
		{
			name: "email",
			text: "reach me at ada.lovelace@example.com anytime",
			kind: KindEmail,
			want: []string{"ada.lovelace@example.com"},
		},
		{
			name: "linkedin and github urls",
			text: "linkedin.com/in/adalovelace | github.com/adalovelace",
			kind: KindLink,
			want: []string{"github.com/adalovelace", "linkedin.com/in/adalovelace"},
		},
		{
			name: "https portfolio url",
			text: "portfolio: https://portfolio.example.dev/ here",
			kind: KindLink,
			want: []string{"https://portfolio.example.dev/"},
		},
		{
			name: "telegram handle",
			text: "ping @jprice_dev on tg",
			kind: KindLink,
			want: []string{"@jprice_dev"},
		},
		{
			name: "phone with country code",
			text: "call +1 (415) 555-2671 for details",
			kind: KindPhone,
			want: []string{"+1 (415) 555-2671"},
		},
		{
			name: "year range is not a phone",
			text: "worked there 2012-2016 as an engineer",
			kind: KindPhone,
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(tc.text, regexSpans(tc.text), tc.kind)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("regexSpans %s: got %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}
