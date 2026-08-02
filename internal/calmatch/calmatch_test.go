package calmatch

import "testing"

func candidates() []Candidate {
	return []Candidate{
		{ApplicationID: 1, UIDs: []string{"derq-round-1@ashbyhq.com"}},
		{ApplicationID: 2},
		{ApplicationID: 3},
	}
}

// The one correspondence that needs no inference: the invitation is already tied to an
// application by the mail matcher, and this identifier says the calendar entry is that
// same meeting.
func TestTheInvitationsIdentifierResolvesTheApplication(t *testing.T) {
	got := Resolve(Event{UID: "derq-round-1@ashbyhq.com"}, candidates())

	if got.Tier != TierUID || got.ApplicationID != 1 {
		t.Errorf("Resolve = %+v, want application 1 at TierUID", got)
	}
}

// A method rather than a convention, and walked in full so a weaker tier cannot be added
// without a verdict on whether it may link. One will be proposed again: the first version
// of this package had a title-matching tier, and it linked meetings to the wrong employer.
func TestOnlyTheIdentifierTierMayLink(t *testing.T) {
	links := map[Tier]bool{TierNone: false, TierUID: true}
	if len(links) != len(Tiers) {
		t.Fatalf("the vocabulary has %d tiers but this test pins %d — a new tier needs a verdict here", len(Tiers), len(links))
	}
	for tier, want := range links {
		if got := tier.Links(); got != want {
			t.Errorf("Tier(%d).Links() = %v, want %v", tier, got, want)
		}
	}
}

func TestAnUnrecognisedMeetingResolvesNothing(t *testing.T) {
	for name, ev := range map[string]Event{
		"nothing to go on": {},
		"an unknown UID":   {UID: "someone-elses@google.com"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Resolve(ev, candidates()); got.Tier != TierNone || got.ApplicationID != 0 {
				t.Errorf("Resolve = %+v, want TierNone and no application", got)
			}
		})
	}
}

// An absent identifier must not match an absent one, or every meeting with no UID would
// attach to the first application holding a message that had none either.
func TestAnEmptyIdentifierMatchesNothing(t *testing.T) {
	withEmpty := []Candidate{{ApplicationID: 9, UIDs: []string{""}}}

	if got := Resolve(Event{}, withEmpty); got.Tier == TierUID {
		t.Errorf("Resolve = %+v, an absent identifier matched an absent one", got)
	}
}
