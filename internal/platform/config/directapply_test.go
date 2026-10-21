package config

import "testing"

func TestLoadDirectApplyDailyLimit(t *testing.T) {
	t.Setenv("DIRECT_APPLY_DAILY_LIMIT", "")
	if n, err := LoadDirectApplyDailyLimit(); err != nil || n != 30 {
		t.Errorf("unset = %d, %v; want the default 30", n, err)
	}
	t.Setenv("DIRECT_APPLY_DAILY_LIMIT", " 5 ")
	if n, err := LoadDirectApplyDailyLimit(); err != nil || n != 5 {
		t.Errorf("5 = %d, %v", n, err)
	}
	for _, bad := range []string{"many", "0", "-3", "2.5"} {
		t.Setenv("DIRECT_APPLY_DAILY_LIMIT", bad)
		if _, err := LoadDirectApplyDailyLimit(); err == nil {
			t.Errorf("%q: want an error, a typo must not look like a tuned cap", bad)
		}
	}
}
