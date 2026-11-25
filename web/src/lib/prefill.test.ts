import { describe, expect, it } from 'vitest';
import { fillEmpty, prefillFound } from './prefill';

describe('prefill helpers', () => {
  it('fills only an empty field, converting the incoming value', () => {
    expect(fillEmpty('', 'Go', (v) => v.toUpperCase())).toBe('GO');
    expect(fillEmpty('  ', 'Go')).toBe('Go');
    expect(fillEmpty('typed', 'Go')).toBe('typed');
    expect(fillEmpty('', undefined)).toBe('');
  });

  it('counts a result as found only when some field carries text or a non-empty list', () => {
    expect(prefillFound({})).toBe(false);
    expect(prefillFound({ skills: [], title: ' ' })).toBe(false);
    expect(prefillFound({ skills: ['go'] })).toBe(true);
    expect(prefillFound({ title: 'Dev' })).toBe(true);
  });
});
