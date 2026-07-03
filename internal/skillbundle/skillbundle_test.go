package skillbundle

import (
	"reflect"
	"testing"
)

func find(bs []Bundle, name string) (Bundle, bool) {
	for _, b := range bs {
		if b.Name == name {
			return b, true
		}
	}
	return Bundle{}, false
}

func TestCoverage_FullyCovered(t *testing.T) {
	bs := Coverage([]string{"docker", "kubernetes", "ci-cd", "terraform", "aws"})
	b, ok := find(bs, "cloud-ops")
	if !ok {
		t.Fatalf("no cloud-ops bundle in %+v", bs)
	}
	if b.Covered != 5 || b.Total != 5 || !b.Has {
		t.Errorf("cloud-ops = %+v, want covered 5/5 and Has", b)
	}
}

func TestCoverage_AtThresholdCovered(t *testing.T) {
	// 3 of 5 = 60% ≥ 50% → Has.
	bs := Coverage([]string{"docker", "kubernetes", "ci-cd"})
	b, _ := find(bs, "cloud-ops")
	if b.Covered != 3 || !b.Has {
		t.Errorf("cloud-ops = %+v, want covered 3 and Has", b)
	}
}

func TestCoverage_BelowThresholdNotCovered(t *testing.T) {
	// 2 of 5 = 40% < 50% → not Has.
	bs := Coverage([]string{"docker", "kubernetes"})
	b, _ := find(bs, "cloud-ops")
	if b.Covered != 2 || b.Has {
		t.Errorf("cloud-ops = %+v, want covered 2 and NOT Has", b)
	}
}

func TestCoverage_Deterministic(t *testing.T) {
	a := Coverage([]string{"go", "sql", "postgresql", "react"})
	b := Coverage([]string{"go", "sql", "postgresql", "react"})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Coverage not deterministic:\n%+v\n%+v", a, b)
	}
}
