package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The account search_jobs takes the non-facet filters the public one does; agents sent them
// to both and the account tool refused them as unknown.
func TestAssistantSearchJobs_PassesNonFacetFilters(t *testing.T) {
	fs := &fakeSearcher{}
	h := &assistantHandlers{search: &searchHandlers{search: fs, descriptions: fixedDescriptions{}}}
	args := `{"query":"go","filters":{"skills":["go"]},"salary_min":150000,"posted_within_days":3,` +
		`"visa_sponsorship":true,"exclude_skills":["php"],"require_all_skills":true}`
	if _, err := h.searchJobs().Run(context.Background(), 1, json.RawMessage(args)); err != nil {
		t.Fatal(err)
	}
	filter := fmt.Sprint(fs.got.Filter)
	for _, want := range []string{"150000", "visa", "php"} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter %s does not carry %q", filter, want)
		}
	}
}
