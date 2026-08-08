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

// TemplateInfo is a registered template's display metadata.
//
// Photo marks the templates that print the member's headshot. It is what stops the
// render path reaching for object storage on a template that would not use the image,
// and what lets the gallery prompt for an upload before the template is chosen.
type TemplateInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Style string `json:"style"`
	Photo bool   `json:"photo"`
}

// templates is the registry of known templates. A CV's template_id must be one of
// these; the seam to add more is: append an entry here and drop a matching
// templates/<id>.typ file, then regenerate the gallery thumbnails with `make cv-previews`.
var templates = []TemplateInfo{
	{ID: DefaultTemplateID, Label: "Classic", Style: "single-column · serif"},
	{ID: "centered", Label: "Centered", Style: "centered · serif"},
	{ID: "modern-sans", Label: "Modern", Style: "single-column · sans"},
	{ID: "timeline", Label: "Timeline", Style: "single-column · rail · serif"},
	{ID: "compact", Label: "Compact", Style: "single-column · dense · serif"},
	{ID: "mono-tech", Label: "Mono", Style: "single-column · monospace"},
	{ID: "sidebar", Label: "Sidebar", Style: "two-column · serif"},
	{ID: "portrait", Label: "Portrait", Style: "two-column · photo · serif", Photo: true},
	{ID: "headshot", Label: "Headshot", Style: "single-column · photo · sans", Photo: true},
}

// ErrUnknownTemplate is returned when a template_id is not in the registry. The handler
// maps it to a 400 (an unknown template is never rendered).
var ErrUnknownTemplate = errors.New("cv: unknown template")

// Template is a resolved CV template: its id, whether it prints a headshot, and the
// Typst source that renders a Document (which the source reads back as json("data.json")).
type Template struct {
	ID    string
	Photo bool

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
	i := slices.IndexFunc(templates, func(t TemplateInfo) bool { return t.ID == id })
	if i < 0 {
		return Template{}, fmt.Errorf("%w: %q", ErrUnknownTemplate, id)
	}
	src, err := templateFS.ReadFile("templates/" + id + ".typ")
	if err != nil {
		return Template{}, fmt.Errorf("cv: read template %q: %w", id, err)
	}
	return Template{ID: id, Photo: templates[i].Photo, source: src}, nil
}
