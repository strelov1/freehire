import { describe, expect, it } from 'vitest';
import { BUDDIES_FAQ } from './buddiesFaq';
import { faqPageJsonLd } from './seo';

describe('BUDDIES_FAQ', () => {
  it('carries entries', () => {
    expect(BUDDIES_FAQ.length).toBeGreaterThan(0);
  });

  it('asks each question once', () => {
    const questions = BUDDIES_FAQ.map((f) => f.question);
    expect(new Set(questions).size).toBe(questions.length);
  });

  it('answers every question', () => {
    for (const { question, answer } of BUDDIES_FAQ) {
      expect(question.trim(), `question: ${question}`).not.toBe('');
      expect(answer.trim(), `answer to: ${question}`).not.toBe('');
    }
  });

  // The page renders this same array into its FAQPage payload, so the structured
  // data cannot drift from the visible text.
  it('feeds the FAQPage payload', () => {
    const jsonLd = faqPageJsonLd(BUDDIES_FAQ) as { mainEntity: { name: string }[] };
    expect(jsonLd.mainEntity.map((e) => e.name)).toEqual(BUDDIES_FAQ.map((f) => f.question));
  });
});
