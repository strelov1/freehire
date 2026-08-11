package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// Parse status values persisted on users.resume_extract_status.
const (
	ExtractStatusPending = "pending"
	ExtractStatusOK      = "ok"
	ExtractStatusFailed  = "failed"
)

// Contacts is the candidate-owned identity block. Independent of resume_structured.
type Contacts struct {
	FullName string   `json:"full_name,omitempty"`
	Email    string   `json:"email,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Location string   `json:"location,omitempty"`
	Links    []string `json:"links,omitempty"`
}

const (
	maxContactName     = 200
	maxContactEmail    = 320
	maxContactPhone    = 64
	maxContactLocation = 200
	maxContactLink     = 500
	maxContactLinks    = 20
)

// Sanitize bounds contact fields the same way CV headers are bounded.
func (c *Contacts) Sanitize() {
	if c == nil {
		return
	}
	c.FullName = clipRunes(strings.TrimSpace(c.FullName), maxContactName)
	c.Email = clipRunes(strings.TrimSpace(c.Email), maxContactEmail)
	c.Phone = clipRunes(strings.TrimSpace(c.Phone), maxContactPhone)
	c.Location = clipRunes(strings.TrimSpace(c.Location), maxContactLocation)
	out := make([]string, 0, len(c.Links))
	for _, l := range c.Links {
		l = clipRunes(strings.TrimSpace(l), maxContactLink)
		if l == "" {
			continue
		}
		out = append(out, l)
		if len(out) >= maxContactLinks {
			break
		}
	}
	c.Links = out
}

// Empty reports whether every contact field is blank.
func (c Contacts) Empty() bool {
	return c.FullName == "" && c.Email == "" && c.Phone == "" && c.Location == "" && len(c.Links) == 0
}

// AsStructured projects contacts onto the structured contact fields for seed/heal.
func (c Contacts) AsStructured() resumeextract.Structured {
	return resumeextract.Structured{
		FullName: c.FullName,
		Email:    c.Email,
		Phone:    c.Phone,
		Location: c.Location,
		Links:    append([]string(nil), c.Links...),
	}
}

// ContactsFromStructured copies contact fields out of a structured résumé.
func ContactsFromStructured(st resumeextract.Structured) Contacts {
	c := Contacts{
		FullName: st.FullName,
		Email:    st.Email,
		Phone:    st.Phone,
		Location: st.Location,
		Links:    append([]string(nil), st.Links...),
	}
	c.Sanitize()
	return c
}

// FillEmpty copies non-empty fields from src into dst only where dst is empty.
func FillEmpty(dst *Contacts, src Contacts) {
	if dst == nil {
		return
	}
	if dst.FullName == "" {
		dst.FullName = src.FullName
	}
	if dst.Email == "" {
		dst.Email = src.Email
	}
	if dst.Phone == "" {
		dst.Phone = src.Phone
	}
	if dst.Location == "" {
		dst.Location = src.Location
	}
	if len(dst.Links) == 0 && len(src.Links) > 0 {
		dst.Links = append([]string(nil), src.Links...)
	}
	dst.Sanitize()
}

func clipRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

func decodeContacts(blob []byte) Contacts {
	if len(blob) == 0 {
		return Contacts{}
	}
	var c Contacts
	if err := json.Unmarshal(blob, &c); err != nil {
		return Contacts{}
	}
	c.Sanitize()
	return c
}

// ExtractStatus is the wire-facing parse status for the current upload.
type ExtractStatus struct {
	Status string // pending | ok | failed | "" when no résumé
	Detail string
}

// ResolveExtractStatus derives status from the row: prefer persisted status when it
// applies to the current upload; otherwise infer from stamp match.
func ResolveExtractStatus(row db.GetUserResumeStructuredRow) ExtractStatus {
	if !row.ResumeUploadedAt.Valid {
		return ExtractStatus{}
	}
	if row.ResumeExtractFor.Valid && row.ResumeExtractFor.Time.Equal(row.ResumeUploadedAt.Time) &&
		row.ResumeExtractStatus.Valid && row.ResumeExtractStatus.String != "" {
		detail := ""
		if row.ResumeExtractDetail.Valid {
			detail = row.ResumeExtractDetail.String
		}
		return ExtractStatus{Status: row.ResumeExtractStatus.String, Detail: detail}
	}
	if stampsEqual(row.ResumeStructuredUploadedAt, row.ResumeUploadedAt) && len(row.ResumeStructured) > 0 {
		return ExtractStatus{Status: ExtractStatusOK}
	}
	return ExtractStatus{Status: ExtractStatusPending}
}

// LastStructureBlob unmarshals resume_structured even when the stamp is stale.
func LastStructureBlob(row db.GetUserResumeStructuredRow) (resumeextract.Structured, bool) {
	if len(row.ResumeStructured) == 0 {
		return resumeextract.Structured{}, false
	}
	var st resumeextract.Structured
	if err := json.Unmarshal(row.ResumeStructured, &st); err != nil {
		return resumeextract.Structured{}, false
	}
	return st, true
}

// CandidateContacts returns the owned contacts, or empty when unset.
func (s *Store) CandidateContacts(ctx context.Context, userID int64) (Contacts, error) {
	blob, err := s.repo.GetCandidateContacts(ctx, userID)
	if err != nil {
		return Contacts{}, err
	}
	return decodeContacts(blob), nil
}

// SetCandidateContacts replaces the owned contacts after Sanitize.
func (s *Store) SetCandidateContacts(ctx context.Context, userID int64, c Contacts) (Contacts, error) {
	c.Sanitize()
	blob, err := json.Marshal(c)
	if err != nil {
		return Contacts{}, fmt.Errorf("resume: marshal contacts: %w", err)
	}
	if err := s.repo.SetCandidateContacts(ctx, userID, blob); err != nil {
		return Contacts{}, err
	}
	return c, nil
}

// FillEmptyContactsFromStructured merges extract contacts into owned contacts
// without overwriting non-empty owned fields, then persists.
func (s *Store) FillEmptyContactsFromStructured(ctx context.Context, userID int64, st resumeextract.Structured) error {
	owned, err := s.CandidateContacts(ctx, userID)
	if err != nil {
		return err
	}
	before := owned
	FillEmpty(&owned, ContactsFromStructured(st))
	if contactsEqual(owned, before) {
		return nil
	}
	_, err = s.SetCandidateContacts(ctx, userID, owned)
	return err
}

func contactsEqual(a, b Contacts) bool {
	if a.FullName != b.FullName || a.Email != b.Email || a.Phone != b.Phone || a.Location != b.Location {
		return false
	}
	if len(a.Links) != len(b.Links) {
		return false
	}
	for i := range a.Links {
		if a.Links[i] != b.Links[i] {
			return false
		}
	}
	return true
}

// ReplaceContactsFromStructured overwrites owned contacts from a current structure.
func (s *Store) ReplaceContactsFromStructured(ctx context.Context, userID int64, st resumeextract.Structured) (Contacts, error) {
	return s.SetCandidateContacts(ctx, userID, ContactsFromStructured(st))
}

// MarkExtractFailed records a failed extract for the given upload stamp.
func (s *Store) MarkExtractFailed(ctx context.Context, userID int64, detail string, uploadedAt time.Time) error {
	detail = clipRunes(strings.TrimSpace(detail), 200)
	return s.repo.SetExtractFailed(ctx, userID, detail, uploadedAt)
}

// MarkExtractPending resets extract status to pending for the given upload stamp.
func (s *Store) MarkExtractPending(ctx context.Context, userID int64, uploadedAt time.Time) error {
	return s.repo.SetExtractPending(ctx, userID, uploadedAt)
}

// StructuredRow returns the raw GetUserResumeStructured row for status composition.
func (s *Store) StructuredRow(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return s.repo.GetStructured(ctx, userID)
}

// StructureForSeed returns a structured résumé suitable for CV seeding: body fields
// come from the current extract when stamped. A superseded blob contributes contacts
// only — semantic sections wait for a matching stamp (same rule as ProvisionalContacts).
// Contact fields prefer candidate-owned contacts when set (owned wins as a block).
// ok is true when there is any identity or body worth seeding (caller still applies seedable).
// See AGENTS.md for the identity table.
func (s *Store) StructureForSeed(ctx context.Context, userID int64) (resumeextract.Structured, bool, error) {
	row, err := s.repo.GetStructured(ctx, userID)
	if err != nil {
		return resumeextract.Structured{}, false, err
	}
	st, hasBlob := LastStructureBlob(row)
	current := hasBlob && stampsEqual(row.ResumeStructuredUploadedAt, row.ResumeUploadedAt)
	if !hasBlob {
		st = resumeextract.Structured{}
	} else if !current {
		// Superseded blob is identity-only: summary/skills/projects wait for a current stamp.
		st = provisionalContacts(st)
	}

	owned, err := s.CandidateContacts(ctx, userID)
	if err != nil {
		return resumeextract.Structured{}, false, err
	}
	if !owned.Empty() {
		st.FullName = owned.FullName
		st.Email = owned.Email
		st.Phone = owned.Phone
		st.Location = owned.Location
		st.Links = append([]string(nil), owned.Links...)
	}

	ok := !owned.Empty() || hasBlob
	return st, ok, nil
}
