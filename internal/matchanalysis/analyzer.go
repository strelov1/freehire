package matchanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// Input bounds for untrusted, user/ingest-supplied text sent to the model. Kept modest
// on purpose: the fit model reasons over every input token, so a large CV/description
// balloons its thinking time (tens of seconds per stage). These caps keep each stage
// responsive while still covering the substance of a CV and a posting.
const (
	maxDescriptionRunes = 6000
	maxCompanyRunes     = 2000
)

// Analyzer runs the fixed three-stage fit prompt-chain over an llm.Client. A nil
// client (LLM unconfigured) makes Analyze a no-op so the endpoint degrades to no
// analysis, mirroring atscheck.Analyzer. The chain scores the fit from the de-identified
// structured résumé (see Input.StructuredResume) — it never sends the raw CV, so it carries
// no direct identifier to the provider and needs no PII detector of its own.
type Analyzer struct {
	client *llm.Client
}

// NewAnalyzer wraps an llm.Client; client may be nil (LLM unconfigured), in which case the
// analysis degrades to a no-op.
// As returns an analyzer that runs on a different client, so one analysis can be spent
// under the caller's own gateway credential. The analyzer is one field, so cloning it per
// request costs nothing beside the model calls it is about to make. Nil-safe both ways.
func (a *Analyzer) As(client *llm.Client) *Analyzer {
	if a == nil || client == nil {
		return a
	}
	clone := *a
	clone.client = client

	return &clone
}

func NewAnalyzer(client *llm.Client) *Analyzer {
	return &Analyzer{client: client}
}

// ModelID returns the underlying model id (empty when unconfigured), so a caller can
// record which model produced a cached analysis. Nil-safe, like AnalyzeStream.
func (a *Analyzer) ModelID() string {
	if a == nil {
		return ""
	}
	return a.client.ModelID()
}

// Input is everything the chain needs, gathered by the handler before the first call:
// the job text, the raw company_info JSON, the candidate's structured résumé, the
// deterministic skills match used as the grounding anchor, and the job geography + the
// candidate's location preferences (raw JSON) used to score location & work-mode fit.
type Input struct {
	JobTitle       string
	JobDescription string
	CompanyInfo    string
	// StructuredResume is the caller's contact-free résumé projection — the sole candidate
	// context sent to the model. De-identification is this field's TYPE, not a step the chain
	// performs: resumeextract.Professional names the fields a model may see, so a field added
	// to the structured résumé is withheld until somebody adds it there too. A zero value (or
	// one with no experience) means the caller has nothing to reason over and no analysis runs.
	StructuredResume resumeextract.Professional
	Match            jobmatch.JobMatch

	// Job geography for the location dimension.
	JobWorkMode  string
	JobRemote    bool
	JobLocation  string
	JobRegions   []string
	JobCountries []string
	// LocationPreferences is the candidate's raw profile location_preferences JSON
	// (accepted work modes, remote reach, base, relocation); empty when unset.
	LocationPreferences string

	// Language is the caller's account language (accounts.User.Language, e.g. "en"/"ru"),
	// naming the language every free-text field in the analysis — dimension comments,
	// strengths, gaps, the recommendation, hidden-signal insights — is written in. It is
	// the candidate's own reading of their fit, not part of the CV, so it follows their
	// profile language rather than the vacancy's (freehire#1837; contrast the tailoring
	// flow, whose CV bullets follow the vacancy's language instead). Empty falls back to
	// English, same as assistant.LanguageName.
	Language string

	// Blockers are the deterministic hard-constraint results (hardconstraint.Evaluate).
	// The unmet ones are fed into the prompt as established constraints so the model
	// respects rather than re-derives them; the same list caps the served overall_score.
	Blockers []hardconstraint.Blocker
}

// stage1Out is the Extract & Match stage's raw output.
type stage1Out struct {
	Requirements  []Requirement `json:"requirements"`
	HiddenSignals []Signal      `json:"hidden_signals"`
}

// EventKind tags a streaming Event (see AnalyzeStream).
type EventKind string

