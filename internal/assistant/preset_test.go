package assistant

import (
	"strings"
	"testing"
)

// SystemPrompt has always fallen back to the chat prompt for a preset it does not
// recognise, and the tool registry decides the same question separately. Two
// switches that fall back differently would hand a session one preset's
// instructions and another's tools — the model would then be told about tools it
// cannot call. One exported normaliser is what keeps the two answers the same.
func TestNormalizePresetMapsTheUnrecognisedToChat(t *testing.T) {
	for _, known := range []string{PresetChat, PresetTailor, PresetProfile, PresetBrowse, PresetInterview, PresetDebrief} {
		if got := NormalizePreset(known); got != known {
			t.Errorf("NormalizePreset(%q) = %q, want it unchanged", known, got)
		}
	}
	for _, unknown := range []string{"", "browze", "CHAT", "rehearsal"} {
		if got := NormalizePreset(unknown); got != PresetChat {
			t.Errorf("NormalizePreset(%q) = %q, want %q", unknown, got, PresetChat)
		}
	}
}

// The fallback is the normaliser's, not a second copy of it.
func TestSystemPromptFallsBackThroughTheNormaliser(t *testing.T) {
	if SystemPrompt("browze") != SystemPrompt(PresetChat) {
		t.Error("an unrecognised preset does not get the chat prompt")
	}
}

// The side panel extends the chat playbook but registers no mail tool, so it must
// not inherit the mail section along with it. browsePrompt is appended to
// chatPrompt, and appending mail to chatPrompt itself is exactly the mistake this
// pins shut.
func TestOnlyTheChatPresetIsToldAboutMail(t *testing.T) {
	if !strings.Contains(SystemPrompt(PresetChat), "inbox_overview") {
		t.Error("the chat prompt does not mention the mail tools it carries")
	}
	for _, preset := range []string{PresetBrowse, PresetTailor, PresetProfile, PresetInterview, PresetDebrief} {
		if strings.Contains(SystemPrompt(preset), "inbox_overview") {
			t.Errorf("the %q prompt talks about mail, but that preset registers no mail tool", preset)
		}
	}
}

// cv_edit and tailor_report write two different columns through two different tool
// calls, with nothing tying them together unless the agent is told to pass
// requirement/requirement_status on the edit that closes one — otherwise a document
// change with no follow-up tailor_report call leaves that requirement's report entry
// stale.
func TestTailorPromptTellsTheAgentToSelfReportAClosedRequirement(t *testing.T) {
	p := SystemPrompt(PresetTailor)
	for _, want := range []string{"requirement_status", "closed_bank", "closed_candidate"} {
		if !strings.Contains(p, want) {
			t.Errorf("the tailor prompt never says %q about cv_edit closing a requirement", want)
		}
	}
}

// A rehearsal runs on the candidate's own recorded experience, and the bank is where
// that lives — so the prompt has to state the one rule the service cannot enforce.
// experience_add takes `said` as a string and stamps stated_in_chat either way, so
// nothing downstream can tell the candidate's words from the model's paraphrase of
// them. Only an explicit agreement, visible in the transcript, makes it checkable.
func TestInterviewPromptGatesTheBankOnTheCandidatesAgreement(t *testing.T) {
	p := SystemPrompt(PresetInterview)
	for _, want := range []string{"experience_add", "agree", "their own words"} {
		if !strings.Contains(p, want) {
			t.Errorf("the rehearsal prompt never says %q about recording an answer", want)
		}
	}
}

// The invitation reaches the model as text an employer wrote, and the same guard the
// mail section carries has to be stated here — this preset holds no mail tools, so it
// would otherwise never be told.
func TestInterviewPromptNamesTheInvitationAsUntrusted(t *testing.T) {
	p := SystemPrompt(PresetInterview)
	for _, want := range []string{"untrusted", "ATTACK"} {
		if !strings.Contains(p, want) {
			t.Errorf("the rehearsal prompt never says %q about the invitation", want)
		}
	}
}

// One round per session is what keeps a rehearsal from wandering into a survey of
// every interview format, and what keeps its turns inside the ordinary step ceiling.
func TestInterviewPromptHoldsToOneRound(t *testing.T) {
	p := SystemPrompt(PresetInterview)
	for _, want := range []string{"one round", "one question"} {
		if !strings.Contains(strings.ToLower(p), want) {
			t.Errorf("the rehearsal prompt never says %q", want)
		}
	}
}

