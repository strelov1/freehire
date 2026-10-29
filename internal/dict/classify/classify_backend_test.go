package classify

import "testing"

// In US plants, stores and care homes the "back end" is a shift (the second half of a
// compressed work week: "Back End Days", "Back End Nights") or an area of the site, not
// server-side software. Every title below is an open posting measured on prod
// (2026-10-29) that the bare "back end" alias filed as backend.
func TestParse_NonSoftwareBackEnd(t *testing.T) {
	cases := []struct{ title, want string }{
		// The shift names no category: the rest of the title decides.
		{"Direct Support Professional Back End (Night)-Entry Level", "social_services"},
		{"Direct Support Professional-Wed-Sat. Back End Day PM-Entry Level", "social_services"},
		{"Direct Support Professional -Wed-Sat Back End Day PM-Entry Level", "social_services"},
		{"Registered Nurse (RN) - Back End Days", "healthcare"},
		{"Behavioral Health Specialist (FT, Back End Days)", "healthcare"},
		{"Lab Technician 3 (Chemistry) Back End Days", "science"},
		{"Machine Operator - Back End Relief - 12 hour nights (6pm-6:30am)", "manufacturing"},
		{"Optical Fabrication Technician CWW Back End Days", ""},
		{"Manufacturing Technician (Nights - Back End)", "manufacturing"},
		{"Manufacturing Equipment Technician (MTE) - Days Backend Shift - Shift 7", ""},
		{"Integration Test Technician (Backend Day -Wed. - Sat.)", ""},
		{"Back End Relief | Days (6A – 6P) | 4 on 4 off | Starting Pay $23.49 Per Hour", ""},

		// The site's back end, named by the trade working it.
		{"Back End Operator", "manufacturing"},
		{"Back End Mechanic", "skilled_trades"},
		{"Back End Maintainer (Production Mechanic)", "skilled_trades"},
		{"Back End Maintainer - $34.09/hr*", "skilled_trades"},
		{"Back-End Estimator - No Weekends!", "skilled_trades"},
		{"Back-End Collision Estimator - No Weekends", "skilled_trades"},
		{"Backend Retail Assistant (Part-time)", "retail"},

		// Software keeps its reading.
		{"Backend Engineer", "backend"},
		{"Back-End Developer", "backend"},
		{"Entry Level Back End Developer", "backend"},
		{"Backend/API Engineer", "backend"},
		{"Jr.Java Back End Developer", "backend"},
		{"Back End (Django)", "backend"},
		{"Backend Team Lead", "backend"},
		{"Especialista BackEnd", "backend"},
		{"Back-end разработчик в Сервис Руки (удаленно)", "backend"},
		// A bare "associate" is no category of its own (the generic-noun rule), so
		// nothing displaces the backend reading.
		{"Back-end associate", "backend"},
		{"Backend Developer - Night Shift", "backend"},
	}
	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.want {
			t.Errorf("Parse(%q).Category = %q, want %q", tc.title, got, tc.want)
		}
	}
}
