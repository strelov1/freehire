package worker

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// EnvInt64 reads a positive tuning knob from the environment. An unset or empty value
// takes fallback; a SET but unparseable or non-positive one is an ERROR, not a
// fallback.
//
// These knobs bound a one-off pass over the catalogue, so a typo that quietly falls back
// looks like a normal run (a mistyped BACKFILL_REQUIREMENTS_FROM_ID silently re-walks the
// whole table). Failing at start costs one line of output.
//
// This is deliberately NOT the rule everywhere: internal/platform/config's env helpers
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

// EnvInt32 is EnvInt64 narrowed to int32, for a knob a caller must hand to an API that only
// takes int32 (e.g. pgtype.Interval.Days). Parsing with ParseInt's bitSize=32 makes an
// out-of-range value a parse error; int32(EnvInt64(...)) would silently truncate it.
func EnvInt32(name string, fallback int32) (int32, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s=%q: want a positive integer", name, raw)
	}
	return int32(v), nil
}

// EnvInt32OrDefault is EnvInt32 for a recurring unattended worker's per-run bound: a set but
// unreadable value (not a positive number that fits an int32) logs and keeps fallback instead
// of failing, because failing hard would stop the whole job over a batch-size typo. One-off
// passes an operator watches use EnvInt32 and fail.
func EnvInt32OrDefault(name string, fallback int32) int32 {
	v, err := EnvInt32(name, fallback)
	if err != nil {
		log.Printf("%v — keeping %d", err, fallback)
		return fallback
	}
	return v
}
