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

// Input bounds for untrusted, user/ingest-supplied text sent to the model.
const (
	maxCVRunes          = 24000
	maxDescriptionRunes = 16000
	maxCompanyRunes     = 4000
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
// the job text, the raw company_info JSON, the candidate's CV text, and the
// deterministic skills match used as the grounding anchor.
type Input struct {
	JobTitle       string
	JobDescription string
	CompanyInfo    string
	CVText         string
	Match          jobmatch.JobMatch
}

// stage1Out is the Extract & Match stage's raw output.
type stage1Out struct {
	Requirements []Requirement `json:"requirements"`
}

// Analyze runs Stage 1 (Extract & Match) → Stage 2 (Recruiter verdict) → Stage 3
// (Adversarial audit) as sequential JSON calls, sanitizes each, and builds the served
// Analysis from the audited dimensions. Returns (nil, nil) when the LLM is
// unconfigured. A Stage 1/2 failure returns an error (nothing to serve); a Stage 3
// failure degrades to the un-audited Stage 2 verdict rather than erroring.
func (a *Analyzer) Analyze(ctx context.Context, in Input) (*Analysis, error) {
	if a == nil || a.client == nil {
		return nil, nil
	}

	// Stage 1 — Extract & Match (the ATS lens).
	var s1 stage1Out
	if err := a.stage(ctx, stage1SystemPrompt(), stage1UserPrompt(in), &s1); err != nil {
		return nil, fmt.Errorf("jobfit: stage 1: %w", err)
	}
	reqs := sanitizeRequirements(s1.Requirements)

	// Stage 2 — Recruiter verdict (the human lens).
	var verdict recruiterVerdict
	if err := a.stage(ctx, stage2SystemPrompt(), stage2UserPrompt(in, reqs), &verdict); err != nil {
		return nil, fmt.Errorf("jobfit: stage 2: %w", err)
	}
	sanitizeVerdict(&verdict)

	// Stage 3 — Adversarial audit. Seed the audit target with the sanitized Stage 2
	// verdict so json.Unmarshal MERGES: the audit overrides only the fields it returns
	// and omitted dimensions keep their Stage 2 scores. A budget model that echoes just
	// the fields it changed can then only refine the verdict, never hollow it out to
	// zeros. Best-effort: on a parse/transport failure keep the un-audited verdict.
	audited := verdict
	if err := a.stage(ctx, stage3SystemPrompt(), stage3UserPrompt(in, reqs, verdict), &audited); err != nil {
		log.Printf("jobfit: stage 3 audit failed, serving un-audited verdict: %v", err)
	} else {
		sanitizeVerdict(&audited)
		verdict = audited
	}

	analysis := buildAnalysis(reqs, verdict)
	return &analysis, nil
}

// stage runs one JSON call and unmarshals it into out.
func (a *Analyzer) stage(ctx context.Context, system, user string, out any) error {
	raw, err := a.client.GenerateJSON(ctx, system, user)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), out); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	return nil
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

// stage2SystemPrompt pins the recruiter five-dimension scoring contract.
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
	b.WriteString("Each of the five is an object {\"score\": int 0-100, \"comment\": string}.\n")
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
	b.WriteString("company_context, strengths, gaps, recommendation). Do NOT fabricate anything.\n")
	return b.String()
}

// stage1UserPrompt carries the (bounded) job text, CV, and the deterministic anchor.
func stage1UserPrompt(in Input) string {
	var b strings.Builder
	writeJob(&b, in)
	writeAnchor(&b, in.Match)
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
