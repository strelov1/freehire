import { describe, expect, it } from 'vitest';
import { verdict } from './sentry-credential-check.mjs';

// The whole point of this check is that a release can tell three states apart that used to
// look identical from the outside: uploading, deliberately not uploading, and broken. Each
// case below is one of those states, and the assertions are on the DECISION (`ok`,
// `uploading`) plus enough of the message to prove the release will say which one it is in.

describe('verdict', () => {
  it('treats no credential as a stated opt-out, not a failure', () => {
    const v = verdict({ configured: false });
    expect(v.ok).toBe(true);
    expect(v.uploading).toBe(false);
    expect(v.message).toMatch(/minified/i);
  });

  it('passes a credential Sentry accepts', () => {
    const v = verdict({ configured: true, status: 200 });
    expect(v.ok).toBe(true);
    expect(v.uploading).toBe(true);
  });

  // The production failure this change exists for: sentry-cli was answered
  // `Invalid token (http status: 401)` on every release and the build carried on regardless.
  it('fails a credential Sentry rejects, and says it was rejected', () => {
    const v = verdict({ configured: true, status: 401 });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/rejected/i);
  });

  // design.md's Risks: a token that reads the org but cannot write releases would sail
  // through a check that only asked "are you a token?".
  it('fails a valid credential that lacks the upload permission', () => {
    const v = verdict({ configured: true, status: 403 });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/permission|scope/i);
  });

  // A 404 on the project's own releases means the org or project slug is wrong, which is a
  // misconfiguration somebody typed — not a condition.
  it('fails when the org or project does not exist', () => {
    const v = verdict({ configured: true, status: 404 });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/org|project/i);
  });

  // The other half of the rule, and the one that keeps this check from becoming an outage:
  // a rejection proves a misconfiguration, an unreachable checker proves nothing. Blocking
  // every deploy on Sentry's uptime is the mistake cmd/llm-probe already records — a red
  // signal meaning "the thing I watch is broken" must not be confusable with "I am broken".
  it.each([[500], [502], [503]])('does not stop the release on a Sentry fault (%i)', (status) => {
    const v = verdict({ configured: true, status });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/could not verify/i);
  });

  it('does not stop the release when Sentry is unreachable', () => {
    const v = verdict({ configured: true, unreachable: 'getaddrinfo ENOTFOUND' });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/could not verify/i);
  });
});