const (
	EventStageStart   EventKind = "stage_start"  // a stage began (Stage set)
	EventStageDone    EventKind = "stage_done"   // a stage finished (Stage set)
	EventThinking     EventKind = "thinking"     // a reasoning-token delta (Stage + Thinking)
	EventRequirements EventKind = "requirements" // Stage-1 result (Requirements + HiddenSignals)
	EventDimensions   EventKind = "dimensions"   // interim post-Stage-2 analysis (Analysis)
	EventFinal        EventKind = "final"        // the audited final analysis (Analysis)
)

// Event is one step of a streaming analysis. Only the fields relevant to Kind are set.
type Event struct {
	Kind          EventKind     `json:"kind"`
	Stage         int           `json:"stage,omitempty"`
	Label         string        `json:"label,omitempty"`
	Thinking      string        `json:"thinking,omitempty"`
	Requirements  []Requirement `json:"requirements,omitempty"`
	HiddenSignals []Signal      `json:"hidden_signals,omitempty"`
	Analysis      *Analysis     `json:"analysis,omitempty"`
}

var stageLabels = map[int]string{1: "Extract & Match", 2: "Recruiter verdict", 3: "Adversarial audit"}

// Analyze runs the three-stage chain and returns the final analysis, discarding the
// stream. Returns (nil, nil) when the LLM is unconfigured. It is a thin collector over
// AnalyzeStream — one chain implementation, no duplication.
func (a *Analyzer) Analyze(ctx context.Context, in Input) (*Analysis, error) {
	return a.AnalyzeStream(ctx, in, func(Event) {})
}

// AnalyzeStream runs Stage 1 (Extract & Match) → Stage 2 (Recruiter verdict) → Stage 3
// (Adversarial audit), emitting stage/thinking/section events through emit as it goes,
// and returns the final analysis. Returns (nil, nil) when the LLM is unconfigured (no
// events). A Stage 1/2 failure returns an error (nothing served); a Stage 3 failure
// degrades to the un-audited Stage 2 verdict rather than erroring. emit must not be nil.
func (a *Analyzer) AnalyzeStream(ctx context.Context, in Input, emit func(Event)) (*Analysis, error) {
	if a == nil || a.client == nil {
		return nil, nil
	}

	// The fit is scored from the candidate's banked work history; without it there is nothing
	// to reason over, so degrade to no analysis (the raw CV is never used as a fallback, and
	// neither is the structured résumé's own copy of the experience).
	candidate := candidateContext(in.StructuredResume)
	if candidate == "" {
		return nil, nil
	}

	// Stage 1 — Extract & Match (the ATS lens).
	emit(Event{Kind: EventStageStart, Stage: 1, Label: stageLabels[1]})
	var s1 stage1Out
	if err := a.streamStage(ctx, 1, stage1SystemPrompt(in.Language), stage1UserPrompt(in, candidate), emit, &s1); err != nil {
		return nil, fmt.Errorf("matchanalysis: stage 1: %w", err)
	}
	reqs := sanitizeRequirements(s1.Requirements)
	signals := sanitizeSignals(s1.HiddenSignals)
	emit(Event{Kind: EventRequirements, Requirements: reqs, HiddenSignals: signals})
	emit(Event{Kind: EventStageDone, Stage: 1, Label: stageLabels[1]})

	// Stage 2 — Recruiter verdict (the human lens).
	emit(Event{Kind: EventStageStart, Stage: 2, Label: stageLabels[2]})
	var verdict recruiterVerdict
	if err := a.streamStage(ctx, 2, stage2SystemPrompt(in.Language), stage2UserPrompt(in, reqs, candidate), emit, &verdict); err != nil {
		return nil, fmt.Errorf("matchanalysis: stage 2: %w", err)
	}
	sanitizeVerdict(&verdict)
	interim := buildAnalysis(reqs, verdict, signals)
	emit(Event{Kind: EventDimensions, Analysis: &interim})
	emit(Event{Kind: EventStageDone, Stage: 2, Label: stageLabels[2]})

	// Stage 3 — Adversarial audit. Seed the audit target with the sanitized Stage 2
	// verdict so json.Unmarshal MERGES: the audit overrides only the fields it returns
	// and omitted dimensions keep their Stage 2 scores. A budget model that echoes just
	// the fields it changed can then only refine the verdict, never hollow it out to
	// zeros. Best-effort: on a parse/transport failure keep the un-audited verdict.
	emit(Event{Kind: EventStageStart, Stage: 3, Label: stageLabels[3]})
	audited := verdict
	if err := a.streamStage(ctx, 3, stage3SystemPrompt(in.Language), stage3UserPrompt(in, reqs, verdict, candidate), emit, &audited); err != nil {
		log.Printf("matchanalysis: stage 3 audit failed, serving un-audited verdict: %v", err)
	} else {
		// An explicit JSON null ("strengths": null) unmarshals to a nil slice, overriding
		// Stage 2's list — but the audit may only refine, never hollow out (an empty
		// array is a deliberate prune and is kept). Restore the Stage 2 value on null.
		if audited.Strengths == nil {
			audited.Strengths = verdict.Strengths
		}
		if audited.Gaps == nil {
			audited.Gaps = verdict.Gaps
		}
		sanitizeVerdict(&audited)
		verdict = audited
	}
	emit(Event{Kind: EventStageDone, Stage: 3, Label: stageLabels[3]})

	analysis := buildAnalysis(reqs, verdict, signals)
	emit(Event{Kind: EventFinal, Analysis: &analysis})
	return &analysis, nil
}

