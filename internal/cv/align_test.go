package cv

import (
	"reflect"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/skilltag"
)

func TestAlign_ChipExpands(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"IaC", "Terraform"}}}}
	preferred := skilltag.PreferredFromText("Experience with infrastructure as code and Terraform.")
	got, changed := Align(doc, preferred)
	// Asserted against the literal spelling, not against preferred[...] — comparing the
	// output to the input the same map supplied cannot fail on a wrong spelling.
	want := []string{"infrastructure as code", "Terraform"}
	if !reflect.DeepEqual(got.Skills[0].Items, want) {
		t.Fatalf("skills = %v, want %v", got.Skills[0].Items, want)
	}
	if !changed {
		t.Error("changed = false, want true")
	}
}

func TestAlign_ChipShrinks(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"Infrastructure as Code"}}}}
	preferred := map[string]string{"infrastructure-as-code": "IaC"}
	got, _ := Align(doc, preferred)
	if got.Skills[0].Items[0] != "IaC" {
		t.Fatalf("chip = %q, want IaC", got.Skills[0].Items[0])
	}
}

func TestAlign_GoChipToGolang(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"Go", "Kubernetes"}}}}
	preferred := map[string]string{"go": "Golang", "kubernetes": "Kubernetes"}
	got, _ := Align(doc, preferred)
	if got.Skills[0].Items[0] != "Golang" {
		t.Fatalf("chip = %q, want Golang", got.Skills[0].Items[0])
	}
}

func TestAlign_NoInventedSkill(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"Python"}}}}
	preferred := map[string]string{"kubernetes": "Kubernetes"}
	got, changed := Align(doc, preferred)
	if !reflect.DeepEqual(got.Skills[0].Items, []string{"Python"}) {
		t.Fatalf("skills = %v, want unchanged Python only", got.Skills[0].Items)
	}
	if changed {
		t.Error("changed = true, want false")
	}
}

// Alignment may change how a skill is spelled and nothing else. The alias tables fold
// narrower terms onto a broader canonical for the search facet, so a vacancy's "Ruby on
// Rails" must never reach a CV that only claims Ruby.
func TestAlign_NarrowerTermNeverReplacesTheSkill(t *testing.T) {
	cases := []struct {
		jd    string
		chips []string
	}{
		{"We are a Ruby on Rails shop.", []string{"Ruby"}},
		{"Java 17 with Spring Boot microservices.", []string{"Java", "Spring"}},
		{"Senior ASP.NET engineer, SQL Server reporting.", []string{".NET", "T-SQL"}},
		{"Embedded C/C++ role.", []string{"C++"}},
		{"Looking for a C developer.", []string{"C"}},
		{"You will design RESTful APIs.", []string{"REST"}},
		{"Frontend with HTML5 and CSS3.", []string{"HTML", "CSS"}},
	}
	for _, tc := range cases {
		doc := Document{Skills: []SkillGroup{{Items: append([]string(nil), tc.chips...)}}}
		got, _ := Align(doc, skilltag.PreferredFromText(tc.jd))
		if !reflect.DeepEqual(got.Skills[0].Items, tc.chips) {
			t.Errorf("JD %q: chips = %v, want unchanged %v", tc.jd, got.Skills[0].Items, tc.chips)
		}
	}
}

// The résumé-only acronym tier exists because "RAG status" is red/amber/green project
// health in ordinary text. It must not reach prose rewriting.
func TestAlign_ResumeOnlyAcronymNotRewrittenInProse(t *testing.T) {
	doc := Document{
		Summary:    "Programme lead reporting RAG status to the steering committee.",
		Experience: []ExperienceItem{{Bullets: []string{"Ran the weekly RAG review with stakeholders."}}},
	}
	preferred := skilltag.PreferredFromText("You will build LLM features using retrieval augmented generation.")
	got, _ := Align(doc, preferred)
	if got.Summary != doc.Summary {
		t.Errorf("summary = %q, want unchanged", got.Summary)
	}
	if got.Experience[0].Bullets[0] != doc.Experience[0].Bullets[0] {
		t.Errorf("bullet = %q, want unchanged", got.Experience[0].Bullets[0])
	}
}

func TestAlign_CollapseDuplicateChips(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"IaC", "Infrastructure as Code", "Terraform"}}}}
	preferred := map[string]string{"infrastructure-as-code": "infrastructure as code", "terraform": "Terraform"}
	got, _ := Align(doc, preferred)
	want := []string{"infrastructure as code", "Terraform"}
	if !reflect.DeepEqual(got.Skills[0].Items, want) {
		t.Fatalf("skills = %v, want %v", got.Skills[0].Items, want)
	}
}

