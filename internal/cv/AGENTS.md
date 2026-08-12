# internal/cv

Per-user structured CVs (CRUD + seed + tailoring) and on-demand PDF rendering. The HTTP
surface lives in `internal/handler/cv.go`; this package owns the domain, storage, and
rendering.

## Templates

Templates are Typst source files under `templates/<id>.typ`, embedded via `//go:embed`. The
registry is `templates []TemplateInfo` in `template.go` (id, label, style, `photo`).
`ResolveTemplate(id)` defaults an empty id to `classic-ats` and rejects unknown ids with
`ErrUnknownTemplate`; `Templates()` exposes the metadata for the UI gallery and preview
generation.

**Adding a template:**
1. Add `templates/<id>.typ` — a self-contained file reading the CV from `json("data.json")`
   (helpers `s`/`arr`/`daterange` are duplicated per file; the renderer only stages
   `template.typ` + `data.json` + fonts, so Typst `#import` of a shared module won't resolve).
2. Append a `TemplateInfo` entry to `templates`, and mark `Photo: true` if it prints the
   headshot (see below).
3. Copy the style preamble from an existing template and set its three fallbacks to your own
   font, size, and leading. **Every internal `size:` must be an em multiple of the base**
   (`(18 / 9.5) * 1em`, not `18pt`) or a raised base size will leave your headings behind and
   flatten the hierarchy. Ratios are written as a division so the old absolute value stays
   legible. **Keep the font-size/leading fallback at 9.5pt / 0.5em unless the template needs a
   different face's metrics** — `web/src/lib/tailor/geometry.ts` (`TEMPLATE_FONT_SIZE_PT`,
   `PREVIEW_FONT_SIZE_PX`, `PREVIEW_LINE_HEIGHT`) hardcodes that base for every template's live
   HTML preview, so a `.typ` file that defaults to something else silently desyncs the preview's
   pagination from the rendered PDF's. Reach for density through spacing between blocks instead
   (see `compact.typ`).
4. **Print every link through `link()`**, and take its target from `hrefAt(kind, i, shown)` rather
   than from the printed text — the helper is duplicated per file like `s`/`arr`. Two reasons, and
   the first is not cosmetic: opt-in link tracing substitutes the *target* while leaving the text
   the candidate wrote, so a template printing a link as inert text reports zero opens forever.
   The second is that stored links are scheme-less (`github.com/ada`), and only the resolved href
   is absolute — printing the raw value as the target yields a relative URI no reader can follow.
   `TestEveryTemplateRendersLinksAsClickableLinks` walks the registry, so a new template is held to
   this without the test being edited.
5. Run `make cv-previews` to regenerate `web/static/cv-previews/<id>.svg` (the gallery
   thumbnails). A preview is committed for every registered id — the generator iterates the
   registry so the set can't drift.

Two Typst traps that cost real debugging time here:

- **In `text(size: X, tracking: Y)` the `em` inside `tracking` resolves against `X`,** not the
  outer base. Converting a heading's 1pt tracking as `1/9.5 em` nearly doubled it at an 18pt
  heading. Take the ratio over the call's own size.
- **A Typst PDF embeds a creation timestamp,** so byte-comparing two PDFs proves nothing —
  two renders of one source in different seconds differ. Compare SVG, which is deterministic.

## Typography (`Document.Style`)

A CV carries a font id, a base type size in points, and a line height (Typst leading, in em).
`Sanitize` clamps a set value and **leaves a zero one zero** — zero means "whatever the active
template uses". That is the opposite of `Margins`, where an unset side resolves to a concrete
0.5in, and the asymmetry is the whole design:

- No migration. Every CV predating the field has no style block and renders exactly as before.
- A template stays a whole design choice: switching one still moves whatever the candidate has
  not overridden.

The clamping stays inline in `Style.sanitized()` — a `clampFloat` per field, each guarded by a
zero check. Writing a `clampFontSize` by analogy with `clampMargin`, the obvious thing, would
rewrite every CV in the database to the minimum size on its next save: `clampMargin` maps an
unset side to a concrete default, and the same shape for a font size maps "unset" to the floor.
`TestSanitizeStyleLeavesUnsetValuesUnset` is the tripwire.

