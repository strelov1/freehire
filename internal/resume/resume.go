// Package resume is the per-user stored-résumé use case: a signed-in user keeps at
// most one résumé, the original file living in S3 (internal/blobstore) under a per-user
// key, with a pointer (object key + upload time) recorded on the users row. It stores,
// reports, deletes, and derives the plain text of that résumé, so the one upload feeds
// both skill extraction and the verdict's coherence check without a second upload. When
// object storage is unconfigured the service is disabled (Enabled reports false) and
// callers degrade to the previous in-request extraction.
package resume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// pdftotextBin is the poppler binary used to extract résumé PDF text, overridable via
// PDFTOTEXT_BIN (default "pdftotext"; the production image installs poppler-utils). It is a
// package var so tests can point it at a stub. Unlike the CV renderer's TypstBin there is
// no "disabled" story to gate — PDF extraction is core to résumé upload — so it is resolved
// here rather than threaded through config.
var pdftotextBin = envOr("PDFTOTEXT_BIN", "pdftotext")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var (
	// ErrStorageDisabled is returned when object storage is unconfigured, so no résumé
	// can be stored, fetched, or deleted. The handler maps it to 501 (or degrades to the
	// per-request upload path).
	ErrStorageDisabled = errors.New("resume: storage is not configured")
	// ErrNotStored is returned when the user has no stored résumé (the handler maps it to
	// a conflict — the SPA prompts a single upload).
	ErrNotStored = errors.New("resume: no résumé stored")
)

// Meta reports whether a résumé is stored and, if so, when it was uploaded.
type Meta struct {
	Present    bool
	UploadedAt *time.Time
}

// Repository persists the per-user résumé pointer on the users row. Owner-scoped by id;
// the object key is derived from the id, never client input.
type Repository interface {
	Get(ctx context.Context, userID int64) (db.GetUserResumeRow, error)
	Set(ctx context.Context, userID int64, key string) error
	Clear(ctx context.Context, userID int64) error
	// SetEmbedding persists the derived CV vector + the embedder identity that produced
	// it (a nil vector clears it — e.g. when re-embedding a new CV failed, so the new
	// CV is never matched by the old vector). GetEmbedding reads them back.
	SetEmbedding(ctx context.Context, userID int64, vec []float64, model string) error
	GetEmbedding(ctx context.Context, userID int64) (db.GetUserResumeEmbeddingRow, error)
	// SetStructured persists the derived structured résumé and the geography derived from
	// it, as one write. GetStructured reads the blob, its stamps, and the current résumé
	// upload time so the Store can tell whether the structure still describes the stored CV.
	SetStructured(ctx context.Context, w StructuredWrite) error
	GetStructured(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error)
	// GetGeography reads the derived candidate geography plus the two stamps needed to
	// judge whether it still describes the stored CV.
	GetGeography(ctx context.Context, userID int64) (db.GetUserResumeGeographyRow, error)
}

// Geography is where the candidate is, as derived from their CV and stored. Countries is
// empty both when the CV named no place and when it named one the dictionary could not
// resolve; the two are distinguished in the database (NULL vs '{}') for the coverage
// metric, and deliberately NOT here — a consumer asking "where is this candidate" gets
// the same answer either way, which is "nowhere I can tell you".
type Geography struct {
	Countries []string
	Regions   []string
	Cities    []string
}

// StructuredWrite is one persist of the derived structured résumé together with the
// candidate geography derived from it — one row, one statement, one stamp. They travel
// together so the monotonic guard (`WHERE resume_uploaded_at = UploadedAt`) covers both:
// a slow extraction for a since-replaced CV writes neither, and a stored geography can
// never describe a different CV than the structure beside it.
//
// Countries/Regions/Cities carry a three-way answer, and the nil/empty distinction is the
// point rather than an accident:
//
//	nil          — not known: the CV states no location at all
//	empty slice  — the CV stated a place the curated dictionary could not resolve
//	non-empty    — what it resolved to
//
// Collapsing nil and empty would confuse "we don't know where this candidate is" with
// "this candidate is nowhere", and would throw away the dictionary's live coverage metric.
type StructuredWrite struct {
	UserID     int64
	Blob       []byte
	Model      string
	UploadedAt time.Time
	Countries  []string
	Regions    []string
	Cities     []string
}

// Store owns the résumé's object storage plus its pointer. blobs is nil when storage is
// unconfigured; the service then reports Enabled()==false and every operation returns
// ErrStorageDisabled (matching the searcher/facetCounter nil-guard pattern).
type Store struct {
	blobs blobstore.Store
	repo  Repository
}

