package cv

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/skilltag"
)

func TestAlign_ChipExpands(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"IaC", "Terraform"}}}}
	preferred := skilltag.PreferredFromText("Experience with infrastructure as code and Terraform.")
	got := Align(doc, preferred)
	if got.Skills[0].Items[0] != preferred["infrastructure-as-code"] {
		t.Fatalf("chip = %q, want %q", got.Skills[0].Items[0], preferred["infrastructure-as-code"])
	}
}

func TestAlign_ChipShrinks(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"Infrastructure as Code"}}}}
	preferred := map[string]string{"infrastructure-as-code": "IaC"}
	got := Align(doc, preferred)
	if got.Skills[0].Items[0] != "IaC" {
		t.Fatalf("chip = %q, want IaC", got.Skills[0].Items[0])
	}
}

func TestAlign_GoChipToGolang(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"Go", "Kubernetes"}}}}
	preferred := map[string]string{"go": "Golang", "kubernetes": "Kubernetes"}
	got := Align(doc, preferred)
	if got.Skills[0].Items[0] != "Golang" {
		t.Fatalf("chip = %q, want Golang", got.Skills[0].Items[0])
	}
}

func TestAlign_NoInventedSkill(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"Python"}}}}
	preferred := map[string]string{"kubernetes": "Kubernetes"}
	got := Align(doc, preferred)
	if !reflect.DeepEqual(got.Skills[0].Items, []string{"Python"}) {
		t.Fatalf("skills = %v, want unchanged Python only", got.Skills[0].Items)
	}
}

func TestAlign_CollapseDuplicateChips(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"IaC", "Infrastructure as Code", "Terraform"}}}}
	preferred := map[string]string{"infrastructure-as-code": "infrastructure as code", "terraform": "Terraform"}
	got := Align(doc, preferred)
	want := []string{"infrastructure as code", "Terraform"}
	if !reflect.DeepEqual(got.Skills[0].Items, want) {
		t.Fatalf("skills = %v, want %v", got.Skills[0].Items, want)
	}
}

func TestAlign_ProseUnambiguousReplaces(t *testing.T) {
	doc := Document{
		Summary: "Built IaC pipelines.",
		Experience: []ExperienceItem{{
			Bullets: []string{"Ran k8s clusters.", "Used infrastructure as code."},
		}},
	}
	preferred := map[string]string{
		"infrastructure-as-code": "Infrastructure as Code",
		"kubernetes":             "Kubernetes",
	}
	got := Align(doc, preferred)
	if got.Summary != "Built Infrastructure as Code pipelines." {
		t.Errorf("summary = %q", got.Summary)
	}
	if got.Experience[0].Bullets[0] != "Ran Kubernetes clusters." {
		t.Errorf("bullet0 = %q", got.Experience[0].Bullets[0])
	}
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
	preferred := map[string]string{
		"go":    "Golang",
		"react": "React",
		"rust":  "Rust",
	}
	got := Align(doc, preferred)
	for i, want := range doc.Experience[0].Bullets {
		if got.Experience[0].Bullets[i] != want {
			t.Errorf("bullet[%d] = %q, want %q (ambiguous prose must stay)", i, got.Experience[0].Bullets[i], want)
		}
	}
}

func TestAlign_ProseShortTokensUntouched(t *testing.T) {
	// "js" aliases javascript (2 letters) — must not rewrite in prose.
	doc := Document{Summary: "Wrote js helpers and C bindings."}
	preferred := map[string]string{"javascript": "JavaScript", "c": "C++"}
	got := Align(doc, preferred)
	if got.Summary != doc.Summary {
		t.Fatalf("summary = %q, want unchanged short-token prose", got.Summary)
	}
}

func TestAlign_SubstringEmbedsUntouched(t *testing.T) {
	doc := Document{Summary: "A reaction to going rusty on the objective-c path."}
	preferred := map[string]string{
		"react": "React",
		"go":    "Golang",
		"rust":  "Rust",
		"c":     "C",
	}
	got := Align(doc, preferred)
	if got.Summary != doc.Summary {
		t.Fatalf("summary = %q, want unchanged embeds", got.Summary)
	}
}

func TestAlign_StackRewritten(t *testing.T) {
	doc := Document{Experience: []ExperienceItem{{Stack: []string{"IaC", "Go"}}}}
	preferred := map[string]string{"infrastructure-as-code": "infrastructure as code", "go": "Golang"}
	got := Align(doc, preferred)
	want := []string{"infrastructure as code", "Golang"}
	if !reflect.DeepEqual(got.Experience[0].Stack, want) {
		t.Fatalf("stack = %v, want %v", got.Experience[0].Stack, want)
	}
}

func TestAlign_Idempotent(t *testing.T) {
	doc := Document{
		Skills:  []SkillGroup{{Items: []string{"IaC", "k8s"}}},
		Summary: "Shipped IaC on k8s.",
	}
	preferred := map[string]string{
		"infrastructure-as-code": "infrastructure as code",
		"kubernetes":             "Kubernetes",
	}
	once := Align(doc, preferred)
	twice := Align(once, preferred)
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("second align changed document:\n once=%+v\ntwice=%+v", once, twice)
	}
	if AlignChanged(once, preferred) {
		t.Fatal("AlignChanged true after alignment — want idempotent false")
	}
}

func TestAlign_FamilyFixtureUnchanged(t *testing.T) {
	// Phase 1 must not treat pgvector and vector-databases as the same skill.
	doc := Document{Skills: []SkillGroup{{Items: []string{"pgvector"}}}}
	preferred := map[string]string{"vector-databases": "vector databases"}
	got := Align(doc, preferred)
	if got.Skills[0].Items[0] != "pgvector" {
		t.Fatalf("got %q, want pgvector left alone (no family link in Phase 1)", got.Skills[0].Items[0])
	}
}

func TestAlign_NilPreferred(t *testing.T) {
	doc := Document{Skills: []SkillGroup{{Items: []string{"IaC"}}}}
	got := Align(doc, nil)
	if !reflect.DeepEqual(got, doc) {
		t.Fatalf("got %+v, want unchanged", got)
	}
}
