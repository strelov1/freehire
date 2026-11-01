import { describe, expect, it } from 'vitest';
import { applyQuestionsBoundLabel, applyQuestionsLabel, equityLabel } from './jobFacts';
import { EQUITY_VALUES } from './generated/contracts';

describe('equityLabel', () => {
  it('labels every value the derivation writes', () => {
    expect(equityLabel('salary_and_equity')).toBe('Salary + equity');
    expect(equityLabel('equity_only')).toBe('Equity only');
    for (const v of EQUITY_VALUES) expect(equityLabel(v)).not.toBeNull();
  });

  // Absent is "not stated", never "no equity", and an unknown code is not guessed at.
  it.each([undefined, null, '', 'not_stated', 'stock_options'])('renders nothing for %s', (v) => {
    expect(equityLabel(v)).toBeNull();
  });
});

describe('applyQuestionsLabel', () => {
  it.each([
    [0, 'No extra questions'],
    [1, '1 question'],
    [2, '2 questions'],
    [12, '12 questions'],
  ])('labels %i as %s', (n, want) => {
    expect(applyQuestionsLabel(n)).toBe(want);
  });

  // No captured form is "we do not know" and must not read like 0.
  it.each([undefined, null, -1, 1.5, Number.NaN])('renders nothing for %s', (n) => {
    expect(applyQuestionsLabel(n)).toBeNull();
  });
});

describe('applyQuestionsBoundLabel', () => {
  it('names the toggle bound Short and states any other bound', () => {
    expect(applyQuestionsBoundLabel(3)).toBe('Short');
    expect(applyQuestionsBoundLabel(0)).toBe('No extra questions');
    expect(applyQuestionsBoundLabel(1)).toBe('Up to 1 question');
    expect(applyQuestionsBoundLabel(5)).toBe('Up to 5 questions');
  });
});
