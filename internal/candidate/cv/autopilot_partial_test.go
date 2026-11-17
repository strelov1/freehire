package cv

import (
	"encoding/json"
	"testing"
)

func TestReportCannotCloseARequirementWithAnUnverifiedPart(t *testing.T) {
	for _, tc := range []struct{ requirement, unverified string }{
		{"AWS Lambda", "AWS Lambda itself is not evidenced"},
		{"Azure AI Search", "Only generic RAG experience is documented"},
		{"3+ years of SQL experience", "Duration is not established"},
		{"3+ years of authentication experience", "Duration is not established"},
		{"3+ years of Node.js experience", "Duration is not established"},
		{"Multiple-testing correction", "Only a statistically significant A/B test is documented"},
		{"AWS serverless architecture", "Serverless implementation is not confirmed"},
	} {
		t.Run(tc.requirement, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]string{
				"requirement": tc.requirement, "status": "closed_bank", "unverified": tc.unverified,
			})
			var entry AutopilotEntry
			if err := json.Unmarshal(raw, &entry); err != nil {
				t.Fatal(err)
			}
			clean, err := SanitizeAutopilotReport([]AutopilotEntry{entry})
			if err != nil {
				t.Fatal(err)
			}
			if clean[0].Status != "partial" {
				t.Fatalf("status = %q, want partial", clean[0].Status)
			}
			blob, _ := json.Marshal(clean[0])
			var saved map[string]any
			_ = json.Unmarshal(blob, &saved)
			if saved["unverified"] != tc.unverified {
				t.Fatal("lost the remaining gap")
			}
		})
	}
}
