package calmatch

import "testing"

func candidates() []Candidate {
	return []Candidate{
		{ApplicationID: 1, Company: "Derq", UIDs: []string{"derq-round-1@ashbyhq.com"}},
		{ApplicationID: 2, Company: "Vercel"},
		{ApplicationID: 3, Company: "Linear"},
	}
}

// The one correspondence that needs no inference: the invitation is already tied to an
// application by the mail matcher, and this identifier says the calendar entry is that
// same meeting.
func TestTheInvitationsIdentifierResolvesTheApplication(t *testing.T) {
	got := Resolve(Event{UID: "derq-round-1@ashbyhq.com", Title: "Chat"}, candidates())

	if got.Tier != TierUID || got.ApplicationID != 1 {
		t.Errorf("Resolve = %+v, want application 1 at TierUID", got)
	}
}

// The load-bearing asymmetry, and the reason this is a method rather than a convention:
// a caller that treated a name match as a link would attach a meeting on the strength of
// a word in a title, and the candidate would prepare for the wrong employer.
func TestOnlyTheIdentifierTierMayLink(t *testing.T) {
	links := map[Tier]bool{
		TierNone:      false,
		TierUID:       true,
		TierName:      false,
		TierAmbiguous: false,
	}
	if len(links) != len(Tiers) {
		t.Fatalf("the vocabulary has %d tiers but this test pins %d — a new tier needs a verdict here", len(Tiers), len(links))
	}
	for tier, want := range links {
		if got := tier.Links(); got != want {
			t.Errorf("Tier(%d).Links() = %v, want %v", tier, got, want)
		}
	}
}

// A name in the title is worth offering and not worth acting on.
func TestAnEmployersNameInTheTitleOnlySuggests(t *testing.T) {
	got := Resolve(Event{Title: "Vercel <> Ivan — Platform Engineer"}, candidates())

	if got.Tier != TierName || got.ApplicationID != 2 {
		t.Errorf("Resolve = %+v, want application 2 at TierName", got)
	}
	if got.Tier.Links() {
		t.Error("a name match reported itself as a link")
	}
}

// Two applications to one employer are two different rounds, and nothing in the title
// says which. Offering the wrong one is worse than offering none.
func TestANameMatchingTwoApplicationsResolvesNeither(t *testing.T) {
	two := append(candidates(), Candidate{ApplicationID: 4, Company: "Derq"})
	got := Resolve(Event{Title: "Derq interview"}, two)

	if got.Tier != TierAmbiguous || got.ApplicationID != 0 {
		t.Errorf("Resolve = %+v, want TierAmbiguous and no application", got)
	}
}

// The identifier wins over the title. A recruiter who types another employer's name into
// a calendar entry has made a typo, not a second meeting.
func TestTheIdentifierOutranksTheTitle(t *testing.T) {
	got := Resolve(Event{UID: "derq-round-1@ashbyhq.com", Title: "Vercel interview"}, candidates())

	if got.Tier != TierUID || got.ApplicationID != 1 {
		t.Errorf("Resolve = %+v, want the identifier's application 1", got)
	}
}

// An organiser's address is evidence about who sent the invitation, not about who is
// interviewing: mailmatch bans domain matching because ATS relays send the mail, and an
// ATS schedules meetings from its own domain just as readily.
func TestAnOrganisersDomainResolvesNothingOnItsOwn(t *testing.T) {
	got := Resolve(Event{Title: "Interview", Organizer: "recruiter@derq.com"}, candidates())

	if got.Tier != TierNone || got.ApplicationID != 0 {
		t.Errorf("Resolve = %+v, want TierNone — a domain is not evidence about the employer", got)
	}
}

func TestAnUnrecognisedMeetingResolvesNothing(t *testing.T) {
	for name, ev := range map[string]Event{
		"nothing to go on":     {},
		"a personal errand":    {Title: "Dentist"},
		"an unknown UID":       {UID: "someone-elses@google.com"},
		"an untracked company": {Title: "Interview with Supabase"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Resolve(ev, candidates()); got.Tier != TierNone || got.ApplicationID != 0 {
				t.Errorf("Resolve = %+v, want TierNone and no application", got)
			}
		})
	}
}

// A UID belongs to whoever's mailbox carried it. Candidates are always one caller's, so
// this is enforced by the caller — but an empty UID must never match an empty one.
func TestAnEmptyIdentifierMatchesNothing(t *testing.T) {
	withEmpty := []Candidate{{ApplicationID: 9, Company: "Derq", UIDs: []string{""}}}

	if got := Resolve(Event{Title: "Something"}, withEmpty); got.Tier == TierUID {
		t.Errorf("Resolve = %+v, an absent identifier matched an absent one", got)
	}
}
