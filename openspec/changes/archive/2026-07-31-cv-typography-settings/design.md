## Context

A CV's presentation lives in two places today. `Document.Margins` is data — four inches-per-side
values that travel with the document, get clamped by `Sanitize`, and are read by both renderers.
Everything else about how a CV looks is code: each `templates/<id>.typ` opens with a hard-coded
`#set text(font: …, size: 9.5pt)` and `#set par(leading: 0.5em)`, and `CvHtmlPreview.svelte` mirrors
that in Tailwind classes (`font-serif`, `text-[13px]`, `leading-snug`).

Three constraints shape everything below.

**Every style knob costs three implementations.** The Typst templates deliberately share no code —
`renderer.go` stages only `template.typ`, `data.json`, and the font directory into the sandbox, so a
Typst `#import` of a shared module would not resolve, and helpers like `s`/`arr`/`mg` are duplicated
in all four files. The HTML preview is a hand-written mirror of the same layout. A value has to be
read in five places (four templates plus the preview) and clamped in one.

**Fonts cannot come from the host.** `renderer.go:88` runs Typst with `--ignore-system-fonts` and a
`--font-path` pointing at a directory `writeFonts` materializes from an `embed.FS`. A typeface is
available only if its TTF is committed to `internal/cv/fonts/`.

**The preview paginates by measurement.** `CvHtmlPreview.svelte` renders every block into a hidden
off-screen layer, reads `offsetTop`/`offsetHeight` with a `ResizeObserver`, and feeds the heights to
`paginateBlocks`. The hidden layer carries its own copy of the type classes. Type applied to one
layer and not the other produces a preview that paginates wrongly and silently.

## Goals / Non-Goals

**Goals:**

- Font family, base type size, and line height become per-CV data, adjustable in the workspace, with
  the preview and the PDF agreeing.
- Existing CVs render exactly as they do now, with no migration and no backfill.
- A template remains a coherent design choice: what the candidate has not overridden still follows
  the template.
- The Settings panel stops overflowing and gains a layout that holds across the splitter's full
  340–720px range.

**Non-Goals:**

- Separate heading and body typefaces, or curated font pairings. One family for the document; the
  hierarchy comes from weight and size, as it does now.
- Accent colours, section reordering, section show/hide. Those are a different change.
- Letting the tailoring agent change presentation. `PatchOps` stays content-only.
- Per-template typography defaults exposed as data. Each template keeps its defaults in its own
  source; "unset" simply means the template decides.

## Decisions

### Zero means "inherit from the template", not "the default value"

`Margins` resolves an unset side to a concrete 0.5″ in `sanitized()`. `Style` deliberately does the
opposite: `Sanitize` clamps only non-zero values and leaves zero alone, and each renderer falls back
to its own hard-coded default when it reads a zero.

Two things follow, and both are the point. Every CV in the database has no `style` key, so every CV
renders byte-identically after this change — there is nothing to migrate and no visual regression to
chase. And switching from Classic to Modern still moves the leading from 0.5em to 0.55em, because
that value was never copied into the document.

*Alternative considered:* resolving defaults in `Sanitize`, as margins do. Rejected because the first
save after this ships would freeze the current template's typography into the document, quietly
turning every template switch into a partial one.

*Consequence to watch:* `Sanitize` must not clamp zero up to the lower bound. A "clamp to
[8.5, 12.0]" written without the zero check would rewrite every CV in the database to 8.5pt on its
next save. This gets an explicit test.

### The document stores a registry id; the renderer resolves it

`Style.FontFamily` holds `"liberation-serif"`, not `"Liberation Serif"`. The Typst family name is a
rendering-engine detail, and the CSS stack is a browser detail; neither belongs in a persisted
user document.

`renderer.go` already marshals the `Document` straight into `data.json` (`renderer.go:59`), and
`Document` is a value type — `withoutContacts` in `cv.go:113` already exploits exactly that to hand
out a stripped copy without touching the stored one. The renderer takes the same route: overwrite
`doc.Style.FontFamily` with the resolved Typst family name on its own copy, then marshal. The
templates read a plain family string and need no lookup table.

*Alternative considered:* a `map` from id to family name duplicated in each `.typ`. Rejected — four
copies of a table that grows with every font added.

### The font registry mirrors the template registry, endpoint included

`fonts.go` grows a `[]FontInfo` registry next to `template.go`'s `[]TemplateInfo`, with the same
shape: a public struct for the API, a resolver, and an id list for validation. A new
`GET /cv/fonts` mirrors `GET /cv/templates` (`handler/cv.go:197`).

The web must not carry its own copy of the list. `TemplateGallery.svelte` already fetches templates
rather than hard-coding them; the font picker does the same. The one thing the client needs that the
server does not is a CSS font stack, so `FontInfo` carries it as a public field — it is presentation
data about a font, not a server secret, and shipping it with the list is what keeps a second registry
from appearing in TypeScript.

### The bundled set is metric compatibility, not variety

Libertinus Serif (already embedded in the Typst binary) and Liberation Sans (already bundled) are
joined by Tinos and Carlito. Those two are metric-compatible with Times New Roman and Calibri — the
faces recruiters and résumé parsers actually expect — and both are SIL OFL.