// stageAttempts is how many times a stage's LLM call is tried. Two failures earn a retry:
// a PARSE failure (the gateway occasionally returns a transient HTML error page that fails
// JSON parsing — a single re-try recovers it, mirroring the enrichment worker), and a
// TIMEOUT (a stage that burns its whole budget is hung, not slow; prod showed the retry
// answering in seconds). A connection error, or a caller who has gone away, is returned
// immediately. Two attempts of matchAnalysisLLMTimeout bound the worst case per stage.
const stageAttempts = 2

// streamStage runs one streaming JSON call, forwarding reasoning deltas as thinking
// events for the given stage, and unmarshals the accumulated JSON into out. A parse
// failure and a timed-out stage are each retried once; anything else returns at once.
func (a *Analyzer) streamStage(ctx context.Context, stage int, system, user string, emit func(Event), out any) error {
	// A snapshot of out exactly as the caller handed it in — the zero value for stages 1
	// and 2, or Stage 2's verdict for Stage 3's seed-and-merge audit (see Analyze: Stage 3
	// pre-populates out so a field the audit's JSON omits keeps its Stage 2 value, since
	// json.Unmarshal only overwrites the keys present in the source). Every attempt
	// restores out to this snapshot before decoding into it: encoding/json does not roll
	// back a partial decode, so a failed attempt can leave the fields it reached before
	// erroring still set, and without restoring first a later attempt's valid-but-partial
	// JSON would silently inherit that stale, never-validated value instead of the seed
	// (zero, or Stage 2's) it should have started from.
	dst := reflect.ValueOf(out).Elem()
	seed := reflect.New(dst.Type()).Elem()
	seed.Set(dst)

	var parseErr error
	for attempt := 1; attempt <= stageAttempts; attempt++ {
		raw, err := a.client.GenerateJSONStream(ctx, system, user, func(t string) {
			emit(Event{Kind: EventThinking, Stage: stage, Thinking: t})
		})
		if err != nil {
			// A stage that burned its own per-call deadline is retried like a parse failure.
			// Production showed such a call hung rather than slow — it ate the full budget
			// while the very next attempt answered in seconds — so failing here throws away
			// an analysis that was one retry away. A caller who went away is final: nobody
			// is left to read a second answer, and it would only spend tokens.
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil && attempt < stageAttempts {
				log.Printf("matchanalysis: stage %d timed out, retrying: %v", stage, err)
				continue
			}
			return err // transport error, or a caller who is gone — retrying wouldn't help
		}
		dst.Set(seed)
		if parseErr = json.Unmarshal([]byte(strings.TrimSpace(raw)), out); parseErr == nil {
			return nil
		}
		parseErr = fmt.Errorf("parse: %w", parseErr)
		if attempt < stageAttempts {
			log.Printf("matchanalysis: stage %d parse failed, retrying: %v", stage, parseErr)
		}
	}
	return parseErr
}