The style block is **not** in `PatchOps`: the tailoring agent edits content, the candidate
edits presentation.

## Fonts

`fontRegistry` in `fonts.go` is the set a CV may choose (id, label, the familiar face it
matches, and a CSS stack for the live HTML preview). `GET /cv-fonts` serves it so the web never
grows a second copy. The document stores the **id**; `renderer.go` swaps in the Typst family
name on its own copy before marshalling `data.json`, so a persisted CV holds no engine name.

**Adding a font:** drop its `.ttf` (regular + bold) into `fonts/` with its licence, then append
an entry naming those files. `//go:embed fonts/*.ttf` picks them up and `writeFonts` stages
them into every compile sandbox.

The set is deliberately small and metric-compatible with what recruiters and résumé parsers
expect — Tinos for Times New Roman, Liberation Sans for Arial, Carlito for Calibri. That also
pays off in the browser: a candidate's explicit choice never ships a webfont, its CSS fallback
lands on the real face the metrics match instead.

**The two *template defaults* — Libertinus Serif (Typst's own built-in) and Liberation Sans —
are the one exception,** and do ship as webfonts: `web/src/lib/tailor/CvHtmlPreview.svelte`
`@font-face`s a Latin-only WOFF2 subset of each (`web/static/fonts/`, ~11-16KB per weight) under
their exact family names, Vite-code-split onto the preview route only. Unlike the three
alternates above, an unset `font_family` is the *common* case — most CVs never touch the style
picker — so approximating with a browser default here, rather than the metric-identical face,
is what let a dense CV's preview and PDF disagree on page count by a whole section. Regenerating
the subset after a font bump: `pyftsubset <ttf> --output-file=<name>.woff2 --flavor=woff2
--unicodes=U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+2000-206F,U+2074,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD
--layout-features='*' --no-hinting` (regular and bold separately — Typst never synthesizes bold,
so the browser must not either, or a bold run's glyph widths stop matching the PDF's).

`TestEveryRegisteredFontIsResolvable` fails the build if an entry names neither a Typst
built-in nor a bundled file. It has to, because under `--ignore-system-fonts` a missing face
is not an error — Typst silently substitutes another one and the CV is quietly wrong.

## Rendering

`TypstRenderer` shells out to the Typst CLI in a sandboxed temp `--root` with
`--ignore-system-fonts`; user data goes through `data.json` (never argv). `compile` is shared
by `Render` (PDF, live) and `GeneratePreviews` (SVG, `cmd/cv-previews`).

Fonts: the Typst binary embeds no proportional sans, so the bundled faces under `fonts/` (all
SIL OFL) are staged into the sandbox and exposed via `--font-path`. A template names its own
default face directly (`#set text(font: "Liberation Sans")`); anything the candidate picks
arrives through `Document.Style` — see **Typography** and **Fonts** above.

## The headshot (`portrait`, `headshot`)

The photo is NOT part of `Document` — it is a profile asset owned by `internal/headshot`,
because a document is client-writable and travels into tailoring prompts. `Render` therefore
takes it as a fourth argument, `photo []byte`, and the handler fetches it only when
`tmpl.Photo` (`headshotForTemplate` in `internal/handler/photo.go`); a photoless template
costs no bucket round trip.

Three things a new photo template must respect:

- **The image is a staged FILE, not a URL.** The sandbox has no network and `--root` blocks
  every outside path, so `compile` writes `photo.jpg` next to `data.json` and the template
  hardcodes that name.
- **`has_photo` decides, and it is produced at render time.** `renderPayload` marshals a
  struct embedding `Document` plus the flag, so it inlines beside the document's own fields
  without existing on the persisted type — a client cannot set it. It is true exactly when
  the file was staged, because `image()` on a missing file is a compile error.
- **The placeholder is drawn, not staged.** Each photo template carries its own
  `#let silhouette()`. An image asset would be base64-inlined into every committed SVG
  preview; shapes keep them vector, and mean a member with no photo needs no file at all.
  `web/src/lib/components/HeadshotSilhouette.svelte` is its twin for the HTML preview — the
  two must keep agreeing, as must the frame sizes (26mm header / 42mm sidebar cap).

## What makes a CV tailored (`cvs.is_tailored`)

A CV is a tailored copy because it was **created** as one, not because it still points at a vacancy.
`cvs_job_id_fkey` is `ON DELETE SET NULL`, and `cmd/prune` deletes vacancies by design — the prune
query states that nulling references is "an accepted cost of the campaign". Inferring tailored-ness
from `job_id IS NOT NULL` therefore meant that pruning one junk vacancy turned its copy into a plain
CV, and since the base lookup takes the **newest** plain CV, the freshly-edited orphan beat the real
one. It would then seed the next tailored copy and back the ATS delta's baseline.

Three things that follow, and are easy to get backwards:

- **`is_tailored` does not track `job_id`.** An orphaned copy is `is_tailored = true, job_id = NULL`.
  Any new reader that wants "is this a tailored copy" must ask the flag; `job_id` answers a different
  question ("which vacancy", and only while the vacancy exists).
- **There is no `is_base`, and no uniqueness.** A user may own several plain CVs (`cv-builder`:
  create/list/update/delete multiple CVs), so "the base" is derived — the most recently edited
  non-tailored CV — and `GetBaseCVByUser` does that choosing. An earlier draft added
  `UNIQUE (user_id) WHERE is_base` and broke creating a second CV; `TestBaseCVAndTailoredCopy` caught
  it.
- **The ATS delta's 409 splits on the flag.** A base CV is refused with "this is a base CV"; a
  tailored copy whose vacancy is gone is refused with "the vacancy … no longer exists". They are
  different situations, and `job_id == 0` alone cannot tell them apart.

### Reset from résumé

`POST /me/cvs/:id/reset-from-resume` (cookie-only) rebuilds a **tailored** CV's content from the
current résumé seed (`bankedSeeder`: experience bank + `resume_structured`) and refreshes the
base CV from the same seed. Same tailored id and agent session; template/margins/style on each
row are preserved. Upload alone does **not** write `cvs` — it only refreshes the seed source;
this endpoint is the explicit apply. 409 when the target is not tailored, the seed is unusable,
or the seed itself exceeds the bullet ceiling (see `internal/cvedit/AGENTS.md`).

The tailored copy commits **before** the base refresh, not after: these are two separate
`CommitDocument` calls, not one transaction, so ordering decides which one a mid-request
failure leaves stale. Committing the caller's actual target first means a refused/failed
write touches nothing at all; a failure in the base refresh after that leaves only the base
stale (self-heals on the next bootstrap freshness check or Reset) rather than a request that
reports failure while having silently rewritten a CV nobody asked to touch.

**Bootstrap freshness:** `POST /me/cvs/tailor` refreshes a base whose `updated_at` is strictly
before `resume_uploaded_at` (when the seed is usable) before copying into a **new** tailored
row. Reloading an existing tailored-for-job stays idempotent and does not rewrite that copy.
Does not write the profile.

Backfill caveat: `0058` reads `job_id` to set the flag, so it is right about every row only because
no prune had orphaned one yet. A row orphaned before that migration would stay marked non-tailored
and would need a heuristic repair.

## ATS delta (tailored vs base)

`GET /me/cvs/:id/ats-delta` reports what tailoring did to a CV's ATS readiness. The scoring
input is the **rendered PDF's text layer**, not the stored document: the orchestration lives in
`internal/handler/cv_ats_delta.go` (render → `resume.ExtractPDFText` → `skilltag.Parse` →
`atscheck.Score`), and the two-report comparison is `atscheck.Compare`. A document field the
active template never renders therefore earns nothing — `sidebar` has no certifications block,
and the handler test pins exactly that.

Four things the code decides and a reader would otherwise have to infer:

- **The comparison holds everything but content constant.** The base CV is rendered with the
  *tailored copy's* template, margins and typography (an in-memory copy — the stored base is
  never touched). Typography is in that list because type size and leading decide how much
  text lands on a page and the score reads the rendered PDF's text layer; omitting it would
  hand the candidate a way to move their own delta by changing a font. Both sides are scored
  against one keyword baseline: the bound vacancy's canonical
  `jobs.skills`. Not the role facet's top skills, and not the LLM's requirement match, whose
  drift would move the delta while the CV stood still.
