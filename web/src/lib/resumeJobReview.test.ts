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
      {
        skill: 'go',
        snippet: 'Built payment APIs in Golang and cut latency by 30%.',
        kind: 'example',
        requirement: 'Production experience with Go and AWS',
        priority: 'required',
      },
      { skill: 'aws', kind: 'missing', requirement: 'Production experience with Go and AWS', priority: 'required' },
      { skill: 'postgresql', snippet: 'Skills: Go, PostgreSQL', kind: 'mention' },
    ]);
  });

  it('recognizes whole aliases without treating prose, other tools, or related platforms as matches', () => {
    const review = reviewResume(
      'I helped the team go faster.\nUsed GitHub and Google Cloud.\nBuilt C++ services without AWS.',
      ['go', 'git', 'aws', 'c', 'cpp'],
      [],
    );
    expect(review.map(({ skill, kind }) => [skill, kind])).toEqual([
      ['go', 'missing'],
      ['git', 'missing'],
      ['aws', 'missing'],
      ['c', 'missing'],
      ['cpp', 'example'],
    ]);
  });
  it.each([
    ['Built services without AWS or Azure.', ['missing', 'missing']],
    ['Built services without AWS, Azure, or Go.', ['missing', 'missing']],
    ['Built services with AWS and Azure.', ['example', 'example']],
    ['Built services without AWS, then deployed to Azure.', ['missing', 'example']],
    ['Built services without downtime, using AWS and Azure.', ['example', 'example']],
    ['Built services without using AWS or Azure.', ['missing', 'missing']],
    ['Built services without .NET or Azure.', ['missing', 'missing']],
    ['Built services without AWS, managed databases, or Azure.', ['missing', 'missing']],
    ['Built services without AWS or Azure, then deployed AWS.', ['example', 'missing']],
  ])('interprets listed skills in %s', (line, kinds) => {
    expect(reviewResume(line, ['aws', 'azure'], []).map(({ kind }) => kind)).toEqual(kinds);
  });
});
