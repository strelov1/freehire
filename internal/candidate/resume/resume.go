// Package resume is the per-user stored-résumé use case: a signed-in user keeps at
// most one résumé, the original file living in S3 (internal/platform/blobstore) under a
// per-upload key, with a pointer (object key + upload time) recorded on the users row — the
// one source of which object is current. It stores,
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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// pdftotextBin is the poppler binary used to extract résumé PDF text, overridable via
// PDFTOTEXT_BIN (default "pdftotext"). A package var so tests can point it at a stub.
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
	// ErrAlreadyStored is PutIfAbsent's refusal: the user already has a stored résumé.
	ErrAlreadyStored = errors.New("resume: a résumé is already stored")
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
	// SetStructured persists the derived structured résumé and its geography as one write,
	// guarded by the monotonic `resume_uploaded_at = w.UploadedAt` stamp check. applied false
	// means a late write for a superseded CV was dropped and must not be treated as landed.
	// GetStructured reads the blob, its stamps, and the current upload time so the Store can
	// tell whether the structure still describes the stored CV.
	SetStructured(ctx context.Context, w StructuredWrite) (applied bool, err error)
	GetStructured(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error)
	// GetGeography reads the derived candidate geography plus the two stamps needed to
	// judge whether it still describes the stored CV.
	GetGeography(ctx context.Context, userID int64) (db.GetUserResumeGeographyRow, error)
	GetCandidateContacts(ctx context.Context, userID int64) ([]byte, error)
	SetCandidateContacts(ctx context.Context, userID int64, blob []byte) error
	SetExtractFailed(ctx context.Context, userID int64, detail string, uploadedAt time.Time) error
	SetExtractPending(ctx context.Context, userID int64, uploadedAt time.Time) error
	// WithUploadLock runs fn holding the user's CV-upload lock, passing the repository fn's
	// reads and writes must go through so they share it. Every pointer write (upload, delete)
	// takes it, which makes PutIfAbsent's presence check and pointer write one step. Keep it short:
	// it holds a pooled connection, so never put an object-store call inside fn.
	WithUploadLock(ctx context.Context, userID int64, fn func(Repository) error) error
}

// Geography is where the candidate is, as derived from their CV and stored. Countries is empty
// both when the CV named no place and when the dictionary could not resolve it; the database
// distinguishes the two (NULL vs '{}') for the coverage metric, consumers do not.
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
// Countries/Regions/Cities carry a three-way answer; the nil/empty distinction is deliberate:
//
//	nil          — not known: the CV states no location at all
//	empty slice  — the CV stated a place the curated dictionary could not resolve
//	non-empty    — what it resolved to
//
// Collapsing nil and empty would confuse "unknown" with "nowhere" and lose the coverage metric.
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
// ErrStorageDisabled.
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

// Put stores (or replaces) the user's résumé: the original bytes go to S3 under a key of this
// upload's own, and the pointer is moved to it and stamped with the upload time. contentType is
// recorded on the object. Returns the resulting metadata.
func (s *Store) Put(ctx context.Context, userID int64, contentType string, data []byte) (Meta, error) {
	return s.put(ctx, userID, contentType, data, false)
}

// PutIfAbsent stores the user's résumé only when none is stored, and returns
// ErrAlreadyStored otherwise. The check and the pointer write hold the same per-user lock
// every pointer write takes, so two first uploads cannot both see no résumé.
func (s *Store) PutIfAbsent(ctx context.Context, userID int64, contentType string, data []byte) (Meta, error) {
	return s.put(ctx, userID, contentType, data, true)
}