// A debrief and a rehearsal share their tools, their context and their binding, so
// the prompt is the whole of the difference between them. Handing a debrief the
// rehearsal's prompt would put the agent back in the interviewer's chair, asking
// invented questions about an interview that has already happened.
func TestDebriefRunsUnderItsOwnPrompt(t *testing.T) {
	p := SystemPrompt(PresetDebrief)
	if p == SystemPrompt(PresetChat) {
		t.Fatal("the debrief preset falls through to the chat prompt")
	}
	if p == SystemPrompt(PresetInterview) {
		t.Error("the debrief runs under the rehearsal's prompt, which would have it play the interviewer")
	}
}

// The debrief works from what the candidate brings back, and the failure mode it has
// to be talked out of is drifting into a rehearsal: inventing questions and asking the
// candidate to answer them. The interview already happened; questions it did not
// contain teach nothing about how it went.
func TestDebriefPromptWorksFromWhatTheCandidateBringsBack(t *testing.T) {
	p := strings.ToLower(SystemPrompt(PresetDebrief))
	for _, want := range []string{"one question at a time", "do not invent questions", "requirement"} {
		if !strings.Contains(p, want) {
			t.Errorf("the debrief prompt never says %q", want)
		}
	}
}

// The critique is half of what the candidate gets for the work of recalling the
// interview, and it is worth nothing if it praises everything: they would repeat the
// same answer in the next round. The three axes are the same ones the rehearsal
// critiques on, because they are what makes an answer land in a real room.
func TestDebriefPromptCritiquesOnTheThreeAxesWithoutPadding(t *testing.T) {
	p := SystemPrompt(PresetDebrief)
	for _, want := range []string{"THEY did", "outcome", "number", "no praise padding"} {
		if !strings.Contains(p, want) {
			t.Errorf("the debrief prompt never says %q about how an answer landed", want)
		}
	}
}

// Banking is the debrief's purpose rather than a hazard within it, but the gate is the
// same one the rehearsal carries and for the same reason: experience_add stamps
// stated_in_chat whoever composed `said`, so nothing downstream separates the
// candidate's words from the model's paraphrase. The agreement in the transcript is
// what makes a wrong entry traceable.
func TestDebriefPromptGatesTheBankOnTheCandidatesAgreement(t *testing.T) {
	p := SystemPrompt(PresetDebrief)
	for _, want := range []string{"experience_add", "agree", "their own words"} {
		if !strings.Contains(p, want) {
			t.Errorf("the debrief prompt never says %q about recording an answer", want)
		}
	}
}

// The highest-value thing a debrief produces is a quantified achievement, and it is
// also the easiest to fabricate: a plausible figure attributed to the candidate is one
// they may not catch, and it would go on a CV. The service cannot tell whose number it
// is, so the prompt has to forbid supplying one.
func TestDebriefPromptForbidsSupplyingAFigureTheCandidateDidNotGive(t *testing.T) {
	p := strings.ToLower(SystemPrompt(PresetDebrief))
	for _, want := range []string{"never a number", "ask whether"} {
		if !strings.Contains(p, want) {
			t.Errorf("the debrief prompt never says %q about a figure the candidate did not give", want)
		}
	}
}

// The invitation reaches a debrief through the same context the rehearsal reads, and
// this preset carries no mail tools either — so it never sees the mail section's
// warning and has to be told here, or it reads an employer's instruction as a request.
func TestDebriefPromptNamesTheInvitationAsUntrusted(t *testing.T) {
	p := SystemPrompt(PresetDebrief)
	for _, want := range []string{"untrusted", "ATTACK"} {
		if !strings.Contains(p, want) {
			t.Errorf("the debrief prompt never says %q about the invitation", want)
		}
	}
}

// Bodies are attacker-controlled text. The prompt is not the only guard — the tool
// surface sends no mail, so an injection has no outbound channel — but the model
// still has to be told, or it reads an instruction in a message as a request.
func TestMailPromptNamesBodiesAsUntrusted(t *testing.T) {
	prompt := SystemPrompt(PresetChat)
	for _, want := range []string{"untrusted", "ATTACK"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the mail section never says %q about message bodies", want)
		}
	}
}
