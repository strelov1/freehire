// Pure presentation logic for the AI fit-analysis block and page. Kept out of the
// components so it is unit-testable (vitest) without a DOM: the verdict/score → colour
// tone, the ATS requirement-status → label + tone, and the SSE stream reducer. The wire
// shapes come from the Go contract.

import type { JobFitAnalysis, JobFitRequirement } from './types';

/** A colour tone bucket the component maps to Tailwind classes. */
export type Tone = 'strong' | 'good' | 'moderate' | 'weak' | 'poor';

/** verdictTone buckets a 0-100 overall score into a colour tone, on the same
 *  thresholds the backend uses to derive the verdict label (kept in sync by hand —
 *  the label is authoritative, this only drives colour). */
export function verdictTone(overall: number): Tone {
  if (overall >= 75) return 'strong';
  if (overall >= 60) return 'good';
  if (overall >= 45) return 'moderate';
  if (overall >= 30) return 'weak';
  return 'poor';
}

/** Display metadata for one ATS requirement-match status: a short human label and a
 *  tone. `covered`/`synonym-only` are positive, `missing-have` is a fixable near-miss,
 *  `missing-gap` is a genuine gap. An unknown status falls back to a neutral label. */
export function requirementStatusMeta(status: string): { label: string; tone: Tone } {
  switch (status) {
    case 'covered':
      return { label: 'Covered', tone: 'strong' };
    case 'synonym-only':
      return { label: 'Synonym', tone: 'good' };
    case 'missing-have':
      return { label: 'Add it', tone: 'moderate' };
    case 'missing-gap':
      return { label: 'Gap', tone: 'poor' };
    default:
      return { label: status, tone: 'moderate' };
  }
}

// ── SSE stream state ────────────────────────────────────────────────────────────
// The page consumes the fit stream (server-sent events) and folds each event into this
// state via reduceFitEvent — pure and DOM-free so the accumulation is unit-tested.

export type StageState = 'pending' | 'active' | 'done';

export interface FitStage {
  n: number;
  label: string;
  state: StageState;
}

/** The full page state built up from the SSE stream. `analysis` holds the interim
 *  post-Stage-2 verdict, then the audited final; `done`/`error` are terminal. */
export interface FitStreamState {
  hasCV: boolean;
  stages: FitStage[];
  thinking: string;
  requirements: JobFitRequirement[];
  analysis: JobFitAnalysis | null;
  done: boolean;
  error: string | null;
}

/** The three stages in fixed order, all pending. */
export function initFitStream(): FitStreamState {
  return {
    hasCV: true,
    stages: [
      { n: 1, label: 'Extract & Match', state: 'pending' },
      { n: 2, label: 'Recruiter verdict', state: 'pending' },
      { n: 3, label: 'Adversarial audit', state: 'pending' },
    ],
    thinking: '',
    requirements: [],
    analysis: null,
    done: false,
    error: null,
  };
}

/** Fold one SSE event (its name + parsed JSON data) into the stream state, returning a
 *  new state (never mutates the input). Unknown events are ignored. */
export function reduceFitEvent(prev: FitStreamState, name: string, data: unknown): FitStreamState {
  const s = { ...prev, stages: prev.stages.map((x) => ({ ...x })) };
  const d = (data ?? {}) as Record<string, unknown>;
  switch (name) {
    case 'meta':
      s.hasCV = d.has_cv !== false;
      return s;
    case 'stage_start':
      setStage(s, d.stage as number, 'active');
      return s;
    case 'stage_done':
      setStage(s, d.stage as number, 'done');
      return s;
    case 'thinking':
      s.thinking = prev.thinking + String(d.thinking ?? '');
      return s;
    case 'requirements':
      s.requirements = (d.requirements as JobFitRequirement[]) ?? [];
      return s;
    case 'dimensions':
      s.analysis = (d.analysis as JobFitAnalysis) ?? prev.analysis;
      return s;
    case 'final':
      s.analysis = (d.analysis as JobFitAnalysis) ?? prev.analysis;
      s.done = true;
      s.stages = s.stages.map((x) => ({ ...x, state: 'done' as StageState }));
      return s;
    case 'stream_error':
      s.error = String(d.message ?? 'Analysis failed');
      s.done = true;
      return s;
    default:
      return prev;
  }
}

function setStage(s: FitStreamState, n: number, state: StageState) {
  const stage = s.stages.find((x) => x.n === n);
  if (stage) stage.state = state;
}
