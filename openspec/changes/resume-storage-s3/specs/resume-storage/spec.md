## ADDED Requirements

### Requirement: Generic S3 blob abstraction

The system SHALL provide a minimal, provider-agnostic object-storage abstraction that stores and retrieves blobs by key. It SHALL be configured only by generic env vars (`S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`) and MUST NOT hard-code any bucket name, host, or provider in application code. When storage is not configured, the abstraction SHALL be absent (nil) and features that depend on it degrade rather than error.

#### Scenario: Storage unconfigured degrades
- **WHEN** the S3 env vars are not all set
- **THEN** no blob client is built and résumé storage is disabled, while the rest of the app runs normally (résumé upload still extracts skills in-request)

#### Scenario: Round-trip by key
- **WHEN** a blob is stored under a key and later fetched by the same key
- **THEN** the retrieved bytes equal the stored bytes

### Requirement: One stored résumé per user

The system SHALL store at most one résumé per user, under a per-user object key. A signed-in user SHALL be able to upload their résumé (PDF or text); the original file is stored in object storage and a pointer (object key + upload timestamp) is recorded on the user. Re-uploading SHALL overwrite the previous résumé.

#### Scenario: Upload stores the résumé and records the pointer
- **WHEN** a signed-in user uploads a résumé and storage is configured
- **THEN** the file is written to object storage under the user's key and the user's résumé pointer + timestamp are set

#### Scenario: Re-upload overwrites
- **WHEN** a user who already has a stored résumé uploads a new one
- **THEN** the stored object is replaced and the timestamp updated (still one résumé per user)

### Requirement: Résumé retrieval and deletion are owner-scoped

A user SHALL be able to see whether they have a stored résumé (and when it was uploaded) and to delete it. A user MUST only ever access their own résumé; keys are derived from the authenticated user id, never from client input.

#### Scenario: Metadata reflects storage state
- **WHEN** a signed-in user with a stored résumé requests their résumé status
- **THEN** the response reports a résumé is present with its upload timestamp

#### Scenario: Delete removes the object and the pointer
- **WHEN** a signed-in user deletes their résumé
- **THEN** the object is removed from storage and the user's résumé pointer is cleared

### Requirement: Single upload feeds both skills and coherence

Uploading a résumé SHALL be a single action that both extracts skills (as today) and stores the résumé for reuse. The verdict's coherence analysis SHALL read the user's stored résumé rather than requiring a fresh upload. When a résumé is already stored, the verdict SHALL offer to re-run coherence without a new upload; only a user with no stored résumé is prompted to upload.

#### Scenario: Coherence reuses the stored résumé
- **WHEN** a user with a stored résumé runs the verdict coherence check
- **THEN** the server reads the résumé text from storage and produces the coherence score without asking for a new upload

#### Scenario: No stored résumé prompts a single upload
- **WHEN** a user without a stored résumé opens the verdict
- **THEN** they are prompted to upload their résumé once, after which coherence can be re-run from storage

### Requirement: Résumé text is derived on read, not stored separately

The stored artifact SHALL be the original résumé file; the plain text used for skill extraction and coherence SHALL be derived from it on read. The system MUST NOT persist the extracted text in a second location that could drift from the stored file.

#### Scenario: Coherence parses the stored file
- **WHEN** coherence runs against a stored PDF résumé
- **THEN** the server fetches the file from storage and parses its text at that moment (no separately stored text copy)
