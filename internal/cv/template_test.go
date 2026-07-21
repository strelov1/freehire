package cv

import (
	"slices"
	"testing"
)

func TestTemplatesReportsMetadata(t *testing.T) {
	tmpls := Templates()
	if len(tmpls) == 0 {
		t.Fatal("Templates() returned nothing")
	}

	idx := slices.IndexFunc(tmpls, func(ti TemplateInfo) bool { return ti.ID == DefaultTemplateID })
	if idx < 0 {
		t.Fatalf("Templates() missing default %q; got %+v", DefaultTemplateID, tmpls)
	}
	def := tmpls[idx]
	if !def.ATSSafe {
		t.Errorf("default template %q should be ATS-safe", def.ID)
	}
	if def.Label == "" {
		t.Errorf("default template %q has no label", def.ID)
	}
}

func TestRegisteredTemplatesResolveToSource(t *testing.T) {
	// Every id the registry advertises must have a matching, non-empty .typ source.
	for _, ti := range Templates() {
		tmpl, err := ResolveTemplate(ti.ID)
		if err != nil {
			t.Errorf("ResolveTemplate(%q): %v", ti.ID, err)
			continue
		}
		if len(tmpl.source) == 0 {
			t.Errorf("template %q has empty source", ti.ID)
		}
	}
}

func TestExpectedTemplatesRegistered(t *testing.T) {
	ids := TemplateIDs()
	for _, want := range []string{"classic-ats", "centered", "modern-sans", "sidebar"} {
		if !slices.Contains(ids, want) {
			t.Errorf("template %q not registered; got %v", want, ids)
		}
	}
}

func TestSidebarIsNotATSSafe(t *testing.T) {
	for _, ti := range Templates() {
		if ti.ID == "sidebar" {
			if ti.ATSSafe {
				t.Error("sidebar must be flagged not ATS-safe")
			}
			return
		}
	}
	t.Error("sidebar template not registered")
}

func TestTemplateIDsMatchTemplates(t *testing.T) {
	ids := TemplateIDs()
	if !slices.Contains(ids, DefaultTemplateID) {
		t.Errorf("TemplateIDs() = %v, want to contain %q", ids, DefaultTemplateID)
	}
	// TemplateIDs is derived from the same registry Templates() reports.
	if len(ids) != len(Templates()) {
		t.Errorf("TemplateIDs() has %d ids but Templates() has %d entries", len(ids), len(Templates()))
	}
}
