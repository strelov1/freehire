package experience

// Author is who is asserting a banked claim. It is decided by the ENTRY POINT that received
// the request and is never read from the request body — a caller naming itself is not evidence
// of who it is. Same rule, and the same reason, as cvedit.Actor.
//
// This is the wall the CV evidence gate stands on. Only a claim the CANDIDATE asserted may be
// written into their CV (Provenance.Publishable); a model's own reading is banked, searchable,
// and refused there. Until this type existed the rule lived in three functions in the HTTP
// layer, so any new writer — a cmd worker, the CLI, a fourth assistant tool, a mobile endpoint
// — could hand the store ProvenanceManual on model-authored text and have it come back
// CV-publishable. The store now derives the label and ignores whatever is in the struct.
type Author string

const (
	// AuthorCandidate is the person themselves, editing their own bank from an authenticated
	// session with no chat behind it. The only honest reading of a typed-in achievement is
	// that the owner typed it.
	AuthorCandidate Author = "candidate"

	// AuthorQuoted is the person's own words, verified by the entry point against what they
	// actually said — the assistant checks the claim against the session transcript. This is
	// the ONLY authorship that can raise a claim's standing, and it can do so only because
	// something outside the model confirmed the words.
	AuthorQuoted Author = "quoted"

	// AuthorAgent is a model's own reading, asserted now: a paraphrase, a summary, an
	// inference. Recorded rather than refused, because the agent needs its hypothesis on
	// record in order to ask the candidate about it — and refused by the CV evidence gate.
	AuthorAgent Author = "agent"

	// AuthorRewrite is an edit that changes wording WITHOUT re-asserting it: a keyed caller
	// rewriting an achievement with no transcript to check the new words against. It keeps
	// whatever the claim was already labelled, because rewriting words is not the same act as
	// claiming them, and a keyed caller must not be able to relabel.
	//
	// It is what stops the laundering route: bank an inference as agent_inferred, edit it,
	// read back manual, cite it in a CV. An absent or unreadable label falls to
	// agent_inferred — a missing label is not evidence that anyone said anything.
	AuthorRewrite Author = "rewrite"
)

// ProvenanceFor derives the label a write records from who is asserting it. existing is the
// label already stored, consulted only by AuthorRewrite; pass the zero value when there is
// none (an insert).
//
// Exported so a test double can behave like the real store rather than echoing back whatever
// it was handed — a fake that skipped this rule would let a handler test pass while the live
// path laundered. It is a derivation, not a door: Store still ignores Atom.Provenance on
// write, so knowing the rule buys a caller nothing.
func ProvenanceFor(author Author, existing Provenance) Provenance {
	switch author {
	case AuthorCandidate:
		return ProvenanceManual
	case AuthorQuoted:
		return ProvenanceStatedInChat
	case AuthorRewrite:
		if existing.Valid() {
			return existing
		}
		return ProvenanceAgentInferred
	default:
		// Every unrecognised authorship reads as the model speaking. A new entry point that
		// forgets to name itself must land on the label the evidence gate refuses, never on
		// one it accepts.
		return ProvenanceAgentInferred
	}
}
