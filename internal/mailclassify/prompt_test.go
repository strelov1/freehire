package mailclassify

import (
	"strings"
	"testing"
)

// The prompt and the vocabulary are two halves of one contract. A signal added to
// validSignals but never described to the model is one the model can never
// produce: the vocabulary silently narrows, and nothing fails. A signal described
// but not valid is worse — Sanitize coerces it to `other`, so the model is being
// asked for an answer that is thrown away.
func TestSystemPromptDescribesEveryValidSignal(t *testing.T) {
	for signal := range validSignals {
		if !strings.Contains(systemPrompt, string(signal)+":") {
			t.Errorf("signal %q is valid but the prompt never describes it — the model cannot produce it", signal)
		}
	}
}

// The reverse direction: every signal the prompt offers must survive Sanitize.
func TestSystemPromptOffersNoUnknownSignal(t *testing.T) {
	for _, line := range strings.Split(systemPrompt, "\n") {
		line = strings.TrimSpace(line)
		name, _, found := strings.Cut(strings.TrimPrefix(line, "- "), ":")
		if !found || !strings.HasPrefix(line, "- ") {
			continue
		}
		// Only the signal list uses the "- name: description" shape; anything else
		// with a colon (a bullet of guidance) is not a signal name.
		if strings.Contains(name, " ") {
			continue
		}
		if !validSignals[StatusSignal(name)] {
			t.Errorf("the prompt offers %q, which Sanitize would coerce to `other`", name)
		}
	}
}
