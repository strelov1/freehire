## Context

The résumé-verdict feature (shipped) analyzes an uploaded résumé for coherence but never stores the text (privacy invariant), so the résumé is uploaded once for skills (profile form) and again for coherence (verdict page). Users find the second upload confusing. Decision: store the résumé once in S3 and reuse it. `hire` must stay infra-agnostic; the bucket + credentials live in `freehire-ops`. telagon already uses Hetzner Object Storage (S3-compatible, `hel1.your-objectstorage.com`) with `S3_ENDPOINT/S3_ACCESS_KEY/S3_SECRET_KEY/S3_BUCKET` — freehire reuses that provider with its own `freehire-resumes` bucket.

## Goals / Non-Goals

**Goals:**
- One résumé per user stored in S3, uploaded once, reused for skill extraction and verdict coherence.
- A generic `internal/blobstore` S3 abstraction in `hire` — no bucket/host/provider baked into code, only `S3_*` env.
- Coherence re-runs from the stored résumé (no re-upload); the verdict page stops asking to upload again when a résumé exists.
- Graceful degradation when S3 is unconfigured (falls back to today's in-request extraction; no regression).
- `freehire-ops` provisions the Hetzner bucket + injects `S3_*` into prod.

**Non-Goals:**
- Encryption at rest of the résumé (deferred; object storage is access-controlled by credentials; revisit if needed).
- Versioning/history of résumés (one per user, overwrite on re-upload).
- Storing the extracted text separately (derived on read to avoid drift).
- Migrating the AWS SDK; a lean S3-compatible client (`minio-go`) suffices.

## Decisions

**1. `internal/blobstore` over `minio-go`.** A tiny interface — `Put(ctx, key, contentType string, r io.Reader, size int64) error`, `Get(ctx, key) (io.ReadCloser, error)`, `Delete(ctx, key) error` — implemented by a `minio.Client`. `minio-go` is a single-purpose, well-maintained S3-compatible client (works against Hetzner hel1, AWS, R2, MinIO) and is far lighter than aws-sdk-go-v2. The handler depends on a `blobStore` interface (nil when unconfigured), matching the `searcher`/`facetCounter` nil-guard pattern. Keys are `resumes/<userID>` — derived from the authenticated id, never client input.

**2. Store the original file; derive text on read.** The stored object is the uploaded PDF/text. Skill extraction and coherence fetch the object and parse it (`pdfText`) at read time. Avoids a second stored copy that could drift, and lets a future feature re-download the original.

**3. Pointer on `users`, blob in S3.** Migration adds `users.resume_object_key TEXT` + `resume_uploaded_at TIMESTAMPTZ` (nullable). Postgres stays free of the blob; the pointer answers "does the user have a résumé, and when".

**4. Endpoints.** `PUT /api/v1/me/resume` (store file → S3, parse text, extract + merge skills is a separate existing call; this one stores + returns metadata), `GET /api/v1/me/resume` (metadata: present + timestamp), `DELETE /api/v1/me/resume`. The verdict `POST .../verdict` is reworked: instead of accepting an upload, it reads the stored résumé from S3, so "re-run coherence" needs no file. If no résumé is stored, it returns a state the UI renders as "upload once".

**5. Server wiring mirrors Meili/LLM.** `cmd/server` builds the blobstore only when all four `S3_*` are set; passes it into `handler.Config`; nil disables storage. `internal/config` gains the four fields.

**6. Degradation.** No S3 → the profile-form upload behaves exactly as today (extract skills in-request, nothing stored); the verdict coherence then falls back to requiring an upload in the request (the current behavior) rather than reading storage. This keeps the feature additive.

## Risks / Trade-offs

- **PII now persisted (unencrypted) in S3** → mitigated by access-controlled credentials and a private bucket; encryption-at-rest is a noted follow-up. Deletion endpoint lets a user remove it.
- **S3 outage/latency on an interactive path** → uploads/reads are bounded and best-effort where possible; a storage error on upload surfaces as a 5xx only for the store step, while skill extraction (in-request) can still succeed. Coherence degrades if the object can't be fetched.
- **Provider coupling via env** → avoided in code (generic `S3_*`); only `freehire-ops` knows the bucket/host.
- **Migration on prod** → nullable columns, backward-compatible; apply before rolling the binary (the manual-migration seam).
- **Orphaned objects** → deleting a user cascades the pointer row but not the S3 object; a follow-up sweep can reconcile (low volume, noted).

## Migration Plan

1. `migrations/0039_users_resume_pointer.sql`: `ALTER TABLE users ADD COLUMN resume_object_key TEXT, ADD COLUMN resume_uploaded_at TIMESTAMPTZ;` (nullable).
2. `freehire-ops`: create the `freehire-resumes` bucket on Hetzner hel1 (private), mint/choose S3 credentials, add `S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY` to `/opt/freehire/.env`.
3. `make sqlc` + commit generated code; `go mod tidy` for `minio-go`.
4. Deploy: apply migration manually before rolling the binary; restart app with the new env.
5. Rollback: drop the two columns; unset `S3_*` (storage disables, no code change).

## Open Questions

- Whether to reuse telagon's existing Hetzner Object Storage access key or mint a freehire-specific key (ops decision; a dedicated key is cleaner for blast-radius).
- Encryption-at-rest: revisit if résumés are deemed high-sensitivity enough to warrant app-level AES-GCM.
