package flexjson

import (
	"encoding/json"
	"testing"
)

func TestInt_NumberOrString(t *testing.T) {
	cases := map[string]int{
		`85`:      85,
		`85.0`:    85,
		`84.6`:    85, // rounds
		`"85"`:    85,
		`"85%"`:   85,
		`"8/10"`:  8,
		`"-3"`:    -3,
		`""`:      0,
		`"n/a"`:   0,
		`null`:    0,
		`"  12 "`: 12,
	}
	for raw, want := range cases {
		var v Int
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Errorf("Int %s: unexpected error %v", raw, err)
			continue
		}
		if int(v) != want {
			t.Errorf("Int %s = %d, want %d", raw, int(v), want)
		}
	}
}

func TestInt64_NumberOrString(t *testing.T) {
	cases := map[string]int64{
		`42`:     42,
		`"42"`:   42,
		`"none"`: 0,
		`""`:     0,
		`null`:   0,
	}
	for raw, want := range cases {
		var v Int64
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Errorf("Int64 %s: unexpected error %v", raw, err)
			continue
		}
		if int64(v) != want {
			t.Errorf("Int64 %s = %d, want %d", raw, int64(v), want)
		}
	}
}

func TestFloat_NumberOrString(t *testing.T) {
	cases := map[string]float64{
		`0.8`:      0.8,
		`"0.8"`:    0.8,
		`1`:        1,
		`"0.85 "`:  0.85,
		`""`:       0,
		`"high"`:   0,
		`null`:     0,
		`"85%"`:    85,
		`"0.9 ok"`: 0.9,
	}
	for raw, want := range cases {
		var v Float
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Errorf("Float %s: unexpected error %v", raw, err)
			continue
		}
		if float64(v) != want {
			t.Errorf("Float %s = %v, want %v", raw, float64(v), want)
		}
	}
}

func TestBool_BoolStringOrNumber(t *testing.T) {
	cases := map[string]bool{
		`true`:    true,
		`false`:   false,
		`"true"`:  true,
		`"yes"`:   true,
		`"1"`:     true,
		`"Y"`:     true,
		`1`:       true,
		`0`:       false,
		`"false"`: false,
		`"no"`:    false,
		`""`:      false,
		`null`:    false,
	}
	for raw, want := range cases {
		var v Bool
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Errorf("Bool %s: unexpected error %v", raw, err)
			continue
		}
		if bool(v) != want {
			t.Errorf("Bool %s = %v, want %v", raw, bool(v), want)
		}
	}
}
