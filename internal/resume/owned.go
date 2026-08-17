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

// Owned is everything about a candidate's résumé that the CANDIDATE edited directly,
// rather than whatever resume_structured most recently re-derived from an upload. It is
// the one owned-overlay mechanism for the whole "profile" — the fields a fresh CV upload
// would otherwise silently overwrite — not identity alone: a re-upload regenerates
// headline/summary/languages/certifications/education exactly as wholesale as it
// regenerates email/phone, so a candidate correcting any of them needs the same
// protection.
//
// Deliberately NOT here: Skills (owned already, as userprofile.Profile.Skills — a
// separate, CV-independent resource with its own edit surface), and Experience/Projects
// (owned already, as the experience bank — provenance-tracked, richer than flat
// overwrite; Education has no equivalent claim-level provenance need, so it stays a flat
// overlay like Languages/Certifications rather than growing a bank of its own).
//
// Persisted as one JSON blob on users.candidate_contacts — the column keeps its original
// name (a rename would need a migration for no reader-facing benefit); only the Go
// concept broadened.
type Owned struct {
	FullName       string                    `json:"full_name,omitempty"`
	Email          string                    `json:"email,omitempty"`
	Phone          string                    `json:"phone,omitempty"`
	Location       string                    `json:"location,omitempty"`
	Links          []string                  `json:"links,omitempty"`
	Headline       string                    `json:"headline,omitempty"`
	Summary        string                    `json:"summary,omitempty"`
	Languages      []string                  `json:"languages,omitempty"`
	Certifications []string                  `json:"certifications,omitempty"`
	Education      []resumeextract.Education `json:"education,omitempty"`

	// The five body fields above are owned per field (see ApplyBody), and a non-empty
	// value has always implied ownership — Sanitize keeps deriving that. But a candidate
	// clearing a field they'd previously set has no non-empty value left to signal it, so
	// an explicit save of that one field carries its own *Set flag: true even while the
	// value is "". Sanitize only ever turns these on (from a non-empty value); it never
	// turns one off, so an explicit clear survives every Sanitize call between the PUT
	// and the read that overlays it. Identity fields need no equivalent — they are
	// overlaid as one IdentityEmpty()-gated block, never field by field, so clearing one
	// while another stays set was never representable to lose in the first place.
	HeadlineSet       bool `json:"headline_set,omitempty"`
	SummarySet        bool `json:"summary_set,omitempty"`
	LanguagesSet      bool `json:"languages_set,omitempty"`
	CertificationsSet bool `json:"certifications_set,omitempty"`
	EducationSet      bool `json:"education_set,omitempty"`
}

const (
	maxContactName      = 200
	maxContactEmail     = 320
	maxContactPhone     = 64
	maxContactLocation  = 200
	maxContactLink      = 500
	maxContactLinks     = 20
	maxOwnedHeadline    = 200
	maxOwnedSummary     = 1200
	maxOwnedShort       = 200 // a language name, a certification title, or an education field
	maxOwnedLanguages   = 20
	maxOwnedCertifCount = 40
	maxOwnedEducation   = 20
)