// freeTextLanguageDirective tells the model which language to write every free-text
// field of a stage's output in — the requirement/evidence text, the hidden-signal
// insights, the dimension comments, the strengths/gaps and the recommendation. These
// are the candidate's own reading of their fit against a vacancy, not text that goes
// onto a CV, so they follow the caller's profile language rather than the vacancy's
// (freehire#1837; contrast internal/assistant's tailoring prompt, whose cv_edit
// bullets follow the vacancy's language instead).
func freeTextLanguageDirective(language string) string {
	return "\n\nWrite every free-text value in " + assistant.LanguageName(language) + ", regardless of what language the job posting or the candidate's CV is in.\n"
}

// stage1LanguageDirective is freeTextLanguageDirective plus one exception: Stage 1's
// "quote" field is defined as "a short verbatim excerpt from the job description" — a
// blanket translation instruction would contradict that and stop it from being
// verifiable against the posting. Every other Stage 1 field (the requirement text, the
// evidence explanation, the hidden-signal insight) is the model's own prose and follows
// the directive as normal.
func stage1LanguageDirective(language string) string {
	return freeTextLanguageDirective(language) +
		"Exception: \"quote\" must stay a verbatim excerpt copied from the job posting, in the posting's own language — never translate it.\n"
}

// stage1SystemPrompt pins the ATS extract-and-match contract.
func stage1SystemPrompt(language string) string {
	var b strings.Builder
	b.WriteString("You are an ATS (applicant tracking system) parser. Return ONLY a JSON object.\n\n")
	b.WriteString("From the job posting, extract the explicit requirements (skills, tools, experience, ")
	b.WriteString("responsibilities) plus the role-title and seniority signals. Classify each against the ")
	b.WriteString("candidate's CV. Return exactly these keys:\n")
	b.WriteString("- \"requirements\": an array (max 30) of objects, each:\n")
	b.WriteString("  - \"text\": the requirement, short.\n")
	b.WriteString("  - \"priority\": \"required\" or \"preferred\".\n")
	b.WriteString("  - \"status\": one of \"covered\" (present verbatim/trivial inflection), ")
	b.WriteString("\"synonym-only\" (the concept is present under a different term), ")
	b.WriteString("\"missing-have\" (the CV evidences it elsewhere but never states the term), ")
	b.WriteString("\"missing-gap\" (a genuine gap — absent, no close equivalent held).\n")
	b.WriteString("  - \"evidence\": where it appears in the CV, or why it is absent.\n")
	b.WriteString("  - \"evidence_strength\": for \"covered\"/\"synonym-only\" only, grade the cited evidence ")
	b.WriteString("as \"metric\" (an accomplishment with a number, scale, or measured outcome), ")
	b.WriteString("\"scope\" (breadth: teams, systems, regions), \"responsibility\" (clear ownership with ")
	b.WriteString("tools or methods), or \"keyword\" (the term is present but only a bare mention or ")
	b.WriteString("duty). Omit it for \"missing-have\"/\"missing-gap\".\n")
	b.WriteString("Base every judgement only on the CV text. NEVER fabricate a skill the CV does not ")
	b.WriteString("evidence — a genuine gap is \"missing-gap\", never hidden.\n\n")
	b.WriteString("Also return \"hidden_signals\": an array (max 5) of unstated culture/pace/team-stage ")
	b.WriteString("signals read from the posting's own wording — not from any explicit requirement. Each ")
	b.WriteString("object has \"quote\" (a short verbatim excerpt from the job description) and \"insight\" ")
	b.WriteString("(one line on what that wording implies about pace, ownership expectations, team stage, ")
	b.WriteString("or culture — e.g. \"comfortable with ambiguity\" implies no one will hand the candidate a ")
	b.WriteString("spec). Base every signal on wording actually present in the posting; if the posting is ")
	b.WriteString("short or generic and carries no distinctive wording, return an empty array — never invent ")
	b.WriteString("a signal to fill it.\n")
	b.WriteString(stage1LanguageDirective(language))
	return b.String()
}

