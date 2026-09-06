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