// Sanitize bounds every field the same way CV headers and the structured extract are
// bounded (mirrors resumeextract.Structured's own caps for the fields they share).
func (o *Owned) Sanitize() {
	if o == nil {
		return
	}
	o.FullName = clipRunes(strings.TrimSpace(o.FullName), maxContactName)
	o.Email = clipRunes(strings.TrimSpace(o.Email), maxContactEmail)
	o.Phone = clipRunes(strings.TrimSpace(o.Phone), maxContactPhone)
	o.Location = clipRunes(strings.TrimSpace(o.Location), maxContactLocation)
	o.Links = clipList(o.Links, maxContactLink, maxContactLinks, false)
	o.Headline = clipRunes(strings.TrimSpace(o.Headline), maxOwnedHeadline)
	o.Summary = clipRunes(strings.TrimSpace(o.Summary), maxOwnedSummary)
	o.Languages = clipList(o.Languages, maxOwnedShort, maxOwnedLanguages, true)
	o.Certifications = clipList(o.Certifications, maxOwnedShort, maxOwnedCertifCount, true)
	o.Education = clipEducation(o.Education, maxOwnedEducation)

	// Additive only: a non-empty value has always meant "owned" (every caller before the
	// *Set fields existed relied on exactly that), so a value surviving Sanitize
	// non-empty still forces its flag on. Never turned off here — that would erase an
	// explicit clear (value "" with Set already true) the moment it round-trips through
	// Sanitize, which every read and write path does.
	if o.Headline != "" {
		o.HeadlineSet = true
	}
	if o.Summary != "" {
		o.SummarySet = true
	}
	if len(o.Languages) > 0 {
		o.LanguagesSet = true
	}
	if len(o.Certifications) > 0 {
		o.CertificationsSet = true
	}
	if len(o.Education) > 0 {
		o.EducationSet = true
	}
}

