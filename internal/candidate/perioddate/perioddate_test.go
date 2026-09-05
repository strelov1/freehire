package perioddate

import (
	"encoding/json"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want *PeriodDate
		ok   bool
	}{
		{"2024", &PeriodDate{Year: 2024}, true},
		{"2023-09", &PeriodDate{Year: 2023, Month: 9}, true},
		{"2023/09", &PeriodDate{Year: 2023, Month: 9}, true},
		{"Jan 2018", &PeriodDate{Year: 2018, Month: 1}, true},
		{"January 2018", &PeriodDate{Year: 2018, Month: 1}, true},
		{"Oct 2018", &PeriodDate{Year: 2018, Month: 10}, true},
		{"October 2018", &PeriodDate{Year: 2018, Month: 10}, true},
		{"May 2018", &PeriodDate{Year: 2018, Month: 5}, true},
		{"Jun 2017", &PeriodDate{Year: 2017, Month: 6}, true},
		{"Present", nil, false},
		{"", nil, false},
		{"sometime", nil, false},
	}
	for _, tc := range cases {
		got, ok := Parse(tc.in)
		if ok != tc.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if *got != *tc.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		in   *PeriodDate
		want string
	}{
		{nil, ""},
		{&PeriodDate{Year: 2018}, "2018"},
		{&PeriodDate{Year: 2018, Month: 3}, "Mar 2018"},
		{&PeriodDate{Year: 0}, ""}, // Year<=0 is treated as absent (see UnmarshalJSON's failure path)
	}
	for _, tc := range cases {
		if got := tc.in.Format(); got != tc.want {
			t.Errorf("Format(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatRange(t *testing.T) {
	cases := []struct {
		start, end *PeriodDate
		current    bool
		want       string
	}{
		{&PeriodDate{Year: 2018, Month: 3}, &PeriodDate{Year: 2021}, false, "Mar 2018 – 2021"},
		{&PeriodDate{Year: 2018, Month: 10}, nil, true, "Oct 2018 – Present"},
		{&PeriodDate{Year: 2018}, nil, false, "2018"},
		{nil, nil, false, ""},
	}
	for _, tc := range cases {
		if got := FormatRange(tc.start, tc.end, tc.current); got != tc.want {
			t.Errorf("FormatRange(%+v, %+v, %v) = %q, want %q", tc.start, tc.end, tc.current, got, tc.want)
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct {
		name string
		in   *PeriodDate
		want *PeriodDate
	}{
		{"nil stays nil", nil, nil},
		{"in range unchanged", &PeriodDate{Year: 2020, Month: 6}, &PeriodDate{Year: 2020, Month: 6}},
		{"year too low clears the whole date", &PeriodDate{Year: 1899, Month: 6}, nil},
		{"year too high clears the whole date", &PeriodDate{Year: 2101}, nil},
		{"month too high coerces to year-only", &PeriodDate{Year: 2020, Month: 13}, &PeriodDate{Year: 2020}},
		{"negative month coerces to year-only", &PeriodDate{Year: 2020, Month: -1}, &PeriodDate{Year: 2020}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sanitize(tc.in)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("Sanitize(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("Sanitize(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	cases := []struct {
		in   PeriodDate
		want string
	}{
		{PeriodDate{Year: 2018, Month: 3}, `{"year":2018,"month":3}`},
		{PeriodDate{Year: 2018}, `{"year":2018}`},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.in)
		if err != nil {
			t.Fatalf("Marshal(%+v): %v", tc.in, err)
		}
		if string(b) != tc.want {
			t.Errorf("Marshal(%+v) = %s, want %s", tc.in, b, tc.want)
		}
	}
}

func TestUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want PeriodDate
	}{
		{"new object shape, month present", `{"year":2018,"month":3}`, PeriodDate{Year: 2018, Month: 3}},
		{"new object shape, year only", `{"year":2018}`, PeriodDate{Year: 2018}},
		{"legacy string, year-month", `"2023-09"`, PeriodDate{Year: 2023, Month: 9}},
		{"legacy string, month-name year", `"October 2018"`, PeriodDate{Year: 2018, Month: 10}},
		{"legacy string, bare year", `"2024"`, PeriodDate{Year: 2024}},
		{"legacy string, unparseable garbage decodes to zero value, no error", `"sometime"`, PeriodDate{}},
		{"legacy string, Present decodes to zero value, no error", `"Present"`, PeriodDate{}},
		{"bare number defensive shim (model returns a raw year)", `2019`, PeriodDate{Year: 2019}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got PeriodDate
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Unmarshal(%s) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestUnmarshalJSON_RoundTripsThroughMarshal(t *testing.T) {
	original := PeriodDate{Year: 2018, Month: 3}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got PeriodDate
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != original {
		t.Errorf("round trip = %+v, want %+v", got, original)
	}
}

func TestIsPresentLabel(t *testing.T) {
	for _, s := range []string{"present", "Present", " Current ", "now", "ongoing", "Today"} {
		if !IsPresentLabel(s) {
			t.Errorf("IsPresentLabel(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"2024", "", "sometime"} {
		if IsPresentLabel(s) {
			t.Errorf("IsPresentLabel(%q) = true, want false", s)
		}
	}
}