// New builds the service over a blob store (nil when unconfigured) and a pointer repo.
func New(blobs blobstore.Store, repo Repository) *Store {
	return &Store{blobs: blobs, repo: repo}
}

// Enabled reports whether object storage is configured.
func (s *Store) Enabled() bool {
	return s != nil && s.blobs != nil
}

// Put stores (or replaces) the user's résumé: the original bytes go to S3 under the
// user's key and the pointer is stamped with the upload time. contentType is recorded on
// the object for a correct future download. Returns the resulting metadata.
func (s *Store) Put(ctx context.Context, userID int64, contentType string, data []byte) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	key := blobstore.ResumeKey(userID)
	if err := s.blobs.Put(ctx, key, contentType, bytes.NewReader(data), int64(len(data))); err != nil {
		return Meta{}, err
	}
	if err := s.repo.Set(ctx, userID, key); err != nil {
		return Meta{}, err
	}
	return s.Status(ctx, userID)
}

// SetEmbedding persists (or clears, with a nil vector) the user's derived CV embedding
// and the embedder identity that produced it. Independent of object storage — it only
// touches the pointer row — so it works whenever a CV was uploadable.
func (s *Store) SetEmbedding(ctx context.Context, userID int64, vec []float64, model string) error {
	return s.repo.SetEmbedding(ctx, userID, vec, model)
}

// Embedding returns the user's persisted CV vector and the embedder identity that
// produced it (both zero when none is stored). The caller ignores a vector whose model
// no longer matches the current embedder (stale).
func (s *Store) Embedding(ctx context.Context, userID int64) (vec []float64, model string, err error) {
	row, err := s.repo.GetEmbedding(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	return row.ResumeEmbedding, row.ResumeEmbeddingModel.String, nil
}

// SetStructured persists the user's derived structured résumé, stamped with the
// producing model and the résumé upload time it was derived from (so the read can tell
// whether it still describes the stored CV). Independent of object storage — it only
// touches the pointer row.
func (s *Store) SetStructured(ctx context.Context, userID int64, st resumeextract.Structured, model string, uploadedAt time.Time) error {
	blob, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("resume: marshal structured: %w", err)
	}
	countries, regions, cities := DeriveGeography(st.Location)
	return s.repo.SetStructured(ctx, StructuredWrite{
		UserID:     userID,
		Blob:       blob,
		Model:      model,
		UploadedAt: uploadedAt,
		Countries:  countries,
		Regions:    regions,
		Cities:     cities,
	})
}

// DeriveGeography derives the candidate's geography from the location line their
// structured résumé carries, and projects it onto the three stored columns.
//
// It is exported because it has TWO callers that must never disagree: the upload path
// (Store.SetStructured, above) and the reconciler (cmd/backfill-resume-geo). If the two
// derived differently, re-running the reconciler would rewrite rows the upload path had
// just written, and "a second run changes nothing" would quietly stop being true.
//
// The derivation itself is location.ParseResidence — the candidate entry point, never
// location.Parse: a job may be open anywhere and a person never is, so the global region
// and the work-mode hint are not part of where someone lives. It is deterministic and
// does no I/O, which is why it runs on the write path rather than as scheduled work.
//
// The nil-versus-empty decision lives here rather than in the parser, because it is a
// question about the INPUT, not about the resolution: ParseResidence returns nothing both
// for a CV that stated no location and for one whose location it could not resolve, and
// only this layer still knows which of the two happened.
func DeriveGeography(stated string) (countries, regions, cities []string) {
	if strings.TrimSpace(stated) == "" {
		return nil, nil, nil
	}
	res := location.ParseResidence(stated)
	return orEmpty(res.Countries), orEmpty(res.Regions), orEmpty(res.Cities)
}

// orEmpty turns a nil slice into an empty one, so a stated-but-unresolved location is
// stored as '{}' ("the dictionary was silent here") rather than NULL ("we don't know").
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Structured returns the user's derived structured résumé, but ONLY when it still
// describes the currently-stored CV — i.e. its stamp equals the résumé's upload time.
// ok is false (with no error) when none is stored or the stamp is stale (a newer CV
// whose extraction has not landed yet, or a persistent extraction outage), so the read
// surface never serves a structure derived from a superseded résumé.
//
// Staleness is keyed on the upload time ALONE, not the model stamp (unlike the CV
// embedding, which is re-checked against the current embedder). The structured résumé has
// no reconciler that re-derives it — only a re-upload does — so gating reads on the model
// would hide the parsed profile forever after an LLM_MODEL upgrade rather than refreshing
// it. Serving a best-effort, display-only structure from an older model is the better
// degradation; resume_structured_model is kept as provenance for a future backfill.
func (s *Store) Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error) {
	row, err := s.repo.GetStructured(ctx, userID)
	if err != nil {
		return resumeextract.Structured{}, false, err
	}
	if len(row.ResumeStructured) == 0 || !stampsEqual(row.ResumeStructuredUploadedAt, row.ResumeUploadedAt) {
		return resumeextract.Structured{}, false, nil
	}
	var st resumeextract.Structured
	if err := json.Unmarshal(row.ResumeStructured, &st); err != nil {
		// A corrupt blob is treated as absent (the next upload re-derives it), not an error.
		return resumeextract.Structured{}, false, nil
	}
	return st, true, nil
}

