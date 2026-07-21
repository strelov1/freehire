package cv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Renderer turns a CV Document into PDF bytes using a resolved template. It is an
// interface so the schema, storage, and handlers do not depend on the concrete engine;
// a Chrome/LaTeX renderer could replace TypstRenderer without touching them.
type Renderer interface {
	Render(ctx context.Context, doc Document, tmpl Template) ([]byte, error)
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

// Render compiles the template against the document and returns the PDF bytes.
func (r *TypstRenderer) Render(ctx context.Context, doc Document, tmpl Template) ([]byte, error) {
	return r.compile(ctx, doc, tmpl, "pdf")
}

// compile stages the document, template, and bundled fonts in a sandboxed temp dir and runs
// typst, returning the output in the requested format ("pdf" for live rendering, "svg" for
// preview generation). Shared by Render and GeneratePreviews so both use the exact same font
// staging and sandbox flags.
func (r *TypstRenderer) compile(ctx context.Context, doc Document, tmpl Template, format string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "cv-render-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	data, err := json.Marshal(doc)
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
