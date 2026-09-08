package worker

import "testing"

func TestEnvInt64TakesTheFallbackWhenUnsetOrBlank(t *testing.T) {
	if got, err := EnvInt64("FREEHIRE_TEST_KNOB", 500); err != nil || got != 500 {
		t.Fatalf("unset: got %d, %v; want 500, nil", got, err)
	}
	t.Setenv("FREEHIRE_TEST_KNOB", "   ")
	if got, err := EnvInt64("FREEHIRE_TEST_KNOB", 500); err != nil || got != 500 {
		t.Fatalf("blank: got %d, %v; want 500, nil", got, err)
	}
}

func TestEnvInt64ReadsASetValue(t *testing.T) {
	t.Setenv("FREEHIRE_TEST_KNOB", " 42 ")
	if got, err := EnvInt64("FREEHIRE_TEST_KNOB", 500); err != nil || got != 42 {
		t.Fatalf("got %d, %v; want 42, nil", got, err)
	}
}

// A typo must stop the run. Falling back would produce a pass that ran with a bound the
// operator never chose and that is printed nowhere — indistinguishable from a normal run.
func TestEnvInt64RefusesAValueItCannotRead(t *testing.T) {
	for _, raw := range []string{"5OO", "abc", "1.5", "0", "-1"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("FREEHIRE_TEST_KNOB", raw)
			if _, err := EnvInt64("FREEHIRE_TEST_KNOB", 500); err == nil {
				t.Fatalf("EnvInt64(%q) returned no error, want the run to fail rather than fall back", raw)
			}
		})
	}
}

func TestEnvInt32TakesTheFallbackWhenUnsetOrBlank(t *testing.T) {
	if got, err := EnvInt32("FREEHIRE_TEST_KNOB", 30); err != nil || got != 30 {
		t.Fatalf("unset: got %d, %v; want 30, nil", got, err)
	}
	t.Setenv("FREEHIRE_TEST_KNOB", "  ")
	if got, err := EnvInt32("FREEHIRE_TEST_KNOB", 30); err != nil || got != 30 {
		t.Fatalf("blank: got %d, %v; want 30, nil", got, err)
	}
}

func TestEnvInt32ReadsASetValue(t *testing.T) {
	t.Setenv("FREEHIRE_TEST_KNOB", " 60 ")
	if got, err := EnvInt32("FREEHIRE_TEST_KNOB", 30); err != nil || got != 60 {
		t.Fatalf("got %d, %v; want 60, nil", got, err)
	}
}

func TestEnvInt32RefusesAValueItCannotRead(t *testing.T) {
	for _, raw := range []string{"5OO", "abc", "1.5", "0", "-1"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("FREEHIRE_TEST_KNOB", raw)
			if _, err := EnvInt32("FREEHIRE_TEST_KNOB", 30); err == nil {
				t.Fatalf("EnvInt32(%q) returned no error, want the run to fail rather than fall back", raw)
			}
		})
	}
}

// The CodeQL finding this guards against: a value parsed as int64 that overflows int32 must
// not be silently truncated (e.g. a typo like "30000000000" is not "3 billion days from now",
// it's whatever garbage two's-complement wraparound produces) — it must fail the same way an
// unparseable value does.
func TestEnvInt32RefusesAValueThatOverflowsInt32(t *testing.T) {
	t.Setenv("FREEHIRE_TEST_KNOB", "3000000000") // > math.MaxInt32 (2147483647)
	if _, err := EnvInt32("FREEHIRE_TEST_KNOB", 30); err == nil {
		t.Fatal("EnvInt32 accepted a value beyond int32 range, want an error")
	}
}
