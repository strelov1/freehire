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

// The body is bounded and the headers were not, which is a bound on the wrong half: an
// address, a display name and a subject all arrive from whoever mailed the candidate, and
// none of them is capped anywhere upstream. A message with a megabyte of Subject is a
// megabyte of prompt — paid for per token, on every classification of that thread.
//
// The figure need not be exact. What must hold is that no field a sender controls reaches
// the prompt at its own length.
func TestUserPromptBoundsEveryFieldTheSenderControls(t *testing.T) {
	huge := strings.Repeat("s", 50_000)
	got := userPrompt(Input{FromName: huge, Subject: huge, Body: huge})

	if len(got) > 4*maxBodyRunes {
		t.Errorf("prompt is %d bytes from three %d-byte fields; nothing bounded the headers", len(got), len(huge))
	}
	if strings.Contains(got, huge) {
		t.Error("a sender-controlled field reached the prompt at its own length")
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