// put writes the bytes to an object of their own OUTSIDE any transaction — an object-store
// round trip must never hold a pooled connection — and then, briefly under the per-user upload
// lock, checks presence (firstOnly) and moves the pointer to it. The object the pointer left is
// deleted after commit, and the new one when the pointer did not move to it. Both are
// best-effort cleanup; account deletion removes every object under the user's prefix, which
// is the backstop for a cleanup that failed or a process that died between the steps.
func (s *Store) put(ctx context.Context, userID int64, contentType string, data []byte, firstOnly bool) (Meta, error) {
	if !s.Enabled() {
		return Meta{}, ErrStorageDisabled
	}
	if firstOnly {
		// A cheap early answer that spares the upload; the check that decides is under the lock.
		if meta, err := s.Status(ctx, userID); err != nil {
			return Meta{}, err
		} else if meta.Present {
			return Meta{}, ErrAlreadyStored
		}
	}
	key := blobstore.ResumeUploadKey(userID, time.Now())
	if err := s.blobs.Put(ctx, key, contentType, bytes.NewReader(data), int64(len(data))); err != nil {
		observability.RecordCVUpload(false)
		return Meta{}, err
	}
	var superseded string
	var written Meta
	err := s.repo.WithUploadLock(ctx, userID, func(repo Repository) error {
		ptr, err := repo.Get(ctx, userID)
		if err != nil {
			return err
		}
		if firstOnly && metaFromPointer(ptr).Present {
			return ErrAlreadyStored
		}
		superseded = ptr.ResumeObjectKey.String
		if err := repo.Set(ctx, userID, key); err != nil {
			return err
		}
		// Read the stamp under the lock: after commit another upload may already have
		// replaced it, and pairing this upload's text with that stamp would let its
		// extraction pass the stamp guard as the other upload's.
		ptr, err = repo.Get(ctx, userID)
		written = metaFromPointer(ptr)
		return err
	})
	if errors.Is(err, ErrAlreadyStored) {
		s.discard(ctx, key)
		return Meta{}, err
	}
	if err != nil {
		// A failed COMMIT can still have committed on the server. Discard the object only once
		// a fresh read shows the pointer does not name it; an otherwise unreadable pointer keeps
		// it, and account deletion is the backstop for that.
		// No account row means account deletion won the race: nothing can name the object.
		ptr, readErr := s.repo.Get(context.WithoutCancel(ctx), userID)
		if errors.Is(readErr, pgx.ErrNoRows) || (readErr == nil && ptr.ResumeObjectKey.String != key) {
			s.discard(ctx, key)
		}
		observability.RecordCVUpload(false)
		return Meta{}, err
	}
	if superseded != "" && superseded != key {
		s.discard(ctx, superseded)
	}
	observability.RecordCVUpload(true)
	return written, nil
}

// discard deletes an object no pointer refers to. Best-effort: the write it follows has
// already succeeded or failed on its own terms, so a failure here is logged, not returned.
func (s *Store) discard(ctx context.Context, key string) {
	if err := s.blobs.Delete(context.WithoutCancel(ctx), key); err != nil {
		log.Printf("resume: deleting unreferenced object %s: %v", key, err)
	}
}

// SetStructured persists the user's derived structured résumé, stamped with the producing model
// and the upload time it was derived from. Independent of object storage.
func (s *Store) SetStructured(ctx context.Context, userID int64, st resumeextract.Structured, model string, uploadedAt time.Time) error {
	blob, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("resume: marshal structured: %w", err)
	}
	countries, regions, cities := DeriveGeography(st.Location)
	applied, err := s.repo.SetStructured(ctx, StructuredWrite{
		UserID:     userID,
		Blob:       blob,
		Model:      model,
		UploadedAt: uploadedAt,
		Countries:  countries,
		Regions:    regions,
		Cities:     cities,
	})
	if err != nil {
		return err
	}
	if !applied {
		// The stamp guard dropped this write (a superseded upload's late extraction). Filling
		// contacts from st would leak it into owned contacts permanently, since FillEmpty never
		// overwrites a non-empty owned field.
		return nil
	}
	// Fill-empty into owned overrides (identity, headline, summary, languages,
	// certifications); never overwrite a candidate's own edit.
	if err := s.FillEmptyOwnedFromStructured(ctx, userID, st); err != nil {
		return fmt.Errorf("resume: fill owned: %w", err)
	}
	return nil
}

