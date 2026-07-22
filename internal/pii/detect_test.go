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
			text: "reach me at strelov1@gmail.com anytime",
			kind: KindEmail,
			want: []string{"strelov1@gmail.com"},
		},
		{
			name: "linkedin and github urls",
			text: "linkedin.com/in/istrelov | github.com/strelov1",
			kind: KindLink,
			want: []string{"github.com/strelov1", "linkedin.com/in/istrelov"},
		},
		{
			name: "https portfolio url",
			text: "portfolio: https://alex-bes.vercel.app/ here",
			kind: KindLink,
			want: []string{"https://alex-bes.vercel.app/"},
		},
		{
			name: "telegram handle",
			text: "ping @Alex_Sage on tg",
			kind: KindLink,
			want: []string{"@Alex_Sage"},
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
