## Why

A CV's presentation is currently decided almost entirely by the four built-in templates. The only
knob a candidate can turn is page margins; typeface, type size, and line spacing are hard-coded in
each Typst template. That leaves the two adjustments people actually reach for when a CV runs three
lines onto a second page — shrink the type a little, tighten the leading — impossible without
switching to a different template and losing the layout they picked.

The Margins control that does exist is also visibly broken in the workspace: it lays out with a
`sm:` viewport breakpoint inside a resizable 340–720px panel, so the four steppers overflow their
column and the "+" buttons are clipped off the edge.

## What Changes

- A CV `Document` gains a `style` block carrying three optional values: font family, base font size
  (pt), and line height (leading, em). Every value is optional and an unset value means "use the
  template's own", so existing CVs render byte-identically and need no migration.
- A font registry alongside the template registry, exposed over a new read endpoint so clients
  discover the available typefaces instead of hard-coding them. Five entries: the template's own
  default plus four faces chosen for metric compatibility with what recruiters and résumé parsers
  expect — Libertinus Serif, Liberation Serif (Times New Roman metrics), Liberation Sans (Arial
  metrics), and Carlito (Calibri metrics).
- Liberation Serif and Carlito are added to the bundled fonts (SIL OFL, with licences) so the
  sandboxed Typst run under `--ignore-system-fonts` can resolve them.
- All four Typst templates read the style block, and their internal type sizes become em-relative so
  raising the base size scales the whole hierarchy rather than flattening it against a fixed 12pt
  name.
- The live HTML preview applies the same three values — including in its hidden measurement layer,
  without which pagination would be computed in one typeface and drawn in another.
- The workspace Settings panel is rebuilt as label→control rows: a Typography block (font, size,
  line height) and a rebuilt Margins block whose default view is two linked axes with per-side
  steppers behind a disclosure, plus a reset-to-template-default action. This fixes the overflowing
  stepper grid.
- The ATS delta carries the tailored copy's style onto the base CV it renders for comparison, so a
  typography change cannot move a score that is meant to measure content.
- The CV patch vocabulary is deliberately NOT extended: the tailoring agent edits content, the
  candidate edits presentation.

## Capabilities

### New Capabilities

None. This extends two existing capabilities.

### Modified Capabilities

- `cv-builder`: the CV `Document` carries typography (font family, size, line height) in addition to
  margins; the sanitizer clamps them and preserves "unset" as inheritance from the template; the
  registered fonts are discoverable over the API; rendering honours the style block.
- `tailor-workspace`: the editor's settings surface covers typography as well as margins, and its
  layout is specified as label→control rows that hold at the panel's full resize range.
- `tailor-ats-delta`: the base-versus-tailored comparison holds typography constant along with
  template and margins.

## Impact

- `internal/cv`: `cv.go` (the `Style` type and its sanitizer), `fonts.go` (font registry + two new
  bundled families), `renderer.go` (resolve the font id to a Typst family name in the render-time
  copy), `templates/*.typ` (all four).
- `internal/handler`: `cv.go` (a fonts list endpoint), `cv_ats_delta.go` (copy style onto the base).
- `web`: `CvHtmlPreview.svelte`, `MarginSettings.svelte` (rewritten), a new `StyleSettings.svelte`
  and a shared settings-row component, `geometry.ts` (leading conversion), the tailor route's
  Settings tab, and `api.ts`.
- Generated: `internal/db` is untouched (no schema change); `web/src/lib/generated/contracts.ts` is
  regenerated from `cv.go`.
- Repository size: four new TTF files (~1.5 MB) embedded in the server binary and staged into each
  render sandbox.