// stage2SystemPrompt pins the recruiter six-dimension scoring contract.
func stage2SystemPrompt(language string) string {
	var b strings.Builder
	b.WriteString("You are a senior technical recruiter judging how well a candidate fits ONE role. ")
	b.WriteString("Return ONLY a JSON object. Base every judgement on the CV and the requirement match ")
	b.WriteString("provided; do not invent facts.\n\n")
	b.WriteString("Score each dimension 0-100 with a one-sentence \"comment\". Job-title alignment and ")
	b.WriteString("experience relevance matter most. Return exactly these keys:\n")
	b.WriteString("- \"title_alignment\": does the candidate's current/target title match this role's title?\n")
	b.WriteString("- \"experience_relevance\": how relevant is their domain and role-type experience?\n")
	b.WriteString("- \"seniority_fit\": does their level match the role's seniority?\n")
	b.WriteString("- \"skills_coverage\": consistent with the provided deterministic skills match.\n")
	b.WriteString("- \"company_context\": fit with the company's stage/industry (from the company info).\n")
	b.WriteString("- \"location_fit\": can the candidate actually take the role given the job's location/work ")
	b.WriteString("mode and their location preferences (accepted work modes, remote reach, base, relocation)? ")
	b.WriteString("For a REMOTE role, judge ONLY whether its region or countries fall within the candidate's ")
	b.WriteString("remote reach — a reach of \"global\", or one naming the job's region, covers any city or ")
	b.WriteString("country in that region; ignore the candidate's physical base and relocation entirely, and ")
	b.WriteString("never treat a remote posting's office city as a relocation requirement. Relocation matters ")
	b.WriteString("only for onsite or hybrid roles: an onsite job where they are based or will relocate scores ")
	b.WriteString("high; an onsite job far from their base with no relocation and a remote-only preference ")
	b.WriteString("scores low. Honour any NOTE about remote reach in the input. If the candidate stated no ")
	b.WriteString("preferences, judge on the job alone and do not penalise.\n")
	b.WriteString("Each of the six is an object {\"score\": int 0-100, \"comment\": string}.\n")
	b.WriteString("Also return \"strengths\" (array, max 6), \"gaps\" (array, max 6), and a ")
	b.WriteString("\"recommendation\" string: two or three short prose paragraphs of hiring judgement ")
	b.WriteString("(no headings, no lists). Do not recap per-requirement statuses or evidence strengths ")
	b.WriteString("— those are already on the page. Do NOT return an overall score — it is computed separately.\n")
	b.WriteString(freeTextLanguageDirective(language))
	return b.String()
}

// stage3SystemPrompt pins the adversarial-audit contract (same output shape as Stage 2).
func stage3SystemPrompt(language string) string {
	var b strings.Builder
	b.WriteString("You are a skeptical hiring manager auditing a recruiter's fit verdict. ")
	b.WriteString("Return ONLY a JSON object in the SAME shape as the verdict you are given.\n\n")
	b.WriteString("Challenge it against the CV evidence: lower any inflated dimension score, remove ")
	b.WriteString("strengths the CV does not actually support, and surface gaps that were glossed over. ")
	b.WriteString("For any requirement marked \"required\", treat weak evidence as thin support: a ")
	b.WriteString("\"synonym-only\" match, or a \"covered\" match graded \"keyword\" strength (a bare ")
	b.WriteString("mention rather than a metric-, scope-, or responsibility-backed one), is adjacent ")
	b.WriteString("exposure, not direct ownership — it may earn partial credit but must not by itself ")
	b.WriteString("sustain a high skills_coverage score. ")
	b.WriteString("Keep what is well-supported. Return the corrected verdict with the same keys ")
	b.WriteString("(title_alignment, experience_relevance, seniority_fit, skills_coverage, ")
	b.WriteString("company_context, location_fit, strengths, gaps, recommendation). ")
	b.WriteString("The recommendation remains two or three short prose paragraphs of hiring judgement ")
	b.WriteString("(no headings, no lists); do not recap per-requirement statuses or evidence strengths. ")
	b.WriteString("Do NOT fabricate anything.\n")
	b.WriteString(freeTextLanguageDirective(language))
	return b.String()
}

// stage1UserPrompt carries the (bounded) job text, the deterministic anchor, and the
// de-identified structured résumé (the candidate context — no raw CV is sent).
func stage1UserPrompt(in Input, candidate string) string {
	var b strings.Builder
	writeJob(&b, in)
	writeAnchor(&b, in.Match)
	writeBlockers(&b, in.Blockers)
	writeCandidate(&b, candidate)
	return b.String()
}