// stampsEqual reports whether two timestamps are both present and equal — the freshness
// rule for the structured résumé (mirrors the matchanalysis cache stamp comparison).
func stampsEqual(a, b pgtype.Timestamptz) bool {
	return a.Valid && b.Valid && a.Time.Equal(b.Time)
}

// Status reports whether the user has a stored résumé and when it was uploaded.
func (s *Store) Status(ctx context.Context, userID int64) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	ptr, err := s.repo.Get(ctx, userID)
	if err != nil {
		return Meta{}, err
	}
	return metaFromPointer(ptr), nil
}

// Text fetches the stored résumé and derives its plain text — parsing a PDF, or reading
// text as-is (the pasted-text path). ErrNotStored when the user has no résumé; the text
// is never persisted separately (derived on read, no drift).
func (s *Store) Text(ctx context.Context, userID int64) (string, error) {
	if !s.Enabled() {
		return "", ErrStorageDisabled
	}
	ptr, err := s.repo.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	if !ptr.ResumeObjectKey.Valid {
		return "", ErrNotStored
	}
	rc, err := s.blobs.Get(ctx, ptr.ResumeObjectKey.String)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("resume: read object: %w", err)
	}
	return extractText(data)
}

// Delete removes the stored object and clears the pointer.
func (s *Store) Delete(ctx context.Context, userID int64) error {
	if !s.Enabled() {
		return ErrStorageDisabled
	}
	if err := s.blobs.Delete(ctx, blobstore.ResumeKey(userID)); err != nil {
		return err
	}
	return s.repo.Clear(ctx, userID)
}

func metaFromPointer(ptr db.GetUserResumeRow) Meta {
	if !ptr.ResumeObjectKey.Valid {
		return Meta{}
	}
	m := Meta{Present: true}
	if ptr.ResumeUploadedAt.Valid {
		t := ptr.ResumeUploadedAt.Time
		m.UploadedAt = &t
	}
	return m
}

// extractText derives plain text from a stored résumé: a PDF (magic number "%PDF") is
// parsed, anything else is read as UTF-8 text (the pasted-text path). Sniffing the bytes
// keeps the blobstore interface a pure key→bytes abstraction, free of content-type
// plumbing.
func extractText(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return ExtractPDFText(data)
	}
	return string(data), nil
}

// pdfExtractTimeout bounds a single pdftotext run so a pathological PDF can never hang a
// request; extracting a normal résumé is well under a second.
const pdfExtractTimeout = 15 * time.Second

// maxPDFTextBytes bounds the text one extraction retains. A PDF's text layer is not bounded
// by the file: a 472KB upload has been observed yielding 12.6MB of text, and the whole string
// then travels onward to the PII filter and the embedder, so the upload limit is not the
// ceiling it looks like. Two megabytes is an order of magnitude past the densest real résumé
// (a hundred pages of prose is a few hundred KB), so only a pathological document is ever
// truncated — and for one of those, truncating beats propagating.
const maxPDFTextBytes = 2 << 20

