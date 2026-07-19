## MODIFIED Requirements

### Requirement: Extract skills from an uploaded resume

The system SHALL provide `POST /api/v1/me/resume/extract`, guarded by cookie session
authentication (`RequireAuth`), that turns an uploaded resume into a list of canonical skill
slugs using the existing deterministic `internal/skilltag` dictionary. The endpoint SHALL
accept either a PDF file via `multipart/form-data` (field `file`) or plain text via
`application/json` (`{"text": "..."}`), dispatched by `Content-Type`. PDF text extraction
SHALL decode any text-bearing PDF regardless of font encoding — including CID fonts with
`Identity-H` encoding mapped through an embedded `ToUnicode` CMap (as produced by Canva and
similar builders). Oversize uploads are rejected by the server's existing global
request-body limit (413). The resume file and text SHALL NOT be persisted to disk or
database, and SHALL NOT be logged; only the resulting skill slugs are returned.

#### Scenario: Extract skills from a PDF resume

- **WHEN** an authenticated user POSTs a PDF resume as `multipart/form-data` field `file`
- **THEN** the system extracts the PDF text, runs `skilltag.Parse`, and responds `200` with
  `{"data": {"skills": [<canonical slugs, deduplicated and sorted>]}}`

#### Scenario: Extract skills from a CID/Identity-H embedded-font PDF

- **WHEN** an authenticated user POSTs a text-based PDF whose fonts are subset CID TrueType
  with `Identity-H` encoding and a `ToUnicode` CMap (e.g. a Canva export)
- **THEN** the system extracts the selectable text layer and responds `200` with the derived
  skill slugs, rather than rejecting the file as a scan or image

#### Scenario: Extract skills from pasted text

- **WHEN** an authenticated user POSTs `application/json` `{"text": "...experienced in Go and Postgres..."}`
- **THEN** the system runs `skilltag.Parse` on the text and responds `200` with the extracted
  canonical skill slugs

#### Scenario: Resume with no recognizable skills

- **WHEN** an authenticated user submits a resume whose text resolves to no dictionary skills
- **THEN** the system responds `200` with `{"data": {"skills": []}}`

#### Scenario: Image-only or malformed PDF is rejected

- **WHEN** the request contains an image-only PDF that yields no extractable text, an
  undecodable/non-PDF file, or empty/whitespace text
- **THEN** the system responds `400` with an `{"error": ...}` envelope naming the cause
  (no text layer) and does not persist anything

#### Scenario: Oversized upload is rejected

- **WHEN** the uploaded request body exceeds the server's global body limit
- **THEN** the server responds `413` (enforced by the framework before the handler runs)

#### Scenario: Unauthenticated request is rejected

- **WHEN** a request without a valid session cookie hits the endpoint
- **THEN** the system responds `401` and performs no extraction
