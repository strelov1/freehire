// Package skillbundle groups skills into a curated set of market-recognised bundles
// (a candidate is expected to hold combinations, not isolated skills — e.g. GenAI+Ops
// appears in most AI-engineering roles) and reports a CV's coverage of each. It is a
// curated dictionary, not a derived co-occurrence graph (mirrors internal/skilltag):
// pure, deterministic, no LLM.
package skillbundle

// BundleCoveredPct is the share of a bundle's member skills a CV must hold for the
// bundle to count as covered. Tunable.
const BundleCoveredPct = 50

// Bundle is one market skill combination and a CV's coverage of it.
type Bundle struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Covered int    `json:"covered"` // member skills the CV holds
	Total   int    `json:"total"`   // bundle size
	Has     bool   `json:"has"`     // Covered/Total ≥ BundleCoveredPct
}

// bundle is a curated definition: a stable name/label and its member skill slugs
// (canonical skilltag slugs).
type bundle struct {
	name    string
	label   string
	members []string
}

// bundles is the curated dictionary, AI/backend-focused first (expand as needed).
var bundles = []bundle{
	{"genai-core", "GenAI core", []string{"llm", "rag", "langchain", "generative-ai", "prompt-engineering"}},
	{"cloud-ops", "Cloud & Ops", []string{"docker", "kubernetes", "ci-cd", "terraform", "aws"}},
	{"web-stack", "Web stack", []string{"react", "typescript", "javascript", "nextjs", "nodejs"}},
	{"data", "Data", []string{"sql", "postgresql", "mongodb", "kafka", "redis"}},
	{"ml", "Machine learning", []string{"pytorch", "tensorflow", "scikit-learn"}},
}

// Coverage returns each bundle's coverage by the CV's parsed skill set. Pure and
// deterministic (bundles keep their declared order).
func Coverage(cvSkills []string) []Bundle {
	owned := make(map[string]bool, len(cvSkills))
	for _, s := range cvSkills {
		owned[s] = true
	}
	out := make([]Bundle, 0, len(bundles))
	for _, b := range bundles {
		covered := 0
		for _, m := range b.members {
			if owned[m] {
				covered++
			}
		}
		total := len(b.members)
		out = append(out, Bundle{
			Name:    b.name,
			Label:   b.label,
			Covered: covered,
			Total:   total,
			Has:     total > 0 && covered*100 >= BundleCoveredPct*total,
		})
	}
	return out
}
