package worker

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EnvInt64 reads a positive tuning knob from the environment. An unset or empty value
// takes fallback; a SET but unparseable or non-positive one is an ERROR, not a
// fallback.
//
// That asymmetry is the whole point, and it is the same reasoning HYDRATION_RETRY_DAYS
// uses. These knobs bound a one-off pass over the catalogue, so a typo that quietly
// falls back does not look like a typo — it looks like a normal run. A mistyped
// BACKFILL_REQUIREMENTS_FROM_ID silently re-walks the whole table; a mistyped
// BACKFILL_CLEARANCE_MAX silently removes the ceiling the operator asked for. Failing
// in the first second of the run costs an operator one line of output; the alternative
// costs hours and is not visible anywhere.
//
// This is deliberately NOT the rule everywhere. internal/platform/config's env helpers
// log and fall back because a server must boot; a knob whose zero is a real value
// (a pause of 0) cannot use this, and neither can a reader with nowhere to return an
// error to.
func EnvInt64(name string, fallback int64) (int64, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s=%q: want a positive integer", name, raw)
	}
	return v, nil
}
