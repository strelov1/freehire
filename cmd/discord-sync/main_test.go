package main

import "testing"

func TestMaxPerRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  bool
		raw  string
		want int32
	}{
		{name: "unset", want: maxPerRunDefault},
		{name: "a number", set: true, raw: "25", want: 25},
		// A recurring worker falls back rather than failing, the way cmd/billing-sync does:
		// a typo must not silently stop reconciling everybody's role for as long as nobody
		// reads systemctl. The line in the log names the value so it is findable.
		{name: "not a number", set: true, raw: "lots", want: maxPerRunDefault},
		{name: "zero", set: true, raw: "0", want: maxPerRunDefault},
		{name: "negative", set: true, raw: "-5", want: maxPerRunDefault},
		{name: "empty", set: true, raw: "", want: maxPerRunDefault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("DISCORD_SYNC_MAX_PER_RUN", tc.raw)
			}
			if got := maxPerRun(); got != tc.want {
				t.Errorf("maxPerRun() = %d, want %d", got, tc.want)
			}
		})
	}
}
