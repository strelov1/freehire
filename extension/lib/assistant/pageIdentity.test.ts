import { describe, expect, it } from 'vitest';
import { pageIdentity } from './pageIdentity';

describe('pageIdentity', () => {
  it.each(['boards.greenhouse.io', 'job-boards.greenhouse.io'])(
    'distinguishes embedded jobs and boards on %s',
    (host) => {
      const first = pageIdentity(`https://${host}/embed/job_app?for=acme&token=123`);
      expect(first).toBe(`https://${host}/embed/job_app?for=acme&token=123`);
      expect(pageIdentity(`https://${host}/embed/job_app?for=acme&token=456`)).not.toBe(first);
      expect(pageIdentity(`https://${host}/embed/job_app?for=other&token=123`)).not.toBe(first);
    },
  );

  it('ignores tracking parameters and canonicalizes embedded identity order', () => {
    expect(
      pageIdentity('https://boards.greenhouse.io/embed/job_app?utm_source=mail&token=123&for=acme&ref=source#form'),
    ).toBe('https://boards.greenhouse.io/embed/job_app?for=acme&token=123');
  });

  it('keeps ordinary job paths distinct while ignoring query and fragment tracking', () => {
    expect(pageIdentity('https://careers.example/jobs/123?utm_source=mail#form')).toBe(
      'https://careers.example/jobs/123',
    );
    expect(pageIdentity('https://careers.example/jobs/456')).toBe('https://careers.example/jobs/456');
  });

  it('does not preserve identity-like parameters on unrelated hosts or paths', () => {
    expect(pageIdentity('https://example.com/embed/job_app?for=acme&token=private')).toBe(
      'https://example.com/embed/job_app',
    );
    expect(pageIdentity('https://boards.greenhouse.io/acme/jobs/123?for=acme&token=private')).toBe(
      'https://boards.greenhouse.io/acme/jobs/123',
    );
  });

  it('removes credentials and fragments from an embedded identity', () => {
    expect(
      pageIdentity('https://user:password@boards.greenhouse.io/embed/job_app?for=acme&token=123&auth=private#private'),
    ).toBe('https://boards.greenhouse.io/embed/job_app?for=acme&token=123');
  });

  it('retains the existing fallback for invalid and empty input', () => {
    expect(pageIdentity('invalid-url')).toBe('invalid-url');
    expect(pageIdentity('')).toBe('');
  });
});