// DeriveGeography derives the candidate's geography from the location line their
// structured résumé carries, and projects it onto the three stored columns.
//
// Exported because two callers must never disagree: the upload path (Store.SetStructured) and
// the reconciler (cmd/backfill-resume-geo); otherwise a re-run would rewrite fresh rows.
//
// The derivation is location.ParseResidence, never location.Parse: a person is never open
// anywhere, so the global region and work-mode hint do not apply. It is deterministic and does
// no I/O, so it runs on the write path.
//
// The nil-versus-empty decision lives here, not in the parser: ParseResidence returns nothing
// both for a CV with no location and for an unresolvable one, and only this layer knows which.
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
// ok is false (with no error) when none is stored or the stamp is stale, so the read surface
// never serves a structure derived from a superseded résumé.
//
// Staleness is keyed on the upload time ALONE, not the model stamp: nothing re-derives the
// structured résumé but a re-upload, so gating on the model would hide the profile forever after
// an LLM_MODEL upgrade. resume_structured_model is kept as provenance.
//
// This is ProfileReadForUser's Current branch, derived there so the two answers cannot diverge.
func (s *Store) Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error) {
	read, err := s.ProfileReadForUser(ctx, userID)
	if err != nil || !read.Current {
		return resumeextract.Structured{}, false, err
	}
	return read.Structure, true, nil
}

// ProfileRead is the cookie résumé status composition: a current structure when the
// stamp matches, otherwise provisional contacts from a superseded blob plus Pending.
// Seed gates keep using Structured (ok=false when stale) and layer ProvisionalContacts
// for identity while pending. See AGENTS.md for the identity table.
type ProfileRead struct {
	Structure resumeextract.Structured
	Current   bool // stamp matches — Structure is the full current extract
	Pending   bool // a résumé is present but the structure stamp does not match it
}

// ProfileReadForUser returns what GET /me/resume needs for the profile Profile tab.
// When the stamp is current, Structure is the full extract. When it is stale or the
// extract never landed, Pending is true and Structure carries only contact fields from
// any superseded blob (empty when none) — semantic sections stay empty until a current
// stamp lands.
func (s *Store) ProfileReadForUser(ctx context.Context, userID int64) (ProfileRead, error) {
	row, err := s.repo.GetStructured(ctx, userID)
	if err != nil {
		return ProfileRead{}, err
	}
	hasResume := row.ResumeUploadedAt.Valid
	hasBlob := len(row.ResumeStructured) > 0
	current := hasBlob && stampsEqual(row.ResumeStructuredUploadedAt, row.ResumeUploadedAt)
	pending := hasResume && !current

	if !hasBlob {
		return ProfileRead{Pending: pending}, nil
	}
	var st resumeextract.Structured
	if err := json.Unmarshal(row.ResumeStructured, &st); err != nil {
		// Corrupt blob: treat like absent content, but still surface pending if a CV exists.
		return ProfileRead{Pending: pending}, nil
	}
	if current {
		return ProfileRead{Structure: st, Current: true}, nil
	}
	return ProfileRead{
		Structure: provisionalContacts(st),
		Pending:   pending,
	}, nil
}

// ProvisionalContacts returns identity fields from a superseded structured blob when the
// stamp is not current with the upload. ok is false when the stamp is current (callers
// use Structured), when no blob exists, or when the blob carries no contact fields.
// Semantic sections are never included — the same slice ProfileRead serves while pending.
func (s *Store) ProvisionalContacts(ctx context.Context, userID int64) (resumeextract.Structured, bool, error) {
	read, err := s.ProfileReadForUser(ctx, userID)
	if err != nil || read.Current || !hasContactFields(read.Structure) {
		return resumeextract.Structured{}, false, err
	}
	return read.Structure, true, nil
}

// provisionalContacts keeps identity fields from a superseded extract for the pending
// window. Education, summary, skills and the rest wait for a current stamp.
func provisionalContacts(st resumeextract.Structured) resumeextract.Structured {
	return resumeextract.Structured{
		FullName: st.FullName,
		Email:    st.Email,
		Phone:    st.Phone,
		Location: st.Location,
		Links:    append([]string(nil), st.Links...),
	}
}

