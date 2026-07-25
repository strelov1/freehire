package autofillagent

import (
	"context"
	"testing"
)

func TestParsePlanReadsTheModelsFills(t *testing.T) {
	plan, err := parsePlan(`{"fills":[{"label":"Email","value":"a@b.c"},{"label":"City","value":"Berlin"}]}`)
	if err != nil {
		t.Fatalf("parsePlan: %v", err)
	}
	if len(plan) != 2 || plan[0].Label != "Email" || plan[1].Value != "Berlin" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestParsePlanDropsAnEntryWithNoLabelToAddressItBy(t *testing.T) {
	plan, err := parsePlan(`{"fills":[{"value":"orphan"},{"label":"Email","value":"a@b.c"}]}`)
	if err != nil {
		t.Fatalf("parsePlan: %v", err)
	}
	if len(plan) != 1 || plan[0].Label != "Email" {
		t.Fatalf("plan = %+v, want only the addressable fill", plan)
	}
}

func TestParsePlanRejectsAnAnswerThatIsNotJSON(t *testing.T) {
	if _, err := parsePlan("I'd be happy to help!"); err == nil {
		t.Fatal("parsePlan accepted prose")
	}
}

func TestLLMPlannerReportsWhenNoModelIsConfigured(t *testing.T) {
	_, err := LLMPlanner{}.Plan(context.Background(), []Field{{Label: "Email"}}, Profile{})
	if err == nil {
		t.Fatal("Plan succeeded with no model configured")
	}
}