- **The baseline is the base CV as it stands NOW.** There is one base CV per user
  (`cv.Store.BaseCV`), not a snapshot taken when the copy was made, so editing the base moves
  the delta. Deliberate — a snapshot would mean storing a second document per tailored CV —
  and pinned by `TestATSDelta_ComparesAgainstTheCurrentBase`.
- **The route is cookie-only, and that is the enforcement.** The tailoring agent authenticates
  with a CLI credential, so the gate is what keeps the score away from the thing being measured.
  Widening it to `mw.key` hands the agent a metric to optimise; `TestCVRegister_ATSDeltaIsCookieOnly`
  is the tripwire.
- **Unavailable is a 200, not a 501.** No renderer or a failed compile answers
  `available: false` with a reason. `RenderCVPDF` 501s for the same condition because rendering
  is what that caller asked for; the delta is an accessory read on a page that must keep working.

Nothing is stored: it is recomputed per request, so a scoring-rule change needs no migration.

## Autopilot runs

A tailored CV carries what the last unattended run left behind: `autopilot_report`, one entry
per requirement the run considered, with an outcome from a fixed vocabulary. The wire shape
lives in `autopilot.go` — it is generated to TypeScript — while `autopilot_store.go` holds what
may be persisted and the owner-scoped writes that persist it. Keeping them in separate files is
what stops the client seeing rules that are ours to enforce.

