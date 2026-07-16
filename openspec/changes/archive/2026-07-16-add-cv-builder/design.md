## Context

freehire already stores one uploaded résumé per user (`internal/resume`, S3 + `users.resume_object_key`) and extracts a typed `resumeextract.Structured` (`users.resume_structured`) in the background. That structure feeds the profile, `job-fit-analysis`, and `resume-verdict`. There is **no PDF generation** in the codebase today — only PDF *reading* via `ledongthuc/pdf`.

The product goal: master profile data → modify per vacancy on demand → render a CV. This change builds the **engine half** (author N CVs, render to PDF); the per-vacancy tailoring is a follow-up phase.

Constraints that shape the design:
- The server ships as a **distroless Docker image** on host2 (native systemd). Any render toolchain must be cheap to bake into that image and must not require a live sidecar process.
- ATS-friendliness is a hard requirement: the PDF must carry a selectable text layer.
- Existing working features (`job-fit-analysis`, `resume-verdict`, `resume-storage`) must not regress — the change is additive.
- Established repo patterns to reuse: `Sanitize` (persist + prompt-injection guard, as in enrich/resumeextract), nil-safe feature-gating (blobstore/meili/llm), integration build-tag tests, `cmd/gen-contracts` for the TS wire shape.

## Goals / Non-Goals

**Goals:**
- Author, store, list, edit, delete multiple structured CVs per user.
- Seed a new CV from the existing `resume_structured` extraction.
- Render any CV to an ATS-clean PDF on demand, streamed, not persisted.
- Keep the rendering engine behind an interface so it is swappable.
- Ship exactly one ATS single-column template with room for more.

**Non-Goals:**
- Per-vacancy tailoring / LLM modification of a CV (next phase; only the `job_id` seam is laid).
- Multiple templates / theme gallery (registry seam only).
- Persisting or caching rendered PDFs in S3 (seam).
- Inline live preview, section reorder/visibility (seams).
- Changing how `job-fit-analysis` / `resume-verdict` consume the résumé.

## Decisions

### D1: CV is a structured JSON document, not markdown/LaTeX
Source of truth is a typed `cv.Document` (extension of `resumeextract.Structured`) stored as `jsonb` in a new `cvs` table; metadata (`title`, `template_id`, timestamps) are columns. **Why:** structured data maps cleanly onto "many CVs in a table", a form editor, and the future tailoring pass (edit fields by section). Markdown/LaTeX-as-content was rejected: poor fit for a form editor and per-section tailoring, weaker ATS control. `jsonb` mirrors the existing `resume_structured` precedent.

### D2: Typst renderer behind a `Renderer` interface
```go
type Renderer interface {
    Render(ctx context.Context, doc Document, tmpl Template) ([]byte, error)
}
```
`TypstRenderer` writes `data.json` into a temp `--root` dir, places the embedded `.typ` template (which reads `json("data.json")`), runs `exec.CommandContext(ctx, typstBin, "compile", "--root", dir, "--ignore-system-fonts", tmpl, out)`, and returns the bytes. The template uses Typst's **binary-embedded Libertinus Serif**, so no fonts are bundled and `--ignore-system-fonts` makes local and prod rendering byte-reproducible — the "distroless has no fonts" risk disappears because the font travels inside the typst binary. **Why Typst over Chrome/LaTeX/pure-Go:** single static binary trivially baked into distroless (no sidecar, no page pool, no cold start), ~50–150 ms/render (in-handler is fine), clean selectable text layer out of the box, and it reads JSON natively so data and layout stay decoupled. Alternatives considered: headless Chrome (max design flexibility but heavy dependency + live process to babysit, ATS pitfalls), LaTeX/tectonic (mature typography but heavy toolchain, escaping bugs, CJK pain), pure-Go maroto (trivial deploy but primitive layout). The interface keeps the door open to add a Chrome renderer later **without touching the schema/storage/handlers** — a decision explicitly requested by the user.

### D3: Separate `cvs` entity, seeded from the structured résumé
The uploaded résumé stays the "master input"; builder CVs live in their own table. The first CV can be seeded from `resume_structured`. **Why:** minimizes risk to the working `job-fit-analysis` / `resume-verdict` paths (they keep reading `resume_structured`), and avoids the double-résumé confusion seen in reference tools. Unifying the single résumé into the CV table was rejected as a larger migration that touches working analysis for no phase-1 benefit.

### D4: On-demand render, streamed, not stored
`GET /me/cvs/:id/pdf` renders and streams `application/pdf`. **Why:** Typst is fast enough that always-fresh rendering beats cache-invalidation complexity, and nothing new needs storing. S3 caching (for a stable link / an immutable artifact attached to a tracking application) is a noted seam.

