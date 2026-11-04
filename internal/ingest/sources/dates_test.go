package sources

import (
	"testing"
	"time"
)

// A future date from any adapter that parses its own layout must read as undated, not freshest.
func TestAdapterDatesDropFuture(t *testing.T) {
	f := time.Now().AddDate(0, 1, 0)
	cases := map[string]*time.Time{
		"arbeitsagentur": arbeitsagenturDate(f.Format("2006-01-02")),
		"catho":          cathoDate(f.Format("2006-01-02")),
		"dassault":       dassaultDate(f.Format("2006/01/02 15:04:05")),
		"peoplesoft":     peoplesoftDate(f.Format("1/2/2006")),
		"tencent":        tencentDate(f.Format("January 2,2006")),
	}
	for name, got := range cases {
		if got != nil {
			t.Errorf("%s: future date = %v, want nil", name, got)
		}
	}
	if arbeitsagenturDate("2020-01-02") == nil {
		t.Error("past date must still parse")
	}
}
