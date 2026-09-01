package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// scriptedDrafter answers per skill slug, so a test can make one skill fail while its
// neighbours succeed. Concurrent by construction — printDrafts runs several at once.
type scriptedDrafter struct {
	replies map[string]string
	fails   map[string]bool
}

func (s *scriptedDrafter) GenerateJSON(_ context.Context, _, user string, _ ...llm.GenOption) (string, error) {
	for slug, reply := range s.replies {
		if strings.Contains(user, "slug: "+slug+"\n") {
			if s.fails[slug] {
				return "", errors.New("gateway refused")
			}
			return reply, nil
		}
	}
	return "", errors.New("unscripted skill")
}

func TestPrintDraftsEmitsRowsInWaveOrder(t *testing.T) {
	d := &scriptedDrafter{replies: map[string]string{
		"go":         `{"description":"A compiled language."}`,
		"kubernetes": `{"description":"A container orchestrator."}`,
	}}
	var out strings.Builder

	err := printDrafts(context.Background(), d, []string{"kubernetes", "go"},
		map[string]int{"go": 9000, "kubernetes": 400}, 4, &out)
	if err != nil {
		t.Fatalf("printDrafts: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "kubernetes\tA container orchestrator.\n") {
		t.Errorf("output %q is missing the kubernetes row", got)
	}
	if strings.Index(got, "kubernetes\t") > strings.Index(got, "go\t") {
		t.Errorf("output %q is not in wave order", got)
	}
}

// A wave is dozens of independent calls. Losing the ones that worked to the one that
// did not would mean paying for them twice.
func TestPrintDraftsKeepsTheSurvivorsOfAPartialFailure(t *testing.T) {
	d := &scriptedDrafter{
		replies: map[string]string{
			"go":         `{"description":"A compiled language."}`,
			"kubernetes": `{"description":"A container orchestrator."}`,
		},
		fails: map[string]bool{"kubernetes": true},
	}
	var out strings.Builder

	err := printDrafts(context.Background(), d, []string{"kubernetes", "go"}, nil, 4, &out)
	if err != nil {
		t.Fatalf("printDrafts: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "go\tA compiled language.\n") {
		t.Errorf("output %q lost the draft that succeeded", got)
	}
	if strings.Contains(got, "kubernetes\t") {
		t.Errorf("output %q emitted a row for the skill that failed", got)
	}
}

// Nothing survived means the gateway, the credential or the prompt is broken, and a
// silent exit 0 would read as "the wave was already described".
func TestPrintDraftsFailsWhenEveryDraftFailed(t *testing.T) {
	d := &scriptedDrafter{
		replies: map[string]string{"go": `{"description":"A compiled language."}`},
		fails:   map[string]bool{"go": true},
	}
	var out strings.Builder

	if err := printDrafts(context.Background(), d, []string{"go"}, nil, 4, &out); err == nil {
		t.Error("printDrafts = nil error, want one")
	}
	if out.String() != "" {
		t.Errorf("output = %q, want nothing", out.String())
	}
}
