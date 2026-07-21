package hardconstraint

import "testing"

func TestDegreeRankOrders(t *testing.T) {
	// The ladder must be strictly increasing none < ged < associate < bachelor < master < phd.
	order := []string{"none", "ged", "associate", "bachelor", "master", "phd"}
	prev := -1
	for _, name := range order {
		rank, ok := degreeRank(name)
		if !ok {
			t.Fatalf("degreeRank(%q) not ok", name)
		}
		if rank <= prev {
			t.Errorf("degreeRank(%q) = %d not greater than previous %d", name, rank, prev)
		}
		prev = rank
	}
}

func TestDegreeRankEquivalences(t *testing.T) {
	cases := map[string]string{
		"Bachelor of Science": "bachelor",
		"bachelor's degree":   "bachelor",
		"BSc":                 "bachelor",
		"B.A.":                "bachelor",
		"Master of Science":   "master",
		"MBA":                 "master",
		"master's":            "master",
		"PhD":                 "phd",
		"Ph.D.":               "phd",
		"Doctorate":           "phd",
		"Associate degree":    "associate",
		"High School Diploma": "ged",
	}
	for input, equiv := range cases {
		got, ok := degreeRank(input)
		if !ok {
			t.Errorf("degreeRank(%q) not ok", input)
			continue
		}
		want, _ := degreeRank(equiv)
		if got != want {
			t.Errorf("degreeRank(%q) = %d; want rank of %q = %d", input, got, equiv, want)
		}
	}
}

func TestDegreeRankUnknown(t *testing.T) {
	if _, ok := degreeRank("certificate of attendance"); ok {
		t.Error("degreeRank(unknown) ok = true; want false")
	}
	if _, ok := degreeRank(""); ok {
		t.Error("degreeRank(empty) ok = true; want false")
	}
}
