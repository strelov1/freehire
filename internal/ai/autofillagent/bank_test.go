package autofillagent_test

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ai/autofillagent"
	"github.com/strelov1/freehire/internal/dict/answertopic"
)

// banked keys answers the way the answer bank stores them: by topic.
func banked(t *testing.T, qa map[string]string) autofillagent.Banked {
	t.Helper()
	out := autofillagent.Banked{}
	for question, answer := range qa {
		topic, ok := answertopic.Of(question)
		if !ok {
			t.Fatalf("question %q does not key", question)
		}
		out[topic] = answer
	}
	return out
}

// The candidate answered this on an earlier form and saved it. The planner never sees the
// bank, so it names nothing here — the recall is what fills it.
func TestRunFillsATypedQuestionFromTheBank(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{{Label: "Why Anthropic?*", Type: "textarea"}}}
	planner := plannerFunc(func([]autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error) {
		return nil, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile(),
		banked(t, map[string]string{"Why Anthropic?": "I like it"}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(rep.Filled, "Why Anthropic?*") {
		t.Fatalf("filled = %v, want the banked question", rep.Filled)
	}
	if len(tools.requested) != 1 || tools.requested[0].Value != "I like it" {
		t.Fatalf("requested = %v, want the banked answer", tools.requested)
	}
}

// A widget is driven with the banked answer as the choice, and only when the widget
// offers exactly that option — the same "no nearest match" rule the planner's choices obey.
func TestRunDrivesAWidgetWithTheBankedAnswer(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{
			{Label: "Have you ever interviewed at Anthropic before?*", Type: "text", Combo: true},
			{Label: "Gender", Type: "text", Combo: true},
		},
		offers: map[string][]string{
			"Have you ever interviewed at Anthropic before?*": {"Yes", "No"},
			"Gender": {"Female", "Non-binary"},
		},
	}
	planner := &widgetPlanner{}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile(), banked(t, map[string]string{
		"Have you ever interviewed at Anthropic before?": "no",
		"Gender": "Male",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := tools.selected["Have you ever interviewed at Anthropic before?*"]; got != "No" {
		t.Fatalf("selected %q, want the widget's own spelling of the banked answer", got)
	}
	if !contains(rep.Filled, "Have you ever interviewed at Anthropic before?*") {
		t.Fatalf("filled = %v, want the banked widget", rep.Filled)
	}
	if _, picked := tools.selected["Gender"]; picked || !contains(rep.Unmapped, "Gender") {
		t.Fatalf("a banked answer the widget does not offer was committed or misreported: %+v", rep)
	}
}

// A work-authorization answer depends on THIS posting's country, so the bank never answers
// it, whatever it holds.
func TestRunNeverRecallsWorkAuthorization(t *testing.T) {
	question := "Are you authorized to work in the country in which this position is located?"
	tools := &fakeTools{fields: []autofillagent.Field{{Label: question, Type: "text"}}}
	planner := plannerFunc(func([]autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error) {
		return nil, nil
	})

	if _, err := autofillagent.Run(context.Background(), tools, planner, profile(),
		banked(t, map[string]string{question: "Yes"})); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.requested) != 0 {
		t.Fatalf("requested = %v, want nothing recalled for a work-authorization question", tools.requested)
	}
}

// The profile's typed facts outrank the bank's free text: a question the planner answered
// from the profile is not overwritten by a banked answer to the same words.
func TestRunPrefersThePlanOverTheBank(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{{Label: "Location", Type: "text"}}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{{Label: "Location", Value: p["location"]}}, nil
	})

	if _, err := autofillagent.Run(context.Background(), tools, planner, profile(),
		banked(t, map[string]string{"Location": "Lisbon"})); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.requested) != 1 || tools.requested[0].Value != "Berlin, Germany" {
		t.Fatalf("requested = %v, want the profile's location only", tools.requested)
	}
}

// The model is never shown the bank: it would carry every answer the candidate ever gave,
// demographics included, into every prompt.
func TestRunKeepsTheBankOutOfThePlannersProfile(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{{Label: "Gender", Type: "text"}}}
	var seen autofillagent.Profile
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		seen = p
		return nil, nil
	})

	if _, err := autofillagent.Run(context.Background(), tools, planner, profile(),
		banked(t, map[string]string{"Gender": "Male"})); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for k, v := range seen {
		if v == "Male" {
			t.Fatalf("the planner saw a banked answer under %q", k)
		}
	}
}
