package cvsection

import (
	"reflect"
	"sort"
	"testing"
)

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func contains(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}

func TestParse_DeclaredAndBackedByExperience(t *testing.T) {
	cv := `John Doe

Skills
Kubernetes, PostgreSQL

Experience
Operated Kubernetes clusters in production.
`
	declared, body, all := Parse(cv)

	if got, want := sorted(declared), []string{"kubernetes", "postgresql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("declared = %v, want %v", got, want)
	}
	if !contains(body, "kubernetes") {
		t.Errorf("body = %v, want it to contain kubernetes", body)
	}
	if got, want := sorted(all), []string{"kubernetes", "postgresql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("all = %v, want %v", got, want)
	}
}

func TestParse_SkillUsedButNotDeclared(t *testing.T) {
	cv := `Skills
Kubernetes

Experience
Used Kafka for the streaming pipeline.
`
	declared, body, _ := Parse(cv)

	if contains(declared, "kafka") {
		t.Errorf("declared = %v, want it NOT to contain kafka", declared)
	}
	if !contains(body, "kafka") {
		t.Errorf("body = %v, want it to contain kafka", body)
	}
}

func TestParse_NoSkillsHeading(t *testing.T) {
	cv := `Experience
Built the platform with Kafka and PostgreSQL.
`
	declared, body, all := Parse(cv)

	if len(declared) != 0 {
		t.Errorf("declared = %v, want empty (no Skills heading)", declared)
	}
	if got, want := sorted(body), []string{"kafka", "postgresql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
	if got, want := sorted(all), []string{"kafka", "postgresql"}; !reflect.DeepEqual(got, want) {
		t.Errorf("all = %v, want %v", got, want)
	}
}

func TestParse_Deterministic(t *testing.T) {
	cv := `Skills
Docker, PostgreSQL

Experience
Shipped services on Docker and Kubernetes.
`
	d1, b1, a1 := Parse(cv)
	d2, b2, a2 := Parse(cv)
	if !reflect.DeepEqual(d1, d2) || !reflect.DeepEqual(b1, b2) || !reflect.DeepEqual(a1, a2) {
		t.Errorf("Parse not deterministic:\n d1=%v b1=%v a1=%v\n d2=%v b2=%v a2=%v", d1, b1, a1, d2, b2, a2)
	}
}
