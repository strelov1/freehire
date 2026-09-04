//go:build llmlive

// Live check that the interpreter still names what a person named, now that it offers
// `category` and `seniority` where it used to offer their product. It costs real
// tokens, so it is behind the llmlive tag and never runs in CI.
//
//	LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=… go test -tags=llmlive ./internal/search/searchintent/ -run Live -v
//
// The `role` enum fused two axes into one value, and the prompt that offered it said
// so — "the category and seniority fields already carry the general case". Removing it
// is the one edit in drop-role-facet where behaviour could get WORSE rather than
// merely narrower, because a model is not a table: it might have leaned on the fused
// spelling. This is how that question gets an answer instead of an assumption.
package searchintent

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/llm"
)

func liveInterpreter(t *testing.T) *Interpreter {
	t.Helper()

	s := llm.Settings{
		BaseURL: os.Getenv("LLM_BASE_URL"),
		APIKey:  os.Getenv("LLM_API_KEY"),
		Model:   os.Getenv("LLM_MODEL"),
	}
	if !s.Enabled() {
		t.Skip("LLM_BASE_URL/LLM_API_KEY/LLM_MODEL not set")
	}
	c, flush, err := llm.NewClient(s, "searchintent-live")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(flush)
	return NewInterpreter(c)
}

func TestLiveInterpretNamesBothAxes(t *testing.T) {
	in := liveInterpreter(t)

	cases := []struct {
		text string
		// want is facet → one value that MUST be present. Not an equality: a model may
		// reasonably add `work_mode` to "remote" or a region to "europe", and a test
		// that forbade extras would fail on the interpreter doing its job.
		want map[string]string
	}{
		{"senior backend in Berlin", map[string]string{"seniority": "senior", "category": "backend"}},
		{"junior QA remote", map[string]string{"seniority": "junior", "category": "qa"}},
		{"staff data engineer", map[string]string{"seniority": "staff", "category": "data_engineering"}},
		{"lead devops in europe", map[string]string{"seniority": "lead", "category": "devops"}},
	}

	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			got, err := in.Interpret(ctx, Request{Text: c.text})
			if err != nil {
				t.Fatalf("Interpret: %v", err)
			}
			for facet, value := range c.want {
				if !slices.Contains(got.Facets[facet], value) {
					t.Errorf("%s = %v, want it to contain %q", facet, got.Facets[facet], value)
				}
			}
			// The grade must not leak into the free-text query. That is the failure the
			// fused slug could hide: a model that cannot express "senior" as a facet
			// might smuggle it into `q`, where it matches the word in a description
			// rather than filtering by the field.
			if got.Query != "" {
				t.Logf("query = %q (worth reading — the filter should carry the intent, not this)", got.Query)
			}
			t.Logf("facets = %v", got.Facets)
		})
	}
}

// The facet is gone from the vocabulary the model is offered, so nothing it returns
// should carry one. A live model is the only thing that can prove the prompt no longer
// invites it — the unit tests prove the resolver refuses it, which is a different
// claim.
func TestLiveInterpretNeverReturnsARole(t *testing.T) {
	in := liveInterpreter(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	got, err := in.Interpret(ctx, Request{Text: "senior backend engineer role"})
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	if v := got.Facets["role"]; len(v) != 0 {
		t.Errorf("role = %v, want none — the facet is not offered", v)
	}
}
