package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// Parse status values persisted on users.resume_extract_status.
const (
	ExtractStatusPending = "pending"
	ExtractStatusOK      = "ok"
	ExtractStatusFailed  = "failed"
)

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
	// maxOwnedTotalYears is longer than any career and shorter than any typo. A larger figure is
	// clamped, not rejected, so a stray digit does not lose the rest of the write.
	maxOwnedTotalYears = 70
)

// Sanitize bounds every field like CV headers and the structured extract (mirrors
// resumeextract.Structured's caps for shared fields).
func (o *Owned) Sanitize() {
	if o == nil {
		return
	}
	o.FullName = llm.TruncateRunes(strings.TrimSpace(o.FullName), maxContactName)
	o.Email = llm.TruncateRunes(strings.TrimSpace(o.Email), maxContactEmail)
	o.Phone = llm.TruncateRunes(strings.TrimSpace(o.Phone), maxContactPhone)
	o.Location = llm.TruncateRunes(strings.TrimSpace(o.Location), maxContactLocation)
	o.Links = clipList(o.Links, maxContactLink, maxContactLinks, false)
	o.Headline = llm.TruncateRunes(strings.TrimSpace(o.Headline), maxOwnedHeadline)
	o.Summary = llm.TruncateRunes(strings.TrimSpace(o.Summary), maxOwnedSummary)
	o.Languages = clipList(o.Languages, maxOwnedShort, maxOwnedLanguages, true)
	o.Certifications = clipList(o.Certifications, maxOwnedShort, maxOwnedCertifCount, true)
	o.Education = clipEducation(o.Education, maxOwnedEducation)
	o.TotalYears = min(max(o.TotalYears, 0), maxOwnedTotalYears)

	// Additive only: a non-empty value forces its flag on. Never turned off here, which would
	// erase an explicit clear (value "" with Set true) on every Sanitize round-trip.
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
	if o.TotalYears > 0 {
		o.TotalYearsSet = true
	}
}

// clipEducation trims each entry's fields, drops entries left with no content, and caps the
// list, like resumeextract.Structured.Sanitize (whose helper is unexported).
func clipEducation(items []resumeextract.Education, maxCount int) []resumeextract.Education {
	out := make([]resumeextract.Education, 0, len(items))
	for _, e := range items {
		e.Degree = llm.TruncateRunes(strings.TrimSpace(e.Degree), maxOwnedShort)
		e.Institution = llm.TruncateRunes(strings.TrimSpace(e.Institution), maxOwnedShort)
		e.Year = perioddate.Sanitize(e.Year)
		if e.Degree == "" && e.Institution == "" && e.Year == nil {
			continue
		}
		out = append(out, e)
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

// clipList trims, bounds, and (when dedupe is set) case-insensitively dedupes a list of short
// strings. Shared by Links, Languages, and Certifications.
func clipList(items []string, maxRunes, maxCount int, dedupe bool) []string {
	var seen map[string]bool
	if dedupe {
		seen = make(map[string]bool, len(items))
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = llm.TruncateRunes(strings.TrimSpace(it), maxRunes)
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

// Empty reports whether every owned field is blank: "nothing to override yet". A body field
// explicitly cleared to "" still counts as owned (its *Set flag is true), as in ApplyBody.
func (o Owned) Empty() bool {
	return o.IdentityEmpty() && !o.headlineOwned() && !o.summaryOwned() &&
		!o.languagesOwned() && !o.certificationsOwned() && !o.educationOwned() &&
		!o.totalYearsOwned()
}

// headlineOwned and its four siblings report whether a body field is the candidate's own:
// explicitly via its *Set flag, or implicitly via a non-empty value (a struct literal that
// skipped Sanitize still reads as owned). Shared by Empty and ApplyBody so they agree.
func (o Owned) headlineOwned() bool       { return o.HeadlineSet || o.Headline != "" }
func (o Owned) summaryOwned() bool        { return o.SummarySet || o.Summary != "" }
func (o Owned) languagesOwned() bool      { return o.LanguagesSet || len(o.Languages) > 0 }
func (o Owned) certificationsOwned() bool { return o.CertificationsSet || len(o.Certifications) > 0 }
func (o Owned) educationOwned() bool      { return o.EducationSet || len(o.Education) > 0 }
func (o Owned) totalYearsOwned() bool     { return o.TotalYearsSet || o.TotalYears > 0 }

// IdentityEmpty reports whether every identity field (name/email/phone/location/links) is
// blank, unlike Empty(), which also considers body fields. Gate an identity overwrite on THIS,
// not Empty(): a candidate who only edited their summary is non-Empty with blank identity, and
// gating on Empty() would blank a real name/email from the current extract.
func (o Owned) IdentityEmpty() bool {
	return o.FullName == "" && o.Email == "" && o.Phone == "" && o.Location == "" && len(o.Links) == 0
}

// ApplyBody overlays the non-identity flat fields — headline, summary, languages,
// certifications, education — onto st, field by field, wherever Owned has a value. The one
// precedence rule GetResume, structuredCV, and StructureForSeed compose with; identity fields
// are each caller's own concern.
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
	o.ApplyTotalYears(st, time.Now())
}

// ApplyTotalYears settles the years of experience: the candidate's own figure when they
// gave one — an explicit choice, never second-guessed, even when lower than their dates
// add up to — and otherwise the extract's figure caught up with its dated roles, since
// the model's number is frozen at parse time and may be wrong.
func (o Owned) ApplyTotalYears(st *resumeextract.Structured, now time.Time) {
	if o.totalYearsOwned() {
		st.TotalYears = o.TotalYears
		return
	}
	st.TotalYears = st.EffectiveYears(now)
}

// AsStructured projects the identity subset onto the structured contact fields for
// seed/heal: CV HEADER fields only (name/email/phone/location/links). Body content comes from
// the full StructureForSeed composition.
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
	// Gated on ownership, not emptiness: an explicitly cleared body field (*Set true, value "")
	// must not be overruled by a fresh extract.
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
	if !dst.totalYearsOwned() {
		dst.TotalYears = src.TotalYears
	}
	dst.Sanitize()
}

// DecodeOwned reads the stored candidate_contacts blob; unreadable or empty is "owns nothing".
func DecodeOwned(blob []byte) Owned {
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
	return DecodeOwned(blob), nil
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
	if a.TotalYears != b.TotalYears || a.TotalYearsSet != b.TotalYearsSet {
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
		if a[i].Degree != b[i].Degree || a[i].Institution != b[i].Institution || !periodDateEqual(a[i].Year, b[i].Year) {
			return false
		}
	}
	return true
}

// periodDateEqual compares by value: Sanitize (see clipEducation) always allocates a
// fresh *PeriodDate, even for an unchanged date, so a plain != on Education structs
// would compare that pointer's address instead of what it points to.
func periodDateEqual(a, b *perioddate.PeriodDate) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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
	detail = llm.TruncateRunes(strings.TrimSpace(detail), 200)
	return s.repo.SetExtractFailed(ctx, userID, detail, uploadedAt)
}

// MarkExtractPending clears a stale status (typically 'failed') for the given upload stamp
// before a retry's background derivation starts, without re-stamping the upload. Otherwise a
// poll of GET /me/resume would mistake the previous attempt's terminal status for the retry's.
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
// Owned overrides win as a block, field by field, over both — a candidate's own edit to
// their headline or summary belongs in a freshly seeded CV, like their edited email. ok is true when there is any identity or body worth seeding (caller still
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
