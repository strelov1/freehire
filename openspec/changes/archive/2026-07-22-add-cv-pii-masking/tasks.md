## 1. `internal/pii` — detectors and redactor

- [x] 1.1 Add span types (`Span{Start,End,Kind}`, `Contacts`) and the regex detectors: email, phone (with `YYYY-YYYY` date-range guard), URL, `@handle`; table tests over both spike-CV shapes
- [x] 1.2 Add the model `Detector` interface + its HTTP client (POST text → spans); tests against a faked HTTP server, incl. transport/parse failure
- [x] 1.3 Implement `Build(ctx, text, known Contacts, d Detector) (*Redactor, error)`: union regex ∪ model spans, numbered reversible placeholders, word-boundary replacement, full/known-value priority; tests for `Redact`, `Restore`, `Restore(Redact(x))` round-trip, distinct-value numbering, over-redaction guard
- [x] 1.4 Fail-closed: `Build` returns an error (no partial redactor) when the detector is unconfigured or its call fails; test

## 2. Config

- [x] 2.1 Add `PII_FILTER_URL` to `internal/config` (server + resume/embed worker) and construct the `pii.Detector`; test that empty URL yields an unconfigured (fail-closed) detector

## 3. `matchanalysis` integration

- [x] 3.1 Build a `Redactor` at the top of `AnalyzeStream` from `in.CVText` + `in.StructuredResume`; return no-analysis (fail-closed) when `Build` errors
- [x] 3.2 Run `Redact` in `writeCV` and `writeStructured` so every stage prompt carries placeholders
- [x] 3.3 Wrap `emit` to `Restore` a copy of each outbound event, and `Restore` the returned/cached analysis; keep internal `reqs`/`verdict` masked (no re-leak into Stage 2/3)
- [x] 3.4 Tests: assert known PII never appears in the Stage 1/2/3 prompt strings, that output is restored, and fail-closed on a failing detector

## 4. `resumeextract` integration

- [x] 4.1 Build a `Redactor`; fill `Structured` contact fields (`FullName/Email/Phone/Links`) from detected spans; send only the redacted CV to the LLM; update the prompt to state contacts are provided separately; fail-closed when `Build` errors
- [x] 4.2 Tests: contacts filled from detection, no PII in the LLM input, semantic fields still parsed, fail-closed leaves upload/embedding untouched

## 5. Privacy-filter span-detection service

- [x] 5.1 Build the detection HTTP service serving `openai/privacy-filter` (ONNX q4, onnxruntime, Viterbi span stitching): `POST /detect` → `[{start,end,kind}]`; verify it recovers the hidden surname and does not over-redact on the two spike CVs
- [x] 5.2 Deploy wiring in `freehire-ops`: ONNX q4 weights + service on the litellm box, systemd/compose unit, health-check, egress allowlist; set `PII_FILTER_URL`

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` green; end-to-end masking check against the live `/detect` endpoint on the two spike CVs (no PII in the outbound prompt, output restored)
