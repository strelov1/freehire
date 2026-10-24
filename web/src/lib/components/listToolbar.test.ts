import { describe, expect, it } from 'vitest';
import { resultCountMessage } from './listToolbar.js';

describe('resultCountMessage', () => {
  it('says nothing while the list is loading or failed', () => {
    expect(resultCountMessage(1234, 'jobs', false)).toBe('');
    expect(resultCountMessage(null, 'jobs', false)).toBe('');
  });

  it('speaks the count once the list has one', () => {
    expect(resultCountMessage(1234, 'jobs', true)).toBe(`${(1234).toLocaleString()} jobs`);
    expect(resultCountMessage(1, 'job', undefined)).toBe('1 job');
  });

  it('claims "no matching" only when the caller said the list settled on nothing', () => {
    expect(resultCountMessage(null, 'jobs', true)).toBe('No matching jobs');
    // Without `settled`, a null total is as likely to mean "still loading".
    expect(resultCountMessage(null, 'companies', undefined)).toBe('');
  });
});
