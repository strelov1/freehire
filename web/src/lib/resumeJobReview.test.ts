import { describe, expect, it } from 'vitest';
import { reviewResume } from './resumeJobReview';

describe('reviewResume', () => {
  it('finds an experience bullet before a summary mention and preserves the posting context', () => {
    const review = reviewResume(
      'Skills: Go, PostgreSQL\n• Built payment APIs in Golang and cut latency by 30%.',
      ['go', 'aws', 'postgresql'],
      [{ text: 'Production experience with Go and AWS', priority: 'required' }],
    );

    expect(review).toEqual([
      { skill: 'go', snippet: 'Built payment APIs in Golang and cut latency by 30%.', kind: 'example', requirement: 'Production experience with Go and AWS', priority: 'required' },
      { skill: 'aws', kind: 'missing', requirement: 'Production experience with Go and AWS', priority: 'required' },
      { skill: 'postgresql', snippet: 'Skills: Go, PostgreSQL', kind: 'mention' },
    ]);
  });

  it('recognizes whole aliases without treating prose, other tools, or related platforms as matches', () => {
    const review = reviewResume('I helped the team go faster.\nUsed GitHub and Google Cloud.\nBuilt C++ services without AWS.', ['go', 'git', 'aws', 'c', 'cpp'], []);
    expect(review.map(({ skill, kind }) => [skill, kind])).toEqual([
      ['go', 'missing'],
      ['git', 'missing'],
      ['aws', 'missing'],
      ['c', 'missing'],
      ['cpp', 'example'],
    ]);
  });
});
