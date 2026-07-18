package jobfit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/llm"
)

// Input bounds for untrusted, user/ingest-supplied text sent to the model. Kept modest
// on purpose: the fit model reasons over every input token, so a large CV/description
// balloons its thinking time (tens of seconds per stage). These caps keep each stage
// responsive while still covering the substance of a CV and a posting.
const (
	maxCVRunes          = 10000
	maxDescriptionRunes = 6000
	maxCompanyRunes     = 2000
)

// Analyzer runs the fixed three-stage fit prompt-chain over an llm.Client. A nil
// client (LLM unconfigured) makes Analyze a no-op so the endpoint degrades to no
// analysis, mirroring atscheck.Analyzer.
type Analyzer struct {
	client *llm.Client
}

// NewAnalyzer wraps an llm.Client; client may be nil (LLM unconfigured).
func NewAnalyzer(client *llm.Client) *Analyzer { return &Analyzer{client: client} }

// ModelID returns the underlying model id (empty when unconfigured), so a caller can
// record which model produced a cached analysis.
func (a *Analyzer) ModelID() string { return a.client.ModelID() }

// Input is everything the chain needs, gathered by the handler before the first call:
// the job text, the raw company_info JSON, the candidate's CV text, the deterministic
// skills match used as the grounding anchor, and the job geography + the candidate's
// location preferences (raw JSON) used to score location & work-mode fit.
type Input struct {
	JobTitle       string
	JobDescription string
	CompanyInfo    string
	CVText         string
	// StructuredResume is the caller's sanitized structured résumé as JSON, supplied
	// beside CVText as pre-normalized context — never a replacement (the raw CV stays the
	// ground truth). Empty when the caller has no current structured résumé, in which case
	// the chain runs exactly as it does on the CV text alone.
	StructuredResume string
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
}

// stage1Out is the Extract & Match stage's raw output.
type stage1Out struct {
	Requirements []Requirement `json:"requirements"`
}

// EventKind tags a streaming Event (see AnalyzeStream).
type EventKind string

const (
	EventStageStart   EventKind = "stage_start"  // a stage began (Stage set)
	EventStageDone    EventKind = "stage_done"   // a stage finished (Stage set)
	EventThinking     EventKind = "thinking"     // a reasoning-token delta (Stage + Thinking)
	EventRequirements EventKind = "requirements" // Stage-1 result (Requirements)
	EventDimensions   EventKind = "dimensions"   // interim post-Stage-2 analysis (Analysis)
	EventFinal        EventKind = "final"        // the audited final analysis (Analysis)
)

