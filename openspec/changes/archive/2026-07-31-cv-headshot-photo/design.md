## Context

The CV builder renders a stored `cv.Document` through a Typst template. `TypstRenderer.compile`
stages `template.typ`, `data.json`, and the bundled fonts into a temp directory and runs the
binary with `--root <dir> --ignore-system-fonts`, so the render is sandboxed and makes no
network call. Templates are self-contained `.typ` files (a Typst `#import` of a shared module
does not resolve inside the sandbox, which is why the `s`/`arr`/`daterange` helpers are already
duplicated per file).

The profile already owns one stored object per user — the uploaded CV — in `internal/resume`:
bytes in S3 under `blobstore.ResumeKey(userID)`, a pointer (`users.resume_object_key`,
`users.resume_uploaded_at`) on the row, and a nil `blobstore.Store` meaning the feature is
disabled rather than broken. There is no image anywhere in the product today: no avatar, no
headshot, no image decoding path.

`cv.Document` is fed to the tailoring LLM and sanitized as untrusted input on every write, so
anything added to it is both a prompt surface and a client-writable field.

## Goals / Non-Goals

**Goals:**

- One headshot per user, uploaded from the profile, normalized server-side to a predictable
  square JPEG.
- Two CV templates that print it, rendering a placeholder when no photo has been uploaded.
- The photo reaches the renderer without touching `cv.Document`, and therefore without ever
  entering an LLM prompt.
- The feature degrades cleanly where object storage is unconfigured (local dev, self-hosters).

**Non-Goals:**

- A public avatar. The headshot appears only where the owner's own CV is rendered: the
  builder, its preview, and the proof PDF a referrer opens (which already carries the
  candidate's name and contacts — a silhouette there would misrepresent the CV, not protect
  anything). It is not shown on community threads, company pages, or any listing.
- A crop/zoom editor. Normalization is a centred square crop; a framing UI can come later
  behind the same endpoint without a schema change.
- Photos in ATS-safe templates. Both new templates are `ats_safe: false`; the existing four
  are untouched.
- Face detection, background removal, or any ML on the image.

## Decisions

### The headshot is a profile asset, not a CV field

The alternative — a `photo` field on `cv.Document` (base64 or an object key) — was rejected on
three counts: it would put an image into every tailoring prompt, it would make the photo
client-writable through `PUT /me/cvs/:id` (so `Sanitize` would need an image policy), and it
would duplicate the same portrait across every CV a user owns. Keeping it on the profile means
one upload serves every CV, and the CV document stays text.

The renderer therefore needs one out-of-band signal: `data.json` carries `has_photo` so the
template knows whether to draw the image or the placeholder. That flag is produced at render
time from an embedded wrapper:

```go
type renderDoc struct {
    Document
    HasPhoto bool `json:"has_photo"`
}
```

Go's `encoding/json` inlines the embedded struct's fields, so `data.json` gains one key while
the persisted `Document` type is unchanged — the flag cannot be set by a client and cannot
drift into storage.

### `internal/headshot`, not an extension of `internal/resume`

`internal/resume` is documented as the stored-CV use case, and it carries embedding, structured
extraction, and pdftotext derivation. A headshot shares only the storage shape. A new package
keeps the image-decoding surface (the riskiest new code here) in one file with one job, and it
mirrors `resume`'s contract exactly: a `Store` over `blobstore.Store` plus a pointer repository,
`Enabled()` false with a nil store, `ErrStorageDisabled`, and `ErrNotStored`.

