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

// Renderer turns a CV Document into PDF bytes using a resolved template. It is an
// interface so the schema, storage, and handlers do not depend on the concrete engine;
// a Chrome/LaTeX renderer could replace TypstRenderer without touching them.
//
// photo is the member's normalized headshot, or nil when they have none (or the template
// does not print one). It is a separate argument rather than a Document field because the
// image is a profile asset: the document is client-writable and travels into tailoring
// prompts, and an image belongs in neither.
type Renderer interface {
	Render(ctx context.Context, doc Document, tmpl Template, photo []byte, hrefs LinkHrefs) ([]byte, error)
}

// defaultRenderTimeout bounds a single compile so a pathological input can never hang a
// request; Typst renders a CV in ~50–150 ms, so this is generous headroom.
const defaultRenderTimeout = 15 * time.Second

// TypstRenderer renders via the Typst CLI. The document is passed as a data.json file the
// template reads (never interpolated into argv), and the compile is sandboxed to a temp
// --root with --ignore-system-fonts so it uses only the font embedded in the binary —
// making local and prod output identical.
type TypstRenderer struct {
	bin     string
	timeout time.Duration
}

// NewTypstRenderer builds a renderer over the typst binary at bin. An empty bin yields a
// nil renderer, so the feature is disabled (the handler returns 501) — the same nil-safe
// gating as blobstore/meili/llm.
func NewTypstRenderer(bin string) *TypstRenderer {
	if bin == "" {
		return nil
	}
	return &TypstRenderer{bin: bin, timeout: defaultRenderTimeout}
}

// Render compiles the template against the document and returns the PDF bytes. photo is
// composed in by the templates that print one; it is ignored by the rest.
//
// hrefs supplies a link target per position, overriding the absolute URL the payload derives from
// the document itself. A zero value is the ordinary render: the links still resolve absolutely,
// they just point where the candidate wrote them.
func (r *TypstRenderer) Render(ctx context.Context, doc Document, tmpl Template, photo []byte, hrefs LinkHrefs) ([]byte, error) {
	return r.compile(ctx, doc, tmpl, "pdf", photo, hrefs)
}

// photoFile is the staged headshot's name inside the sandbox. The templates hardcode it,
// because the sandbox has no network and --root blocks every path outside dir: an image
// can only reach Typst as a file staged next to the template.
const photoFile = "photo.jpg"

// compile stages the document, template, and bundled fonts in a sandboxed temp dir and runs
// typst, returning the output in the requested format ("pdf" for live rendering, "svg" for
// preview generation). Shared by Render and GeneratePreviews so both use the exact same font
// staging and sandbox flags.
func (r *TypstRenderer) compile(ctx context.Context, doc Document, tmpl Template, format string, photo []byte, hrefs LinkHrefs) ([]byte, error) {
	dir, err := os.MkdirTemp("", "cv-render-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// Swap the font registry id for the family name Typst asks for. doc is a value, so this
	// is a copy and the caller's document — the one the handler is about to persist — keeps
	// the id. Same trick as Document.withoutContacts; it is what lets the stored document stay
	// free of engine-specific names. An unregistered or unset id resolves to nothing, leaving
	// the template's own #set text(font:) to decide.
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
	// Stage the bundled fonts so --font-path exposes faces (e.g. Liberation Sans) that the
	// Typst binary does not embed — without them --ignore-system-fonts would silently fall
	// back to a default serif for the sans templates.
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

// LinkHrefs carries one link target per link in the document, aligned by index with
// Header.Links and with Projects. An empty entry means "use what the payload derived".
//
// It rides on the render payload and never on the Document: cvs.data is what the candidate edits
// and the tailoring agent patches, and a link target chosen at render time is neither.
type LinkHrefs struct {
	Header   []string `json:"header,omitempty"`
	Projects []string `json:"projects,omitempty"`
}

// renderPayload marshals what the template reads back as json("data.json"): the document's
// own fields plus has_photo and the resolved link targets. The embedding inlines Document's
// fields, so the templates keep reading cv.header — and because the extras live only on this
// render-time wrapper, they are neither settable by a client nor persisted with the CV.
//
// Experience/Education/Certifications are re-declared here with their perioddate.PeriodDate
// fields formatted back to the plain display strings every template's daterange-style
// helper has always expected (Go's json package resolves the same-named, shallower
// field in favor of the one promoted from the embedded Document) — no .typ file needs
// to know the stored value is now structured.
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

// resolveHrefs fills in a target for every link position: the caller's, when it supplied one, and
// otherwise the document's own link made absolute.
//
// The normalisation is not part of tracing and applies to every render. CVs store links as a
// candidate writes them on paper — "github.com/ada" — and a PDF annotation carrying that verbatim
// is a relative URI that no reader can follow. Every template had that defect long before this
// feature; the payload is the one place to fix it, rather than six copies of Typst string handling.
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

// overlay writes the caller's non-empty entries over the derived ones, position by position. A
// supplied slice shorter than the document's is not an error: it simply names fewer links.
func overlay(dst, src []string) {
	for i := 0; i < len(src) && i < len(dst); i++ {
		if src[i] != "" {
			dst[i] = src[i]
		}
	}
}
