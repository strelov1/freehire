## Why

The résumé is uploaded twice today — once on the profile form (to extract skills, text discarded) and again on the verdict page (to score coherence, text discarded) — because we never keep the text. Users read the second prompt as "why upload again?". Storing the résumé once (in S3) removes the double upload: skills extraction and the verdict's coherence check both reuse the one stored résumé, and coherence can be re-run without a new upload.

## What Changes

- **Store the résumé once, in S3.** A signed-in user uploads their résumé (PDF/text) once; the original file is stored in object storage under a per-user key. The extracted text is derived on read (for skills and coherence).
- **`hire` stays infra-agnostic**: a new `internal/blobstore` exposes a minimal S3 Put/Get behind an interface, configured only by generic `S3_ENDPOINT`/`S3_BUCKET`/`S3_ACCESS_KEY`/`S3_SECRET_KEY` env vars. No bucket name, host, or provider is hard-coded — `freehire-ops` owns the bucket and credentials.
- **Single upload point**: uploading a résumé stores it to S3 and extracts skills in one step. The verdict's coherence reads the stored résumé (no re-upload); the verdict page offers "Re-run coherence" instead of "upload again". A first-time user with no stored résumé is prompted to upload once.
- **Graceful degradation**: when S3 is unconfigured, résumé storage is disabled and the app falls back to the current in-request extraction (no regression); the LLM coherence layer stays independently optional.
- **Ops**: `freehire-ops` provisions a Hetzner Object Storage bucket (`freehire-resumes`, hel1) and injects `S3_*` into the prod env.

## Capabilities

### New Capabilities
- `resume-storage`: uploading, storing, retrieving, and deleting a user's résumé in S3 object storage (per-user key), plus the generic S3 blob abstraction (`internal/blobstore`) and its config/degradation.

### Modified Capabilities
- `resume-verdict`: the coherence analysis now reads the user's **stored** résumé from S3 (no per-verdict upload); the verdict page gains a "Re-run coherence" affordance and stops asking for a fresh upload when a résumé is already stored.
- `resume-skill-extraction`: the profile-form upload now **stores** the résumé to S3 in addition to returning extracted skills (the single upload point), when storage is configured.

## Impact

- **New Go package** `internal/blobstore` (S3 via a lean S3-compatible client, e.g. `minio-go`); new dependency in `go.mod`.
- **Config + `cmd/server`**: read `S3_*`, build a blobstore client when configured (nil-guard like Meili/LLM), pass into `handler.Config`.
- **DB**: migration adds `resume_object_key`/`resume_uploaded_at` to `users` (pointer + timestamp; the blob lives in S3, not Postgres); new sqlc queries.
- **Handler**: `PUT/GET/DELETE /api/v1/me/resume` (store/metadata/delete) and rework of the verdict POST to read the stored résumé; `resume.go` extraction path stores the file.
- **Frontend**: single résumé upload UX; verdict "Re-run coherence"; a "My résumé" indicator.
- **freehire-ops**: create the Hetzner Object Storage bucket + credentials, add `S3_*` to `/opt/freehire/.env`.
- Supersedes the "parsed and discarded" privacy note for the résumé (now stored in S3, access-controlled).
