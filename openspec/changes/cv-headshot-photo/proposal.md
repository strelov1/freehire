## Why

Outside the US and UK a CV without a photo reads as incomplete — German, Austrian, Swiss,
Polish, Spanish, and most CIS employers expect a headshot, and several ATS-free application
forms ask for one outright. The CV builder currently offers four photoless templates, so a
candidate applying into those markets exports our CV and then pastes a photo into it by hand
in Word, which destroys the layout the builder just produced.

There is nowhere in the product to put a photo either: the profile stores a CV, an embedding,
and a structured extraction, but no portrait.

## What Changes

- A signed-in user can upload, replace, and remove **one headshot** on their profile. The
  upload is normalized server-side (square crop, 512×512, JPEG) so what is stored is
  predictable regardless of what the phone produced.
- The photo is stored as an object in the same S3 bucket as the stored CV, under a key derived
  from the user id, with a pointer on the `users` row. When object storage is unconfigured the
  feature reports itself unavailable instead of half-working.
- Two new CV templates print the photo: `portrait` (two-column, photo atop the sidebar) and
  `headshot` (single column, portrait in the header). Both are marked not ATS-safe.
- A photo-bearing template chosen by a user who has not uploaded a photo renders a **neutral
  silhouette placeholder**, so the layout is never half-empty and the gallery thumbnail shows
  what the template actually is.
- The template registry gains a `photo` flag, so a client can tell which templates use a photo
  and the server only reaches for object storage when the template needs it.
- The photo never enters the CV document, so it never reaches the tailoring LLM.

## Capabilities

### New Capabilities
- `profile-headshot`: the per-user profile photo — upload with server-side normalization,
  owner-scoped retrieval and removal, storage-disabled degradation, and the privacy boundary
  that keeps the image out of the CV document and out of LLM prompts.

### Modified Capabilities
- `cv-builder`: the template registry gains a `photo` flag exposed on the templates endpoint;
  two photo-bearing templates join the registry; rendering a photo-bearing template composes
  the stored headshot (or the silhouette placeholder) into the PDF.
- `account-deletion`: the erased object set gains the member's headshot object.

## Impact

- **Schema:** new migration adding `users.photo_object_key` and `users.photo_uploaded_at`;
  new sqlc queries for the pointer.
- **API:** `PUT/GET/DELETE /api/v1/me/photo` and `GET /api/v1/me/photo/image` (presence is a
  metadata read; the bytes are a sub-resource, mirroring `/me/resume`).
- **Go:** new `internal/headshot` package; new `internal/handler/photo.go`; `blobstore.PhotoKey`;
  `cv.Renderer.Render` gains a photo parameter (**BREAKING** for the in-repo interface and its
  test doubles only — no wire change); `cv.TemplateInfo`/`cv.Template` gain a `photo` flag;
  `internal/accountdelete` erases the new object.
- **New dependency:** `golang.org/x/image` (`draw.CatmullRom` for the resize).
- **Templates/assets:** `internal/cv/templates/portrait.typ`, `internal/cv/templates/headshot.typ`,
  and two regenerated `web/static/cv-previews/*.svg`.
- **Web:** photo controls in the profile Settings tab, photo rendering in the CV HTML preview,
  and an upload nudge in the template gallery.
- **Ops:** no new configuration — the feature rides the S3 settings the stored CV already uses.
