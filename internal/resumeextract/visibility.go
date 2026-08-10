package resumeextract

// This file holds the Talent Network public-profile projections (see
// openspec/changes/talent-network-profile-visibility). It is deliberately NOT part of
// structured.go: cmd/gen-contracts generates TypeScript from structured.go alone, and
// these projections are pure Go-side view logic with no wire contract of their own yet
// — the handler that serves them (a later task) decides what crosses the wire and gets
// its own generated type then, rather than this file leaking one prematurely.

// currentEmployerLabel is the generic label anonymous mode substitutes for the newest
// experience entry's employer name. This is the literal wording from design.md's
// "Anonymous-mode masking is Professional() plus one extra step" decision — not a
// placeholder to be reworded later.
const currentEmployerLabel = "Current employer"

// Anonymous is Structured's anonymous-mode projection for the Talent Network public
// page: Professional() (already contact-free — no name/email/phone/links) with the
// newest experience entry's employer replaced by the generic label above. Older entries
// pass through unmodified: the design deliberately scopes the mask to the current role,
// not the whole work history, since the rest of the history is useful signal for anyone
// else the candidate shares the link with (see design.md, "Risks / Trade-offs").
//
// Experience is ordered newest-first — the same reverse-chronological convention
// internal/experience.Store.ListEmployments documents and enforces for the bank, and the
// order a CV is conventionally written in, which the extraction prompt does not disturb
// — so the newest entry is index 0.
func (s Structured) Anonymous() Professional {
	p := s.Professional()
	if len(p.Experience) == 0 {
		return p
	}

	// Copy before mutating: Professional's Experience slice shares its backing array
	// with s.Experience, and masking must not leak back into the caller's Structured.
	masked := make([]Experience, len(p.Experience))
	copy(masked, p.Experience)
	masked[0].Company = currentEmployerLabel
	p.Experience = masked

	return p
}

// Public is Structured's public-mode projection: the contact-free Professional fields
// plus the candidate's name. Structured carries no photo field today, so a public-mode
// photo (e.g. users.photo_object_key) is a separate seam a later task composes in from
// elsewhere — nothing here invents one.
type Public struct {
	FullName string `json:"full_name,omitempty"`
	Professional
}

// Public projects Structured onto the public-mode view: name shown, work history and
// skills shown unmodified (including the newest employer — unlike Anonymous), contact
// fields still withheld. Public mode reuses the same contact-stripped base as anonymous
// mode because the page is unauthenticated and publicly reachable (see design.md,
// "Public mode still strips contact info").
func (s Structured) Public() Public {
	return Public{
		FullName:     s.FullName,
		Professional: s.Professional(),
	}
}
