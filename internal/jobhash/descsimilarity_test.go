package jobhash

import (
	"math"
	"testing"
)

func TestDescriptionSignatureKeepsMeaningfulWordsOnly(t *testing.T) {
	got := DescriptionSignature("<p>We build <b>Go</b> services at scale — the on&nbsp;call is shared.</p>")

	for _, want := range []string{"build", "services", "scale", "call", "shared"} {
		if _, ok := got[want]; !ok {
			t.Errorf("signature missing %q; got %v", want, keys(got))
		}
	}
	// Markup, entities and short tokens are noise, not vocabulary. "the" and "go" are dropped
	// by the length floor: a stop word appears in every posting, so it inflates the overlap of
	// unrelated jobs without ever telling two related ones apart.
	for _, unwanted := range []string{"p", "b", "go", "at", "the", "nbsp", "<p>"} {
		if _, ok := got[unwanted]; ok {
			t.Errorf("signature should not contain %q; got %v", unwanted, keys(got))
		}
	}
}

func TestDescriptionSignatureIsCaseInsensitiveAndDeduplicated(t *testing.T) {
	got := DescriptionSignature("Kubernetes kubernetes KUBERNETES scaling")

	if len(got) != 2 {
		t.Errorf("len = %d, want 2 distinct tokens; got %v", len(got), keys(got))
	}
	if _, ok := got["kubernetes"]; !ok {
		t.Errorf("expected the lowercased token; got %v", keys(got))
	}
}

// The signal the spike validated: a role reposted per city differs only in a small localized
// block, so its word overlap stays far above any threshold, while genuinely different roles
// sit far below.
func TestDescriptionSimilaritySeparatesCityVariantsFromDistinctRoles(t *testing.T) {
	// Proportions matter, so they mirror prod: a real description runs ~11 000 characters and the
	// localized block that differs between cities is 120–200 of them. A toy base with a
	// one-sentence delta would score ~0.6 and prove nothing about the threshold.
	base := "We are hiring a fullstack engineer to build and operate our payments platform. " +
		"You will work with Golang, TypeScript and Postgres, own services end to end, and share the on call rotation. " +
		"Responsibilities include designing resilient APIs, reviewing pull requests from colleagues, " +
		"instrumenting services with metrics and traces, running incident retrospectives without blame, " +
		"mentoring junior engineers, shaping the quarterly technical roadmap alongside product managers, " +
		"migrating legacy batch pipelines towards streaming, tightening database indexes and query plans, " +
		"automating deployments through continuous delivery, writing architectural decision records, " +
		"participating in hiring interviews, improving developer tooling, reducing build times, " +
		"and keeping documentation accurate as the platform evolves across regions and payment providers. " +
		"Our stack runs containerised workloads orchestrated by Kubernetes across several availability zones, " +
		"with asynchronous messaging, idempotent consumers, and careful backpressure handling under load. " +
		"We practise trunk based development, keep feature flags short lived, and prefer boring technology " +
		"where reliability matters more than novelty. Engineers rotate through customer facing support weeks " +
		"so that operational pain reaches the people able to remove it permanently. Compensation reviews happen " +
		"twice yearly, equipment budgets renew annually, conference attendance receives generous support, " +
		"remote colleagues gather quarterly for planning workshops, and internal mobility between squads " +
		"remains actively encouraged rather than merely tolerated by leadership across every department."
	cityVariant := base + " Salary in Poland is quoted gross per month and the contract follows Polish labour law."
	otherRole := "We are looking for a management consultant to advise our retail clients on supply chain " +
		"strategy, run workshops with stakeholders and prepare board level presentations."

	same := DescriptionSimilarity(DescriptionSignature(base), DescriptionSignature(cityVariant))
	diff := DescriptionSimilarity(DescriptionSignature(base), DescriptionSignature(otherRole))

	if same < 0.9 {
		t.Errorf("city variant similarity = %.3f, want >= 0.9", same)
	}
	if diff > 0.5 {
		t.Errorf("distinct role similarity = %.3f, want <= 0.5", diff)
	}
}

func TestDescriptionSimilarityIdenticalIsOne(t *testing.T) {
	s := DescriptionSignature("Build reliable distributed systems in Go and Kubernetes")

	if got := DescriptionSimilarity(s, s); math.Abs(got-1) > 1e-9 {
		t.Errorf("identical similarity = %v, want 1", got)
	}
}

// An empty signature must never look similar to anything: a posting whose description did not
// survive normalization would otherwise merge with every other empty one in its bucket.
func TestDescriptionSimilarityEmptyIsNeverSimilar(t *testing.T) {
	empty := DescriptionSignature("<p></p>")
	real := DescriptionSignature("Build reliable distributed systems in Go and Kubernetes")

	if got := DescriptionSimilarity(empty, real); got != 0 {
		t.Errorf("empty vs real = %v, want 0", got)
	}
	if got := DescriptionSimilarity(empty, empty); got != 0 {
		t.Errorf("empty vs empty = %v, want 0 (two unknowns are not a match)", got)
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
