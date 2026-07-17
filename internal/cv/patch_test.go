package cv

import (
	"errors"
	"reflect"
	"testing"
)

// sampleDoc returns a small but multi-section document for patch tests.
func sampleDoc() Document {
	return Document{
		Header:  Header{FullName: "Ada Lovelace", Email: "ada@example.com"},
		Summary: "Backend engineer",
		Experience: []ExperienceItem{
			{Role: "Engineer", Company: "Acme", Bullets: []string{"Shipped API", "Cut latency"}},
			{Role: "Intern", Company: "Beta", Bullets: []string{"Wrote docs"}},
		},
		Skills: []SkillGroup{{Group: "Languages", Items: []string{"Go"}}},
	}
}

func TestApply_SetSummary(t *testing.T) {
	in := sampleDoc()
	out, err := Apply(in, Patch{Op: PatchSetSummary, Value: "Senior backend engineer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Summary != "Senior backend engineer" {
		t.Errorf("summary = %q, want updated", out.Summary)
	}
	// Every other section is unchanged.
	if !reflect.DeepEqual(out.Experience, in.Experience) || !reflect.DeepEqual(out.Skills, in.Skills) || !reflect.DeepEqual(out.Header, in.Header) {
		t.Errorf("set_summary touched other sections")
	}
}

func TestApply_AddBullet(t *testing.T) {
	in := sampleDoc()
	out, err := Apply(in, Patch{Op: PatchAddBullet, Experience: 0, Value: "Led migration"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Shipped API", "Cut latency", "Led migration"}
	if !reflect.DeepEqual(out.Experience[0].Bullets, want) {
		t.Errorf("bullets = %v, want %v", out.Experience[0].Bullets, want)
	}
	// The other experience entry is untouched.
	if !reflect.DeepEqual(out.Experience[1], in.Experience[1]) {
		t.Errorf("add_bullet touched a different experience entry")
	}
}

func TestApply_ReplaceBullet(t *testing.T) {
	in := sampleDoc()
	out, err := Apply(in, Patch{Op: PatchReplaceBullet, Experience: 0, Bullet: 1, Value: "Cut p99 latency 40%"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Shipped API", "Cut p99 latency 40%"}
	if !reflect.DeepEqual(out.Experience[0].Bullets, want) {
		t.Errorf("bullets = %v, want %v", out.Experience[0].Bullets, want)
	}
}

func TestApply_RemoveBullet(t *testing.T) {
	in := sampleDoc()
	out, err := Apply(in, Patch{Op: PatchRemoveBullet, Experience: 0, Bullet: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Cut latency"}
	if !reflect.DeepEqual(out.Experience[0].Bullets, want) {
		t.Errorf("bullets = %v, want %v", out.Experience[0].Bullets, want)
	}
}

func TestApply_ReorderBullets(t *testing.T) {
	in := sampleDoc()
	out, err := Apply(in, Patch{Op: PatchReorderBullets, Experience: 0, Order: []int{1, 0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Cut latency", "Shipped API"}
	if !reflect.DeepEqual(out.Experience[0].Bullets, want) {
		t.Errorf("bullets = %v, want %v (order applied)", out.Experience[0].Bullets, want)
	}
}

func TestApply_ReorderBullets_rejectsNonPermutation(t *testing.T) {
	cases := map[string][]int{
		"wrong length": {0},
		"out of range": {0, 2},
		"duplicate":    {0, 0},
	}
	for name, order := range cases {
		t.Run(name, func(t *testing.T) {
			in := sampleDoc()
			_, err := Apply(in, Patch{Op: PatchReorderBullets, Experience: 0, Order: order})
			if !errors.Is(err, ErrInvalidPatch) {
				t.Errorf("order %v: err = %v, want ErrInvalidPatch", order, err)
			}
		})
	}
}

func TestApply_SetHeaderField(t *testing.T) {
	in := sampleDoc()
	out, err := Apply(in, Patch{Op: PatchSetHeaderField, Field: "location", Value: "Berlin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Header.Location != "Berlin" {
		t.Errorf("location = %q, want Berlin", out.Header.Location)
	}
	if out.Header.FullName != in.Header.FullName {
		t.Errorf("set_header_field clobbered another header field")
	}
}

func TestApply_SetHeaderField_unknownField(t *testing.T) {
	in := sampleDoc()
	_, err := Apply(in, Patch{Op: PatchSetHeaderField, Field: "nickname", Value: "x"})
	if !errors.Is(err, ErrInvalidPatch) {
		t.Errorf("err = %v, want ErrInvalidPatch", err)
	}
}

func TestApply_SetSkillGroup(t *testing.T) {
	t.Run("replaces items of an existing group by name", func(t *testing.T) {
		in := sampleDoc()
		out, err := Apply(in, Patch{Op: PatchSetSkillGroup, Group: "Languages", Items: []string{"Go", "Rust"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []SkillGroup{{Group: "Languages", Items: []string{"Go", "Rust"}}}
		if !reflect.DeepEqual(out.Skills, want) {
			t.Errorf("skills = %v, want %v", out.Skills, want)
		}
	})
	t.Run("appends a new group when the name is new", func(t *testing.T) {
		in := sampleDoc()
		out, err := Apply(in, Patch{Op: PatchSetSkillGroup, Group: "Cloud", Items: []string{"AWS"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out.Skills) != 2 || out.Skills[1].Group != "Cloud" {
			t.Errorf("expected a new Cloud group appended, got %v", out.Skills)
		}
	})
}

func TestApply_OutOfRangeExperience(t *testing.T) {
	in := sampleDoc()
	_, err := Apply(in, Patch{Op: PatchAddBullet, Experience: 5, Value: "x"})
	if !errors.Is(err, ErrInvalidPatch) {
		t.Errorf("err = %v, want ErrInvalidPatch", err)
	}
}

func TestApply_OutOfRangeBullet(t *testing.T) {
	in := sampleDoc()
	_, err := Apply(in, Patch{Op: PatchReplaceBullet, Experience: 0, Bullet: 9, Value: "x"})
	if !errors.Is(err, ErrInvalidPatch) {
		t.Errorf("err = %v, want ErrInvalidPatch", err)
	}
}

func TestApply_UnknownOp(t *testing.T) {
	in := sampleDoc()
	_, err := Apply(in, Patch{Op: "frobnicate"})
	if !errors.Is(err, ErrInvalidPatch) {
		t.Errorf("err = %v, want ErrInvalidPatch", err)
	}
}

func TestApply_DoesNotMutateInput(t *testing.T) {
	in := sampleDoc()
	before := sampleDoc() // independent identical copy
	if _, err := Apply(in, Patch{Op: PatchAddBullet, Experience: 0, Value: "New"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(in, before) {
		t.Errorf("Apply mutated its input: got %+v, want %+v", in, before)
	}
}