Tinos rather than Liberation Serif, which is what this design first named: the liberation-fonts
project publishes sources, not built TTFs, so bundling it would mean running FontForge in the build.
Tinos is the same face by another name — Liberation 2.x was rebased onto the Croscore fonts, of which
Tinos is the serif — and Google Fonts ships it built. The licence file already in `internal/cv/fonts/`
names Tinos among its reserved font names, so this is the lineage the repository was already carrying.

This is why the list is short and boring on purpose. A CV font picker's job is not to be expressive;
it is to let someone match the house style of the firm they are applying to.

*Cost accepted:* four more TTFs (~2.4 MB) embedded in the binary and written into the sandbox on
every render, taking `writeFonts` from two files to eight. A Typst compile is already hundreds of
milliseconds; a few megabytes of temp-directory writes next to it is noise. The seam for caching the
staged font directory across renders is obvious if it ever stops being noise, and is not built now.

### Internal type sizes become em-relative

`classic-ats.typ:46` sets the candidate's name at `12pt` and `:26` sets section labels at `9.5pt`,
both absolute against a 9.5pt base. Raise the base to 12pt and the name stops being a heading.

Every internal `size:` in all four templates becomes an em multiple of the base (`1.25em`, `1em`).
Typst resolves `em` against the enclosing `text.size`, so the hierarchy scales as one. This is a
prerequisite for the size stepper, not an incidental cleanup.

### Line height is stored as Typst leading, presented as named presets

The stored value is the Typst `leading` in em, because that is what three of the five consumers need
directly. The HTML preview converts it to a CSS `line-height` ratio through a single calibration
constant that lives in `geometry.ts` with a comment and a unit test — the two engines define line
spacing differently, and burying that conversion in a Svelte template is how the preview drifts from
the PDF.

The UI shows named presets (Compact / Standard / Relaxed) and no number. A leading of "0.55em" means
nothing to a candidate, and a ratio like "1.3" is false precision about a value they are choosing by
eye against a live preview.

### Margins collapse to two axes by default

Four steppers do not fit a 340px panel — that is the bug. Rather than shrinking them, the default
view offers two: side margins (left and right linked) and top-and-bottom. Asymmetric margins are rare
but real, so the four independent steppers stay reachable behind a disclosure, and the linked view
shows when an axis is no longer uniform rather than lying about it.

*Alternative considered:* keeping the four-up grid and switching it to Tailwind v4 container queries
(`@container`), which would correctly track the panel rather than the viewport. That fixes the
overflow but leaves the panel dense and inconsistent with the new Typography rows. Container queries
are the right tool if a future control genuinely needs to reflow; the row layout means none do.

### The ATS delta must copy style with template and margins

`cv_ats_delta.go` renders the base CV with the tailored copy's template and margins so the comparison
measures content alone. Typography changes how much text fits a page, so it changes the extracted
text layer, so it belongs in that list. Missing this would hand the candidate a way to move their own
score by picking a font.

## Risks / Trade-offs

- **The zero-means-inherit rule is easy to break** → An implementer adding a fourth style field, or
  refactoring `Sanitize`, will reach for the `clampMargin` pattern and freeze defaults into every
  document on its next save. Mitigated by a named test asserting a zero stays zero, and by a comment
  on the `Style` type stating the rule and why it differs from `Margins`.
- **The preview's hidden measurement layer can be forgotten** → Pagination is then computed under one
  typeface and drawn under another, and the preview disagrees with the PDF in a way no error reports.
  Mitigated by deriving both layers' styles from one `$derived` value in the component, so they
  cannot be set independently.
- **Typst and CSS line spacing do not correspond exactly** → Preview and PDF pagination will diverge
  slightly at the extremes of the line-height range. This already true of the existing preview, which
  approximates Typst's layout by measurement; the calibration constant keeps the default exact and the
  presets close. Accepted.
- **Eight fonts staged per render** → More temp-directory I/O on a path that runs twice per ATS delta.
  Accepted as noise against Typst's own compile cost; the caching seam is noted above.
- **A silently dropped font id** → An API client sending an unregistered id gets a CV rendered in the
  template's face with no signal. This is `Sanitize`'s existing contract (an out-of-range margin is
  clamped, not refused) and consistency beats a special case here; the fonts endpoint exists so a
  client never has to guess an id.

## Migration Plan

None required. No schema change, no backfill, no migration file: `style` is a new optional key inside
an existing JSONB document, and its absence is the pre-change behaviour by construction.

Deploy is ordinary. The new `GET /cv/fonts` endpoint and the regenerated TypeScript contracts ship
together with the web build; an old client against a new server simply never asks for fonts.

Rollback is ordinary too, with one wrinkle worth stating: a CV saved with typography under the new
code and then read by rolled-back code renders with template defaults, because the old templates
ignore the `style` key. The values are not lost — they sit in the document and take effect again on
roll-forward.

## Open Questions

None. The scope, the font set, the selector shape, and the panel layout were settled before this
document was written.
