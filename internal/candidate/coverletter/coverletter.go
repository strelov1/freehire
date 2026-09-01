// Package coverletter drafts the cover letter a vacancy's application form asks for, from the
// achievement atoms the candidate has actually asserted.
//
// The draft is produced by a fixed three-stage chain — select evidence, draft, audit — and not by
// an autonomous tool-calling agent, for the reason internal/candidate/matchanalysis gives for its
// own chain: deterministic, typed, cacheable. See AGENTS.md.
package coverletter