### D5: `job_id` nullable seam for tailoring
`cvs.job_id bigint NULL REFERENCES jobs(id) ON DELETE SET NULL`. Unused in phase 1 (all CVs are general). **Why:** the follow-up tailoring phase creates a CV bound to a vacancy; laying the column now avoids a future migration while adding zero phase-1 logic.

### D6: Feature-gating and template registry
Renderer is nil-safe like blobstore/meili/llm: absent `TYPST_BIN`/binary → renderer disabled → PDF endpoint `501`, CRUD unaffected. `template_id` resolves through a small registry (id → embedded `.typ`), defaulting to `classic-ats`; unknown ids are rejected, not rendered.

### D7: Package layout
`internal/cv/`: `cv.go` (`Document` + `Sanitize`, the only file `gen-contracts` reads), `seed.go` (`Document` ← `Structured`), `renderer.go` (`Renderer` + `TypstRenderer`), `template.go` (registry + `go:embed templates/*.typ` + fonts), `store.go` (repo over sqlc). Plus `internal/db/queries/cvs.sql` + migration, `internal/handler/cv.go`, `web/src/routes/my/cvs/`.

## Risks / Trade-offs

- **Distroless has no system fonts** → Typst finds no fonts and renders tofu. *Mitigation:* bundle a `fonts/` dir into the image and always pass `--font-path`; the ATS text-extraction test guards against silent font breakage.
- **Shelling out to a binary from the handler** (latency, zombie processes, arbitrary-input injection into args). *Mitigation:* `exec.CommandContext` with a timeout; never interpolate user data into the command line (data goes through a written `data.json`, only fixed flags are passed); Typst `--root` sandboxes filesystem access.
- **Typst template language is new to the team.** *Mitigation:* only one template in phase 1; layout logic lives entirely in the `.typ`, Go only emits JSON.
- **jobs FK on `cvs`** could complicate deletes. *Mitigation:* `ON DELETE SET NULL` (a CV outlives its source vacancy); `user_id` is `ON DELETE CASCADE`.
- **Prompt-injection / oversized input** in CV text (matters once tailoring feeds it to an LLM). *Mitigation:* `Sanitize` bounds/caps on every persist, same invariant as enrich/resumeextract.

## Migration Plan

- New migration `0024_cvs.sql` creating the `cvs` table (`id bigint IDENTITY` PK — matches the codebase; no uuid) + `cvs_user_id_updated_at_idx`. Applied **manually on prod** before deploy, per repo convention (Postgres initdb only runs on first volume init).
- Docker image: a multi-stage stage downloads the pinned Typst release binary; the final distroless stage `COPY`s just that binary in — **no fonts** (Libertinus Serif is embedded in the binary and selected with `--ignore-system-fonts`). New optional `TYPST_BIN` env, resolved via `exec.LookPath` so an absent binary cleanly disables rendering (501).
- Rollback: the change is additive — dropping the routes / not setting `TYPST_BIN` disables the feature with no impact on existing endpoints; the `cvs` table can be left in place.

## Resolved during implementation

- **PK is `bigint IDENTITY`, not uuid** — no uuid exists anywhere in the schema; owner-scoped queries make enumerable ids safe (a foreign id is a 404).
- **No bundled fonts** — Typst embeds Libertinus Serif; `--ignore-system-fonts` makes local == prod output. This removes the "distroless has no fonts" risk entirely.
- **Beta-gated rollout** — every `/me/cvs` route is behind `RequireModeratorOrBeta` (the existing restricted-rollout gate); the SPA hides the nav/pages from non-beta users.
- **TS name collision** — `cv.Experience`/`cv.Education` were renamed to `ExperienceItem`/`EducationItem` because `resumeextract` already exports `Experience`/`Education` into `contracts.ts` (same precedent as `verdict.ScoreCategory`).
- **Seeding is opt-in** via the `seed` request flag; it fills from `resume_structured` when a structured résumé exists, else an empty skeleton. Seeding reads the structure from Postgres and is **not gated on S3** (résumé object storage) — only on the structure being present.
- **Résumé extraction was extended** to make seeded CVs complete (this touches the `resume-structured-profile` capability, to be captured as a MODIFIED delta at archive): `resumeextract.Structured` gained `skills`, per-role `highlights` (achievement bullets), `location`, per-role `stack`, and a `projects` array. `cv.Seed` maps these into the Document; the extraction prompt asks for them.
- **One "summary" term, no headline** — the CV `Header` has no `headline`; the tagline under the name is `Document.Summary`. Seeding prefers the extracted summary, falling back to the résumé's headline line. (`resumeextract.Structured.Headline` is kept for the profile readiness view.)
- **Template `classic-ats` matches a compact single-page résumé**: name + contacts on one pipe-separated line, `Company | Location | Title (dates)` role headers with a context line, bullets, and a per-role `Stack:` line; Education / Skills / Languages render inline. User text reaches Typst only through `data.json` (read via `json()`), never as markup or argv, so it cannot inject Typst.
