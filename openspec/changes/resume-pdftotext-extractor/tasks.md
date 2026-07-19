# Tasks

## 1. Replace the PDF extractor with `pdftotext`

- [x] 1.1 Add a real Canva/CID `Identity-H` PDF as a test fixture under
  `internal/resume/testdata/` and a failing test asserting `ExtractPDFText` returns
  non-empty text for it (RED — passes only once poppler-backed). Skip the test when
  `pdftotext` is not on `PATH` (mirror `internal/cv/renderer_test.go`).
- [x] 1.2 Rewrite `internal/resume.ExtractPDFText` to shell out to `pdftotext` (temp file
  in → stdout), add the package-level `pdftotextBin` var (`PDFTOTEXT_BIN`, default
  `pdftotext`), and remove the `ledongthuc/pdf` import + `defer/recover`. Keep the
  signature and the error/empty-text contract (non-zero exit → error; exit 0 + empty →
  `("", nil)`).
- [x] 1.3 Preserve `extractText`'s existing plain-text vs `%PDF` sniffing test
  (`resume_test.go`); ensure the corrupt-PDF case still returns an error via the new path.
- [x] 1.4 `go build ./... && go vet ./...`. (`ledongthuc/pdf` stays in `go.mod` — still a
  test-only dep of `internal/cv`; do not remove it.)

## 2. Ship `pdftotext` in the runtime image

- [x] 2.1 Change the `Dockerfile` runtime stage to `debian:stable-slim`, install
  `poppler-utils` + `ca-certificates`, add a non-root user, copy the Go binaries and the
  `typst` binary, keep `TYPST_BIN`, and set the entrypoint. Verify `pdftotext -v` in the
  build.

## 3. Verify

- [x] 3.1 Run `go test ./...`; confirm the fixture test passes locally (poppler present).
- [x] 3.2 Build the image and confirm `pdftotext` resolves and extracts the fixture end to
  end (`pdftotext -q -nopgbrk` inside the built image returns 5068 chars from the fixture,
  vs 0 from the old library).
