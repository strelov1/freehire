import { describe, expect, it } from 'vitest';
import { INBOX_FAQ } from './inboxFaq';
import { faqPageJsonLd } from './seo';

describe('INBOX_FAQ', () => {
  it('carries entries', () => {
    expect(INBOX_FAQ.length).toBeGreaterThan(0);
  });

  it('asks each question once', () => {
    const questions = INBOX_FAQ.map((f) => f.question);
    expect(new Set(questions).size).toBe(questions.length);
  });

  it('answers every question', () => {
    for (const { question, answer } of INBOX_FAQ) {
      expect(question.trim(), `question: ${question}`).not.toBe('');
      expect(answer.trim(), `answer to: ${question}`).not.toBe('');
    }
  });

  // Google drops the rich result when the structured data and the visible text
  // disagree, so the page renders this same array — the JSON-LD is derived, not
  // written twice.
  it('feeds the FAQPage payload', () => {
    const jsonLd = faqPageJsonLd(INBOX_FAQ) as { mainEntity: { name: string }[] };
    expect(jsonLd.mainEntity.map((e) => e.name)).toEqual(INBOX_FAQ.map((f) => f.question));
  });
});
