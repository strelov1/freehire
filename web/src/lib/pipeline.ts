// Presentation layer for the application pipeline. The server returns the count at each
// stage; this module is how the Pipeline tab draws them — the bands, their colours, and the
// two derived rates.
//
// The bands are the generated groups, so the funnel and the board cannot disagree about what
// to call a settled application. This file used to hold seven bucket names of its own —
// `No answer`, `In progress`, `Declined` — words the reader met on no other screen.

import { STAGE_GROUPS } from './generated/contracts';
import type { PipelineStats } from './types';

export interface PipelineBand {
  id: (typeof STAGE_GROUPS)[number]['id'];
  label: string;
  /** The stages this band totals, in pipeline order — and its breakdown, so a settled band
   *  can say what settled it instead of reading as one undifferentiated block. */
  stages: readonly string[];
  /** Sankey ribbon/node fill. */
  color: string;
}

// One colour per group. `closed` is deliberately neutral rather than the rose the old
// `rejected` bucket carried: the band holds accepted offers too, and painting a settled group
// as a failure editorialises a number the reader is trying to read.
const BAND_COLORS: Record<string, string> = {
  applied: '#cbd5e1',
  interview: '#93c5fd',
  offer: '#86efac',
  closed: '#c4b5fd',
};

/** Bands in funnel order, from earliest to settled. */
export const PIPELINE_BANDS: PipelineBand[] = STAGE_GROUPS.map((g) => ({
  id: g.id,
  label: g.label,
  stages: g.stages,
  color: BAND_COLORS[g.id] ?? '#cbd5e1',
}));

const countAt = (s: PipelineStats, stage: string): number =>
  s.stages[stage as keyof typeof s.stages] ?? 0;

/** The total at a band: the sum of the stages it holds. */
export function bandTotal(s: PipelineStats, band: PipelineBand): number {
  return band.stages.reduce((sum, stage) => sum + countAt(s, stage), 0);
}

/** The per-stage breakdown of a band, in pipeline order, for the line beneath it. */
export function bandBreakdown(
  s: PipelineStats,
  band: PipelineBand,
): { stage: string; count: number }[] {
  return band.stages.map((stage) => ({ stage, count: countAt(s, stage) }));
}

/** Share of applications that reached an interview or beyond. A current-status
 *  snapshot, so it is a lower bound (a job rejected after interviewing counts
 *  only as rejected). Zero applications yields 0 without dividing by zero. */
export function interviewRate(s: PipelineStats): number {
  if (s.applications === 0) return 0;
  return (s.stages.interview + s.stages.offer + s.stages.accepted) / s.applications;
}

/** Share of applications that reached an offer or beyond. Same snapshot caveat. */
export function offerRate(s: PipelineStats): number {
  if (s.applications === 0) return 0;
  return (s.stages.offer + s.stages.accepted) / s.applications;
}
