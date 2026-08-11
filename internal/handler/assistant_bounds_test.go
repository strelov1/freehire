package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/assistant"
)

func TestTurnBoundsRaisesTailorCeilings(t *testing.T) {
	chat := turnBounds(assistant.Session{Preset: assistant.PresetChat}, "hello")
	if chat.MaxSteps != 0 {
		t.Errorf("chat MaxSteps = %d, want 0 (runner default)", chat.MaxSteps)
	}

	tailor := turnBounds(assistant.Session{Preset: assistant.PresetTailor}, "add Salesforce")
	if tailor.MaxSteps != tailorMaxSteps {
		t.Errorf("interactive tailor MaxSteps = %d, want %d", tailor.MaxSteps, tailorMaxSteps)
	}

	auto := turnBounds(assistant.Session{Preset: assistant.PresetTailor}, autopilotBrief)
	if auto.MaxSteps != autopilotMaxSteps {
		t.Errorf("autopilot retry MaxSteps = %d, want %d", auto.MaxSteps, autopilotMaxSteps)
	}
}
