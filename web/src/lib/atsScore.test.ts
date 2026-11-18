import { describe, expect, it } from 'vitest';

import { atsTone, atsVerdict, topAtsFixes } from './atsScore';
import type { ATSReport } from './types';

function report(over: Partial<ATSReport> = {}): ATSReport {
  return {
    overall: 60,
    potential: 85,
    strong_keywords: [],
    recommended_keywords: [],
    reviewed: false,
    categories: [
      {
        id: 'structure',
        label: 'Structure',
        score: 10,
        max: 20,
        items: [
          { points: 5, text: 'Has a skills section', status: 'pass' },
          { points: 0, text: 'Add dates to every role', status: 'warn' },
        ],
      },
      {
        id: 'impact',
        label: 'Impact',
        score: 5,
        max: 20,
        items: [
          { points: 0, text: 'Quantify your results', status: 'fail' },
          { points: 0, text: 'Lead bullets with a verb', status: 'warn' },
          { points: 0, text: 'Cut the objective statement', status: 'fail' },
        ],
      },
    ],
    ...over,
  };
}

describe('atsTone', () => {
  it('reads 75 and up as good, 50 and up as middling, below as poor', () => {
    expect(atsTone(75)).toBe('text-primary');
    expect(atsTone(50)).toBe('text-warning-strong');
    expect(atsTone(49)).toBe('text-destructive');
  });
});

describe('atsVerdict', () => {
  it('names the band the tone colours, at the same thresholds', () => {
    expect(atsVerdict(75)).toBe('Strong');
    expect(atsVerdict(74)).toBe('Fair');
    expect(atsVerdict(50)).toBe('Fair');
    expect(atsVerdict(49)).toBe('Needs work');
  });
});

describe('topAtsFixes', () => {
  it('prefers the review suggestions when there are some', () => {
    expect(topAtsFixes(report({ suggestions: ['a', 'b', 'c', 'd'] }))).toEqual(['a', 'b', 'c']);
  });

  it('otherwise lists the failed checks before the warnings, three at most', () => {
    expect(topAtsFixes(report())).toEqual([
      'Quantify your results',
      'Cut the objective statement',
      'Add dates to every role',
    ]);
  });

  it('names a fix once even when two checks share its text', () => {
    const twice = report({
      categories: [
        {
          id: 'a',
          label: 'A',
          score: 0,
          max: 10,
          items: [
            { points: 0, text: 'Add dates', status: 'fail' },
            { points: 0, text: 'Add dates', status: 'warn' },
            { points: 0, text: 'Quantify', status: 'warn' },
          ],
        },
      ],
    });
    expect(topAtsFixes(twice)).toEqual(['Add dates', 'Quantify']);
    expect(topAtsFixes(report({ suggestions: ['x', 'x', 'y'] }))).toEqual(['x', 'y']);
  });

  it('is empty for a report with nothing to fix', () => {
    expect(topAtsFixes(report({ categories: [] }))).toEqual([]);
  });
});
