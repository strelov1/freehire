import { describe, it, expect } from 'vitest';
import { toSearchString } from './urlSearchString';

describe('toSearchString', () => {
  it('keeps a comma-joined facet value literal instead of percent-encoded', () => {
    const p = new URLSearchParams();
    p.set('skills', 'go,react');
    expect(toSearchString(p)).toBe('skills=go,react');
  });

  it('leaves a value with no comma unaffected', () => {
    const p = new URLSearchParams();
    p.set('work_mode', 'remote');
    expect(toSearchString(p)).toBe('work_mode=remote');
  });

  it('the decoded string still parses back to the original value', () => {
    const p = new URLSearchParams();
    p.set('skills', 'go,react');
    const reparsed = new URLSearchParams(toSearchString(p));
    expect(reparsed.get('skills')).toBe('go,react');
  });
});
