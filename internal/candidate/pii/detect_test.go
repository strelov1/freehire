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

// A month-precision employment range is not a phone number. phoneRe cuts "2016 - 06" out of
// "05/2016 - 06/2024"; masking it hid the year from the model, which then invented one.
func TestRegexSpans_MonthPrecisionRangesAreNotPhones(t *testing.T) {
	for _, text := range []string{
		"08/2023 - 07/2024 | Stockholm",
		"05/2016 - 06/2024",
		"12/2015 - 04/2016",
		"10.2019 - 05.2021",
		"2016.05 - 2024.06",
		"01.01.2020 - 31.12.2022",
	} {
		if got := collect(text, regexSpans(text), KindPhone); len(got) != 0 {
			t.Errorf("%q: masked %q as a phone", text, got)
		}
	}
}

// The fix must not teach the detector that anything near a slash is a date: a number that
// is not flanked by a date separator and a digit is still a phone.
func TestRegexSpans_PhonesNextToDatesStillMasked(t *testing.T) {
	for text, want := range map[string]string{
		"Updated 05/2016. Call +44 20 7946 0958": "+44 20 7946 0958",
		"tel 2016 - 06 4521 7788":                "2016 - 06 4521 7788",
	} {
		got := collect(text, regexSpans(text), KindPhone)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%q: phones = %q, want [%q]", text, got, want)
		}
	}
}

func TestRegexSpans_IsoAndStackedRangesAreNotPhones(t *testing.T) {
	for _, text := range []string{
		"2016-05 - 2024-06",
		"2016-05-01 - 2024-06-30",
		"2013 - 2015\n2015 - 2016",
		"2013 - 2015\n2015 - 2016\n2016 - 2019",
		"03-2021 - 05-2023",
		"2021-03 - 2023-05",
	} {
		if got := collect(text, regexSpans(text), KindPhone); len(got) != 0 {
			t.Errorf("%q: masked %q as a phone", text, got)
		}
	}
}

// 0166-23-3355 has the 4-2-4 shape of an ISO range but its "years" are not 19xx/20xx, so it
// is a phone and must still be masked.
func TestRegexSpans_HyphenatedPhoneIsNotARange(t *testing.T) {
	text := "Tel 0166-23-3355"
	got := collect(text, regexSpans(text), KindPhone)
	if len(got) != 1 || got[0] != "0166-23-3355" {
		t.Errorf("%q: phones = %q, want [0166-23-3355]", text, got)
	}
}
