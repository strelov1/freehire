package experience

import "testing"

// TestProvenanceFor is the anti-laundering rule, and it is the reason Author exists.
//
// The CV evidence gate admits only a claim the CANDIDATE asserted. If an edit could raise a
// claim's standing, the route is trivial: an agent banks its own reading as agent_inferred
// (which the gate refuses), edits it, reads back manual, and cites it — a model's invention on
// a real person's CV. So the label follows who is asserting, and only a verified quote can
// raise it.
func TestProvenanceFor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		author   Author
		existing Provenance
		want     Provenance
	}{
		{"the person typed it", AuthorCandidate, "", ProvenanceManual},
		{"the person confirms their own inferred claim", AuthorCandidate, ProvenanceAgentInferred, ProvenanceManual},
		{"the person corrects an imported one", AuthorCandidate, ProvenanceCVImport, ProvenanceManual},

		// The only way standing goes UP, and only because something outside the model — the
		// session transcript — confirmed the words.
		{"words verified against what they said", AuthorQuoted, "", ProvenanceStatedInChat},
		{"a verified quote overrides a weaker label", AuthorQuoted, ProvenanceAgentInferred, ProvenanceStatedInChat},

		{"a model's own reading", AuthorAgent, "", ProvenanceAgentInferred},
		// An agent asserting afresh cannot keep a stronger label it did not earn: editing a
		// confirmed claim into a paraphrase makes it the model's again.
		{"a model re-asserting a confirmed claim downgrades it", AuthorAgent, ProvenanceStatedInChat, ProvenanceAgentInferred},

		{"a keyed rewrite may not promote a model's reading", AuthorRewrite, ProvenanceAgentInferred, ProvenanceAgentInferred},
		{"a keyed rewrite keeps what the candidate said", AuthorRewrite, ProvenanceStatedInChat, ProvenanceStatedInChat},
		{"a keyed rewrite keeps a manual claim manual", AuthorRewrite, ProvenanceManual, ProvenanceManual},
		// A row whose label is somehow unset must not become publishable by being edited.
		// Falling back to the strongest claim would be the same laundering.
		{"a keyed rewrite of an unlabelled row", AuthorRewrite, "", ProvenanceAgentInferred},
		{"a keyed rewrite of a corrupt label", AuthorRewrite, Provenance("vibes"), ProvenanceAgentInferred},

		// A new entry point that forgets to name itself must land where the gate refuses.
		{"an unnamed authorship", Author("cli"), ProvenanceManual, ProvenanceAgentInferred},
		{"the zero authorship", "", ProvenanceManual, ProvenanceAgentInferred},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProvenanceFor(tc.author, tc.existing); got != tc.want {
				t.Errorf("ProvenanceFor(%q, %q) = %q, want %q", tc.author, tc.existing, got, tc.want)
			}
		})
	}
}

// TestOnlyQuotedIsPublishableFromAnAgentPath states the invariant the table above serves, so a
// future value added to Author cannot quietly become CV-publishable: of everything an AGENT
// can produce, nothing the model authored itself may reach a CV.
func TestOnlyVerifiedAgentWritesArePublishable(t *testing.T) {
	if ProvenanceFor(AuthorAgent, "").Publishable() {
		t.Error("a model's own reading became CV-publishable — this is the wall")
	}
	if ProvenanceFor(AuthorRewrite, "").Publishable() {
		t.Error("an unlabelled row became publishable by being rewritten")
	}
	if !ProvenanceFor(AuthorQuoted, "").Publishable() {
		t.Error("a verified quote must be publishable, or the candidate cannot use their own words")
	}
	if !ProvenanceFor(AuthorCandidate, "").Publishable() {
		t.Error("what the person typed themselves must be publishable")
	}
}