// stage2UserPrompt adds the company info and the Stage-1 requirement match.
func stage2UserPrompt(in Input, reqs []Requirement, candidate string) string {
	var b strings.Builder
	writeJob(&b, in)
	if info := strings.TrimSpace(in.CompanyInfo); info != "" {
		b.WriteString("Company info (JSON):\n")
		b.WriteString(llm.TruncateRunes(info, maxCompanyRunes))
		b.WriteString("\n\n")
	}
	writeAnchor(&b, in.Match)
	writeLocation(&b, in)
	writeBlockers(&b, in.Blockers)
	writeRequirements(&b, reqs)
	writeCandidate(&b, candidate)
	return b.String()
}

// stage3UserPrompt carries the Stage-2 verdict to audit plus the same evidence.
func stage3UserPrompt(in Input, reqs []Requirement, v recruiterVerdict, candidate string) string {
	var b strings.Builder
	b.WriteString("Verdict to audit (JSON):\n")
	if blob, err := json.Marshal(v); err == nil {
		b.Write(blob)
		b.WriteString("\n\n")
	}
	writeBlockers(&b, in.Blockers)
	writeRequirements(&b, reqs)
	writeCandidate(&b, candidate)
	return b.String()
}

// writeBlockers lists the deterministic hard constraints the candidate does NOT meet,
// so the model treats them as established facts — factoring them into the scores and
// gaps rather than re-deriving or inflating past them. Met constraints need no mention.
func writeBlockers(b *strings.Builder, blockers []hardconstraint.Blocker) {
	var unmet []string
	for _, bl := range blockers {
		if !bl.Met {
			unmet = append(unmet, bl.Reason)
		}
	}
	if len(unmet) == 0 {
		return
	}
	b.WriteString("Hard constraints the candidate does NOT meet (already determined; treat as established, ")
	b.WriteString("factor into the scores and gaps, and never claim the candidate meets them):\n")
	for _, reason := range unmet {
		b.WriteString("- ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func writeJob(b *strings.Builder, in Input) {
	b.WriteString("Job title: ")
	b.WriteString(in.JobTitle)
	b.WriteString("\n\nJob description:\n")
	b.WriteString(llm.TruncateRunes(in.JobDescription, maxDescriptionRunes))
	b.WriteString("\n\n")
}

// maxStructuredRunes bounds the structured-résumé JSON added to the prompt — it is a
// compact summary, so a modest cap covers it while keeping the stage responsive.
const maxStructuredRunes = 3000

// candidateContext renders the candidate context sent to the model. Empty when the caller has
// no banked experience — the chain then produces no analysis, and the raw CV is never a
// fallback — or, defensively, when the projection will not marshal.
//
// There is nothing to strip here and nothing to re-project: de-identification is the argument's
// type. resumeextract.Professional names the fields a model may see, so a field the projection
// does not name never reaches the prompt, including one that does not exist yet. Deleting known
// contact keys would have sent every future addition to the model until somebody remembered to
// extend the list.
func candidateContext(candidate resumeextract.Professional) string {
	if len(candidate.Experience) == 0 {
		return ""
	}
	blob, err := json.Marshal(candidate)
	if err != nil {
		return ""
	}
	return llm.TruncateRunes(string(blob), maxStructuredRunes)
}

// writeCandidate appends the de-identified structured résumé as the candidate context.
// Omitted when empty (the chain never reaches a stage with an empty candidate).
func writeCandidate(b *strings.Builder, candidate string) {
	if candidate == "" {
		return
	}
	b.WriteString("Candidate (structured résumé, JSON — contacts removed):\n")
	b.WriteString(candidate)
	b.WriteString("\n\n")
}

// writeAnchor renders the deterministic skills match so the model explains and
// augments it rather than recomputing skills from scratch.
func writeAnchor(b *strings.Builder, m jobmatch.JobMatch) {
	if len(m.Matched)+len(m.Adjacent)+len(m.Missing) == 0 {
		return
	}
	fmt.Fprintf(b, "Deterministic skills match (coverage %d%%):\n", m.CoveragePercent)
	if len(m.Matched) > 0 {
		b.WriteString("- has: " + strings.Join(m.Matched, ", ") + "\n")
	}
	for _, adj := range m.Adjacent {
		b.WriteString("- close: " + adj.Name + " (via " + adj.Via + ")\n")
	}
	if len(m.Missing) > 0 {
		b.WriteString("- missing: " + strings.Join(m.Missing, ", ") + "\n")
	}
	b.WriteString("\n")
}

// writeLocation renders the job geography and the candidate's location preferences so
// the model can score location & work-mode fit. Omitted entirely when neither side
// carries any geography (nothing to reason about).
func writeLocation(b *strings.Builder, in Input) {
	hasJob := in.JobWorkMode != "" || in.JobRemote || in.JobLocation != "" || len(in.JobRegions) > 0 || len(in.JobCountries) > 0
	hasPref := strings.TrimSpace(in.LocationPreferences) != ""
	if !hasJob && !hasPref {
		return
	}
	b.WriteString("Location & work mode:\n")
	if in.JobWorkMode != "" {
		b.WriteString("- job work mode: " + in.JobWorkMode + "\n")
	}
	if in.JobRemote {
		b.WriteString("- job is remote\n")
	}
	if in.JobLocation != "" {
		b.WriteString("- job location: " + in.JobLocation + "\n")
	}
	if len(in.JobRegions) > 0 {
		b.WriteString("- job regions: " + strings.Join(in.JobRegions, ", ") + "\n")
	}
	if len(in.JobCountries) > 0 {
		b.WriteString("- job countries: " + strings.Join(in.JobCountries, ", ") + "\n")
	}
	if hasPref {
		b.WriteString("- candidate location preferences (JSON): ")
		b.WriteString(llm.TrimTruncateRunes(in.LocationPreferences, maxCompanyRunes))
		b.WriteString("\n")
	}
	if remoteWithinReach(in) {
		b.WriteString("- NOTE: this is a remote role and its region is within the candidate's stated ")
		b.WriteString("remote reach, so they can work it from where they are — score location fit high and ")
		b.WriteString("do not penalise their base or relocation stance.\n")
	}
	b.WriteString("\n")
}

// remoteWithinReach reports whether the job is remote AND its region or country falls
// within the candidate's stated remote reach (their location_preferences remote.regions
// and remote.countries — two independent fields the profile editor lets a candidate set
// separately, e.g. reach scoped to specific countries with no region picked at all). A
// reach of "global", or one naming the job's region or country, covers the posting
// regardless of the posted office city — a remote worker within that reach can take it
// without relocating. This is deterministic on purpose: it stops the model from reading a
// remote role's HQ city (e.g. a LATAM-remote job posted from Santo Domingo) as a
// relocation requirement and scoring location_fit 0. Unset/unparseable prefs or a
// non-remote job → false (the model judges).
func remoteWithinReach(in Input) bool {
	if !in.JobRemote && !strings.EqualFold(in.JobWorkMode, "remote") {
		return false
	}
	regions, countries := candidateRemoteReach(in.LocationPreferences)
	for _, r := range regions {
		if strings.EqualFold(r, "global") {
			return true
		}
		for _, jr := range in.JobRegions {
			if strings.EqualFold(r, jr) {
				return true
			}
		}
	}
	for _, c := range countries {
		for _, jc := range in.JobCountries {
			if strings.EqualFold(c, jc) {
				return true
			}
		}
	}
	return false
}

// candidateRemoteReach pulls the remote.regions and remote.countries lists out of the raw
// location_preferences JSON — the same shape userprofile.GeoSet writes. Both empty on
// unset/unparseable input (the caller then degrades to model judgement).
func candidateRemoteReach(prefsJSON string) (regions, countries []string) {
	var p struct {
		Remote struct {
			Regions   []string `json:"regions"`
			Countries []string `json:"countries"`
		} `json:"remote"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(prefsJSON)), &p) != nil {
		return nil, nil
	}
	return p.Remote.Regions, p.Remote.Countries
}

func writeRequirements(b *strings.Builder, reqs []Requirement) {
	if len(reqs) == 0 {
		return
	}
	b.WriteString("Requirement match (from the ATS stage):\n")
	for _, r := range reqs {
		tag := r.Priority + "/" + r.Status
		if r.EvidenceStrength != "" {
			tag += "/" + r.EvidenceStrength
		}
		b.WriteString("- [" + tag + "] " + r.Text + "\n")
	}
	b.WriteString("\n")
}
