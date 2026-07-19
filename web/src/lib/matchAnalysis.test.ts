import { describe, it, expect } from 'vitest';
import { verdictTone, requirementStatusMeta, initMatchStream, reduceMatchEvent } from './matchAnalysis';

describe('verdictTone', () => {
  it('buckets scores on the backend thresholds', () => {
    expect(verdictTone(90)).toBe('strong');
    expect(verdictTone(75)).toBe('strong');
    expect(verdictTone(74)).toBe('good');
    expect(verdictTone(60)).toBe('good');
    expect(verdictTone(59)).toBe('moderate');
    expect(verdictTone(45)).toBe('moderate');
    expect(verdictTone(44)).toBe('weak');
    expect(verdictTone(30)).toBe('weak');
    expect(verdictTone(29)).toBe('poor');
    expect(verdictTone(0)).toBe('poor');
  });
});

describe('requirementStatusMeta', () => {
  it('labels and tones each ATS status', () => {
    expect(requirementStatusMeta('covered')).toEqual({ label: 'Covered', tone: 'strong' });
    expect(requirementStatusMeta('synonym-only')).toEqual({ label: 'Synonym', tone: 'good' });
    expect(requirementStatusMeta('missing-have')).toEqual({ label: 'Add it', tone: 'moderate' });
    expect(requirementStatusMeta('missing-gap')).toEqual({ label: 'Gap', tone: 'poor' });
  });

  it('falls back to a neutral label for an unknown status', () => {
    expect(requirementStatusMeta('weird')).toEqual({ label: 'weird', tone: 'moderate' });
  });
});

describe('reduceMatchEvent', () => {
  it('folds a full stream: meta → stages → sections → final', () => {
    let s = initMatchStream();
    s = reduceMatchEvent(s, 'meta', { has_cv: true });
    s = reduceMatchEvent(s, 'stage_start', { stage: 1 });
    expect(s.stages[0]?.state).toBe('active');
    s = reduceMatchEvent(s, 'thinking', { stage: 1, thinking: 'The ' });
    s = reduceMatchEvent(s, 'thinking', { stage: 1, thinking: 'candidate' });
    expect(s.thinking).toBe('The candidate');
    s = reduceMatchEvent(s, 'requirements', { requirements: [{ text: 'Go', priority: 'required', status: 'covered', evidence: '' }] });
    expect(s.requirements).toHaveLength(1);
    s = reduceMatchEvent(s, 'stage_done', { stage: 1 });
    expect(s.stages[0]?.state).toBe('done');
    s = reduceMatchEvent(s, 'dimensions', { analysis: { overall_score: 50, verdict: 'Moderate Fit', dimensions: [], requirement_match: [], strengths: [], gaps: [], recommendation: '' } });
    expect(s.analysis?.overall_score).toBe(50);
    s = reduceMatchEvent(s, 'final', { analysis: { overall_score: 71, verdict: 'Good Fit', dimensions: [], requirement_match: [], strengths: [], gaps: [], recommendation: 'Apply.' } });
    expect(s.done).toBe(true);
    expect(s.analysis?.overall_score).toBe(71);
    expect(s.stages.every((x) => x.state === 'done')).toBe(true);
  });

  it('records has_cv=false from meta', () => {
    const s = reduceMatchEvent(initMatchStream(), 'meta', { has_cv: false });
    expect(s.hasCV).toBe(false);
  });

  it('captures an error event as terminal', () => {
    const s = reduceMatchEvent(initMatchStream(), 'stream_error', { message: 'boom' });
    expect(s.error).toBe('boom');
    expect(s.done).toBe(true);
  });

  it('does not mutate the previous state', () => {
    const prev = initMatchStream();
    const next = reduceMatchEvent(prev, 'stage_start', { stage: 1 });
    expect(prev.stages[0]?.state).toBe('pending');
    expect(next).not.toBe(prev);
  });
});