Storage key: `blobstore.PhotoKey(userID)` → `photos/<id>`, derived from the session's user id.
Pointer: `users.photo_object_key`, `users.photo_uploaded_at`, added by migration
`0059_user_headshot.sql` (`0056` and `0057` are each already used twice, so "one past the
last file" is not the same as "one past the highest number").

### Normalization: decode, orient, crop, scale, re-encode

Accepted input: JPEG, PNG, WebP, at most 8 MiB. The service decodes, applies the source's
declared orientation, crops to the centred square, scales to 512×512 with
`golang.org/x/image/draw` (`draw.CatmullRom` — the standard library has no resampling scaler),
and re-encodes JPEG at quality 85. The stored object is then always ~40 KB and always the same
shape, which is what makes the Typst frame predictable and the S3 footprint bounded.

Two guards precede the decode: the byte cap, and an `image.DecodeConfig` dimension check, so a
40 000 × 40 000 PNG is refused on its header rather than after allocating 6 GB.

**EXIF orientation** is handled by a small in-package reader for tag `0x0112`, covering values
3, 6, and 8 (180°, 90° CW, 90° CCW) — the three a phone camera actually produces. The mirrored
values (2, 4, 5, 7) are left alone: they come from deliberate flips, and silently un-mirroring
someone's photo is worse than leaving it. This is ~50 lines of APP1/TIFF walking with a table
test, chosen over a third dependency whose whole surface would be one tag.

### The renderer takes the photo as bytes

`Renderer.Render(ctx, doc, tmpl, photo []byte)`. Passing a URL was never an option — the
sandbox has no network and `--root` blocks the path — so the bytes must be staged as a file
(`photo.jpg`) next to `data.json`. `compile` stages it only when `tmpl.Photo` is set and the
slice is non-empty, and sets `has_photo` from the same condition, so the two can never disagree.

This breaks the `Renderer` interface for in-repo callers (`RenderCVPDF`, `GeneratePreviews`, and
the test doubles). That is a four-line change and keeps the interface honest; the alternative —
a package-level "current photo" or a renderer constructed per request — hides a per-render input
in state.

### `TemplateInfo.Photo` gates the storage read

Without a registry flag, `RenderCVPDF` would either fetch from S3 on every render (a network
round trip added to `classic-ats`, which will remain the common case) or hard-code a set of
template ids in the handler. The flag is also what the gallery uses to decide whether to prompt
for an upload, so it earns its place on the wire alongside `ats_safe`.

### The placeholder is Typst code, not an asset

A bundled silhouette image would have to be staged like the fonts, and — because the gallery
thumbnails are SVG — would be base64-embedded into every committed preview. Drawing it (a rounded
grey frame, a circle for the head, an arc for the shoulders) keeps the thumbnails vector and
removes a staging path entirely: when there is no photo, no file is staged at all. The helper is
duplicated in both templates, consistent with the existing per-file helper duplication that the
sandbox forces.

### Serving: cookie-only, owner-derived, cache-busted

`PUT/GET/DELETE /api/v1/me/photo`, all `RequireAuth` (cookie-only, like the other authoring
mutations). The read streams `image/jpeg` with `Cache-Control: private`; the SPA appends
`?v=<uploaded_at>` so a replacement is visible immediately without disabling caching. No
pre-signed or public URL: a headshot is PII, and the only consumers are the owner's browser and
the server's own renderer.

### Account deletion needs no Go change

`ListUserBlobKeys` already collects every object key in one query; the headshot key joins that
`UNION`. `accountdelete.Service` iterates whatever it is given, so the erasure and its `503`
abort semantics extend to the photo for free.

## Risks / Trade-offs

- **Decoding untrusted images is new attack surface.** → Byte cap before decode, dimension cap
  from `DecodeConfig` before full decode, stdlib/`x/image` decoders only, and the decode happens
  in the API process with no shell-out. Nothing derived from the filename or content type
  reaches a path or argv.
- **A user picks a photo template for a US application and hurts their chances.** → Both
  templates carry `ats_safe: false`, which the gallery already renders as a warning. Enforcing
  market-specific advice is out of scope.
- **The placeholder could be exported unnoticed** (the user chose a placeholder over a collapsed
  layout). → The template gallery shows an explicit "upload a photo in your profile" nudge for
  photo-bearing templates with no stored photo, and the profile links straight to the upload.
- **HTML preview and PDF can drift**, since the preview is a hand-written Svelte double of each
  Typst template. → The preview gets the same two branches and the same silhouette as inline SVG
  in the same change; this is the standing cost of the two-renderer design, not new debt.
- **New dependency `golang.org/x/image`.** → Semi-standard, maintained by the Go team, used only
  for `draw.CatmullRom` and the WebP decoder.
- **Object storage is not configured everywhere.** → `Enabled()` false ⇒ `501` on the photo
  endpoints, and photo-bearing templates still render with the placeholder, so no CV becomes
  unrenderable because of an ops gap.

## Migration Plan

1. Apply `0059_user_headshot.sql` (two nullable columns; no rewrite, no lock of consequence).
2. Deploy. The new templates appear in the registry immediately and render the placeholder for
   everyone until photos are uploaded.
3. `make cv-previews` output is committed with the change, so the gallery has thumbnails on the
   first request.

Rollback: revert the deploy. The columns are additive and unread by the previous binary; stored
objects under `photos/` become orphans, removed by dropping the prefix if the feature is
abandoned.

## Open Questions

None blocking. Deferred by choice: a crop/zoom framing UI, and whether the headshot should later
back a public profile avatar (which would need a different, non-PII-by-default serving path).
