package handler

import (
	"strings"
	"testing"
)

func envOf(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestRateTiersFromEnv_Overrides(t *testing.T) {
	got, err := RateTiersFromEnv(envOf(map[string]string{
		"API_RATE_PRO_PER_MINUTE":    "900",
		"API_RATE_PRO_PER_DAY":       " 40000 ",
		"API_RATE_FREE_PER_DAY":      "0",
		"API_RATE_DATA_PER_MINUTE":   "7000",
		"API_RATE_UNKNOWN_PER_DAY":   "1",
		"API_RATE_ANONYMOUS_PER_DAY": "",
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	def := DefaultRateTiers()
	if got[ratePro] != (rateLimit{PerMinute: 900, PerDay: 40000}) {
		t.Errorf("pro = %+v", got[ratePro])
	}
	if got[rateFree] != (rateLimit{PerMinute: def[rateFree].PerMinute}) {
		t.Errorf("free = %+v, want the daily cap lifted and the minute kept", got[rateFree])
	}
	if got[rateData].PerMinute != 7000 || got[rateAnonymous] != def[rateAnonymous] || got[rateUltra] != def[rateUltra] {
		t.Errorf("tiers = %+v", got)
	}
}

// A value that cannot be read fails the boot and names the variable: falling back would
// enforce a figure the operator did not set, with nothing saying so.
func TestRateTiersFromEnv_RefusesWhatItCannotRead(t *testing.T) {
	for name, value := range map[string]string{
		"API_RATE_PRO_PER_MINUTE":    "0",
		"API_RATE_FREE_PER_MINUTE":   "-5",
		"API_RATE_ULTRA_PER_MINUTE":  "1k",
		"API_RATE_API_PER_DAY":       "-1",
		"API_RATE_ANONYMOUS_PER_DAY": "lots",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RateTiersFromEnv(envOf(map[string]string{name: value}), true)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("err = %v, want a refusal naming %s", err, name)
			}
		})
	}
}

// A ladder that makes a key worse than no key fails the boot, measured against the anonymous
// mode the server will run in.
func TestRateTiersFromEnv_RefusesAKeyWorseThanNone(t *testing.T) {
	cases := []struct {
		name   string
		vars   map[string]string
		strict bool
		ok     bool
	}{
		{"a dropped zero", map[string]string{"API_RATE_FREE_PER_MINUTE": "60"}, false, false},
		{"below the agent budget before strict", map[string]string{"API_RATE_PRO_PER_MINUTE": "299"}, false, false},
		{"below the read budget before strict", map[string]string{"API_RATE_PRO_PER_MINUTE": "500"}, false, false},
		{"fine under strict", map[string]string{"API_RATE_FREE_PER_MINUTE": "200"}, true, true},
		{"below the anonymous minute under strict", map[string]string{"API_RATE_FREE_PER_MINUTE": "100"}, true, false},
		{"below the anonymous day under strict", map[string]string{"API_RATE_ULTRA_PER_DAY": "1000"}, true, false},
		{"a low day is not in force before strict", map[string]string{"API_RATE_ULTRA_PER_DAY": "1000"}, false, true},
		{"no daily cap is never worse", map[string]string{"API_RATE_FREE_PER_DAY": "0", "API_RATE_PRO_PER_DAY": "0", "API_RATE_ULTRA_PER_DAY": "0"}, true, true},
		{"an upgrade that shrinks the day", map[string]string{"API_RATE_FREE_PER_DAY": "0"}, true, false},
		{"an upgrade that shrinks the minute", map[string]string{"API_RATE_ULTRA_PER_MINUTE": "700"}, true, false},
		{"a capped key beside an uncapped address", map[string]string{"API_RATE_ANONYMOUS_PER_DAY": "0"}, true, false},
		{"an uncapped ladder beside an uncapped address", map[string]string{"API_RATE_ANONYMOUS_PER_DAY": "0", "API_RATE_FREE_PER_DAY": "0", "API_RATE_PRO_PER_DAY": "0", "API_RATE_ULTRA_PER_DAY": "0"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RateTiersFromEnv(envOf(tc.vars), tc.strict)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if err != nil && !strings.Contains(err.Error(), "worse than none") {
				t.Fatalf("err = %v, want it to say why", err)
			}
		})
	}
	if _, err := RateTiersFromEnv(envOf(nil), false); err != nil {
		t.Fatalf("the shipped ladder before strict: %v", err)
	}
	if _, err := RateTiersFromEnv(envOf(nil), true); err != nil {
		t.Fatalf("the shipped ladder under strict: %v", err)
	}
}
