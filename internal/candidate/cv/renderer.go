package cv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/tracerlink"
)

// Renderer turns a CV Document into PDF bytes using a resolved template.
//
// photo is the member's normalized headshot, or nil. It is not a Document field: the
// document is client-writable and travels into tailoring prompts, and an image belongs in neither.
type Renderer interface {
	Render(ctx context.Context, doc Document, tmpl Template, photo []byte, hrefs LinkHrefs) ([]byte, error)
}

// defaultRenderTimeout bounds a single compile so a pathological input cannot hang a request.
const defaultRenderTimeout = 15 * time.Second

// TypstRenderer renders via the Typst CLI. The document is passed as a data.json file the
// template reads (never interpolated into argv), and the compile is sandboxed to a temp
// --root with --ignore-system-fonts, so local and prod output are identical.
type TypstRenderer struct {
	bin     string
	timeout time.Duration
}

// NewTypstRenderer builds a renderer over the typst binary at bin. An empty bin yields a
// nil renderer, which disables the feature (the handler returns 501).
func NewTypstRenderer(bin string) *TypstRenderer {
	if bin == "" {
		return nil
	}
	return &TypstRenderer{bin: bin, timeout: defaultRenderTimeout}
}

// Render compiles the template against the document and returns the PDF bytes. photo is
// composed in by the templates that print one; it is ignored by the rest.
//
// hrefs supplies a link target per position, overriding the absolute URL derived from the
// document. A zero value is the ordinary render.
func (r *TypstRenderer) Render(ctx context.Context, doc Document, tmpl Template, photo []byte, hrefs LinkHrefs) ([]byte, error) {
	return r.compile(ctx, doc, tmpl, "pdf", photo, hrefs)
}

// photoFile is the staged headshot's name inside the sandbox. The templates hardcode it,
// because the sandbox has no network and --root blocks every path outside dir: an image
// can only reach Typst as a file staged next to the template.
const photoFile = "photo.jpg"

// compile stages the document, template, and bundled fonts in a sandboxed temp dir and runs
// typst, returning the output in the requested format ("pdf" or "svg"). Shared by Render and
// GeneratePreviews.
func (r *TypstRenderer) compile(ctx context.Context, doc Document, tmpl Template, format string, photo []byte, hrefs LinkHrefs) ([]byte, error) {
	dir, err := os.MkdirTemp("", "cv-render-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// Swap the font registry id for the family name Typst asks for. doc is a copy, so the
	// caller's document keeps the id and the stored document stays free of engine-specific
	// names. An unregistered or unset id leaves the template's own #set text(font:) in charge.
	if family, ok := ResolveFontFamily(doc.Style.FontFamily); ok {
		doc.Style.FontFamily = family
	} else {
		doc.Style.FontFamily = ""
	}

	// The photo is staged only for a template that prints one, so the same condition
	// decides the file and the flag: image() on a missing file is a compile error, and a
	// template that draws the placeholder must never be told a photo exists.
	withPhoto := tmpl.Photo && len(photo) > 0
	if withPhoto {
		if err := os.WriteFile(filepath.Join(dir, photoFile), photo, 0o600); err != nil {
			return nil, err
		}
	}
	data, err := renderPayload(doc, withPhoto, hrefs)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), data, 0o600); err != nil {
		return nil, err
	}
	tmplPath := filepath.Join(dir, "template.typ")
	if err := os.WriteFile(tmplPath, tmpl.source, 0o600); err != nil {
		return nil, err
	}
	// Stage the bundled fonts: without them --ignore-system-fonts silently falls back to a
	// default serif for faces the Typst binary does not embed (e.g. Liberation Sans).
	fontDir := filepath.Join(dir, "fonts")
	if err := os.Mkdir(fontDir, 0o700); err != nil {
		return nil, err
	}
	if err := writeFonts(fontDir); err != nil {
		return nil, err
	}
	outPath := filepath.Join(dir, "out."+format)

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Only fixed flags and temp paths reach argv — user data lives in data.json, so the
	// command line is not an injection surface. --root confines file access to dir.
	cmd := exec.CommandContext(ctx, r.bin, "compile", "--format", format,
		"--root", dir, "--ignore-system-fonts", "--font-path", fontDir, tmplPath, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("cv: typst compile: %w: %s", err, out)
	}
	return os.ReadFile(outPath)
}

