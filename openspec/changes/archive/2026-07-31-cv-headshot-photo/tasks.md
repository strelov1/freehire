## 1. Storage foundation

- [x] 1.1 Add migration `migrations/0059_user_headshot.sql` adding `users.photo_object_key` and `users.photo_uploaded_at`, and add `blobstore.PhotoKey(userID)` (`photos/<id>`) with a test
- [x] 1.2 Add the pointer queries to `internal/db/queries/users.sql` (`GetUserPhoto`, `SetUserPhoto`, `ClearUserPhoto`), extend `ListUserBlobKeys` with the photo key, and run `make sqlc`
- [x] 1.3 Add `golang.org/x/image` to go.mod (`go get golang.org/x/image`, `go mod tidy`)

## 2. Image normalization (`internal/headshot`)

- [x] 2.1 EXIF orientation reader: parse tag `0x0112` from a JPEG's APP1 segment, returning the declared orientation (table test over both byte orders, a file with no EXIF, and a truncated segment)
- [x] 2.2 `Normalize(data []byte) ([]byte, error)`: byte cap, `DecodeConfig` dimension cap, decode JPEG/PNG/WebP, apply orientation 3/6/8, centre-square crop, scale to 512×512 with `draw.CatmullRom`, re-encode JPEG q85; reject non-images and oversized input
- [x] 2.3 `Store` over `blobstore.Store` + pointer repository: `Enabled`, `Put` (normalize then store, stamping the pointer), `Get`, `Status`, `Delete`; nil store ⇒ `ErrStorageDisabled`, no pointer ⇒ `ErrNotStored`

## 3. HTTP surface

- [x] 3.1 `internal/handler/photo.go`: `PUT/GET/DELETE /api/v1/me/photo` + `GET /api/v1/me/photo/image`, cookie-only, owner-derived key; `PUT` and the meta `GET` return `{"data":{"enabled":…,"present":…,"uploaded_at":…}}`, the image read streams `image/jpeg` with `Cache-Control: private`, storage disabled ⇒ `501` (meta still answers, reporting it off), undecodable/oversized ⇒ `400`
- [x] 3.2 Wire the headshot store in `cmd/server/main.go` and register the routes; confirm `accountdelete` erases the photo object through the extended `ListUserBlobKeys` (integration test)

## 4. Renderer and template registry

- [x] 4.1 Add `Photo bool` to `cv.TemplateInfo` and `cv.Template`, expose it from `Templates()`/`ResolveTemplate`, and assert the flag in the registry test
- [x] 4.2 Change `cv.Renderer.Render` to take `photo []byte`; stage `photo.jpg` and set `has_photo` in `data.json` (via the embedded `renderDoc` wrapper) only when the template is photo-bearing and bytes are present; update `GeneratePreviews` and every test double
- [x] 4.3 `RenderCVPDF` fetches the headshot only when `tmpl.Photo`, treating "no photo" and "storage disabled" alike as nil bytes

## 5. Templates

- [x] 5.1 `internal/cv/templates/portrait.typ`: two-column serif, photo atop the sidebar, own `#let silhouette()` placeholder; register as `portrait` (`ats_safe: false`, `photo: true`)
- [x] 5.2 `internal/cv/templates/headshot.typ`: single-column sans, portrait in the header, own `#let silhouette()`; register as `headshot` (`ats_safe: false`, `photo: true`)
- [x] 5.3 Run `make cv-previews` and commit the two new `web/static/cv-previews/*.svg`; verify both thumbnails show the placeholder frame

## 6. Web

- [x] 6.1 Photo controls in the profile Settings tab (`ProfileForm.svelte`): round preview, upload/replace/remove, client-side type and size precheck, `?v=<uploaded_at>` cache busting
- [x] 6.2 `CvHtmlPreview.svelte`: `isPortrait`/`isHeadshot` branches rendering the photo (or the same silhouette as inline SVG) so the preview matches the PDF
- [x] 6.3 `TemplateGallery.svelte`: for `photo: true` templates with no stored photo, show the upload nudge linking to the profile Settings tab

## 7. Documentation

- [x] 7.1 Update `internal/cv/AGENTS.md` (photo-bearing templates, the render photo parameter, `has_photo`) and note the headshot in the profile/storage docs
