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

// templateIDs is the registry of known templates. A CV's template_id must be one of
// these; the seam to add more is simply extending this list and dropping a matching
// templates/<id>.typ file.
var templateIDs = []string{DefaultTemplateID}

// ErrUnknownTemplate is returned when a template_id is not in the registry. The handler
// maps it to a 400 (an unknown template is never rendered).
var ErrUnknownTemplate = errors.New("cv: unknown template")

// Template is a resolved CV template: its id and the Typst source that renders a Document
// (which the source reads back as json("data.json")).
type Template struct {
	ID     string
	source []byte
}

// TemplateIDs returns the registered template ids (for request validation and the UI).
func TemplateIDs() []string { return slices.Clone(templateIDs) }

// ResolveTemplate returns the template for id, defaulting an empty id to DefaultTemplateID
// and rejecting any id not in the registry with ErrUnknownTemplate.
func ResolveTemplate(id string) (Template, error) {
	if id == "" {
		id = DefaultTemplateID
	}
	if !slices.Contains(templateIDs, id) {
		return Template{}, fmt.Errorf("%w: %q", ErrUnknownTemplate, id)
	}
	src, err := templateFS.ReadFile("templates/" + id + ".typ")
	if err != nil {
		return Template{}, fmt.Errorf("cv: read template %q: %w", id, err)
	}
	return Template{ID: id, source: src}, nil
}
