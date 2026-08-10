import { describe, it, expect } from 'vitest';
import { pageReadTarget } from './pageRead';
import type { ToolCall } from './tool-formatters';

const read = (result?: string): ToolCall => ({ name: 'read_current_page', input: {}, result });

describe('pageReadTarget', () => {
  it('names the page that was read', () => {
    const call = read(JSON.stringify({ url: 'https://boards.greenhouse.io/acme/jobs/12' }));

    expect(pageReadTarget(call)).toBe('boards.greenhouse.io/acme/jobs/12');
  });

  it('keeps the host alone when the page is a root', () => {
    expect(pageReadTarget(read(JSON.stringify({ url: 'https://example.test/' })))).toBe('example.test');
  });

  // One-time tokens and session ids live here. Putting them in the transcript is
  // the leak this display exists to guard against.
  it('drops the query string and the fragment', () => {
    const call = read(
      JSON.stringify({ url: 'https://jobs.example.test/apply?token=s3cr3t&ref=mail#stage=2' }),
    );

    expect(pageReadTarget(call)).toBe('jobs.example.test/apply');
  });

  // The read still happened; saying so imprecisely beats saying nothing, so the
  // caller falls back to its plain label.
  it('yields nothing it cannot make sense of', () => {
    expect(pageReadTarget(read(undefined))).toBe('');
    expect(pageReadTarget(read('not json'))).toBe('');
    expect(pageReadTarget(read(JSON.stringify({ title: 'no url here' })))).toBe('');
    expect(pageReadTarget(read(JSON.stringify({ url: 'not a url' })))).toBe('');
  });

  // A failed read has an error envelope, not a page — there is nothing to attribute.
  it('yields nothing for a failed call', () => {
    const call: ToolCall = {
      name: 'read_current_page',
      input: {},
      result: JSON.stringify({ error: 'no browser is attached' }),
      isError: true,
    };

    expect(pageReadTarget(call)).toBe('');
  });
});