// A stack line is a chip list too: rewriting two spellings of one skill into the same
// spelling must not leave the reader looking at it twice.
func TestAlign_CollapseDuplicateStack(t *testing.T) {
	doc := Document{Experience: []ExperienceItem{{Stack: []string{"Kubernetes", "k8s", "Go"}}}}
	preferred := map[string]string{"kubernetes": "Kubernetes"}
	got, _ := Align(doc, preferred)
	want := []string{"Kubernetes", "Go"}
	if !reflect.DeepEqual(got.Experience[0].Stack, want) {
		t.Fatalf("stack = %v, want %v", got.Experience[0].Stack, want)
	}
}

// The candidate's own casing survives when they already use the vacancy's spelling: the
// point is to match its wording, not its shouting.
func TestAlign_ChipCasingKeptWhenSpellingAlreadyMatches(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"KUBERNETES", "postgres"}}}}
	preferred := map[string]string{"kubernetes": "Kubernetes", "postgresql": "Postgres"}
	got, changed := Align(doc, preferred)
	if !reflect.DeepEqual(got.Skills[0].Items, doc.Skills[0].Items) {
		t.Fatalf("skills = %v, want the candidate's own casing", got.Skills[0].Items)
	}
	if changed {
		t.Error("changed = true, want false")
	}
}

func TestAlign_ProseUnambiguousReplaces(t *testing.T) {
	doc := Document{
		Summary: "Built IaC pipelines.",
		Experience: []ExperienceItem{{
			Bullets: []string{"Ran k8s clusters.", "Used infrastructure-as-code."},
		}},
	}
	preferred := map[string]string{
		"infrastructure-as-code": "Infrastructure as Code",
		"kubernetes":             "Kubernetes",
	}
	got, _ := Align(doc, preferred)
	if got.Summary != "Built Infrastructure as Code pipelines." {
		t.Errorf("summary = %q", got.Summary)
	}
	if got.Experience[0].Bullets[0] != "Ran Kubernetes clusters." {
		t.Errorf("bullet0 = %q", got.Experience[0].Bullets[0])
	}
	// The hyphenated spelling is the same term; the vacancy side gets that for free.
	if got.Experience[0].Bullets[1] != "Used Infrastructure as Code." {
		t.Errorf("bullet1 = %q", got.Experience[0].Bullets[1])
	}
}

func TestAlign_ProseAmbiguousWordsUntouched(t *testing.T) {
	doc := Document{
		Experience: []ExperienceItem{{
			Bullets: []string{
				"Must go home after standup.",
				"We react to incidents quickly.",
				"A reaction to going rusty.",
			},
		}},
	}
	preferred := map[string]string{"go": "Golang", "react": "React.js", "rust": "Rust"}
	got, _ := Align(doc, preferred)
	for i, want := range doc.Experience[0].Bullets {
		if got.Experience[0].Bullets[i] != want {
			t.Errorf("bullet[%d] = %q, want %q (ambiguous prose must stay)", i, got.Experience[0].Bullets[i], want)
		}
	}
}

func TestAlign_ProseShortTokensUntouched(t *testing.T) {
	doc := Document{Summary: "Wrote TS helpers and C bindings."}
	preferred := map[string]string{"typescript": "TypeScript", "c": "C++"}
	got, _ := Align(doc, preferred)
	if got.Summary != doc.Summary {
		t.Fatalf("summary = %q, want unchanged short-token prose", got.Summary)
	}
}

func TestAlign_SubstringEmbedsUntouched(t *testing.T) {
	doc := Document{Summary: "A reaction to going rusty on the objective-c path."}
	preferred := map[string]string{"react": "React", "go": "Golang", "rust": "Rust", "c": "C"}
	got, _ := Align(doc, preferred)
	if got.Summary != doc.Summary {
		t.Fatalf("summary = %q, want unchanged embeds", got.Summary)
	}
}

func TestAlign_StackRewritten(t *testing.T) {
	doc := Document{Experience: []ExperienceItem{{Stack: []string{"IaC", "Go"}}}}
	preferred := map[string]string{"infrastructure-as-code": "infrastructure as code", "go": "Golang"}
	got, _ := Align(doc, preferred)
	want := []string{"infrastructure as code", "Golang"}
	if !reflect.DeepEqual(got.Experience[0].Stack, want) {
		t.Fatalf("stack = %v, want %v", got.Experience[0].Stack, want)
	}
}

