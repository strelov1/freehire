package matchanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// Input bounds for untrusted text sent to the model. Kept modest because the fit model reasons
// over every input token, so large inputs balloon each stage's thinking time.
const (
	maxDescriptionRunes = 6000
	maxCompanyRunes     = 2000
)

// Analyzer runs the fixed three-stage fit prompt-chain over an llm.Client. A nil
// client (LLM unconfigured) makes Analyze a no-op so the endpoint degrades to no
// analysis, mirroring atscheck.Analyzer. The chain scores from the de-identified structured
// résumé (see Input.StructuredResume) and never sends the raw CV, so it needs no PII detector.
type Analyzer struct {
	client *llm.Client
}

// As returns an analyzer spending as `caller`, so one analysis is billed to the candidate's
// own gateway credential. Nil-safe.
//
// It takes a caller and not a client so the chain keeps the per-stage timeout it was built
// with (matchAnalysisLLMTimeout). See llm.Caller.
func (a *Analyzer) As(caller llm.Caller) *Analyzer {
	if a == nil {
		return a
	}
	clone := *a
	clone.client = a.client.AsCaller(caller)

	return &clone
}

// NewAnalyzer wraps an llm.Client; client may be nil (LLM unconfigured), in which case the
// analysis degrades to a no-op.
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

	// Language is the caller's account language (e.g. "en"/"ru"), used for every free-text
	// field in the analysis. It follows the candidate's profile, not the vacancy's (unlike
	// tailoring's CV bullets). Empty falls back to English, as assistant.LanguageName does.
	Language string

	// Blockers are the deterministic hard-constraint results (hardconstraint.Evaluate). Unmet
	// ones go into the prompt as established facts; the same list caps the served overall_score.
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

// StageLabel names one of the three chain stages (1-3) for a caller that describes a stage
// without running the chain. Empty for an unknown stage.
func StageLabel(stage int) string { return stageLabels[stage] }

// Analyze runs the three-stage chain and returns the final analysis, discarding the
// stream. Returns (nil, nil) when the LLM is unconfigured.
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

	// Without banked work history there is nothing to reason over: no analysis, and the raw CV
	// is never a fallback.
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

// stageAttempts is how many times a stage's LLM call is tried. A parse failure (the gateway
// occasionally returns a transient HTML error page) and a timeout (a hung stage, not a slow
// one) are retried; a connection error or a caller who has gone away returns immediately.
// See attemptsForStage for the audit's single attempt.
const stageAttempts = 2

// streamStage runs one streaming JSON call, forwarding reasoning deltas as thinking
// events for the given stage, and unmarshals the accumulated JSON into out. A parse
// failure and a timed-out stage are each retried once; anything else returns at once.
func (a *Analyzer) streamStage(ctx context.Context, stage int, system, user string, emit func(Event), out any) error {
	// Snapshot of out as the caller handed it in (zero, or Stage 2's verdict for the Stage 3
	// seed-and-merge audit). Every attempt restores it before decoding: encoding/json does not
	// roll back a partial decode, so a later attempt's partial JSON would inherit stale fields.
	dst := reflect.ValueOf(out).Elem()
	seed := reflect.New(dst.Type()).Elem()
	seed.Set(dst)

	var parseErr error
	// The audit is allowed longer (see timeoutForStage); the clone leaves a.client's own bound alone.
	client := a.client.WithTimeout(timeoutForStage(stage, a.client.Timeout()))

	attempts := attemptsForStage(stage)
	for attempt := 1; attempt <= attempts; attempt++ {
		raw, err := client.GenerateJSONStream(ctx, system, user, func(t string) {
			emit(Event{Kind: EventThinking, Stage: stage, Thinking: t})
		}, stageGenOptions(stage)...)
		if err != nil {
			// A stage that hit its own per-call deadline is retried like a parse failure. A caller
			// who went away is final: nobody would read a second answer.
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil && attempt < attempts {
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
		if attempt < attempts {
			log.Printf("matchanalysis: stage %d parse failed, retrying: %v", stage, parseErr)
		}
	}
	return parseErr
}

// Every stage's user prompt opens with the candidate context, before anything about the job.
// It is the one block that repeats when a candidate analyses several vacancies, and a provider
// prompt cache reuses only an identical prefix: placed after the job it is recomputed on every
// call, placed first it rides on the stage's static system prompt.

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

// maxStructuredRunes bounds the structured-résumé JSON added to the prompt.
const maxStructuredRunes = 3000

// HasCandidateContext reports whether there is enough banked work history for the chain to
// reason over. False means AnalyzeStream will decline and produce nothing.
//
// Exported so a caller can tell "no gateway" (nothing the candidate did) from "unreadable CV"
// (only they can fix it). One predicate, one owner: a caller re-deriving it would drift.
func HasCandidateContext(candidate resumeextract.Professional) bool {
	return candidateContext(candidate) != ""
}

// candidateContext renders the candidate context sent to the model. Empty when there is no
// banked experience (the raw CV is never a fallback) or the projection will not marshal.
//
// De-identification is the argument's type: resumeextract.Professional names the fields a
// model may see, so a field it does not name never reaches the prompt. Stripping known contact
// keys instead would leak every future addition.
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

// writeCandidate appends the de-identified structured résumé; omitted when empty.
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

// remoteWithinReach reports whether the job is remote AND its region or country falls within
// the candidate's stated remote reach (location_preferences remote.regions / remote.countries,
// which are independent fields). A reach of "global", or one naming the job's region or country,
// covers the posting regardless of the office city. Deterministic on purpose: it stops the model
// reading a remote role's HQ city as a relocation requirement and scoring location_fit 0.
// Unset/unparseable prefs or a non-remote job give false (the model judges).
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