// Event is one step of a streaming analysis. Only the fields relevant to Kind are set.
type Event struct {
	Kind         EventKind     `json:"kind"`
	Stage        int           `json:"stage,omitempty"`
	Label        string        `json:"label,omitempty"`
	Thinking     string        `json:"thinking,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Analysis     *Analysis     `json:"analysis,omitempty"`
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

	// Stage 1 — Extract & Match (the ATS lens).
	emit(Event{Kind: EventStageStart, Stage: 1, Label: stageLabels[1]})
	var s1 stage1Out
	if err := a.streamStage(ctx, 1, stage1SystemPrompt(), stage1UserPrompt(in), emit, &s1); err != nil {
		return nil, fmt.Errorf("jobfit: stage 1: %w", err)
	}
	reqs := sanitizeRequirements(s1.Requirements)
	emit(Event{Kind: EventRequirements, Requirements: reqs})
	emit(Event{Kind: EventStageDone, Stage: 1, Label: stageLabels[1]})

	// Stage 2 — Recruiter verdict (the human lens).
	emit(Event{Kind: EventStageStart, Stage: 2, Label: stageLabels[2]})
	var verdict recruiterVerdict
	if err := a.streamStage(ctx, 2, stage2SystemPrompt(), stage2UserPrompt(in, reqs), emit, &verdict); err != nil {
		return nil, fmt.Errorf("jobfit: stage 2: %w", err)
	}
	sanitizeVerdict(&verdict)
	interim := buildAnalysis(reqs, verdict)
	emit(Event{Kind: EventDimensions, Analysis: &interim})
	emit(Event{Kind: EventStageDone, Stage: 2, Label: stageLabels[2]})

	// Stage 3 — Adversarial audit. Seed the audit target with the sanitized Stage 2
	// verdict so json.Unmarshal MERGES: the audit overrides only the fields it returns
	// and omitted dimensions keep their Stage 2 scores. A budget model that echoes just
	// the fields it changed can then only refine the verdict, never hollow it out to
	// zeros. Best-effort: on a parse/transport failure keep the un-audited verdict.
	emit(Event{Kind: EventStageStart, Stage: 3, Label: stageLabels[3]})
	audited := verdict
	if err := a.streamStage(ctx, 3, stage3SystemPrompt(), stage3UserPrompt(in, reqs, verdict), emit, &audited); err != nil {
		log.Printf("jobfit: stage 3 audit failed, serving un-audited verdict: %v", err)
	} else {
		sanitizeVerdict(&audited)
		verdict = audited
	}
	emit(Event{Kind: EventStageDone, Stage: 3, Label: stageLabels[3]})

	analysis := buildAnalysis(reqs, verdict)
	emit(Event{Kind: EventFinal, Analysis: &analysis})
	return &analysis, nil
}

// stageAttempts is how many times a stage's LLM call is tried on a PARSE failure. The
// gateway occasionally returns a transient HTML error page (a 502/504) that fails JSON
// parsing; a single re-try recovers it, mirroring the enrichment worker. A transport
// error (timeout, connection) is NOT retried — the model is slow, so a retry with the
// same timeout would only double the wait — it is returned immediately.
const stageAttempts = 2

// streamStage runs one streaming JSON call, forwarding reasoning deltas as thinking
// events for the given stage, and unmarshals the accumulated JSON into out. A transport
// failure returns at once; a parse failure (non-JSON gateway error page) is retried once.
func (a *Analyzer) streamStage(ctx context.Context, stage int, system, user string, emit func(Event), out any) error {
	var parseErr error
	for attempt := 1; attempt <= stageAttempts; attempt++ {
		raw, err := a.client.GenerateJSONStream(ctx, system, user, func(t string) {
			emit(Event{Kind: EventThinking, Stage: stage, Thinking: t})
		})
		if err != nil {
			return err // transport/timeout — retrying wouldn't help, fail fast
		}
		if parseErr = json.Unmarshal([]byte(strings.TrimSpace(raw)), out); parseErr == nil {
			return nil
		}
		parseErr = fmt.Errorf("parse: %w", parseErr)
		if attempt < stageAttempts {
			log.Printf("jobfit: stage %d parse failed, retrying: %v", stage, parseErr)
		}
	}
	return parseErr
}

// stage1SystemPrompt pins the ATS extract-and-match contract.
func stage1SystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are an ATS (applicant tracking system) parser. Return ONLY a JSON object.\n\n")
	b.WriteString("From the job posting, extract the explicit requirements (skills, tools, experience, ")
	b.WriteString("responsibilities) plus the role-title and seniority signals. Classify each against the ")
	b.WriteString("candidate's CV. Return exactly one key:\n")
	b.WriteString("- \"requirements\": an array (max 30) of objects, each:\n")
	b.WriteString("  - \"text\": the requirement, short.\n")
	b.WriteString("  - \"priority\": \"required\" or \"preferred\".\n")
	b.WriteString("  - \"status\": one of \"covered\" (present verbatim/trivial inflection), ")
	b.WriteString("\"synonym-only\" (the concept is present under a different term), ")
	b.WriteString("\"missing-have\" (the CV evidences it elsewhere but never states the term), ")
	b.WriteString("\"missing-gap\" (a genuine gap — absent, no close equivalent held).\n")
	b.WriteString("  - \"evidence\": where it appears in the CV, or why it is absent.\n")
	b.WriteString("Base every judgement only on the CV text. NEVER fabricate a skill the CV does not ")
	b.WriteString("evidence — a genuine gap is \"missing-gap\", never hidden.\n")
	return b.String()
}

// stage2SystemPrompt pins the recruiter six-dimension scoring contract.
func stage2SystemPrompt() string {
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
	b.WriteString("Also return \"strengths\" (array, max 6), \"gaps\" (array, max 6), and a single ")
	b.WriteString("\"recommendation\" string. Do NOT return an overall score — it is computed separately.\n")
	return b.String()
}

// stage3SystemPrompt pins the adversarial-audit contract (same output shape as Stage 2).
func stage3SystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are a skeptical hiring manager auditing a recruiter's fit verdict. ")
	b.WriteString("Return ONLY a JSON object in the SAME shape as the verdict you are given.\n\n")
	b.WriteString("Challenge it against the CV evidence: lower any inflated dimension score, remove ")
	b.WriteString("strengths the CV does not actually support, and surface gaps that were glossed over. ")
	b.WriteString("Keep what is well-supported. Return the corrected verdict with the same keys ")
	b.WriteString("(title_alignment, experience_relevance, seniority_fit, skills_coverage, ")
	b.WriteString("company_context, location_fit, strengths, gaps, recommendation). Do NOT fabricate anything.\n")
	return b.String()
}

// stage1UserPrompt carries the (bounded) job text, CV, and the deterministic anchor,
// plus the pre-normalized structured résumé when present (additive to the raw CV).
func stage1UserPrompt(in Input) string {
	var b strings.Builder
	writeJob(&b, in)
	writeAnchor(&b, in.Match)
	writeStructured(&b, in)
	writeCV(&b, in)
	return b.String()
}

// stage2UserPrompt adds the company info and the Stage-1 requirement match.
func stage2UserPrompt(in Input, reqs []Requirement) string {
	var b strings.Builder
	writeJob(&b, in)
	if info := strings.TrimSpace(in.CompanyInfo); info != "" {
		b.WriteString("Company info (JSON):\n")
		b.WriteString(llm.TruncateRunes(info, maxCompanyRunes))
		b.WriteString("\n\n")
	}
	writeAnchor(&b, in.Match)
	writeLocation(&b, in)
	writeRequirements(&b, reqs)
	writeCV(&b, in)
	return b.String()
}

// stage3UserPrompt carries the Stage-2 verdict to audit plus the same evidence.
func stage3UserPrompt(in Input, reqs []Requirement, v recruiterVerdict) string {
	var b strings.Builder
	b.WriteString("Verdict to audit (JSON):\n")
	if blob, err := json.Marshal(v); err == nil {
		b.Write(blob)
		b.WriteString("\n\n")
	}
	writeRequirements(&b, reqs)
	writeCV(&b, in)
	return b.String()
}

func writeJob(b *strings.Builder, in Input) {
	b.WriteString("Job title: ")
	b.WriteString(in.JobTitle)
	b.WriteString("\n\nJob description:\n")
	b.WriteString(llm.TruncateRunes(in.JobDescription, maxDescriptionRunes))
	b.WriteString("\n\n")
}

func writeCV(b *strings.Builder, in Input) {
	b.WriteString("CV:\n")
	b.WriteString(llm.TruncateRunes(in.CVText, maxCVRunes))
	b.WriteString("\n")
}

// maxStructuredRunes bounds the structured-résumé JSON added to the prompt — it is a
// compact summary, so a modest cap covers it while keeping the stage responsive.
const maxStructuredRunes = 3000

// writeStructured appends the caller's pre-normalized structured résumé as context, when
// present. Omitted entirely when empty, so an un-extracted CV yields exactly today's
// prompt. It is labelled as a parsed summary so the model treats the raw CV as ground
// truth and this as an aid.
func writeStructured(b *strings.Builder, in Input) {
	s := strings.TrimSpace(in.StructuredResume)
	if s == "" {
		return
	}
	b.WriteString("Structured résumé (parsed summary, JSON — the CV below is ground truth):\n")
	b.WriteString(llm.TruncateRunes(s, maxStructuredRunes))
	b.WriteString("\n\n")
}

// writeAnchor renders the deterministic skills match so the model explains and
// augments it rather than recomputing skills from scratch.
func writeAnchor(b *strings.Builder, m jobmatch.JobMatch) {
	if len(m.Matched)+len(m.Adjacent)+len(m.Missing) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("Deterministic skills match (coverage %d%%):\n", m.CoveragePercent))
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
		b.WriteString(llm.TruncateRunes(strings.TrimSpace(in.LocationPreferences), maxCompanyRunes))
		b.WriteString("\n")
	}
	if remoteWithinReach(in) {
		b.WriteString("- NOTE: this is a remote role and its region is within the candidate's stated ")
		b.WriteString("remote reach, so they can work it from where they are — score location fit high and ")
		b.WriteString("do not penalise their base or relocation stance.\n")
	}
	b.WriteString("\n")
}

// remoteWithinReach reports whether the job is remote AND its region falls within the
// candidate's stated remote reach (their location_preferences remote.regions). A reach of
// "global", or one naming the job's region, covers the posting regardless of the posted
// office city/country — a remote worker in that region can take it without relocating. This
// is deterministic on purpose: it stops the model from reading a remote role's HQ city (e.g.
// a LATAM-remote job posted from Santo Domingo) as a relocation requirement and scoring
// location_fit 0. Unset/unparseable prefs or a non-remote job → false (the model judges).
func remoteWithinReach(in Input) bool {
	if !in.JobRemote && !strings.EqualFold(in.JobWorkMode, "remote") {
		return false
	}
	for _, r := range candidateRemoteReach(in.LocationPreferences) {
		if strings.EqualFold(r, "global") {
			return true
		}
		for _, jr := range in.JobRegions {
			if strings.EqualFold(r, jr) {
				return true
			}
		}
	}
	return false
}

// candidateRemoteReach pulls the remote.regions list out of the raw location_preferences
// JSON. Empty on unset/unparseable input (the caller then degrades to model judgement).
func candidateRemoteReach(prefsJSON string) []string {
	var p struct {
		Remote struct {
			Regions []string `json:"regions"`
		} `json:"remote"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(prefsJSON)), &p) != nil {
		return nil
	}
	return p.Remote.Regions
}

func writeRequirements(b *strings.Builder, reqs []Requirement) {
	if len(reqs) == 0 {
		return
	}
	b.WriteString("Requirement match (from the ATS stage):\n")
	for _, r := range reqs {
		b.WriteString("- [" + r.Priority + "/" + r.Status + "] " + r.Text + "\n")
	}
	b.WriteString("\n")
}