// A replacement must never be re-read by another rule: a sequential replace turned
// "REST API" into "REST APIs APIs", and autopilot re-aligns on every run.
func TestAlign_IdempotentOverRepeatedRuns(t *testing.T) {
	doc := Document{
		Skills:  []SkillGroup{{Items: []string{"IaC", "k8s", "Node.js"}}},
		Summary: "Shipped IaC on k8s with Node.js and unit test coverage.",
		Experience: []ExperienceItem{{
			Stack:   []string{"k8s", "IaC"},
			Bullets: []string{"Built infrastructure as code for the k8s estate.", "Grew unit test coverage."},
		}},
	}
	preferred := skilltag.PreferredFromText(
		"Kubernetes estate managed with infrastructure as code, Node JS services, unit testing throughout.")
	once, changed := Align(doc, preferred)
	if !changed {
		t.Fatal("changed = false on the first pass, want true")
	}
	for i := 2; i <= 5; i++ {
		next, changedAgain := Align(once, preferred)
		if changedAgain {
			t.Fatalf("pass %d reported a change; summary now %q", i, next.Summary)
		}
		if !reflect.DeepEqual(next, once) {
			t.Fatalf("pass %d changed the document:\n once=%+v\n next=%+v", i, once, next)
		}
	}
}

// The map has no order, so the output must not depend on one.
func TestAlign_DeterministicAcrossRuns(t *testing.T) {
	doc := Document{
		Summary:    "Ran Node JS services on k8s with infrastructure as code.",
		Experience: []ExperienceItem{{Bullets: []string{"Owned the K8S estate and its infrastructure as code."}}},
	}
	preferred := map[string]string{
		"kubernetes":             "Kubernetes",
		"infrastructure-as-code": "IaC",
		"nodejs":                 "Node.js",
		"go":                     "Golang",
	}
	first, _ := Align(doc, preferred)
	for i := 0; i < 300; i++ {
		got, _ := Align(doc, preferred)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differed:\n%q\n%q", i, first.Summary, got.Summary)
		}
	}
}

// One rune whose lowercase is a different byte length used to disable every replacement
// after it — or run off the end of the string.
func TestAlign_NonASCIIProseStillAligns(t *testing.T) {
	for _, name := range []string{"İstanbul", "Ⱥrhus", "Straẞe", "Кёльн"} {
		doc := Document{Summary: "Led the " + name + " team that owned k8s and infrastructure as code."}
		got, _ := Align(doc, map[string]string{"kubernetes": "Kubernetes", "infrastructure-as-code": "IaC"})
		want := "Led the " + name + " team that owned Kubernetes and IaC."
		if got.Summary != want {
			t.Errorf("%s: summary = %q, want %q", name, got.Summary, want)
		}
	}
}

// A vacancy is arbitrary text from an external board; no input may crash a tailor request.
func TestAlign_PathologicalJDDoesNotPanic(t *testing.T) {
	for _, jd := range []string{
		strings.Repeat("Ⱥ", 200) + " Kubernetes and infrastructure as code",
		strings.Repeat("İ", 200) + " k8s",
		strings.Repeat("K", 100) + " IaC",
		"\x00\x01 Kubernetes � infrastructure as code",
	} {
		doc := Document{Summary: "Ran k8s.", Skills: []SkillGroup{{Items: []string{"IaC"}}}}
		// A panic is the failure; there is nothing else to assert.
		Align(doc, skilltag.PreferredFromText(jd))
	}
}

func TestAlign_FamilyFixtureUnchanged(t *testing.T) {
	// pgvector and vector-databases are not the same skill.
	doc := Document{Skills: []SkillGroup{{Items: []string{"pgvector"}}}}
	preferred := map[string]string{"vector-databases": "vector databases"}
	got, _ := Align(doc, preferred)
	if got.Skills[0].Items[0] != "pgvector" {
		t.Fatalf("got %q, want pgvector left alone", got.Skills[0].Items[0])
	}
}

func TestAlign_NilPreferred(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"IaC"}}}}
	got, changed := Align(doc, nil)
	if !reflect.DeepEqual(got, doc) || changed {
		t.Fatalf("got %+v changed=%v, want unchanged", got, changed)
	}
}

func TestAlign_DoesNotMutateInput(t *testing.T) {
	doc := Document{
		Skills:     []SkillGroup{{Items: []string{"IaC"}}},
		Experience: []ExperienceItem{{Stack: []string{"k8s"}, Bullets: []string{"Ran k8s."}}},
	}
	snapshot := Document{
		Skills:     []SkillGroup{{Items: []string{"IaC"}}},
		Experience: []ExperienceItem{{Stack: []string{"k8s"}, Bullets: []string{"Ran k8s."}}},
	}
	Align(doc, map[string]string{"infrastructure-as-code": "infrastructure as code", "kubernetes": "Kubernetes"})
	if !reflect.DeepEqual(doc, snapshot) {
		t.Fatalf("input was mutated: %+v", doc)
	}
}
