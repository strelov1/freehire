# Design

## Context

`internal/resume.ExtractPDFText(data []byte) (string, error)` is the single PDF→text
primitive, called from two places:

- `internal/handler/resume.go` `readResumeUpload` — the in-request upload path.
- `internal/resume` `extractText` → `Store.Text` — deriving text from a stored résumé.

It wraps `github.com/ledongthuc/pdf`'s `GetPlainText()`, which returns **empty text, no
error** for CID / `Identity-H` fonts (verified against a real Canva export: `len=0`,
`err=nil`). `poppler`'s `pdftotext` extracts the same file's 5 KB text layer cleanly.

## Goals / Non-goals

- **Goal:** every text-bearing PDF (incl. Canva/CID exports) extracts; only image-only
  PDFs yield empty text.
- **Goal:** minimal blast radius — no signature churn across ~22 `resume.New` callsites.
- **Non-goal:** OCR of genuine scans (still rejected with the existing message).
- **Non-goal:** a general Renderer-style abstraction — there is one tool and one call.

## Decisions

### Extractor: shell out to `pdftotext`, keep the signature

`ExtractPDFText`'s signature is unchanged, so no call site (handler, Store, or tests)
moves. Internally it writes the bytes to a temp file (mirroring `cv.TypstRenderer`'s
temp-dir sandbox — no user data on argv) and runs `pdftotext -nopgbrk <in> -`, capturing
stdout.

Error mapping preserves the current handler contract:
- **non-zero exit** (corrupt/undecodable PDF) → error → handler renders `400 invalid PDF`.
- **exit 0, empty stdout** (image-only PDF, no text layer) → `("", nil)` → handler's
  `TrimSpace(text) == ""` check renders `400 errResumeNoText` ("scan or image").

The `ledongthuc` `defer/recover` (guarded against that library's panics) is removed; a CLI
subprocess cannot panic the request.

### Tool resolution: package-level var, not threaded config

The binary is a package-level `var pdftotextBin`, initialized from `PDFTOTEXT_BIN`
(default `pdftotext`). This avoids adding a field to `resume.New` (which would ripple to
~22 test callsites) and avoids threading a new `Config` field through the handler for a
leaf infra concern. Unlike `TypstBin`, PDF extraction has no "disabled/501" feature flag
to gate, so config centralization buys nothing here. The `var` is overridable in tests.

*(Considered and rejected: adding `bin` to `resume.New` — too much test churn; a new
`config.PdftotextBin` threaded to the API — ceremony with no feature-flag payoff.)*

### Dependency & licensing

- `github.com/ledongthuc/pdf` leaves the résumé path but stays in `go.mod` as a test-only
  dependency of `internal/cv` (its render test parses typst PDF output — standard fonts,
  where the library works fine). No `go mod tidy` removal.
- `pdftotext` (poppler, GPL) is invoked as a **separate process**, not linked — the MIT
  codebase is unaffected (mere aggregation, like calling `git`).
- `unipdf`/MuPDF were rejected: AGPL, incompatible with the project's MIT license.

### Runtime image

Switch the runtime stage from `gcr.io/distroless/static-debian12:nonroot` to
`debian:stable-slim` with `poppler-utils` + `ca-certificates`, and a non-root user. The
Go binaries remain `CGO_ENABLED=0` static, so they run unchanged on debian. The `typst`
stage/binary and `TYPST_BIN` are preserved.

## Risks / trade-offs

- **Larger image / attack surface** vs distroless-static — accepted for reliable
  extraction; the hosts are bare-metal (image size non-critical).
- **Dev/CI without poppler:** the PDF path errors; the paste-text path is unaffected.
  PDF-extraction tests skip when `pdftotext` is not on `PATH` (mirrors the typst test's
  `exec.LookPath` skip), keeping `go test ./...` green.

## Migration

None. Same endpoints, same request/response shapes; a previously-rejected class of PDFs
now succeeds.