// clipEducation trims each entry's fields, drops entries left with no content, and caps
// the list — the same shape of cleanup resumeextract.Structured.Sanitize applies to its
// own Education, kept independent since Owned has no access to that unexported helper.
func clipEducation(items []resumeextract.Education, maxCount int) []resumeextract.Education {
	out := make([]resumeextract.Education, 0, len(items))
	for _, e := range items {
		e.Degree = clipRunes(strings.TrimSpace(e.Degree), maxOwnedShort)
		e.Institution = clipRunes(strings.TrimSpace(e.Institution), maxOwnedShort)
		e.Year = clipRunes(strings.TrimSpace(e.Year), maxOwnedShort)
		if e.Degree == "" && e.Institution == "" && e.Year == "" {
			continue
		}
		out = append(out, e)
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

// clipList trims, bounds, and — when dedupe is set — case-insensitively dedupes a list of
// short strings. Shared by Links, Languages, and Certifications: same shape of field,
// same cleanup.
func clipList(items []string, maxRunes, maxCount int, dedupe bool) []string {
	var seen map[string]bool
	if dedupe {
		seen = make(map[string]bool, len(items))
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = clipRunes(strings.TrimSpace(it), maxRunes)
		if it == "" {
			continue
		}
		if dedupe {
			key := strings.ToLower(it)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, it)
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

// Empty reports whether every owned field is blank — "nothing to override yet". A body
// field explicitly cleared to "" still counts as owned here (its *Set flag is true), same
// as the case ApplyBody overlays: an owned empty summary is not "nothing to override," it
// is an override to nothing.
func (o Owned) Empty() bool {
	return o.IdentityEmpty() && !o.headlineOwned() && !o.summaryOwned() &&
		!o.languagesOwned() && !o.certificationsOwned() && !o.educationOwned()
}

// headlineOwned and its four siblings report whether a body field is this candidate's own
// — either explicitly, via its *Set flag, or implicitly, because it carries a non-empty
// value (every caller before the *Set flags existed relied on exactly that, and a value
// built as a plain struct literal rather than through Sanitize still needs to read as
// owned). Shared by Empty and ApplyBody so the two can never disagree about what counts.
func (o Owned) headlineOwned() bool       { return o.HeadlineSet || o.Headline != "" }
func (o Owned) summaryOwned() bool        { return o.SummarySet || o.Summary != "" }
func (o Owned) languagesOwned() bool      { return o.LanguagesSet || len(o.Languages) > 0 }
func (o Owned) certificationsOwned() bool { return o.CertificationsSet || len(o.Certifications) > 0 }
func (o Owned) educationOwned() bool      { return o.EducationSet || len(o.Education) > 0 }

// IdentityEmpty reports whether every identity field (name/email/phone/location/links)
// is blank — distinct from Empty(), which also considers the body fields. A caller that
// is about to overwrite a CV header/contact block with Owned's identity subset must gate
// on THIS, not Empty(): a candidate who has only ever edited their summary has an Owned
// that is non-Empty (Summary is set) but whose identity fields are still blank, and
// gating the identity copy on Empty() would blank out a real name/email pulled from the
// current extract instead of leaving it alone.
func (o Owned) IdentityEmpty() bool {
	return o.FullName == "" && o.Email == "" && o.Phone == "" && o.Location == "" && len(o.Links) == 0
}

// ApplyBody overlays the non-identity flat fields — headline, summary, languages,
// certifications, education — onto st, field by field, wherever Owned actually has a
// value. This is the one precedence rule GetResume, structuredCV, and StructureForSeed
// each compose with; identity fields are each caller's own concern (StructureForSeed
// sets them unconditionally once Owned is non-empty, structuredCV's contact-free view
// never touches them at all).
func (o Owned) ApplyBody(st *resumeextract.Structured) {
	if o.headlineOwned() {
		st.Headline = o.Headline
	}
	if o.summaryOwned() {
		st.Summary = o.Summary
	}
	if o.languagesOwned() {
		st.Languages = append([]string(nil), o.Languages...)
	}
	if o.certificationsOwned() {
		st.Certifications = append([]string(nil), o.Certifications...)
	}
	if o.educationOwned() {
		st.Education = append([]resumeextract.Education(nil), o.Education...)
	}
}

// AsStructured projects the identity subset onto the structured contact fields for
// seed/heal — CV HEADER fields only (name/email/phone/location/links). Headline, summary,
// languages, and certifications are body content, not header identity, so callers that
// need those read the full StructureForSeed composition instead.
func (o Owned) AsStructured() resumeextract.Structured {
	return resumeextract.Structured{
		FullName: o.FullName,
		Email:    o.Email,
		Phone:    o.Phone,
		Location: o.Location,
		Links:    append([]string(nil), o.Links...),
	}
}

// OwnedFromStructured copies every field Owned tracks out of a structured résumé.
func OwnedFromStructured(st resumeextract.Structured) Owned {
	o := Owned{
		FullName:       st.FullName,
		Email:          st.Email,
		Phone:          st.Phone,
		Location:       st.Location,
		Links:          append([]string(nil), st.Links...),
		Headline:       st.Headline,
		Summary:        st.Summary,
		Languages:      append([]string(nil), st.Languages...),
		Certifications: append([]string(nil), st.Certifications...),
		Education:      append([]resumeextract.Education(nil), st.Education...),
	}
	o.Sanitize()
	return o
}

// FillEmpty copies non-empty fields from src into dst only where dst is empty — a
// candidate's own edit, whatever field it touched, is never overwritten by a later
// upload's extraction.
func FillEmpty(dst *Owned, src Owned) {
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
	// Gated on ownership, not emptiness: a candidate who explicitly cleared a body field
	// (its *Set flag true, value "") has made a choice a fresh extract must not overrule,
	// the same protection an explicitly non-empty field already had.
	if !dst.headlineOwned() {
		dst.Headline = src.Headline
	}
	if !dst.summaryOwned() {
		dst.Summary = src.Summary
	}
	if !dst.languagesOwned() && len(src.Languages) > 0 {
		dst.Languages = append([]string(nil), src.Languages...)
	}
	if !dst.certificationsOwned() && len(src.Certifications) > 0 {
		dst.Certifications = append([]string(nil), src.Certifications...)
	}
	if !dst.educationOwned() && len(src.Education) > 0 {
		dst.Education = append([]resumeextract.Education(nil), src.Education...)
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

func decodeOwned(blob []byte) Owned {
	if len(blob) == 0 {
		return Owned{}
	}
	var o Owned
	if err := json.Unmarshal(blob, &o); err != nil {
		return Owned{}
	}
	o.Sanitize()
	return o
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

// CandidateOwned returns the candidate's owned overrides, or empty when unset.
func (s *Store) CandidateOwned(ctx context.Context, userID int64) (Owned, error) {
	blob, err := s.repo.GetCandidateContacts(ctx, userID)
	if err != nil {
		return Owned{}, err
	}
	return decodeOwned(blob), nil
}

// SetCandidateOwned replaces the candidate's owned overrides after Sanitize.
func (s *Store) SetCandidateOwned(ctx context.Context, userID int64, o Owned) (Owned, error) {
	o.Sanitize()
	blob, err := json.Marshal(o)
	if err != nil {
		return Owned{}, fmt.Errorf("resume: marshal owned: %w", err)
	}
	if err := s.repo.SetCandidateContacts(ctx, userID, blob); err != nil {
		return Owned{}, err
	}
	return o, nil
}

// FillEmptyOwnedFromStructured merges newly-extracted fields into the owned overrides
// without overwriting anything the candidate already edited, then persists.
func (s *Store) FillEmptyOwnedFromStructured(ctx context.Context, userID int64, st resumeextract.Structured) error {
	owned, err := s.CandidateOwned(ctx, userID)
	if err != nil {
		return err
	}
	before := owned
	FillEmpty(&owned, OwnedFromStructured(st))
	if ownedEqual(owned, before) {
		return nil
	}
	_, err = s.SetCandidateOwned(ctx, userID, owned)
	return err
}

func ownedEqual(a, b Owned) bool {
	if a.FullName != b.FullName || a.Email != b.Email || a.Phone != b.Phone || a.Location != b.Location ||
		a.Headline != b.Headline || a.Summary != b.Summary {
		return false
	}
	if a.HeadlineSet != b.HeadlineSet || a.SummarySet != b.SummarySet || a.LanguagesSet != b.LanguagesSet ||
		a.CertificationsSet != b.CertificationsSet || a.EducationSet != b.EducationSet {
		return false
	}
	return stringsEqual(a.Links, b.Links) && stringsEqual(a.Languages, b.Languages) &&
		stringsEqual(a.Certifications, b.Certifications) && educationEqual(a.Education, b.Education)
}

func educationEqual(a, b []resumeextract.Education) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ReplaceOwnedFromStructured overwrites the owned overrides from a current structure —
// the "replace from CV" action.
func (s *Store) ReplaceOwnedFromStructured(ctx context.Context, userID int64, st resumeextract.Structured) (Owned, error) {
	return s.SetCandidateOwned(ctx, userID, OwnedFromStructured(st))
}

// MarkExtractFailed records a failed extract for the given upload stamp.
func (s *Store) MarkExtractFailed(ctx context.Context, userID int64, detail string, uploadedAt time.Time) error {
	detail = clipRunes(strings.TrimSpace(detail), 200)
	return s.repo.SetExtractFailed(ctx, userID, detail, uploadedAt)
}

// StructuredRow returns the raw GetUserResumeStructured row for status composition.
func (s *Store) StructuredRow(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return s.repo.GetStructured(ctx, userID)
}

// StructureForSeed returns a structured résumé suitable for CV seeding: body fields
// come from the current extract when stamped. A superseded blob contributes contacts
// only — semantic sections wait for a matching stamp (same rule as ProvisionalContacts).
// Owned overrides win as a block, field by field, over both — a candidate's own edit to
// their headline or summary belongs in a freshly seeded CV exactly as their own edited
// email does. ok is true when there is any identity or body worth seeding (caller still
// applies seedable). See AGENTS.md for the identity table.
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

	owned, err := s.CandidateOwned(ctx, userID)
	if err != nil {
		return resumeextract.Structured{}, false, err
	}
	if !owned.IdentityEmpty() {
		st.FullName = owned.FullName
		st.Email = owned.Email
		st.Phone = owned.Phone
		st.Location = owned.Location
		st.Links = append([]string(nil), owned.Links...)
	}
	owned.ApplyBody(&st)

	ok := !owned.Empty() || hasBlob
	return st, ok, nil
}
