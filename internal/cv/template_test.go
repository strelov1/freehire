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
	for _, want := range []string{"classic-ats", "centered", "modern-sans", "timeline", "compact", "mono-tech", "sidebar", "portrait", "headshot"} {
		if !slices.Contains(ids, want) {
			t.Errorf("template %q not registered; got %v", want, ids)
		}
	}
}

func TestPhotoFlagMarksTheTemplatesThatPrintAPortrait(t *testing.T) {
	want := map[string]bool{
		"classic-ats": false,
		"centered":    false,
		"modern-sans": false,
		"timeline":    false,
		"compact":     false,
		"mono-tech":   false,
		"sidebar":     false,
		"portrait":    true,
		"headshot":    true,
	}
	for _, ti := range Templates() {
		w, known := want[ti.ID]
		if !known {
			t.Errorf("template %q is registered but the photo expectation is unstated", ti.ID)
			continue
		}
		if ti.Photo != w {
			t.Errorf("template %q: photo = %v, want %v", ti.ID, ti.Photo, w)
		}
	}
}

func TestResolveTemplateCarriesThePhotoFlag(t *testing.T) {
	// The renderer decides whether to stage an image from the resolved template, so the
	// flag must survive resolution and not live on the registry entry alone.
	for _, ti := range Templates() {
		tmpl, err := ResolveTemplate(ti.ID)
		if err != nil {
			t.Fatalf("ResolveTemplate(%q): %v", ti.ID, err)
		}
		if tmpl.Photo != ti.Photo {
			t.Errorf("template %q: resolved photo = %v, registry says %v", ti.ID, tmpl.Photo, ti.Photo)
		}
	}
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
