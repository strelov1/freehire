package location

import "testing"

func TestNormalizeCountry(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"alpha-2 lowercase", "ae", "ae"},
		{"alpha-2 uppercase (Lever/Workday style)", "AE", "ae"},
		{"alpha-3 uppercase (Ashby style)", "USA", "us"},
		{"alpha-3 lowercase", "deu", "de"},
		{"padded", "  gb  ", "gb"},
		{"empty", "", ""},
		{"country outside the curated region set", "AFG", ""},
		{"garbage", "not-a-code", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeCountry(tc.in); got != tc.want {
				t.Errorf("NormalizeCountry(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
