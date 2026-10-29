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
	}))
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
			_, err := RateTiersFromEnv(envOf(map[string]string{name: value}))
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("err = %v, want a refusal naming %s", err, name)
			}
		})
	}
}
