import { describe, it, expect } from 'vitest';
import { answerableQuestions } from './answerBank';

describe('answerableQuestions', () => {
  it('offers an input for a question that has readable text', () => {
    const pending = [
      { label: 'What is your desired salary?', will_draft_at_submission: false },
      { label: 'Which state do you currently reside in?', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending).map((q) => q.label)).toEqual([
      'What is your desired salary?',
      'Which state do you currently reside in?'
    ]);
  });

  // A question the model will fill needs no input from the candidate — offering one asks
  // them to do work that is already handled.
  it('skips a question that will be drafted at submission', () => {
    const pending = [{ label: 'Why do you want to work here?', will_draft_at_submission: true }];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  // A labelless entry cannot be answered: there is nothing to show the candidate, and the
  // server refuses to key it. Rendering a blank input invites an answer to a question
  // nobody can see.
  it('skips a question with no readable text', () => {
    const pending = [
      { label: '', will_draft_at_submission: false },
      { label: '   ', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  it('is empty for no pending questions at all', () => {
    expect(answerableQuestions(undefined)).toEqual([]);
    expect(answerableQuestions([])).toEqual([]);
  });
});