func hasContactFields(st resumeextract.Structured) bool {
	return st.FullName != "" || st.Email != "" || st.Phone != "" || st.Location != "" || len(st.Links) > 0
}

// stampsEqual reports whether two timestamps are both present and equal: the freshness rule for
// the structured résumé.
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

// UploadedAt returns the résumé upload stamp when one is recorded, without requiring
// object storage to be configured. Used by CV bootstrap freshness (base behind upload).
func (s *Store) UploadedAt(ctx context.Context, userID int64) (*time.Time, error) {
	row, err := s.repo.GetStructured(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !row.ResumeUploadedAt.Valid {
		return nil, nil
	}
	t := row.ResumeUploadedAt.Time
	return &t, nil
}

// Text fetches the stored résumé and derives its plain text — parsing a PDF, or reading
// text as-is (the pasted-text path). ErrNotStored when the user has no résumé; the text
// is never persisted separately (derived on read, no drift).
func (s *Store) Text(ctx context.Context, userID int64) (string, error) {
	text, _, err := s.TextAndUploadedAt(ctx, userID)
	return text, err
}

// TextAndUploadedAt behaves like Text, but also returns the upload timestamp from the SAME
// pointer read. Do not use Text and Status as two reads: a résumé replaced between them would
// pair the old upload's text with the new upload's stamp.
func (s *Store) TextAndUploadedAt(ctx context.Context, userID int64) (string, time.Time, error) {
	data, uploadedAt, err := s.read(ctx, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	text, err := extractText(data)
	if err != nil {
		return "", time.Time{}, err
	}
	return text, uploadedAt, nil
}

// File returns the stored résumé exactly as it was uploaded — a PDF, or the text a
// candidate pasted. ErrNotStored when there is none.
func (s *Store) File(ctx context.Context, userID int64) ([]byte, error) {
	data, _, err := s.read(ctx, userID)
	return data, err
}

// read fetches the stored bytes and the upload stamp from the SAME pointer read (see
// TextAndUploadedAt for why the two must not come from separate reads).
func (s *Store) read(ctx context.Context, userID int64) ([]byte, time.Time, error) {
	if !s.Enabled() {
		return nil, time.Time{}, ErrStorageDisabled
	}
	ptr, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !ptr.ResumeObjectKey.Valid || !ptr.ResumeUploadedAt.Valid {
		return nil, time.Time{}, ErrNotStored
	}
	rc, err := s.blobs.Get(ctx, ptr.ResumeObjectKey.String)
	if err != nil {
		if blobstore.IsNotFound(err) {
			return nil, time.Time{}, ErrNotStored
		}
		return nil, time.Time{}, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		// MinIO/S3 GetObject is lazy: a missing key often surfaces only on the first read.
		if blobstore.IsNotFound(err) {
			return nil, time.Time{}, ErrNotStored
		}
		return nil, time.Time{}, fmt.Errorf("resume: read object: %w", err)
	}
	return data, ptr.ResumeUploadedAt.Time, nil
}

// Delete clears the pointer under the per-user upload lock, so it cannot interleave with an
// upload, and then removes the object the pointer named. The object goes after the commit, like
// a superseded upload's: a pointer must never name an object that is already gone. Its removal
// is best-effort cleanup — the CV is deleted once the pointer is, and a retry would find nothing
// to name the object by — so a failure is logged; account deletion removes every object under
// the user's prefix.
func (s *Store) Delete(ctx context.Context, userID int64) error {
	if !s.Enabled() {
		return ErrStorageDisabled
	}
	var key string
	err := s.repo.WithUploadLock(ctx, userID, func(repo Repository) error {
		ptr, err := repo.Get(ctx, userID)
		if err != nil {
			return err
		}
		key = ptr.ResumeObjectKey.String
		return repo.Clear(ctx, userID)
	})
	if err != nil {
		return err
	}
	if key != "" {
		s.discard(ctx, key)
	}
	return nil
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

// maxPDFTextBytes bounds the text one extraction retains. A PDF's text layer is not bounded by
// the file size, and the string travels on to structured extraction and the embedder. Two
// megabytes is far past any real résumé, so only a pathological document is truncated.
const maxPDFTextBytes = 2 << 20

// ExtractPDFText extracts plain text from PDF bytes, shared by the résumé store and the upload
// handler (which wraps an error into a 400). It shells out to poppler's pdftotext, which decodes
// CID/Identity-H fonts through their ToUnicode CMap (as Canva produces) where a pure-Go parser
// returned empty text. The bytes go to a temp file (never argv); at most maxPDFTextBytes is read
// from stdout. A non-zero exit is an error; a clean run with no text (an image-only PDF) returns
// ("", nil), which the handler renders as the "scan or image" rejection.
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
	// stderr only becomes part of an error message, so a few kilobytes is plenty.
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
// It reports the discarded bytes as written on purpose: an error or short count would break the
// child's pipe and turn an enormous document into an extraction error. The timeout bounds the
// child's runtime.
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
// leaves behind when the cap falls inside a multi-byte rune; the text goes into JSON, prompts
// and an embedder. A genuine U+FFFD decodes with size 3 and is left alone.
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
// users columns. pool is what WithUploadLock opens its transaction on.
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository wraps *db.Queries as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

// WithUploadLocks gives the repository the pool WithUploadLock needs. Every wiring that
// stores CVs must set it; one that only reads them (a worker) has nothing to serialise.
func (r *QueriesRepository) WithUploadLocks(pool *pgxpool.Pool) *QueriesRepository {
	r.pool = pool
	return r
}

// WithUploadLock runs fn in a transaction holding the user's upload lock, with a repository
// bound to that transaction. Without a pool it runs fn unlocked.
func (r *QueriesRepository) WithUploadLock(ctx context.Context, userID int64, fn func(Repository) error) error {
	if r.pool == nil {
		return fn(r)
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if err := q.LockUserResumeUpload(ctx, userID); err != nil {
			return err
		}
		return fn(&QueriesRepository{q: q})
	})
}

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

func (r *QueriesRepository) SetStructured(ctx context.Context, w StructuredWrite) (bool, error) {
	rows, err := r.q.SetUserResumeStructured(ctx, db.SetUserResumeStructuredParams{
		ID:                         w.UserID,
		ResumeStructured:           w.Blob,
		ResumeStructuredModel:      pgtype.Text{String: w.Model, Valid: w.Model != ""},
		ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: w.UploadedAt, Valid: true},
		ResumeCountries:            w.Countries,
		ResumeRegions:              w.Regions,
		ResumeCities:               w.Cities,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *QueriesRepository) GetStructured(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return r.q.GetUserResumeStructured(ctx, userID)
}

// Geography returns the candidate geography derived from the user's CV, under the SAME
// freshness rule as Structured: served only while its stamp equals the upload time. ok is false
// (with no error) when none is stored or the stamp is stale.
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

func (r *QueriesRepository) GetCandidateContacts(ctx context.Context, userID int64) ([]byte, error) {
	return r.q.GetUserCandidateContacts(ctx, userID)
}

func (r *QueriesRepository) SetCandidateContacts(ctx context.Context, userID int64, blob []byte) error {
	return r.q.SetUserCandidateContacts(ctx, db.SetUserCandidateContactsParams{
		ID:                userID,
		CandidateContacts: blob,
	})
}

func (r *QueriesRepository) SetExtractFailed(ctx context.Context, userID int64, detail string, uploadedAt time.Time) error {
	return r.q.SetUserResumeExtractFailed(ctx, db.SetUserResumeExtractFailedParams{
		ID:                  userID,
		ResumeExtractDetail: pgtype.Text{String: detail, Valid: detail != ""},
		ResumeExtractFor:    pgtype.Timestamptz{Time: uploadedAt, Valid: true},
	})
}

func (r *QueriesRepository) SetExtractPending(ctx context.Context, userID int64, uploadedAt time.Time) error {
	return r.q.SetUserResumeExtractPending(ctx, db.SetUserResumeExtractPendingParams{
		ID:               userID,
		ResumeExtractFor: pgtype.Timestamptz{Time: uploadedAt, Valid: true},
	})
}