Two rules the code encodes rather than documents:

- **Nothing is coerced.** A status outside `closed_bank` / `closed_candidate` / `open` /
  `not_reached` is refused with the valid ones named, because that message is the model's only
  route to correcting itself inside the turn. Text is trimmed and truncated silently — those
  are display concerns.
- **A report is replaced whole.** There is no partial update: a requirement closed later from
  the candidate's own words arrives as the same list with one entry changed, so the stored
  value is always the current truth and there is one write path instead of two.

Undoing a run is undoing the revisions it made (see below), not restoring a snapshot. The
`autopilot_undo` column that used to hold one is retired: it could only answer "put the whole
run back", and two runs started at once each took a snapshot, so the second captured a
half-edited document and undoing returned to the middle of the first. Reverting a batch clears
the report with it, because a report describing edits that no longer exist misdescribes the CV.

One edge remains, deliberately:

- **A run lays down its own plan.** The handler writes the vacancy's requirements as
  `not_reached` before the turn starts, because a run that exhausts the step cap gets its final
  model call with no tools offered and therefore cannot report. The agent's report replaces the
  plan wholesale.

## Who may write a stored CV

Nothing in this package writes `cvs.data`. `internal/cvedit` owns the only path, and the seam
is held by absence rather than by review: `Store` has no method that writes a document at all
(its one unexported helper, `tailoredForJob`, is a read), and `Repository` declares none either
— one declared here, even unused, would be an invitation to write a document with no revision,
no policy and no evidence gate behind it. The write itself lives in cvedit's `Tx.Save`, which
runs the `UpdateCV` query inside the locked commit.

Every entry point — the editor's autosave, the template picker, the CLI's `PATCH`, the
assistant's `cv_edit`, seeding a tailored copy — commits through `cvedit.Editor`, which applies
the change, records what it did and what would undo it, and writes both in one transaction
against a locked row. Three consequences worth knowing before touching this:

- **A whole-document save is an input format, not a write path.** `PUT /me/cvs/:id` still
  carries the document; the differ derives the operations, and from the editor's point of view
  it is indistinguishable from an agent's batch.
- **The actor follows the credential, never the body.** A cookie is the candidate; an API key
  is the agent, and meets the agent's path policy — which is what keeps the contact block
  closed to it now that no gap in a vocabulary does the job.
- **The row lock serialises edits to one CV.** `GetCVForEdit … FOR UPDATE` is why two agent
  turns arriving together no longer interleave.