// ExtractPDFText extracts plain text from PDF bytes, shared by the résumé store and the
// upload handler (which wraps a returned error into a 400). It shells out to poppler's
// pdftotext, which — unlike a pure-Go parser — decodes CID/Identity-H fonts through their
// ToUnicode CMap (as produced by Canva and similar builders), where the previous library
// silently returned empty text. The bytes go to a temp file (never onto argv) and the text
// is read from stdout, retaining at most maxPDFTextBytes. A non-zero exit (corrupt/undecodable
// PDF) is an error; a clean run yielding no text (an image-only PDF with no text layer)
// returns ("", nil), which the handler renders as the "scan or image" rejection.
func ExtractPDFText(data []byte) (string, error) {
	dir, err := os.MkdirTemp("", "resume-pdf-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	inPath := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(inPath, data, 0o600); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), pdfExtractTimeout)
	defer cancel()

	// Only fixed flags and a temp path reach argv: -q silences warnings on slightly
	// malformed but readable PDFs; "-" writes the extracted text to stdout.
	cmd := exec.CommandContext(ctx, pdftotextBin, "-q", "-nopgbrk", inPath, "-")
	out := cappedBuffer{max: maxPDFTextBytes}
	// stderr only ever becomes part of an error message, so a few kilobytes is plenty; it is
	// bounded for the same reason stdout is.
	errBuf := cappedBuffer{max: 8 << 10}
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("resume: invalid PDF: %w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return trimPartialRune(out.String()), nil
}

// cappedBuffer accumulates at most max bytes and discards the rest, so a subprocess writing
// an unbounded amount of output cannot grow this process's heap with it.
//
// It reports the discarded bytes as written on purpose. Returning an error (or short count)
// would break the child's stdout pipe mid-run and surface as a failed command, turning a
// merely enormous document into an extraction error; the existing timeout is what bounds how
// long the child may keep producing output nobody keeps.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }

// trimPartialRune drops a trailing incomplete UTF-8 sequence, which truncating at a byte cap
// leaves behind whenever the cap falls inside a multi-byte rune. The text goes on into a JSON
// request body, an LLM prompt and an embedder, so it must not end mid-rune. A genuine U+FFFD
// in the document decodes with size 3 and is left alone.
func trimPartialRune(s string) string {
	for s != "" {
		if r, size := utf8.DecodeLastRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// QueriesRepository adapts *db.Queries to Repository, mapping the pointer to the nullable
// users columns.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository wraps *db.Queries as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func (r *QueriesRepository) Get(ctx context.Context, userID int64) (db.GetUserResumeRow, error) {
	return r.q.GetUserResume(ctx, userID)
}

func (r *QueriesRepository) Set(ctx context.Context, userID int64, key string) error {
	return r.q.SetUserResume(ctx, db.SetUserResumeParams{
		ID:              userID,
		ResumeObjectKey: pgtype.Text{String: key, Valid: true},
	})
}

func (r *QueriesRepository) Clear(ctx context.Context, userID int64) error {
	return r.q.ClearUserResume(ctx, userID)
}

func (r *QueriesRepository) SetEmbedding(ctx context.Context, userID int64, vec []float64, model string) error {
	return r.q.SetUserResumeEmbedding(ctx, db.SetUserResumeEmbeddingParams{
		ID:                   userID,
		ResumeEmbedding:      vec,
		ResumeEmbeddingModel: pgtype.Text{String: model, Valid: model != ""},
	})
}

func (r *QueriesRepository) GetEmbedding(ctx context.Context, userID int64) (db.GetUserResumeEmbeddingRow, error) {
	return r.q.GetUserResumeEmbedding(ctx, userID)
}

func (r *QueriesRepository) SetStructured(ctx context.Context, w StructuredWrite) error {
	return r.q.SetUserResumeStructured(ctx, db.SetUserResumeStructuredParams{
		ID:                         w.UserID,
		ResumeStructured:           w.Blob,
		ResumeStructuredModel:      pgtype.Text{String: w.Model, Valid: w.Model != ""},
		ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: w.UploadedAt, Valid: true},
		ResumeCountries:            w.Countries,
		ResumeRegions:              w.Regions,
		ResumeCities:               w.Cities,
	})
}

func (r *QueriesRepository) GetStructured(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return r.q.GetUserResumeStructured(ctx, userID)
}

// Geography returns the candidate geography derived from the user's CV, under the SAME
// freshness rule as Structured: it is served only while its stamp still equals the
// résumé's upload time. ok is false (with no error) when none is stored or the stamp is
// stale, so a geography derived from a superseded CV is treated as absent rather than
// answering "where is this candidate" from a document that has been replaced.
func (s *Store) Geography(ctx context.Context, userID int64) (Geography, bool, error) {
	row, err := s.repo.GetGeography(ctx, userID)
	if err != nil {
		return Geography{}, false, err
	}
	if !stampsEqual(row.ResumeStructuredUploadedAt, row.ResumeUploadedAt) {
		return Geography{}, false, nil
	}
	return Geography{
		Countries: row.ResumeCountries,
		Regions:   row.ResumeRegions,
		Cities:    row.ResumeCities,
	}, true, nil
}

func (r *QueriesRepository) GetGeography(ctx context.Context, userID int64) (db.GetUserResumeGeographyRow, error) {
	return r.q.GetUserResumeGeography(ctx, userID)
}
