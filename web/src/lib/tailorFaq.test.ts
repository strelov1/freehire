import { describe, expect, it } from 'vitest';
import { TAILOR_FAQ } from './tailorFaq';
import { faqPageJsonLd } from './seo';

describe('TAILOR_FAQ', () => {
  it('carries entries', () => {
    expect(TAILOR_FAQ.length).toBeGreaterThan(0);
  });

  it('asks each question once', () => {
    const questions = TAILOR_FAQ.map((f) => f.question);
    expect(new Set(questions).size).toBe(questions.length);
  });

  it('answers every question', () => {
    for (const { question, answer } of TAILOR_FAQ) {
      expect(question.trim(), `question: ${question}`).not.toBe('');
      expect(answer.trim(), `answer to: ${question}`).not.toBe('');
    }
  });

  // The page renders this same array into its FAQPage payload, so the structured
  // data cannot drift from the visible text.
  it('feeds the FAQPage payload', () => {
    const jsonLd = faqPageJsonLd(TAILOR_FAQ) as { mainEntity: { name: string }[] };
    expect(jsonLd.mainEntity.map((e) => e.name)).toEqual(TAILOR_FAQ.map((f) => f.question));
  });
});
