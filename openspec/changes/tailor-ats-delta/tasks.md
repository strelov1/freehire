## 1. The comparison (pure, no I/O)

- [x] 1.1 Add `internal/atscheck/delta.go`: the wire type (overall base/tailored/delta, one entry per
      category with the same ids and labels the report uses, a regression flag, and the worst-drop
      category id) plus `Compare(base, tailored Report) Delta`. Table tests cover: every category's
      delta equals tailored minus base; identical reports yield all-zero deltas and no regression; a
      lower tailored overall sets the regression flag and names the most negative category; equal
      overalls are not a regression; a tie on the worst drop resolves deterministically.
- [x] 1.2 Add `delta.go` to the atscheck entry in `cmd/gen-contracts/main.go` `IncludeFiles` and
      regenerate, so `web/src/lib/generated/contracts.ts` carries the new type. Verify the type is
      present in the generated file — codegen omission here is silent.

## 2. Scoring one CV from its rendered artifact

- [x] 2.1 Add `internal/handler/cv_ats_delta.go` with the single-side scorer: render a `cv.Document`
      with a given template and margins through `cvHandlers.cvRenderer`, extract the text layer with
      `resume.ExtractPDFText`, parse the CV's own skills from that text with
      `skilltag.Parse(..., skilltag.WithResumeAcronyms())`, and score with `atscheck.Score` against a
      supplied keyword baseline. Tests use a fake `cv.Renderer` returning fixture PDF bytes and cover:
      the score is computed from the extracted text; a render error and an extraction error are both
      reported as unavailable-with-reason rather than as failures.
- [x] 2.2 Assert the anti-regression the design turns on: a document field the active template does
      not render contributes nothing to the score (score the same document under two templates that
      differ in what they render).

## 3. The delta endpoint

- [x] 3.1 Register `GET /me/cvs/:id/ats-delta` with `mw.cookie` in `internal/handler/cv.go`. Test that
      a full-scope Bearer key is refused — the cookie-only route is what keeps the tailoring agent
      from reading the score, so it needs a test that fails if someone widens it to `mw.key`.
- [x] 3.2 Implement the handler: resolve the tailored CV owner-scoped (another account's id is the
      same not-found as a missing id), resolve its base CV and bound vacancy, refuse a CV that is not
      a tailored copy with a conflict naming the reason, read the vacancy's canonical `Skills` through
      `jobReader.GetJob`, score both sides with the **tailored copy's** template and margins, and
      return `Compare`'s result. Tests cover the owner-scoping, the not-a-tailored-copy conflict, and
      that both sides were rendered with the tailored copy's template even when the base CV's stored
      template differs.
- [x] 3.3 Test the degrade path end to end: with `cvRenderer` nil the response is a success status
      carrying `available: false` and a reason — not the 501 `RenderCVPDF` returns.
- [x] 3.4 Test that scoring leaves the base CV untouched: its stored document, template and margins
      are identical after a delta read.
- [x] 3.5 Add a renderer-backed test that skips when typst or pdftotext is absent (the skip pattern in
      `internal/cv/renderer_test.go`), asserting a real render of two documents produces a coherent
      delta — this is the only test that proves the real toolchain path works.

## 4. The workspace surface

- [x] 4.1 Add the client read for the new endpoint in the web app and render the delta in the tailoring
      workspace: overall change, per-category breakdown, and the regression warning naming the
      category that fell. An unavailable delta renders as an absence, with no error state.
- [x] 4.2 Request the delta when the workspace opens and again when an autopilot run completes, so a
      regression reaches the candidate without them asking for a check.
- [x] 4.3 Verify visually at desktop and mobile widths (the workspace collapses to a tabbed view on
      mobile — the delta must have a home there too, not overflow the column).

## 5. Close out

- [x] 5.1 `go build ./... && go vet ./... && go test ./...`; web lint and build.
- [x] 5.2 Document the delta in the CV/tailoring reference material that names the ATS report today,
      including the one trade-off a reader would otherwise trip on: the baseline is the base CV as it
      stands now, not a snapshot from when the copy was made.
