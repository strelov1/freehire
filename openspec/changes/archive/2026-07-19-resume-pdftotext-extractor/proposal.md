## Why

Résumé PDF text is extracted with the pure-Go `github.com/ledongthuc/pdf` library,
which cannot decode CID fonts with `Identity-H` encoding via their `ToUnicode` CMap —
it returns **empty text with no error**. That is exactly how Canva (one of the most
common résumé builders) exports PDFs, so every Canva résumé is silently rejected with a
misleading "it looks like a scan or image" message even though the file has a perfectly
good, selectable text layer. `poppler`'s `pdftotext` extracts these files cleanly.

## What Changes

- **BREAKING (internal):** Rewrite `internal/resume.ExtractPDFText` to shell out to the
  poppler `pdftotext` CLI instead of `github.com/ledongthuc/pdf`. (The library remains a
  test-only dependency of `internal/cv`, whose render test parses typst output — it is no
  longer on any résumé-extraction path.)
- Add a `PDFTOTEXT_BIN` env knob (default `pdftotext`) resolved in the `resume` package
  (no `config` threading — PDF extraction has no feature flag to gate, unlike `TYPST_BIN`).
- Switch the runtime Docker stage from `gcr.io/distroless/static-debian12:nonroot` to
  `debian:*-slim` with `poppler-utils` (+ `ca-certificates`, non-root user) installed, so
  `pdftotext` is present in production. The Go binaries stay `CGO_ENABLED=0` static.
- Canva / CID-font / any text-layer PDF now extracts; the "scan or image" rejection is
  reserved for genuinely image-only PDFs (still empty text after `pdftotext`).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `resume-skill-extraction`: the PDF-extraction requirement now succeeds for text-based
  PDFs regardless of font encoding (including CID/`Identity-H` embedded-font exports such
  as Canva); the empty-text rejection applies only to image-only PDFs.

## Impact

- Code: `internal/resume/resume.go` (extractor), `internal/config/config.go`
  (`PDFTOTEXT_BIN`), `cmd/server/main.go` wiring if the binary path is threaded through.
- Dependencies: `github.com/ledongthuc/pdf` leaves the résumé path (still used by
  `internal/cv`'s render test); add a runtime dependency on the `pdftotext` binary
  (poppler-utils), invoked as a separate process (GPL binary, MIT code unaffected — no
  linking).
- Infra: `Dockerfile` runtime base image change (distroless/static → debian-slim); larger
  image and attack surface, accepted for reliable extraction. Local dev / CI without
  poppler degrade gracefully (PDF path errors; the paste-text path is unaffected).
- Behavior: résumé upload (`POST /api/v1/me/resume/extract`) and stored-résumé text
  derivation both go through the new extractor.
