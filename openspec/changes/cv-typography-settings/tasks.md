## 1. Document model and sanitizer

- [x] 1.1 Add the `Style` type to `internal/cv/cv.go` (font family id, font size in pt, line height in
      em) and a `Style` field on `Document`, with a doc comment stating the zero-means-inherit rule
      and why it differs from `Margins`
- [x] 1.2 Add the style bounds constants (font size 8.5–12.0 pt rounded to 0.5, line height 0.3–0.9 em)
      and extend `Sanitize` to clamp only non-zero values
- [x] 1.3 `TestSanitizeStyle`: out-of-range size clamps both ways, 10.3 rounds to 10.5, out-of-range
      line height clamps both ways, and — named separately — a zero size and a zero line height stay
      zero rather than clamping up to the lower bound

## 2. Font registry and bundled faces

- [x] 2.1 Add `FontInfo` and the registry to `internal/cv/fonts.go` (id, label, note, CSS stack public;
      Typst family name private), plus `Fonts()`, `FontIDs()`, and a resolver
- [x] 2.2 Wire the registry into `Sanitize`: an unregistered font family id is reset to unset
- [x] 2.3 Commit `Tinos-Regular/Bold.ttf` and `Carlito-Regular/Bold.ttf` to `internal/cv/fonts/`
      with their SIL OFL licence files
- [x] 2.4 Test that every registry entry either names a Typst built-in face or has its TTF present in
      the embedded FS, so a registry entry can never be added without its font

## 3. Rendering

- [ ] 3.1 Resolve the font id to its Typst family name on the renderer's own copy of the `Document`
      before marshalling `data.json`, leaving the stored document untouched
- [ ] 3.2 Add the style preamble to all four `templates/*.typ`: read `style` with a fallback to each
      template's current hard-coded font, 9.5pt size, and its own leading
- [ ] 3.3 Convert every internal absolute `size:` in all four templates to an em multiple of the base
      (name `1.25em`, section labels `1em`, and any others)
- [ ] 3.4 Renderer test: a document with all three style values set compiles under every registered
      template, and its text layer still extracts
- [ ] 3.5 Renderer test: rendering does not mutate the caller's document — the font family it holds
      afterwards is still the registry id
- [ ] 3.6 Run `make cv-previews` and commit the regenerated thumbnails if the em conversion moved them

## 4. HTTP surface

- [ ] 4.1 Add the fonts list handler and route mirroring `GET /cv/templates`, under the same auth
- [ ] 4.2 Handler test: the endpoint returns every registered font with id, label, note, and CSS stack
- [ ] 4.3 Regenerate `web/src/lib/generated/contracts.ts` and add `listCvFonts` to `web/src/lib/api.ts`

## 5. ATS delta

- [ ] 5.1 Copy the tailored copy's `Style` onto the in-memory base document in
      `internal/handler/cv_ats_delta.go`, alongside template and margins
- [ ] 5.2 Handler test: a base CV whose stored typography differs from the tailored copy's is scored
      with the tailored copy's typography, and the stored base is unchanged afterwards

## 6. Live HTML preview

- [ ] 6.1 Add the Typst-leading-to-CSS-line-height conversion to `web/src/lib/tailor/geometry.ts`
      with the calibration constant documented, and unit-test it (including that the default
      reproduces today's `leading-snug`)
- [ ] 6.2 Derive one style object in `CvHtmlPreview.svelte` and apply it to both the visible sheets
      and the hidden measurement layer, replacing the `font-serif`/`font-sans`/`text-[13px]`/
      `leading-snug` classes
- [ ] 6.3 Verify visually with headless Chrome that raising the font size re-paginates the preview and
      that the preview and the downloaded PDF break pages at the same place

## 7. Settings panel

- [ ] 7.1 Add `SettingRow.svelte` — the shared label-left, control-right row
- [ ] 7.2 Add `StyleSettings.svelte`: font picker fed by `listCvFonts`, font-size stepper, line-height
      preset picker, each with a "template default" choice, plus the reset-all action
- [ ] 7.3 Extract the stepper into a shared component so margins and font size share one control
- [ ] 7.4 Rewrite `MarginSettings.svelte` onto rows: linked side and top-and-bottom steppers by
      default, four per-side steppers behind a disclosure, with the linked view reflecting a
      non-uniform axis
- [ ] 7.5 Unit-test the linked-axis stepping and the non-uniform detection as pure functions in
      `geometry.ts`
- [ ] 7.6 Wire both blocks into the Settings tab of `web/src/routes/tailor/[slug]/+page.svelte`
- [ ] 7.7 Verify visually at 340px and 720px panel widths that nothing clips or overflows

## 8. Documentation

- [ ] 8.1 Update `internal/cv/AGENTS.md`: the font registry and how to add a face, the
      zero-means-inherit rule, and the em-relative size requirement for new templates
