import { describe, expect, it } from 'vitest';
import { EXTENSION_FAQ } from './extensionFaq';
import { faqPageJsonLd } from './seo';

describe('EXTENSION_FAQ', () => {
  it('carries entries', () => {
    expect(EXTENSION_FAQ.length).toBeGreaterThan(0);
  });

  it('asks each question once', () => {
    const questions = EXTENSION_FAQ.map((f) => f.question);
    expect(new Set(questions).size).toBe(questions.length);
  });

  it('answers every question', () => {
    for (const { question, answer } of EXTENSION_FAQ) {
      expect(question.trim(), `question: ${question}`).not.toBe('');
      expect(answer.trim(), `answer to: ${question}`).not.toBe('');
    }
  });

  // The page renders this same array into its FAQPage payload, so the structured
  // data cannot drift from the visible text.
  it('feeds the FAQPage payload', () => {
    const jsonLd = faqPageJsonLd(EXTENSION_FAQ) as { mainEntity: { name: string }[] };
    expect(jsonLd.mainEntity.map((e) => e.name)).toEqual(EXTENSION_FAQ.map((f) => f.question));
  });
});
