// Presentation layer for the application pipeline. The server owns the
// stage→bucket mapping and returns the bucket counts; this module is purely how
// the Pipeline tab renders them — the bucket order, labels, colors, and the two
// derived rates. The seven buckets are stable, so the vocabulary is a static map
// here rather than generated (a documented seam: if a bucket is ever added, this
// list and the Go mapping must change together).

import type { PipelineBuckets, PipelineStats } from './types';

export interface BucketConfig {
  key: keyof PipelineBuckets;
  label: string;
  /** Sankey ribbon/node fill. Status-conventional: greens positive, rose negative. */
  color: string;
}

/** Buckets in funnel order, from earliest/most-applications to terminal. */
export const PIPELINE_BUCKETS: BucketConfig[] = [
  { key: 'no_answer', label: 'No answer', color: '#cbd5e1' },
  { key: 'in_progress', label: 'In progress', color: '#fcd34d' },
  { key: 'interviewing', label: 'Interviewing', color: '#93c5fd' },
  { key: 'offer', label: 'Offer', color: '#86efac' },
  { key: 'accepted', label: 'Accepted', color: '#22c55e' },
  { key: 'rejected', label: 'Rejected', color: '#fb7185' },
  { key: 'declined', label: 'Declined', color: '#c4b5fd' },
];

/** Share of applications that reached an interview or beyond. A current-status
 *  snapshot, so it is a lower bound (a job rejected after interviewing counts
 *  only as rejected). Zero applications yields 0 without dividing by zero. */
export function interviewRate(s: PipelineStats): number {
  if (s.applications === 0) return 0;
  return (s.buckets.interviewing + s.buckets.offer + s.buckets.accepted) / s.applications;
}

/** Share of applications that reached an offer or beyond. Same snapshot caveat. */
export function offerRate(s: PipelineStats): number {
  if (s.applications === 0) return 0;
  return (s.buckets.offer + s.buckets.accepted) / s.applications;
}