// LinkHrefs carries one link target per link, aligned by index with Header.Links and Projects.
// An empty entry means "use what the payload derived".
//
// It rides on the render payload, never on the Document: a render-time target is not something
// the candidate edits or the tailoring agent patches.
type LinkHrefs struct {
	Header   []string `json:"header,omitempty"`
	Projects []string `json:"projects,omitempty"`
}

// renderPayload marshals what the template reads as json("data.json"): the document's fields
// plus has_photo and the resolved link targets. The extras live only on this wrapper, so a
// client cannot set them and they are never persisted.
//
// Experience/Education/Certifications are re-declared with their perioddate fields formatted
// to plain display strings (the shallower same-named field wins in encoding/json), so no .typ
// file sees the structured value.
func renderPayload(doc Document, hasPhoto bool, hrefs LinkHrefs) ([]byte, error) {
	return json.Marshal(struct {
		Document
		Experience     []renderExperienceItem `json:"experience,omitempty"`
		Education      []renderEducationItem  `json:"education,omitempty"`
		Certifications []renderCertification  `json:"certifications,omitempty"`
		HasPhoto       bool                   `json:"has_photo"`
		LinkHrefs      LinkHrefs              `json:"link_hrefs"`
	}{
		Document:       doc,
		Experience:     formatExperienceForRender(doc.Experience),
		Education:      formatEducationForRender(doc.Education),
		Certifications: formatCertificationsForRender(doc.Certifications),
		HasPhoto:       hasPhoto,
		LinkHrefs:      resolveHrefs(doc, hrefs),
	})
}

// renderExperienceItem, renderEducationItem, and renderCertification are the render-time
// shape of their Document counterparts, with Start/End/Year formatted to plain strings.
type renderExperienceItem struct {
	ExperienceItem
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type renderEducationItem struct {
	EducationItem
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type renderCertification struct {
	Certification
	Year string `json:"year,omitempty"`
}

func formatExperienceForRender(items []ExperienceItem) []renderExperienceItem {
	out := make([]renderExperienceItem, len(items))
	for i, e := range items {
		out[i] = renderExperienceItem{
			ExperienceItem: e,
			Start:          e.Start.Format(),
			End:            perioddate.FormatEnd(e.End, e.Current),
		}
	}
	return out
}

func formatEducationForRender(items []EducationItem) []renderEducationItem {
	out := make([]renderEducationItem, len(items))
	for i, e := range items {
		out[i] = renderEducationItem{EducationItem: e, Start: e.Start.Format(), End: e.End.Format()}
	}
	return out
}

func formatCertificationsForRender(items []Certification) []renderCertification {
	out := make([]renderCertification, len(items))
	for i, c := range items {
		out[i] = renderCertification{Certification: c, Year: c.Year.Format()}
	}
	return out
}

// resolveHrefs fills in a target for every link position: the caller's, when supplied, else
// the document's own link made absolute.
//
// The normalisation applies to every render: CVs store links as written ("github.com/ada"),
// and a PDF annotation carrying that verbatim is a relative URI no reader can follow.
func resolveHrefs(doc Document, supplied LinkHrefs) LinkHrefs {
	projectLinks := make([]string, len(doc.Projects))
	for i, p := range doc.Projects {
		projectLinks[i] = p.Link
	}
	out := LinkHrefs{
		Header:   make([]string, len(doc.Header.Links)),
		Projects: make([]string, len(projectLinks)),
	}
	for _, t := range tracerlink.Targets(nil, doc.Header.Links, projectLinks) {
		switch t.Section {
		case tracerlink.SectionHeaderLinks:
			out.Header[t.Index] = t.URL
		case tracerlink.SectionProjectLink:
			out.Projects[t.Index] = t.URL
		}
	}
	overlay(out.Header, supplied.Header)
	overlay(out.Projects, supplied.Projects)
	return out
}

// overlay writes src's non-empty entries over dst, by position. A shorter src is not an error.
func overlay(dst, src []string) {
	for i := 0; i < len(src) && i < len(dst); i++ {
		if src[i] != "" {
			dst[i] = src[i]
		}
	}
}
