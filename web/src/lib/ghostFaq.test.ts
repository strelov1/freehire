import { describe, expect, it } from 'vitest';
import { GHOST_FAQ } from './ghostFaq';
import { faqPageJsonLd } from './seo';

describe('GHOST_FAQ', () => {
  it('carries entries', () => {
    expect(GHOST_FAQ.length).toBeGreaterThan(0);
  });

  it('asks each question once', () => {
    const questions = GHOST_FAQ.map((f) => f.question);
    expect(new Set(questions).size).toBe(questions.length);
  });

  it('answers every question', () => {
    for (const { question, answer } of GHOST_FAQ) {
      expect(question.trim(), `question: ${question}`).not.toBe('');
      expect(answer.trim(), `answer to: ${question}`).not.toBe('');
    }
  });

  // The page renders this same array into its FAQPage payload, so the structured
  // data cannot drift from the visible text. Google drops the rich result when the
  // two disagree, and collapsing the visible FAQ behind disclosures does not change
  // that — the answers stay in the served HTML precisely so they still match.
  it('feeds the FAQPage payload', () => {
    const jsonLd = faqPageJsonLd(GHOST_FAQ) as { mainEntity: { name: string }[] };
    expect(jsonLd.mainEntity.map((e) => e.name)).toEqual(GHOST_FAQ.map((f) => f.question));
  });

  // Deliberately NOT here: the word-blacklist check that guards GHOST_SIGNALS. That
  // test works on the criteria vocabulary because `label`, `fact` and `why` are
  // assertions — they never argue with anybody. A FAQ is a dialogue, and its work is to
  // voice the reader's suspicion and refuse it: "An unanswered application means the job
  // is fake, then?" is answered "No — that is why it takes two people and a second
  // signal." A blacklist flags that as an accusation, and the only way to satisfy it is
  // to blunt a question that is doing its job. A test that makes the copy worse is worse
  // than no test.
  //
  // For the same reason "What is a ghost job?" belongs here. The word is barred from
  // interface attached to a posting, where it would accuse a named employer; on the page
  // that explains the signal it is the term the reader searched for.
});
