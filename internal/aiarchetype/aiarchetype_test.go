package aiarchetype

import "testing"

func TestDerive(t *testing.T) {
	cases := []struct {
		name     string
		skills   []string
		category string
		want     string
	}{
		{
			"rag app builder",
			[]string{"rag", "langchain", "langgraph", "vector-databases"},
			"ai_engineering",
			"rag_app_builder",
		},
		{
			"rag app builder with llamaindex instead of langchain",
			[]string{"rag", "llamaindex", "vector-databases"},
			"ml_ai",
			"rag_app_builder",
		},
		{
			"agent builder",
			[]string{"agentic-ai", "prompt-engineering", "rag"},
			"ai_engineering",
			"agent_builder",
		},
		{
			"cloud ml platform engineer",
			[]string{"mlops", "kubernetes", "pytorch"},
			"ai_engineering",
			"cloud_ml_platform_engineer",
		},
		{
			"ml trainer researcher",
			[]string{"pytorch", "tensorflow"},
			"ai_engineering",
			"ml_trainer_researcher",
		},
		{
			"fullstack ai engineer",
			[]string{"react", "nodejs", "openai"},
			"ai_engineering",
			"fullstack_ai_engineer",
		},
		{
			"devops infra engineer",
			[]string{"terraform", "kubernetes", "docker", "ci-cd"},
			"ai_engineering",
			"devops_infra_engineer",
		},
		{
			"rag app builder wins over agent builder on overlapping skills",
			[]string{"rag", "langchain", "langgraph", "vector-databases", "agentic-ai", "prompt-engineering"},
			"ai_engineering",
			"rag_app_builder",
		},
		{
			"ml trainer excludes rag-flavored jobs",
			[]string{"pytorch", "tensorflow", "rag"},
			"ai_engineering",
			"", // 64% of ai-first roles need "some" ML per the field guide — pytorch/tensorflow
			// alone must not steal a job an earlier, more AI-specific rule would otherwise claim.
			// This case satisfies none of the six rules: rule 1/2 need more than bare "rag",
			// rule 4 explicitly excludes "rag".
		},
		{
			"category outside ai/ml scope yields no archetype regardless of skills",
			[]string{"rag", "langchain", "langgraph", "vector-databases"},
			"backend",
			"",
		},
		{
			"in-scope category with no matching rule yields no archetype",
			[]string{"go", "postgresql"},
			"ai_engineering",
			"",
		},
		{
			"empty skills yields no archetype",
			nil,
			"ai_engineering",
			"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Derive(tc.skills, tc.category)
			if got != tc.want {
				t.Fatalf("Derive(%v, %q) = %q, want %q", tc.skills, tc.category, got, tc.want)
			}
		})
	}
}
