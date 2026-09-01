package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// fakeDrafter answers with a canned body and records the prompts it was given.
type fakeDrafter struct {
	reply  string
	err    error
	system string
	user   string
}

func (f *fakeDrafter) GenerateJSON(_ context.Context, system, user string, _ ...llm.GenOption) (string, error) {
	f.system, f.user = system, user
	return f.reply, f.err
}

func TestDraftReturnsTheModelsSentence(t *testing.T) {
	d := &fakeDrafter{reply: `{"description":"A container orchestrator that schedules and restarts workloads across a cluster."}`}

	got, err := draft(context.Background(), d, skill{canonical: "kubernetes", label: "Kubernetes"})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	want := "A container orchestrator that schedules and restarts workloads across a cluster."
	if got != want {
		t.Errorf("draft = %q, want %q", got, want)
	}
}

// The dictionary is one row per skill, so a description carrying a newline or a tab
// would break the file it is destined for. Collapsing here rather than rejecting keeps
// a wave usable: the model wrapping a sentence is not a reason to lose it.
func TestDraftCollapsesAnAnswerIntoOneRow(t *testing.T) {
	d := &fakeDrafter{reply: "{\"description\":\"  A build tool.\\nIt also\\truns tests.  \"}"}

	got, err := draft(context.Background(), d, skill{canonical: "gradle", label: "Gradle"})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if got != "A build tool. It also runs tests." {
		t.Errorf("draft = %q, want the answer on one line", got)
	}
}

// Production hands this back: the gateway wraps the model's JSON in a string under its
// own key, so the object arrives one level deeper than it was asked for. A schema is the
// first line against that and not a proof — AGENTS.md is explicit that a gateway which
// stops honouring one still answers 200 — so the decoder unwraps it too.
func TestDraftUnwrapsADoubleEncodedAnswer(t *testing.T) {
	d := &fakeDrafter{reply: `{"answer":"{\"description\": \"A container orchestrator.\"}"}`}

	got, err := draft(context.Background(), d, skill{canonical: "kubernetes", label: "Kubernetes"})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if got != "A container orchestrator." {
		t.Errorf("draft = %q, want the description from inside the envelope", got)
	}
}

// The same gateway also nests the object WITHOUT stringifying it, and sometimes puts the
// sentence straight under its own key. All three shapes came out of one production run,
// which is why the unwrapping is shaped by evidence rather than by guesswork.
func TestDraftUnwrapsTheOtherEnvelopesProductionSent(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{"nested object", `{"answer":{"description":"Bluebeam is a PDF markup tool."}}`,
			"Bluebeam is a PDF markup tool."},
		{"sentence under the wrapper's key", `{"answer":"These algorithms analyze user behavior."}`,
			"These algorithms analyze user behavior."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fakeDrafter{reply: tc.reply}
			got, err := draft(context.Background(), d, skill{canonical: "x", label: "X"})
			if err != nil {
				t.Fatalf("draft: %v", err)
			}
			if got != tc.want {
				t.Errorf("draft = %q, want %q", got, tc.want)
			}
		})
	}
}

// One level, not any number. A wrapper around a wrapper is a gateway doing something
// this has not seen, and guessing deeper would turn an unknown shape into a plausible
// sentence rather than an error the operator can read.
func TestDraftDoesNotChaseNestedEnvelopesForever(t *testing.T) {
	d := &fakeDrafter{reply: `{"a":"{\"b\": \"{\\\"description\\\": \\\"Too deep.\\\"}\"}"}`}

	if _, err := draft(context.Background(), d, skill{canonical: "kubernetes", label: "Kubernetes"}); err == nil {
		t.Error("draft = nil error, want one")
	}
}

func TestDraftRejectsAnUnusableAnswer(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{"empty description", `{"description":""}`},
		{"whitespace only", `{"description":"   "}`},
		{"missing field", `{"text":"A container orchestrator."}`},
		{"not json", `A container orchestrator.`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &fakeDrafter{reply: tc.reply}
			if _, err := draft(context.Background(), d, skill{canonical: "kubernetes", label: "Kubernetes"}); err == nil {
				t.Errorf("draft(%q) = nil error, want one", tc.reply)
			}
		})
	}
}

func TestDraftPropagatesTheModelsError(t *testing.T) {
	d := &fakeDrafter{err: errors.New("gateway refused")}
	if _, err := draft(context.Background(), d, skill{canonical: "kubernetes", label: "Kubernetes"}); err == nil {
		t.Error("draft = nil error, want the model's")
	}
}

// The slug alone is a poor prompt: "1c" and "as400" are unrecognisable without the
// label and the spellings the parser accepts. Dropping either from the prompt is the
// regression this catches.
func TestDraftPromptsWithTheLabelAndAliases(t *testing.T) {
	d := &fakeDrafter{reply: `{"description":"An enterprise Java build tool."}`}
	s := skill{canonical: "k8s-thing", label: "Kubernetes", aliases: []string{"k8s", "kube"}}

	if _, err := draft(context.Background(), d, s); err != nil {
		t.Fatalf("draft: %v", err)
	}
	for _, want := range []string{"k8s-thing", "Kubernetes", "k8s", "kube"} {
		if !strings.Contains(d.user, want) {
			t.Errorf("prompt %q is missing %q", d.user, want)
		}
	}
}
