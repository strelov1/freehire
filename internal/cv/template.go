package cv

import (
	"embed"
	"errors"
	"fmt"
	"slices"
)

//go:embed templates/*.typ
var templateFS embed.FS

// DefaultTemplateID is the template assigned when a CV names none.
const DefaultTemplateID = "classic-ats"

// TemplateInfo is a registered template's display metadata. ATSSafe marks the
// single-column, standard-heading layouts that parse cleanly in résumé-scanning
// software; richer layouts (e.g. a sidebar) are listed but flagged unsafe.
type TemplateInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Style   string `json:"style"`
	ATSSafe bool   `json:"ats_safe"`
}

// templates is the registry of known templates. A CV's template_id must be one of
// these; the seam to add more is: append an entry here and drop a matching
// templates/<id>.typ file, then regenerate the gallery thumbnails with `make cv-previews`.
var templates = []TemplateInfo{
	{ID: DefaultTemplateID, Label: "Classic", Style: "single-column · serif", ATSSafe: true},
	{ID: "centered", Label: "Centered", Style: "centered · serif", ATSSafe: true},
	{ID: "modern-sans", Label: "Modern", Style: "single-column · sans", ATSSafe: true},
	{ID: "sidebar", Label: "Sidebar", Style: "two-column · serif", ATSSafe: false},
}

// ErrUnknownTemplate is returned when a template_id is not in the registry. The handler
// maps it to a 400 (an unknown template is never rendered).
var ErrUnknownTemplate = errors.New("cv: unknown template")

// Template is a resolved CV template: its id and the Typst source that renders a Document
// (which the source reads back as json("data.json")).
type Template struct {
	ID     string
	source []byte
}

// Templates returns the registered templates' display metadata (for the UI and preview
// generation).
func Templates() []TemplateInfo { return slices.Clone(templates) }

// TemplateIDs returns the registered template ids (for request validation).
func TemplateIDs() []string {
	ids := make([]string, len(templates))
	for i, t := range templates {
		ids[i] = t.ID
	}
	return ids
}

// ResolveTemplate returns the template for id, defaulting an empty id to DefaultTemplateID
// and rejecting any id not in the registry with ErrUnknownTemplate.
func ResolveTemplate(id string) (Template, error) {
	if id == "" {
		id = DefaultTemplateID
	}
	if !slices.ContainsFunc(templates, func(t TemplateInfo) bool { return t.ID == id }) {
		return Template{}, fmt.Errorf("%w: %q", ErrUnknownTemplate, id)
	}
	src, err := templateFS.ReadFile("templates/" + id + ".typ")
	if err != nil {
		return Template{}, fmt.Errorf("cv: read template %q: %w", id, err)
	}
	return Template{ID: id, source: src}, nil
}
